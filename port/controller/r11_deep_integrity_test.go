package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "mime/multipart"
    "net/http"
    "net/http/httptest"
    "os"
    "testing"
)

func assertNoDBChangeOnRejectedRequest(t *testing.T, h *controller, path, body string) {
    t.Helper()
    before, err := os.ReadFile(h.mainDBPath())
    if err != nil { t.Fatal(err) }
    rec := callR11Core(h, http.MethodPost, path, body)
    if rec.Code != http.StatusBadRequest {
        t.Fatalf("%s must reject bad input: got %d (%s)", path, rec.Code, rec.Body.String())
    }
    after, err := os.ReadFile(h.mainDBPath())
    if err != nil { t.Fatal(err) }
    if !bytes.Equal(before, after) {
        t.Fatalf("%s rejected bad input but modified the R11 database", path)
    }
}

// Invalid hours must never silently become a persisted incorrect plan or fact.
func TestR11DeepAuditDaySaveRejectsBadTypesAndHoursWithoutWriting(t *testing.T) {
    h := newR11TestController(t)
    r11MustPost(t, h, "/api/employee/save", `{"tab_no":"AUDIT-1","full_name":"Водитель Аудита"}`)
    r11MustPost(t, h, "/api/day/save", `{"TabNo":"AUDIT-1","Date":"2026-11-20","Plan":"1/8","PlanHours":8,"Fact":"1/8","FactHours":8}`)
    tests := []struct{ name, body string }{
        {"negative plan hours", `{"TabNo":"AUDIT-1","Date":"2026-11-20","Plan":"1/8","PlanHours":-1}`},
        {"hours beyond day", `{"TabNo":"AUDIT-1","Date":"2026-11-20","Plan":"1/30","PlanHours":30}`},
        {"plan hours string", `{"TabNo":"AUDIT-1","Date":"2026-11-20","Plan":"1/8","PlanHours":"8"}`},
        {"plan hours null", `{"TabNo":"AUDIT-1","Date":"2026-11-20","Plan":"1/8","PlanHours":null}`},
        {"plan hours object", `{"TabNo":"AUDIT-1","Date":"2026-11-20","Plan":"1/8","PlanHours":{}}`},
        {"fact hours negative", `{"TabNo":"AUDIT-1","Date":"2026-11-20","FactHours":-0.5}`},
        {"fact hours string", `{"TabNo":"AUDIT-1","Date":"2026-11-20","FactHours":"8.5"}`},
        {"plan is not string", `{"TabNo":"AUDIT-1","Date":"2026-11-20","Plan":[1,8]}`},
        {"fact is not string", `{"TabNo":"AUDIT-1","Date":"2026-11-20","Fact":42}`},
        {"comment is not string", `{"TabNo":"AUDIT-1","Date":"2026-11-20","Comment":{"unexpected":1}}`},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            assertNoDBChangeOnRejectedRequest(t, h, "/api/day/save", tc.body)
        })
    }
    // 8.5 hours of measured fact is allowed; do not round or reject valid decimals.
    r11MustPost(t, h, "/api/day/save", `{"TabNo":"AUDIT-1","Date":"2026-11-20","Fact":"1/8","FactHours":8.5}`)
    actual := r11Rows(t, readR11CoreState(t, h), "days")
    if len(actual) != 1 || r11Float(actual[0], "fact_hours") != 8.5 {
        t.Fatalf("valid fractional fact hours lost: %#v", actual)
    }
}

func TestR11DeepAuditUnknownEmployeeIDMustNotCreateDuplicate(t *testing.T) {
    h := newR11TestController(t)
    r11MustPost(t, h, "/api/employee/save", `{"tab_no":"AUDIT-1","full_name":"Первый"}`)
    assertNoDBChangeOnRejectedRequest(t, h, "/api/employee/save",
      `{"id":987654321,"tab_no":"AUDIT-2","full_name":"Несуществующий"}`)
}

func TestR11DeepAuditRejectsInvalidEmployeeAndCalendarPayloads(t *testing.T) {
    h := newR11TestController(t)
    r11MustPost(t, h, "/api/employee/save", `{"tab_no":"AUDIT-1","full_name":"Первый"}`)
    for _, body := range []string{
        `{"tab_no":"AUDIT-2","full_name":"Другой","is_quarter":"false"}`,
        `{"tab_no":"AUDIT-2","full_name":"Другой","probation":1}`,
        `{"tab_no":"AUDIT-2","full_name":"Другой","hire_date":"2026-02-30"}`,
        `{"tab_no":"AUDIT-2","full_name":"Другой","collection_point":{"unexpected":"type"}}`,
    } {
        assertNoDBChangeOnRejectedRequest(t, h, "/api/employee/save", body)
    }
    for _, body := range []string{
        `{"date":"2026-11-21","day_type":"WORK","norm_hours":-5}`,
        `{"date":"2026-11-21","day_type":"WORK","norm_hours":"8"}`,
        `{"date":"2026-11-21","day_type":"WORK","norm_hours":null}`,
    } {
        assertNoDBChangeOnRejectedRequest(t, h, "/api/calendar/day/save", body)
    }
}

 
func TestR11DeepAuditRejectsArchivedRevisionIDCollisionsAndDuplicateDays(t *testing.T) {
    for _, tc := range []struct{name string; corrupt func([]map[string]json.RawMessage, int64)}{
        {"foreign day ID",func(ds []map[string]json.RawMessage, otherID int64) {
            r11Put(ds[0], "id", otherID)
        }},
        {"duplicate snapshot date",func(ds []map[string]json.RawMessage, _ int64) {
            duplicate := map[string]json.RawMessage{}
            for key, val := range ds[0] { duplicate[key] = append(json.RawMessage(nil), val...) }
            r11Put(duplicate, "id", int64(987654321))
            ds = append(ds, duplicate)
        }},
    } {
        t.Run(tc.name, func(t *testing.T) {
            h := newR11TestController(t)
            r11MustPost(t,h,"/api/employee/save",`{"tab_no":"AUDIT-A","full_name":"Тест А"}`)
            r11MustPost(t,h,"/api/employee/save",`{"tab_no":"AUDIT-B","full_name":"Тест Б"}`)
            r11MustPost(t,h,"/api/day/save",`{"TabNo":"AUDIT-A","Date":"2026-11-20","Plan":"1/8","PlanHours":8}`)
            r11MustPost(t,h,"/api/day/save",`{"TabNo":"AUDIT-B","Date":"2026-11-20","Plan":"2/8","PlanHours":8}`)
            r11MustPost(t,h,"/api/autograf/revision/create",`{"tab_no":"AUDIT-A","effective_date":"2026-11-01","reason":"первая"}`)
            r11MustPost(t,h,"/api/day/quick-save",`{"TabNo":"AUDIT-A","Date":"2026-11-20","Plan":"1/10","PlanHours":10}`)
            r11MustPost(t,h,"/api/autograf/revision/create",`{"tab_no":"AUDIT-A","effective_date":"2026-11-21","reason":"вторая"}`)
            root := readR11CoreState(t,h)
            rows := r11Rows(t,root,"days")
            var otherID int64
            for _, d := range rows {
                if r11String(d,"date")=="2026-11-20" && r11String(d,"plan")=="2/8" { otherID = r11Int(d,"id") }
            }
            if otherID==0 { t.Fatal("second employee day not created") }
            revisions := r11Rows(t,root,"graph_revisions")
            target := revisions[0]
            snapshot := r11Rows(t,target,"days_snapshot")
            if len(snapshot) != 1 { t.Fatalf("invalid setup: %#v",snapshot) }
            if tc.name=="duplicate snapshot date" {
                duplicate:=map[string]json.RawMessage{}
                for key, val:=range snapshot[0] { duplicate[key]=append(json.RawMessage(nil),val...) }
                r11Put(duplicate,"id",int64(987654321))
                snapshot=append(snapshot,duplicate)
            } else {
                tc.corrupt(snapshot,otherID)
            }
            r11Put(target,"days_snapshot",snapshot)
            r11Put(root,"graph_revisions",revisions)
            encoded,err:=json.MarshalIndent(root,"","  ");if err!=nil{t.Fatal(err)}
            if err:=os.WriteFile(h.mainDBPath(),encoded,0600);err!=nil{t.Fatal(err)}
            assertNoDBChangeOnRejectedRequest(t,h,"/api/autograf/revision/activate",
                fmt.Sprintf(`{"id":%d}`,r11Int(target,"id")))
        })
    }
}

 
func TestR11DeepAuditIndividualFlightCannotOverwriteDifferentDriver(t *testing.T) {
    h := newR11TestController(t)
    r11MustPost(t,h,"/api/employee/save",`{"tab_no":"AUDIT-A","full_name":"А"}`)
    r11MustPost(t,h,"/api/employee/save",`{"tab_no":"AUDIT-B","full_name":"Б"}`)
    r11MustPost(t,h,"/api/employee-flight/import-confirm",
        `{"rows":[{"tab_no":"AUDIT-B","date":"2026-11-20","kind":"DEPARTURE","collection_point":"Тюмень"}]}`)
    records := r11Rows(t,readR11CoreState(t,h),"employee_flights")
    if len(records)!=1 {t.Fatalf("wrong synthetic setup: %#v",records)}
    id:=r11Int(records[0],"id")
    assertNoDBChangeOnRejectedRequest(t,h,"/api/employee-flight/import-confirm",
      fmt.Sprintf(`{"rows":[{"tab_no":"AUDIT-A","replace_id":%d,"date":"2026-11-21","kind":"ARRIVAL","collection_point":"Уфа"}]}`,id))
}

func TestR11DeepAuditCalendarImportRejectsMalformedRowsWithoutWriting(t *testing.T) {
    h := newR11TestController(t)
    r11MustPost(t,h,"/api/employee/save",`{"tab_no":"AUDIT-1","full_name":"А"}`)
    badRows:=[]string{
        `{"rows":[{"date":"2026-11-20","day_type":"WORK","norm_hours":-8}]}`,
        `{"rows":[{"date":"2026-11-20","day_type":"WORK","norm_hours":"8"}]}`,
        `{"rows":[{"date":"2026-11-20","day_type":"WORK","norm_hours":null}]}`,
        `{"rows":[{"date":"2026-11-20","day_type":"WORK","norm_hours":8},{"date":"2026-11-20","day_type":"OFF","norm_hours":0}]}`,
    }
    for _,body:=range badRows{
        assertNoDBChangeOnRejectedRequest(t,h,"/api/calendar/import-confirm",body)
    }
}

 
func TestR11DeepAuditCSVEventImportIsIdempotent(t *testing.T) {
    h := newR11TestController(t)
    r11MustPost(t,h,"/api/employee/save",`{"tab_no":"AUDIT-E","full_name":"Водитель"}`)
    csv := "tab_no;date_from;date_to;event_type;source_code;source;note\nAUDIT-E;2026-11-20;2026-11-21;SICK;2001;SAP;синтетический тест\n"
    var body bytes.Buffer
    mw := multipart.NewWriter(&body)
    fw, err := mw.CreateFormFile("file", "synthetic_events.csv")
    if err != nil { t.Fatal(err) }
    if _,err=fw.Write([]byte(csv));err!=nil{t.Fatal(err)}
    if err=mw.Close();err!=nil{t.Fatal(err)}
    for i:=0;i<2;i++{
        req:=httptest.NewRequest(http.MethodPost,"/api/import/events",bytes.NewReader(body.Bytes()))
        req.Header.Set("Content-Type",mw.FormDataContentType())
        rec:=httptest.NewRecorder()
        h.r11ImportEvents(rec,req)
        if rec.Code!=http.StatusOK {t.Fatalf("import %d failed: HTTP %d: %s",i,rec.Code,rec.Body.String())}
    }
    events:=r11Rows(t,readR11CoreState(t,h),"events")
    if len(events)!=1 {
        t.Fatalf("repeated import created %d identical events; expected idempotent single event",len(events))
    }
}
