package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
