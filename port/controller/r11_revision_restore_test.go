package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
)

func r11MustPost(t *testing.T, h *controller, path, payload string) {
	t.Helper()
	rec:=callR11Core(h,http.MethodPost,path,payload)
	if rec.Code!=http.StatusOK{
		t.Fatalf("%s HTTP %d: %s",path,rec.Code,rec.Body.String())
	}
}

func r11Rows(t *testing.T, root map[string]json.RawMessage,key string) []map[string]json.RawMessage {
	t.Helper()
	var rows []map[string]json.RawMessage
	if err:=json.Unmarshal(root[key],&rows);err!=nil{t.Fatalf("decode %s: %v",key,err)}
	return rows
}

func TestR11OfficialRevisionActivationRestoresFourCollectionsAndKeepsOtherEmployee(t *testing.T) {
	h:=newR11TestController(t)
	r11MustPost(t,h,"/api/employee/save",`{"tab_no":"REV-1","full_name":"Тест редакции","schedule_type":"A"}`)
	r11MustPost(t,h,"/api/employee/save",`{"tab_no":"REV-2","full_name":"Другой сотрудник","schedule_type":"B"}`)
	data:=readR11CoreState(t,h)
	emps:=r11Rows(t,data,"employees")
	id1,id2:=r11Int(emps[0],"id"),r11Int(emps[1],"id")

	r11MustPost(t,h,"/api/day/save",`{"TabNo":"REV-1","Date":"2026-11-20","Plan":"1/8","PlanHours":8,"Fact":"1/8","FactHours":8}`)
	r11MustPost(t,h,"/api/day/save",`{"TabNo":"REV-2","Date":"2026-11-20","Plan":"2/8","PlanHours":8,"Fact":"2/8","FactHours":8}`)
	r11MustPost(t,h,"/api/event/save",`{"TabNo":"REV-1","DateFrom":"2026-11-21","EventType":"VACATION"}`)
	r11MustPost(t,h,"/api/note/save",`{"tab_no":"REV-1","date_from":"2026-11-21","date_to":"2026-11-21","text":"Первая заметка"}`)
	r11MustPost(t,h,"/api/color/save",`{"tab_no":"REV-1","date_from":"2026-11-21","date_to":"2026-11-21","color":"yellow"}`)
	r11MustPost(t,h,"/api/autograf/revision/create",`{"tab_no":"REV-1","effective_date":"2026-11-01","schedule_type":"A","reason":"Исходное состояние"}`)
	data=readR11CoreState(t,h)
	revisions:=r11Rows(t,data,"graph_revisions")
	if len(revisions)!=1||r11String(revisions[0],"status")!="ACTIVE"{t.Fatalf("initial revision: %+v",revisions)}
	r1:=r11Int(revisions[0],"id")

	r11MustPost(t,h,"/api/day/quick-save",`{"TabNo":"REV-1","Date":"2026-11-20","Plan":"1/10","PlanHours":10}`)
	r11MustPost(t,h,"/api/event/save",`{"TabNo":"REV-1","DateFrom":"2026-11-22","EventType":"SICK"}`)
	r11MustPost(t,h,"/api/note/save",`{"tab_no":"REV-1","date_from":"2026-11-22","date_to":"2026-11-22","text":"Вторая заметка"}`)
	r11MustPost(t,h,"/api/color/save",`{"tab_no":"REV-1","date_from":"2026-11-22","date_to":"2026-11-22","color":"pink"}`)
	r11MustPost(t,h,"/api/autograf/revision/create",`{"tab_no":"REV-1","effective_date":"2026-11-20","schedule_type":"B","reason":"Исправленный график"}`)
	data=readR11CoreState(t,h)
	revisions=r11Rows(t,data,"graph_revisions")
	if len(revisions)!=2{t.Fatalf("revision count=%d",len(revisions))}
	r2:=r11Int(revisions[1],"id")
	if r11String(revisions[0],"status")!="ARCHIVED"||r11String(revisions[1],"status")!="ACTIVE"{t.Fatal("incorrect revision statuses after creation")}
	initialDays:=r11Rows(t,revisions[0],"days_snapshot")
	if len(initialDays)!=1||r11Float(initialDays[0],"plan_hours")!=8{t.Fatalf("historical snapshot unexpectedly overwritten: %+v",initialDays)}

	r11MustPost(t,h,"/api/autograf/revision/activate",fmt.Sprintf(`{"id":%d}`,r1))
	data=readR11CoreState(t,h)
	for _,test:=range []struct{key string;expected int}{
		{"days",1},{"events",1},{"notes",1},{"manual_colors",1},
	}{
		got:=0
		for _,item:=range r11Rows(t,data,test.key){if r11Int(item,"employee_id")==id1{got++}}
		if got!=test.expected{t.Fatalf("restored %s count=%d want %d",test.key,got,test.expected)}
	}
	days:=r11Rows(t,data,"days")
	for _,day:=range days{
		switch r11Int(day,"employee_id"){
		case id1:if r11Float(day,"plan_hours")!=8||r11String(day,"plan")!="1/8"{t.Fatalf("first employee restored incorrectly: %+v",day)}
		case id2:if r11String(day,"plan")!="2/8"{t.Fatalf("second employee changed unexpectedly: %+v",day)}
		default:t.Fatalf("unexpected employee id: %+v",day)
		}
	}
	revisions=r11Rows(t,data,"graph_revisions")
	if r11String(revisions[0],"status")!="ACTIVE"||r11String(revisions[1],"status")!="ARCHIVED"{t.Fatalf("statuses after activation: %+v",revisions)}
	currentB:=r11Rows(t,revisions[1],"days_snapshot")
	if len(currentB)!=1||r11Float(currentB[0],"plan_hours")!=10{t.Fatalf("new revision did not capture current state before rollback: %+v",currentB)}

	r11MustPost(t,h,"/api/autograf/revision/activate",fmt.Sprintf(`{"id":%d}`,r2))
	data=readR11CoreState(t,h)
	events:=r11Rows(t,data,"events");notes:=r11Rows(t,data,"notes");colors:=r11Rows(t,data,"manual_colors")
	if len(events)!=2||len(notes)!=2||len(colors)!=2{t.Fatalf("re-activation failed to restore events/notes/colors: %d/%d/%d",len(events),len(notes),len(colors))}
	days=r11Rows(t,data,"days")
	for _,day:=range days{if r11Int(day,"employee_id")==id1&&r11String(day,"plan")!="1/10"{t.Fatalf("re-activation failed to restore plan: %+v",day)}}
}

func TestR11OfficialRevisionActivationRejectsIncompleteSnapshotWithoutWrite(t *testing.T) {
	h:=newR11TestController(t)
	r11MustPost(t,h,"/api/employee/save",`{"tab_no":"REV-M","full_name":"Проверка целостности"}`)
	r11MustPost(t,h,"/api/day/save",`{"TabNo":"REV-M","Date":"2026-11-20","Plan":"1/8","PlanHours":8}`)
	r11MustPost(t,h,"/api/autograf/revision/create",`{"tab_no":"REV-M","effective_date":"2026-11-01","schedule_type":"A"}`)
	r11MustPost(t,h,"/api/day/quick-save",`{"TabNo":"REV-M","Date":"2026-11-20","Plan":"1/10","PlanHours":10}`)
	r11MustPost(t,h,"/api/autograf/revision/create",`{"tab_no":"REV-M","effective_date":"2026-11-21","schedule_type":"B"}`)
	before:=readR11CoreState(t,h)
	revisions:=r11Rows(t,before,"graph_revisions")
	id:=r11Int(revisions[0],"id")
	delete(revisions[0],"colors_snapshot")
	r11Put(before,"graph_revisions",revisions)
	data,err:=json.MarshalIndent(before,"","  ");if err!=nil{t.Fatal(err)}
	if err:=os.WriteFile(h.mainDBPath(),data,0600);err!=nil{t.Fatal(err)}
	original,err:=os.ReadFile(h.mainDBPath());if err!=nil{t.Fatal(err)}

	rec:=callR11Core(h,http.MethodPost,"/api/autograf/revision/activate",fmt.Sprintf(`{"id":%d}`,id))
	if rec.Code!=http.StatusBadRequest{t.Fatalf("expected safe rejection, got HTTP %d: %s",rec.Code,rec.Body.String())}
	after,err:=os.ReadFile(h.mainDBPath());if err!=nil{t.Fatal(err)}
	if string(after)!=string(original){t.Fatal("failed restore changed database despite invalid historical snapshot")}
}
