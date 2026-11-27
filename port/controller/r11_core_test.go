package main

import (
	"fmt"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"mime/multipart"
	"os"
    "strconv"
	"strings"
	"testing"
)

func callR11Core(h *controller, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.r11CoreAPI(rec, req)
	return rec
}

func readR11CoreState(t *testing.T, h *controller) map[string]json.RawMessage {
	t.Helper()
	rec := callR11Core(h, http.MethodGet, "/api/state", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/state status=%d body=%s", rec.Code, rec.Body.String())
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(envelope["data"], &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestR11CoreEmployeeAndDayCRUDPreservesFactOnQuickSave(t *testing.T) {
	h := newR11TestController(t)
	created := callR11Core(h, http.MethodPost, "/api/employee/save", `{"tab_no":"T-001","full_name":"Тестовый водитель","position":"Водитель","unit":"Колонна 1","collection_point":"Тюмень","schedule_type":"A","is_quarter":false}`)
	if created.Code != http.StatusOK {
		t.Fatalf("employee save status=%d body=%s", created.Code, created.Body.String())
	}
	var employeeResult map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &employeeResult); err != nil {
		t.Fatal(err)
	}
	if employeeResult["ok"] != true {
		t.Fatalf("employee save response=%v", employeeResult)
	}
	day := callR11Core(h, http.MethodPost, "/api/day/save", `{"TabNo":"T-001","Date":"2026-11-20","Plan":"1/8","PlanHours":8,"Fact":"1/8","FactHours":8,"Shift":"DAY","State":"WORK","Comment":"source"}`)
	if day.Code != http.StatusOK {
		t.Fatalf("day save status=%d body=%s", day.Code, day.Body.String())
	}
	quick := callR11Core(h, http.MethodPost, "/api/day/quick-save", `{"TabNo":"T-001","Date":"2026-11-20","Plan":"1/10","PlanHours":10,"State":"WORK","Comment":"manual"}`)
	if quick.Code != http.StatusOK {
		t.Fatalf("quick save status=%d body=%s", quick.Code, quick.Body.String())
	}
	data := readR11CoreState(t, h)
	var employees []map[string]json.RawMessage
	var days []map[string]json.RawMessage
	if err := json.Unmarshal(data["employees"], &employees); err != nil { t.Fatal(err) }
	if err := json.Unmarshal(data["days"], &days); err != nil { t.Fatal(err) }
	if len(employees)!=1 || len(days)!=1 { t.Fatalf("employees=%d days=%d",len(employees),len(days)) }
	if got:=r11String(days[0],"plan");got!="1/10"{t.Fatalf("plan=%q",got)}
	if got:=r11Float(days[0],"plan_hours");got!=10{t.Fatalf("plan_hours=%v",got)}
	if got:=r11String(days[0],"fact");got!="1/8"{t.Fatalf("quick-save overwrote fact: %q",got)}
	if got:=r11Float(days[0],"fact_hours");got!=8{t.Fatalf("quick-save overwrote fact_hours: %v",got)}
	if got:=r11String(days[0],"shift");got!="DAY"{t.Fatalf("quick-save overwrote shift: %q",got)}
	if got:=r11String(days[0],"comment");got!="manual"{t.Fatalf("comment=%q",got)}
}

func TestR11CoreDuplicateEmployeeRejectedWithoutChangingState(t *testing.T) {
	h := newR11TestController(t)
	first := callR11Core(h,http.MethodPost,"/api/employee/save",`{"tab_no":"T-001","full_name":"Первый"}`)
	if first.Code!=http.StatusOK { t.Fatalf("first create=%d %s",first.Code,first.Body.String()) }
	second := callR11Core(h,http.MethodPost,"/api/employee/save",`{"tab_no":"T-001","full_name":"Дубликат"}`)
	if second.Code!=http.StatusBadRequest { t.Fatalf("duplicate status=%d body=%s",second.Code,second.Body.String()) }
	data:=readR11CoreState(t,h)
	var employees []map[string]json.RawMessage
	if err:=json.Unmarshal(data["employees"],&employees);err!=nil{t.Fatal(err)}
	if len(employees)!=1 || r11String(employees[0],"full_name")!="Первый" { t.Fatalf("state changed after rejected duplicate: %+v",employees) }
}

func TestR11CoreEventsNotesColorsAndRevisionSnapshots(t *testing.T) {
	h:=newR11TestController(t)
	emp:=callR11Core(h,http.MethodPost,"/api/employee/save",`{"tab_no":"T-002","full_name":"Водитель два"}`)
	if emp.Code!=http.StatusOK{t.Fatal(emp.Body.String())}
	for _,tc:=range []struct{path,body string}{
		{"/api/event/save",`{"TabNo":"T-002","DateFrom":"2026-11-01","DateTo":"2026-11-03","EventType":"VACATION","SourceCode":"SAP","Source":"SAP","Note":"Отпуск"}`},
		{"/api/note/save",`{"id":0,"tab_no":"T-002","date_from":"2026-11-05","date_to":"2026-11-07","text":"Проверить документ","reminder":"2026-11-04"}`},
		{"/api/color/save",`{"tab_no":"T-002","date_from":"2026-11-05","date_to":"2026-11-07","color":"yellow"}`},
		{"/api/day/save",`{"TabNo":"T-002","Date":"2026-11-05","Plan":"1/8","PlanHours":8,"Fact":"","FactHours":0,"Shift":"DAY","State":"WORK"}`},
		{"/api/autograf/revision/create",`{"tab_no":"T-002","effective_date":"2026-11-01","control_date":"","fixed_hours":0,"schedule_type":"A","reason":"Исходный график","label":""}`},
	} {
		rec:=callR11Core(h,http.MethodPost,tc.path,tc.body)
		if rec.Code!=http.StatusOK{t.Fatalf("%s status=%d body=%s",tc.path,rec.Code,rec.Body.String())}
	}
	data:=readR11CoreState(t,h)
	var events,notes,colors,revisions []map[string]json.RawMessage
	_ = json.Unmarshal(data["events"],&events);_ = json.Unmarshal(data["notes"],&notes);_ = json.Unmarshal(data["manual_colors"],&colors);_ = json.Unmarshal(data["graph_revisions"],&revisions)
	if len(events)!=1||len(notes)!=1||len(colors)!=3||len(revisions)!=1{t.Fatalf("events=%d notes=%d colors=%d revisions=%d",len(events),len(notes),len(colors),len(revisions))}
	if r11String(revisions[0],"status")!="ACTIVE"{t.Fatalf("initial revision status=%q",r11String(revisions[0],"status"))}
	var daySnapshot []map[string]json.RawMessage
	if err:=json.Unmarshal(revisions[0]["days_snapshot"],&daySnapshot);err!=nil{t.Fatal(err)}
	if len(daySnapshot)!=1{t.Fatalf("revision days snapshot=%d",len(daySnapshot))}
	second:=callR11Core(h,http.MethodPost,"/api/autograf/revision/create",`{"tab_no":"T-002","effective_date":"2026-12-01","control_date":"2026-11-30","fixed_hours":80,"schedule_type":"B","reason":"Смена вида графика","label":""}`)
	if second.Code!=http.StatusOK{t.Fatalf("second revision=%d %s",second.Code,second.Body.String())}
	data=readR11CoreState(t,h);_ = json.Unmarshal(data["graph_revisions"],&revisions)
	if len(revisions)!=2||r11String(revisions[0],"status")!="ARCHIVED"||r11String(revisions[1],"status")!="ACTIVE"{t.Fatalf("revision statuses: %+v",revisions)}
	var archivedEvents,archivedNotes,archivedColors []map[string]json.RawMessage
	_ = json.Unmarshal(revisions[0]["events_snapshot"],&archivedEvents);_ = json.Unmarshal(revisions[0]["notes_snapshot"],&archivedNotes);_ = json.Unmarshal(revisions[0]["colors_snapshot"],&archivedColors)
	if len(archivedEvents)!=1||len(archivedNotes)!=1||len(archivedColors)!=3{t.Fatalf("archived snapshot events=%d notes=%d colors=%d",len(archivedEvents),len(archivedNotes),len(archivedColors))}
	// A date separated from a multi-day note becomes its own standalone note.
	splitBody,_:=json.Marshal(map[string]any{"id":r11Int(notes[0],"id"),"date":"2026-11-06"})
	split:=callR11Core(h,http.MethodPost,"/api/note/split",string(splitBody))
	if split.Code!=http.StatusOK{t.Fatalf("note split=%d %s",split.Code,split.Body.String())}
	data=readR11CoreState(t,h);_ = json.Unmarshal(data["notes"],&notes)
	if len(notes)!=3{t.Fatalf("notes after split=%d",len(notes))}
}

func TestR11CoreFlightCycleAndCalendarDraft(t *testing.T) {
	h:=newR11TestController(t)
	emp:=callR11Core(h,http.MethodPost,"/api/employee/save",`{"tab_no":"T-003","full_name":"Водитель три","collection_point":"Тюмень"}`)
	if emp.Code!=http.StatusOK{t.Fatal(emp.Body.String())}
	for _,date:=range []string{"2026-11-01","2026-11-29"}{
		rec:=callR11Core(h,http.MethodPost,"/api/autograf/flight/save",`{"id":0,"collection_point":"Тюмень","date":"`+date+`","source":"Тест","note":"LCSEQ:1"}`)
		if rec.Code!=http.StatusOK{t.Fatalf("flight save: %d %s",rec.Code,rec.Body.String())}
	}
	cal:=callR11Core(h,http.MethodPost,"/api/calendar/init",`{"year":2027}`)
	if cal.Code!=http.StatusOK{t.Fatalf("calendar init: %d %s",cal.Code,cal.Body.String())}
	data:=readR11CoreState(t,h)
	var flights,calendar []map[string]json.RawMessage
	_ = json.Unmarshal(data["flight_dates"],&flights);_ = json.Unmarshal(data["calendar"],&calendar)
	if len(flights)!=2{t.Fatalf("flight dates=%d",len(flights))}
	if len(calendar)!=365{t.Fatalf("calendar rows=%d, want 365",len(calendar))}
}

func TestR11CoreSaveCreatesBackupAfterFirstWrite(t *testing.T) {
	h:=newR11TestController(t)
	rec:=callR11Core(h,http.MethodPost,"/api/employee/save",`{"tab_no":"T-004","full_name":"Водитель четыре"}`)
	if rec.Code!=http.StatusOK{t.Fatal(rec.Body.String())}
	rec=callR11Core(h,http.MethodPost,"/api/note/save",`{"tab_no":"T-004","date_from":"2026-11-01","date_to":"2026-11-01","text":"test"}`)
	if rec.Code!=http.StatusOK{t.Fatalf("note save=%d %s",rec.Code,rec.Body.String())}
	var response map[string]any
	if err:=json.Unmarshal(rec.Body.Bytes(),&response);err!=nil{t.Fatal(err)}
	if response["backup"]==nil{t.Fatalf("expected backup in response: %v",response)}
	entries,err:=os.ReadDir(filepathJoin(h.baseDir,"backups"))
	if err!=nil{t.Fatal(err)}
	if len(entries)==0{t.Fatal("backup directory is empty")}
}

func filepathJoin(a,b string) string { return a + string(os.PathSeparator) + b }


func TestR11CoreScheduleChangeUpdatesEmployeeAndHistory(t *testing.T) {
	h:=newR11TestController(t)
	emp:=callR11Core(h,http.MethodPost,"/api/employee/save",`{"tab_no":"T-22","full_name":"Смена графика","schedule_type":"A"}`)
	if emp.Code!=http.StatusOK{t.Fatal(emp.Body.String())}
	day:=callR11Core(h,http.MethodPost,"/api/day/save",`{"TabNo":"T-22","Date":"2026-11-05","Plan":"1/8","PlanHours":8,"Fact":"","FactHours":0}`)
	if day.Code!=http.StatusOK{t.Fatal(day.Body.String())}
	change:=callR11Core(h,http.MethodPost,"/api/autograf/schedule-change",`{"tab_no":"T-22","effective_date":"2026-11-10","new_schedule":"B","reason":"Переход"}`)
	if change.Code!=http.StatusOK{t.Fatalf("schedule change=%d %s",change.Code,change.Body.String())}
	data:=readR11CoreState(t,h)
	var employees []map[string]json.RawMessage;var changes []map[string]json.RawMessage
	_ = json.Unmarshal(data["employees"],&employees);_ = json.Unmarshal(data["schedule_changes"],&changes)
	if len(changes)!=1||r11String(changes[0],"old_schedule")!="A"||r11String(changes[0],"new_schedule")!="B"{t.Fatalf("schedule change history=%+v",changes)}
	if r11String(employees[0],"schedule_type")!="B"{t.Fatalf("employee schedule_type=%q",r11String(employees[0],"schedule_type"))}
	if r11Float(changes[0],"hours_before")!=8{t.Fatalf("hours_before=%v",r11Float(changes[0],"hours_before"))}
}

func TestR11CoreVersionRestoreAndCSVFactImport(t *testing.T) {
	h:=newR11TestController(t)
	emp:=callR11Core(h,http.MethodPost,"/api/employee/save",`{"tab_no":"T-33","full_name":"Версия и факт"}`)
	if emp.Code!=http.StatusOK{t.Fatal(emp.Body.String())}
	day:=callR11Core(h,http.MethodPost,"/api/day/save",`{"TabNo":"T-33","Date":"2026-11-20","Plan":"1/8","PlanHours":8,"Fact":"1/8","FactHours":8,"Shift":"DAY"}`)
	if day.Code!=http.StatusOK{t.Fatal(day.Body.String())}
	ver:=callR11Core(h,http.MethodPost,"/api/autograf/version/create",`{"tab_no":"T-33","year":2026,"label":"До правки","reason":"Тест"}`)
	if ver.Code!=http.StatusOK{t.Fatal(ver.Body.String())}
	data:=readR11CoreState(t,h);var versions []map[string]json.RawMessage;_ = json.Unmarshal(data["graph_versions"],&versions)
	id:=r11Int(versions[0],"id")
	quick:=callR11Core(h,http.MethodPost,"/api/day/quick-save",`{"TabNo":"T-33","Date":"2026-11-20","Plan":"2/10","PlanHours":10}`)
	if quick.Code!=http.StatusOK{t.Fatal(quick.Body.String())}
	restoreBody,_:=json.Marshal(map[string]any{"id":id})
	restore:=callR11Core(h,http.MethodPost,"/api/autograf/version/restore",string(restoreBody))
	if restore.Code!=http.StatusOK{t.Fatalf("restore=%d %s",restore.Code,restore.Body.String())}
	data=readR11CoreState(t,h);var days []map[string]json.RawMessage;_ = json.Unmarshal(data["days"],&days)
	if len(days)!=1||r11String(days[0],"plan")!="1/8"{t.Fatalf("restored day=%+v",days)}

	// CSV with a UTF-8 BOM and semicolon separator is accepted by the import endpoint.
	var multipartBody bytes.Buffer
	mw:=multipart.NewWriter(&multipartBody)
	fw,err:=mw.CreateFormFile("file","fact.csv");if err!=nil{t.Fatal(err)}
	_,_ = fw.Write([]byte("\xef\xbb\xbfТабельный номер;Дата;Факт;Часы;Смена\nT-33;21.11.2026;2/8;8;NIGHT\n"))
	if err:=mw.Close();err!=nil{t.Fatal(err)}
	req:=httptest.NewRequest(http.MethodPost,"/api/import/fact",&multipartBody);req.Header.Set("Content-Type",mw.FormDataContentType())
	rec:=httptest.NewRecorder();h.r11ImportFact(rec,req)
	if rec.Code!=http.StatusOK{t.Fatalf("fact import=%d %s",rec.Code,rec.Body.String())}
	data=readR11CoreState(t,h);_ = json.Unmarshal(data["days"],&days)
	if len(days)!=2{t.Fatalf("days after import=%d",len(days))}
	var imported map[string]json.RawMessage
	for _,d:=range days{if r11String(d,"date")=="2026-11-21"{imported=d}}
	if imported==nil||r11String(imported,"fact")!="2/8"||r11Float(imported,"fact_hours")!=8||r11String(imported,"shift")!="NIGHT"{t.Fatalf("imported fact=%+v",imported)}
	var history []map[string]json.RawMessage;_ = json.Unmarshal(data["import_history"],&history)
	if len(history)!=1||r11String(history[0],"kind")!="FACT"{t.Fatalf("import history=%+v",history)}
}


func TestR11FullUIIncludesOriginalModulesAndLocalImport(t *testing.T) {
	h:=newR11TestController(t)
	req:=httptest.NewRequest(http.MethodGet,"/r11",nil)
	rec:=httptest.NewRecorder()
	h.serveFullR11UI(rec,req)
	if rec.Code!=http.StatusOK{t.Fatalf("UI status=%d",rec.Code)}
	page:=rec.Body.String()
	for _,want:=range []string{"АвтоГраф","Проверка табеля","Перенос базы из Windows R11","/api/state/import","R11 Linux Core"}{
		if !strings.Contains(page,want){t.Fatalf("full UI is missing %q",want)}
	}
	if strings.Contains(page,"R11-LINUX-STATE-COMPAT/0.1"){t.Fatal("state-only compatibility UI is still being served")}
}

func TestR11TerminationStopsPlanWritesButKeepsPastDays(t *testing.T) {
    h := newR11TestController(t)
    create := callR11Core(h,http.MethodPost,"/api/employee/save",`{"tab_no":"9001","full_name":"Тестов Иван Сергеевич","hire_date":"2026-11-01","probation":true,"probation_end":"2026-11-20","fire_date":""}`)
    if create.Code!=http.StatusOK { t.Fatalf("create employee: %d %s",create.Code,create.Body.String()) }
    for _,date := range []string{"2026-11-18","2026-11-19"} {
        rec := callR11Core(h,http.MethodPost,"/api/day/quick-save",`{"TabNo":"9001","Date":"`+date+`","Plan":"1/8","PlanHours":8,"State":"WORK"}`)
        if rec.Code!=http.StatusOK { t.Fatalf("setup date %s: %d %s",date,rec.Code,rec.Body.String()) }
    }
    var emps []map[string]json.RawMessage
    snapshot := readR11CoreState(t,h)
    if err := json.Unmarshal(snapshot["employees"],&emps); err!=nil || len(emps)!=1 { t.Fatalf("setup employees: %v",err) }
    termination := `{"id":`+strconv.FormatInt(r11Int(emps[0],"id"),10)+`,"tab_no":"9001","full_name":"Тестов Иван Сергеевич","hire_date":"2026-11-01","probation":true,"probation_end":"2026-11-20","fire_date":"2026-11-18"}`
    update := callR11Core(h,http.MethodPost,"/api/employee/save",termination)
    if update.Code!=http.StatusOK { t.Fatalf("set termination: %d %s",update.Code,update.Body.String()) }
    for _,tc := range []struct{name,path,body string; want int}{
        {"last day quick save","/api/day/quick-save",`{"TabNo":"9001","Date":"2026-11-18","Plan":"1/10","PlanHours":10}`,200},
        {"after termination quick save","/api/day/quick-save",`{"TabNo":"9001","Date":"2026-11-19","Plan":"1/11","PlanHours":11}`,400},
        {"after termination full day save","/api/day/save",`{"TabNo":"9001","Date":"2026-11-20","Plan":"1/8","PlanHours":8}`,400},
        {"no deletion of previous history","/api/day/delete",`{"tab_no":"9001","date":"2026-11-19"}`,400},
    } {
        t.Run(tc.name,func(t *testing.T) {
            rec := callR11Core(h,http.MethodPost,tc.path,tc.body)
            if rec.Code!=tc.want { t.Fatalf("status=%d want=%d body=%s",rec.Code,tc.want,rec.Body.String()) }
            if tc.want==400&&!strings.Contains(rec.Body.String(),"после увольнения") { t.Fatalf("missing explanation: %s",rec.Body.String()) }
        })
    }
    state := readR11CoreState(t,h)
    var days []map[string]json.RawMessage
    if err:=json.Unmarshal(state["days"],&days);err!=nil{t.Fatal(err)}
    if len(days)!=2 { t.Fatalf("previous days lost, got %d",len(days)) }
    for _,day:=range days {
        if r11String(day,"date")=="2026-11-19"&&r11Float(day,"plan_hours")!=8 {
            t.Fatalf("historical day silently modified: %+v",day)
        }
    }
    ordinary:=callR11Core(h,http.MethodPost,"/api/employee/save",`{"tab_no":"9002","full_name":"Действующий водитель","hire_date":"2026-11-01"}`)
    if ordinary.Code!=http.StatusOK{t.Fatalf("ordinary employee: %s",ordinary.Body.String())}
    allowed:=callR11Core(h,http.MethodPost,"/api/day/quick-save",`{"TabNo":"9002","Date":"2026-11-20","Plan":"1/8","PlanHours":8}`)
    if allowed.Code!=http.StatusOK{t.Fatalf("ordinary employee incorrectly blocked: %s",allowed.Body.String())}
}

func readSchedulePairRows(t *testing.T, h *controller) []map[string]json.RawMessage {
	t.Helper()
	data := readR11CoreState(t, h)
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(data["schedule_types"], &rows); err != nil { t.Fatal(err) }
	return rows
}

func TestR11SchedulePairAtomicCreateAndIdempotentConflict(t *testing.T) {
	h := newR11TestController(t)
	rec := callR11Core(h, http.MethodPost, "/api/autograf/schedule-type/pair",
		`{"collection_point":"Тюмень","name":"Тюм-Тал-Тюм 9","phase":"A"}`)
	if rec.Code != http.StatusOK { t.Fatalf("create pair status=%d: %s", rec.Code, rec.Body.String()) }
	rows := readSchedulePairRows(t, h)
	if len(rows) != 2 { t.Fatalf("expected atomic creation of two rows, got %d", len(rows)) }
	if r11String(rows[0], "name") != "Тюм-Тал-Тюм 9" ||
		r11String(rows[0], "pair_name") != "Тюм-Тал-Тюм 9." ||
		r11String(rows[0], "phase") != "A" ||
		!r11Bool(rows[0], "active") {
		t.Fatalf("first row invalid: %+v", rows[0])
	}
	if r11String(rows[1], "name") != "Тюм-Тал-Тюм 9." ||
		r11String(rows[1], "pair_name") != "Тюм-Тал-Тюм 9" ||
		r11String(rows[1], "phase") != "B" ||
		!r11Bool(rows[1], "active") {
		t.Fatalf("second row invalid: %+v", rows[1])
	}
	again := callR11Core(h, http.MethodPost, "/api/autograf/schedule-type/pair",
		`{"collection_point":"Тюмень","name":"Тюм-Тал-Тюм 9","phase":"A"}`)
	if again.Code != http.StatusBadRequest { t.Fatalf("repeat must refuse duplicate, status %d", again.Code) }
	if got := len(readSchedulePairRows(t, h)); got != 2 { t.Fatalf("duplicate changed rows to %d", got) }
	// Another pair can choose a free first phase, and the dotted name is always opposite.
	other := callR11Core(h, http.MethodPost, "/api/autograf/schedule-type/pair",
		`{"collection_point":"Тюмень","name":"Тюм-Тал-Тюм 10","phase":"B"}`)
	if other.Code != http.StatusOK { t.Fatalf("create reverse phase: %d %s", other.Code, other.Body.String()) }
	rows = readSchedulePairRows(t, h)
	if len(rows) != 4 || r11String(rows[2], "phase") != "B" || r11String(rows[3], "phase") != "A" {
		t.Fatalf("opposite phase not saved: %+v", rows)
	}
}

func TestR11SchedulePairCompleteExistingHalfAndRejectConflict(t *testing.T) {
	h := newR11TestController(t)
	first := callR11Core(h, http.MethodPost, "/api/autograf/schedule-type/save",
		`{"collection_point":"Тюмень","name":"Тюм-Тал-Тюм 9","pair_name":"Тюм-Тал-Тюм 9.","phase":"A","active":true,"note":"Учебная проверка"}`)
	if first.Code != http.StatusOK { t.Fatalf("setup: %d %s", first.Code, first.Body.String()) }
	rows := readSchedulePairRows(t,h)
	id := r11Int(rows[0],"id")
	complete := callR11Core(h,http.MethodPost,"/api/autograf/schedule-type/pair",
		fmt.Sprintf(`{"id":%d}`,id))
	if complete.Code != http.StatusOK { t.Fatalf("complete pair: %d %s",complete.Code,complete.Body.String()) }
	rows=readSchedulePairRows(t,h)
	if len(rows)!=2 || r11Int(rows[0],"id")!=id { t.Fatalf("original row overwritten/duplicate: %+v",rows) }
	if r11String(rows[1],"name")!="Тюм-Тал-Тюм 9." || r11String(rows[1],"pair_name")!="Тюм-Тал-Тюм 9" ||
		r11String(rows[1],"phase")!="B" || r11String(rows[1],"note")!="Учебная проверка" {
		t.Fatalf("completed partner invalid: %+v",rows[1])
	}
	again:=callR11Core(h,http.MethodPost,"/api/autograf/schedule-type/pair",fmt.Sprintf(`{"id":%d}`,id))
	if again.Code!=http.StatusBadRequest || len(readSchedulePairRows(t,h))!=2 {
		t.Fatalf("repeat must be rejected without duplicates: %d",again.Code)
	}
}

func TestR11SchedulePairRejectConflictingExistingPartnerWithoutWrites(t *testing.T) {
	h:=newR11TestController(t)
	for _,body:=range []string{
		`{"collection_point":"Тюмень","name":"Тюм-Тал-Тюм 9","pair_name":"Тюм-Тал-Тюм 9.","phase":"A","active":true}`,
		`{"collection_point":"Тюмень","name":"Тюм-Тал-Тюм 9.","pair_name":"Тюм-Тал-Тюм 9","phase":"A","active":true}`,
	} {
		rec:=callR11Core(h,http.MethodPost,"/api/autograf/schedule-type/save",body)
		if rec.Code!=http.StatusOK { t.Fatalf("setup save: %d %s",rec.Code,rec.Body.String()) }
	}
	rows:=readSchedulePairRows(t,h)
	rec:=callR11Core(h,http.MethodPost,"/api/autograf/schedule-type/pair",
		fmt.Sprintf(`{"id":%d}`,r11Int(rows[0],"id")))
	if rec.Code!=http.StatusBadRequest { t.Fatalf("conflict should fail: %d %s",rec.Code,rec.Body.String()) }
	after:=readSchedulePairRows(t,h)
	if len(after)!=2 || r11String(after[1],"phase")!="A" || r11String(after[1],"pair_name")!="Тюм-Тал-Тюм 9" {
		t.Fatalf("server rewrote conflicting partner: %+v",after)
	}
}

func TestR11SchedulePairDifferentNumberedNames(t *testing.T) {
	h := newR11TestController(t)
	b := "{\"collection_point\":\"Сургут\",\"name\":\"Сург-Тал-Сург 7\",\"partner_name\":\"Сург-Тал-Сург 16\",\"phase\":\"A\"}"
	rec := callR11Core(h, http.MethodPost, "/api/autograf/schedule-type/pair", b)
	if rec.Code != http.StatusOK { t.Fatalf("save custom numbered pair: %d %s", rec.Code, rec.Body.String()) }
	rows := readSchedulePairRows(t, h)
	if len(rows) != 2 ||
		r11String(rows[0],"name") != "Сург-Тал-Сург 7" || r11String(rows[0],"pair_name") != "Сург-Тал-Сург 16" ||
		r11String(rows[0],"phase") != "A" ||
		r11String(rows[1],"name") != "Сург-Тал-Сург 16" || r11String(rows[1],"pair_name") != "Сург-Тал-Сург 7" ||
		r11String(rows[1],"phase") != "B" {
		t.Fatalf("incorrect custom partner pair: %+v", rows)
	}
	again:=callR11Core(h,http.MethodPost,"/api/autograf/schedule-type/pair",b)
	if again.Code!=http.StatusBadRequest || len(readSchedulePairRows(t,h))!=2 {
		t.Fatalf("custom repeat must not create duplicates: %d %s",again.Code,again.Body.String())
	}
}

func TestR11SchedulePairLinkTwoExistingNumberedTypes(t *testing.T) {
	h:=newR11TestController(t)
	for _,body:=range []string{
		"{\"collection_point\":\"Сургут\",\"name\":\"Сург-Тал-Сург 7\",\"phase\":\"A\",\"active\":true,\"note\":\"первый\"}",
		"{\"collection_point\":\"Сургут\",\"name\":\"Сург-Тал-Сург 16\",\"phase\":\"B\",\"active\":true,\"note\":\"второй\"}",
	} {
		rec:=callR11Core(h,http.MethodPost,"/api/autograf/schedule-type/save",body)
		if rec.Code!=http.StatusOK { t.Fatalf("seed: %d %s",rec.Code,rec.Body.String()) }
	}
	rows:=readSchedulePairRows(t,h)
	firstId,secondId:=r11Int(rows[0],"id"),r11Int(rows[1],"id")
	rec:=callR11Core(h,http.MethodPost,"/api/autograf/schedule-type/pair",
		fmt.Sprintf("{\"id\":%d,\"partner_name\":\"Сург-Тал-Сург 16\"}",firstId))
	if rec.Code!=http.StatusOK { t.Fatalf("link existing pair: %d %s",rec.Code,rec.Body.String()) }
	rows=readSchedulePairRows(t,h)
	if len(rows)!=2 || r11Int(rows[0],"id")!=firstId || r11Int(rows[1],"id")!=secondId ||
		r11String(rows[0],"pair_name")!="Сург-Тал-Сург 16" ||
		r11String(rows[1],"pair_name")!="Сург-Тал-Сург 7" ||
		r11String(rows[0],"note")!="первый" || r11String(rows[1],"note")!="второй" {
		t.Fatalf("existing rows unexpectedly changed: %+v",rows)
	}
}

func TestR11SchedulePairRejectUnsafeExistingPartnerChanges(t *testing.T) {
	h:=newR11TestController(t)
	for _,body:=range []string{
		"{\"collection_point\":\"Сургут\",\"name\":\"Сург-Тал-Сург 7\",\"phase\":\"A\",\"pair_name\":\"Сург-Тал-Сург 7.\",\"active\":true}",
		"{\"collection_point\":\"Сургут\",\"name\":\"Сург-Тал-Сург 7.\",\"phase\":\"B\",\"pair_name\":\"Сург-Тал-Сург 7\",\"active\":true}",
		"{\"collection_point\":\"Сургут\",\"name\":\"Сург-Тал-Сург 16\",\"phase\":\"B\",\"active\":true}",
	} {
		rec:=callR11Core(h,http.MethodPost,"/api/autograf/schedule-type/save",body)
		if rec.Code!=http.StatusOK { t.Fatalf("seed: %d %s",rec.Code,rec.Body.String()) }
	}
	rows:=readSchedulePairRows(t,h)
	rec:=callR11Core(h,http.MethodPost,"/api/autograf/schedule-type/pair",
		fmt.Sprintf("{\"id\":%d,\"partner_name\":\"Сург-Тал-Сург 16\"}",r11Int(rows[0],"id")))
	if rec.Code!=http.StatusBadRequest { t.Fatalf("unsafe re-pair must fail: %d %s",rec.Code,rec.Body.String()) }
	after:=readSchedulePairRows(t,h)
	if len(after)!=3 || r11String(after[0],"pair_name")!="Сург-Тал-Сург 7." ||
		r11String(after[1],"pair_name")!="Сург-Тал-Сург 7" ||
		r11String(after[2],"pair_name")!="" {
		t.Fatalf("unsafe re-pair mutated existing links: %+v",after)
	}
}

func TestR11SchedulePairRejectWrongPhaseForExistingNumberedPartner(t *testing.T) {
	h:=newR11TestController(t)
	for _,body:=range []string{
		"{\"collection_point\":\"Сургут\",\"name\":\"Сург-Тал-Сург 7\",\"phase\":\"A\",\"active\":true}",
		"{\"collection_point\":\"Сургут\",\"name\":\"Сург-Тал-Сург 16\",\"phase\":\"A\",\"active\":true}",
	} {
		rec:=callR11Core(h,http.MethodPost,"/api/autograf/schedule-type/save",body)
		if rec.Code!=http.StatusOK { t.Fatalf("seed: %d %s",rec.Code,rec.Body.String()) }
	}
	rows:=readSchedulePairRows(t,h)
	rec:=callR11Core(h,http.MethodPost,"/api/autograf/schedule-type/pair",
		fmt.Sprintf("{\"id\":%d,\"partner_name\":\"Сург-Тал-Сург 16\"}",r11Int(rows[0],"id")))
	if rec.Code!=http.StatusBadRequest { t.Fatalf("wrong phase must refuse: %d %s",rec.Code,rec.Body.String()) }
	if len(readSchedulePairRows(t,h))!=2 { t.Fatal("wrong-phase link wrote new rows") }
}

func r11CycleRowsForTest(t *testing.T, h *controller, key string) []map[string]json.RawMessage {
	t.Helper()
	root:=readR11CoreState(t,h)
	var rows []map[string]json.RawMessage
	if len(root[key])>0 {
		if err:=json.Unmarshal(root[key],&rows);err!=nil {t.Fatalf("%s: %v",key,err)}
	}
	return rows
}
func r11MakeCycleForTest(t *testing.T,h *controller,point,first,second,name string,withTypes,adoptOnly bool) int64 {
	t.Helper()
	b:=fmt.Sprintf("{\"collection_point\":%q,\"first\":%q,\"second\":%q,\"year\":2026,\"create_types\":%t,\"adopt_only\":%t,\"name\":%q,\"phase\":\"A\"}",point,first,second,withTypes,adoptOnly,name)
	resp:=callR11Core(h,http.MethodPost,"/api/autograf/cycle/save",b)
	if resp.Code!=http.StatusOK {t.Fatalf("create/adopt cycle: %d %s",resp.Code,resp.Body.String())}
	var result map[string]any
	if err:=json.Unmarshal(resp.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
	return int64(result["cycle_id"].(float64))
}
func TestR11IndependentSurgutCyclesAtomicSave(t *testing.T) {
	h:=newR11TestController(t)
	a:=r11MakeCycleForTest(t,h,"Сургут","2026-01-07","2026-01-29","Сург-Тал-Сург 7",true,false)
	b:=r11MakeCycleForTest(t,h,"Сургут","2026-01-16","2026-02-13","Сург-Тал-Сург 16",true,false)
	if a==b||a<=0||b<=0 {t.Fatalf("cycle identifiers must be distinct: %d %d",a,b)}
	cycles:=r11CycleRowsForTest(t,h,"flight_cycles")
	if len(cycles)!=2 {t.Fatalf("expected exactly 2 independent cycles, got %d",len(cycles))}
	counts:=map[int64]int{}
	for _,x:=range r11CycleRowsForTest(t,h,"flight_dates"){
		counts[r11Int(x,"cycle_id")]++
		if q:=r11Int(x,"sequence");q!=1&&q!=2 {t.Fatalf("flight without proper phase: %+v",x)}
	}
	if counts[a]!=14 || counts[b]!=13 {t.Fatalf("Surgut flight counts must be 14/13, got %+v",counts)}
	// A standalone type pair must never be created without a cycle link when two cycles exist.
	standalone:=callR11Core(h,http.MethodPost,"/api/autograf/schedule-type/pair",
		"{\"collection_point\":\"Сургут\",\"name\":\"Вид без цикла\",\"phase\":\"A\"}")
	if standalone.Code!=http.StatusBadRequest {t.Fatalf("unbound type for 2 cycles must be refused: %d",standalone.Code)}

	types:=r11CycleRowsForTest(t,h,"schedule_types")
	if len(types)!=4 {t.Fatalf("expected four types for two separate pairs, got %d",len(types))}
	want:=[]struct{title,pair,phase string;id int64}{
		{"Сург-Тал-Сург 7","Сург-Тал-Сург 7.","A",a},
		{"Сург-Тал-Сург 7.","Сург-Тал-Сург 7","B",a},
		{"Сург-Тал-Сург 16","Сург-Тал-Сург 16.","A",b},
		{"Сург-Тал-Сург 16.","Сург-Тал-Сург 16","B",b},
	}
	for i,x:=range want {
		y:=types[i]
		if r11String(y,"name")!=x.title||r11String(y,"pair_name")!=x.pair||
		   r11String(y,"phase")!=x.phase||r11Int(y,"cycle_id")!=x.id{
			t.Fatalf("type %d not independently bound: %+v",i,y)
		}
	}
	// Repeating the same save cannot duplicate dates or types.
	again:=r11MakeCycleForTest(t,h,"Сургут","2026-01-07","2026-01-29","Сург-Тал-Сург 7",true,false)
	if again!=a || len(r11CycleRowsForTest(t,h,"flight_dates"))!=27 ||
		len(r11CycleRowsForTest(t,h,"schedule_types"))!=4 {
		t.Fatal("repeat cycle save created duplicate or lost identifier")
	}
}
func TestR11CycleAdoptPreservesExistingLegacyDatesWithoutRebuilding(t *testing.T) {
	h:=newR11TestController(t)
	legacy:=[]struct{date,sequence string}{
		{"2026-01-07","Первая последовательность рейса"},
		{"2026-01-29","Вторая последовательность рейса"},
	}
	for _,x:=range legacy{
		note:=x.sequence+". Цикл от 07.01.2026 / 29.01.2026"
		body:=fmt.Sprintf("{\"collection_point\":\"Сургут\",\"date\":%q,\"note\":%q,\"source\":\"Ручной ввод\"}",x.date,note)
		rec:=callR11Core(h,http.MethodPost,"/api/autograf/flight/save",body)
		if rec.Code!=http.StatusOK{t.Fatalf("legacy save: %d %s",rec.Code,rec.Body.String())}
	}
	original:=r11CycleRowsForTest(t,h,"flight_dates")
	if len(original)!=2 {t.Fatal("legacy setup failed")}
	cid:=r11MakeCycleForTest(t,h,"Сургут","2026-01-07","2026-01-29","Сург-Тал-Сург 7",true,true)
	after:=r11CycleRowsForTest(t,h,"flight_dates")
	if len(after)!=2 {t.Fatalf("adopt_only invented %d extra dates",len(after)-2)}
	for i,x:=range after {
		if r11Int(x,"id")!=r11Int(original[i],"id")||r11String(x,"note")!=r11String(original[i],"note")||
			r11Int(x,"cycle_id")!=cid||r11Int(x,"sequence")!=int64(i+1){
			t.Fatalf("legacy date was overwritten: %+v",x)
		}
	}
	types:=r11CycleRowsForTest(t,h,"schedule_types")
	if len(types)!=2||r11Int(types[0],"cycle_id")!=cid||r11Int(types[1],"cycle_id")!=cid{
		t.Fatalf("adopt did not create bound pair: %+v",types)
	}
}
func TestR11CycleFlightCorrectionKeepsOtherFlightsAndPlans(t *testing.T){
	h:=newR11TestController(t)
	cid:=r11MakeCycleForTest(t,h,"Сургут","2026-01-07","2026-01-29","Сург-Тал-Сург 7",true,false)
	otherId:=r11MakeCycleForTest(t,h,"Сургут","2026-01-16","2026-02-13","Сург-Тал-Сург 16",true,false)
	before:=r11CycleRowsForTest(t,h,"flight_dates")
	var target int64
	var oldDates=map[int64]string{}
	for _,x:=range before{
		oldDates[r11Int(x,"id")]=r11String(x,"date")
		if r11Int(x,"cycle_id")==cid&&r11String(x,"date")=="2026-01-29"{target=r11Int(x,"id")}
	}
	if target<=0 {t.Fatal("missing Jan 29 reference flight")}
	// Recorded work plan is never implicitly touched by editing the source flight.
	emp:=callR11Core(h,http.MethodPost,"/api/employee/save",
		"{\"tab_no\":\"SYN-9001\",\"full_name\":\"Тестовый сотрудник\",\"collection_point\":\"Сургут\",\"schedule_type\":\"Сург-Тал-Сург 7\"}")
	if emp.Code!=http.StatusOK {t.Fatalf("employee setup %d %s",emp.Code,emp.Body.String())}
	day:=callR11Core(h,http.MethodPost,"/api/day/save",
		"{\"TabNo\":\"SYN-9001\",\"Date\":\"2026-01-30\",\"Plan\":\"1/11\",\"PlanHours\":11}")
	if day.Code!=http.StatusOK {t.Fatalf("saved plan setup %d %s",day.Code,day.Body.String())}
	daysBefore:=string(readR11CoreState(t,h)["days"])
	change:=fmt.Sprintf("{\"id\":%d,\"date\":\"2026-01-25\",\"reason\":\"Согласован перенос рейса\"}",target)
	rec:=callR11Core(h,http.MethodPost,"/api/autograf/cycle/flight/move",change)
	if rec.Code!=http.StatusOK {t.Fatalf("move failed: %d %s",rec.Code,rec.Body.String())}
	after:=r11CycleRowsForTest(t,h,"flight_dates")
	if len(after)!=len(before) {t.Fatalf("date move changed flight count: %d %d",len(before),len(after))}
	for _,x:=range after{
		id:=r11Int(x,"id")
		want:=oldDates[id]
		if id==target {
			want="2026-01-25"
			if r11String(x,"adjusted_from")!="2026-01-29"||r11Int(x,"cycle_id")!=cid||r11Int(x,"sequence")!=2{
				t.Fatalf("corrected date lost cycle/sequence: %+v",x)
			}
		}
		if r11String(x,"date")!=want {t.Fatalf("unrelated flight changed: id=%d got=%s expected=%s",id,r11String(x,"date"),want)}
		if id!=target && r11Int(x,"cycle_id")==otherId && oldDates[id]!=r11String(x,"date"){
			t.Fatal("other independent cycle was modified")
		}
	}
	if string(readR11CoreState(t,h)["days"])!=daysBefore {t.Fatal("flight correction changed employee plans")}
	// The former generic editor must no longer mutate dates of a bound cycle.
	bypass:=callR11Core(h,http.MethodPost,"/api/autograf/flight/save",
		fmt.Sprintf("{\"id\":%d,\"collection_point\":\"Сургут\",\"date\":\"2026-01-24\"}",target))
	if bypass.Code!=http.StatusBadRequest {t.Fatalf("generic editor bypassed safety guard: %d",bypass.Code)}
	// A new date coinciding with a date of the SAME cycle must be rejected.
	bad:=callR11Core(h,http.MethodPost,"/api/autograf/cycle/flight/move",
		fmt.Sprintf("{\"id\":%d,\"date\":\"2026-02-26\",\"reason\":\"ошибочный перенос\"}",target))
	if bad.Code!=http.StatusBadRequest {t.Fatalf("same-cycle conflict not rejected: %d",bad.Code)}
	if len(r11CycleRowsForTest(t,h,"flight_dates"))!=27 {t.Fatal("rejected operation changed state")}
	// Rerunning an old cycle calculation must not silently recreate a date
	// that the operator already moved (Jan 29 → Jan 25).
	rebuild:=callR11Core(h,http.MethodPost,"/api/autograf/cycle/save",
		"{\"collection_point\":\"Сургут\",\"first\":\"2026-01-07\",\"second\":\"2026-01-29\",\"year\":2026,\"create_types\":false}")
	if rebuild.Code!=http.StatusBadRequest {t.Fatalf("mass rebuild after manual date move must need separate scenario: %d %s",rebuild.Code,rebuild.Body.String())}
	for _,flight:=range r11CycleRowsForTest(t,h,"flight_dates") {
		if r11Int(flight,"id")==target && r11String(flight,"date")!="2026-01-25" {t.Fatal("rejected mass rebuild changed the corrected date")}
	}

}
// Returning an individually moved flight to its original date removes only
// the *active* shift marker. The audit journal retains both corrections.
func TestR11FlightRestoreClearsActiveMoveWithoutAffectingOtherCycle(t *testing.T) {
    h:=newR11TestController(t)
    cid:=r11MakeCycleForTest(t,h,"Сургут","2026-01-07","2026-01-29","Сург-Тал-Сург 7",true,false)
    other:=r11MakeCycleForTest(t,h,"Сургут","2026-01-16","2026-02-13","Сург-Тал-Сург 16",true,false)
    before:=r11CycleRowsForTest(t,h,"flight_dates")
    target:=int64(0)
    for _,item:=range before {
        if r11Int(item,"cycle_id")==cid && r11String(item,"date")=="2026-01-29" {target=r11Int(item,"id")}
    }
    if target==0 {t.Fatal("missing January 29 flight")}
    for _,date:=range []string{"2026-01-25","2026-01-29"} {
        body:=fmt.Sprintf("{\"id\":%d,\"date\":%q,\"reason\":\"Учебный перенос и восстановление\"}",target,date)
        r:=callR11Core(h,http.MethodPost,"/api/autograf/cycle/flight/move",body)
        if r.Code!=http.StatusOK {t.Fatalf("move to %s: %d %s",date,r.Code,r.Body.String())}
    }
    after:=r11CycleRowsForTest(t,h,"flight_dates")
    if len(after)!=27 {t.Fatalf("flight count changed: %d",len(after))}
    for _,item:=range after {
        if r11Int(item,"id")==target {
            if r11String(item,"date")!="2026-01-29" || r11String(item,"adjusted_from")!="" {
                t.Fatalf("restored flight still appears moved: %+v",item)
            }
        } else if r11Int(item,"cycle_id")==other {
            matched:=false
            for _,original:=range before {
                if r11Int(original,"id")==r11Int(item,"id") && r11String(original,"date")==r11String(item,"date"){matched=true}
            }
            if !matched {t.Fatalf("independent cycle changed: %+v",item)}
        }
    }
    // This was blocked before the fix because adjusted_from remained set.
    rebuild:=callR11Core(h,http.MethodPost,"/api/autograf/cycle/save",
       `{"collection_point":"Сургут","first":"2026-01-07","second":"2026-01-29","year":2026,"create_types":false}`)
    if rebuild.Code!=http.StatusOK {
        t.Fatalf("fully restored cycle is still blocked: %d %s",rebuild.Code,rebuild.Body.String())
    }
    data:=readR11CoreState(t,h)
    if strings.Count(string(data["journal"]),"Учебный перенос и восстановление")<2 {
        t.Fatalf("two correction steps missing from journal")
    }
}

func TestR11TwoCyclesMayContainSameCalendarDateWithoutOverwrite(t *testing.T) {
	h:=newR11TestController(t)
	a:=r11MakeCycleForTest(t,h,"Сургут","2026-01-01","2026-01-29","Сург-Тал-Сург 1",true,false)
	b:=r11MakeCycleForTest(t,h,"Сургут","2026-01-29","2026-02-26","Сург-Тал-Сург 29",true,false)
	found:=map[int64]bool{}
	for _,x:=range r11CycleRowsForTest(t,h,"flight_dates"){
		if r11String(x,"date")=="2026-01-29"{found[r11Int(x,"cycle_id")]=true}
	}
	if len(found)!=2||!found[a]||!found[b] {t.Fatalf("same calendar day must belong to two distinct cycles: %+v",found)}
}

func TestR11CyclePairDefaultsToStandardDotConvention(t *testing.T) {
 h:=newR11TestController(t)
 resp:=callR11Core(h,http.MethodPost,"/api/autograf/cycle/save",
   `{"collection_point":"Сургут","first":"2026-01-07","second":"2026-01-29","year":2026,"create_types":true,"name":"Сург-Тал-Сург 7"}`)
 if resp.Code!=http.StatusOK {t.Fatalf("default pair: %d %s",resp.Code,resp.Body.String())}
 rows:=r11CycleRowsForTest(t,h,"schedule_types")
 if len(rows)!=2||r11String(rows[0],"name")!="Сург-Тал-Сург 7"||r11String(rows[0],"phase")!="A"||r11String(rows[1],"name")!="Сург-Тал-Сург 7."||r11String(rows[1],"phase")!="B"||r11Int(rows[0],"cycle_id")<=0||r11Int(rows[0],"cycle_id")!=r11Int(rows[1],"cycle_id"){t.Fatalf("wrong default pair: %v",rows)}
}
