package main

import (
    "bytes"
    "net/http"
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
