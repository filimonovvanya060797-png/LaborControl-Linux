package main

import (
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
    "testing"
)

// Only synthetic employees and an isolated in-memory/test state are used.
func TestR11IndividualHoursPersistAndGuardHireDate(t *testing.T) {
    h:=newR11TestController(t)
    input:=`{"tab_no":"W-036","full_name":"Учебный сотрудник 36 часов","collection_point":"Сургут","hire_date":"2026-11-01","weekly_hours":36,"shift_hours":10,"norm_basis":"ст. 320 ТК РФ"}`
    rec:=callR11Core(h,http.MethodPost,"/api/employee/save",input)
    if rec.Code!=http.StatusOK {t.Fatalf("employee %d: %s",rec.Code,rec.Body.String())}
    data:=readR11CoreState(t,h)
    var people []map[string]json.RawMessage
    if err:=json.Unmarshal(data["employees"],&people);err!=nil {t.Fatal(err)}
    if len(people)!=1 || r11Int(people[0],"weekly_hours")!=36 || r11Int(people[0],"shift_hours")!=10 ||
        r11String(people[0],"norm_basis")!="ст. 320 ТК РФ" || r11String(people[0],"hire_date")!="2026-11-01" {
        t.Fatalf("employee hours not saved: %+v",people)
    }
    before:=callR11Core(h,http.MethodPost,"/api/day/quick-save",
        `{"TabNo":"W-036","Date":"2026-10-31","Plan":"10","PlanHours":10,"State":"WORK"}`)
    if before.Code!=http.StatusBadRequest || !strings.Contains(before.Body.String(),"до приёма"){
        t.Fatalf("pre-employment write must fail: %d %s",before.Code,before.Body.String())
    }
    if ds:=r11CycleRowsForTest(t,h,"days");len(ds)!=0 {t.Fatal("rejected plan mutated days")}
    day:=callR11Core(h,http.MethodPost,"/api/day/quick-save",
        `{"TabNo":"W-036","Date":"2026-11-01","Plan":"10","PlanHours":10,"State":"WORK"}`)
    if day.Code!=http.StatusOK {t.Fatalf("first employment day: %d %s",day.Code,day.Body.String())}
    ds:=r11CycleRowsForTest(t,h,"days")
    if len(ds)!=1 || r11Int(ds[0],"plan_hours")!=10 {t.Fatalf("unexpected plan: %+v",ds)}
}

func TestR11IndividualHoursRejectsInvalidOrAmbiguousValues(t *testing.T) {
    h:=newR11TestController(t)
    cases:=[]string{
        `"weekly_hours":37,"shift_hours":10`,
        `"weekly_hours":"36","shift_hours":10`,
        `"weekly_hours":36.5,"shift_hours":10`,
        `"weekly_hours":36,"shift_hours":12`,
        `"weekly_hours":36,"shift_hours":null`,
    }
    for i,fields:=range cases {
        body:=fmt.Sprintf(`{"tab_no":"BAD-%d","full_name":"Тест","collection_point":"Сургут",%s}`,i,fields)
        rec:=callR11Core(h,http.MethodPost,"/api/employee/save",body)
        if rec.Code!=http.StatusBadRequest {t.Fatalf("case %d wrongly accepted: %d %s",i,rec.Code,rec.Body.String())}
    }
    if es:=r11CycleRowsForTest(t,h,"employees");len(es)!=0 {t.Fatal("invalid employee payloads were saved")}
}

func TestR11IndividualHoursKeepOnPartialCardUpdate(t *testing.T) {
    h:=newR11TestController(t)
    first:=callR11Core(h,http.MethodPost,"/api/employee/save",
        `{"tab_no":"W-040","full_name":"Тест 40","weekly_hours":40,"shift_hours":11}`)
    if first.Code!=http.StatusOK {t.Fatal(first.Body.String())}
    rows:=r11CycleRowsForTest(t,h,"employees")
    id:=r11Int(rows[0],"id")
    update:=callR11Core(h,http.MethodPost,"/api/employee/save",fmt.Sprintf(
        `{"id":%d,"tab_no":"W-040","full_name":"Изменено"}`,id))
    if update.Code!=http.StatusOK {t.Fatal(update.Body.String())}
    rows=r11CycleRowsForTest(t,h,"employees")
    if r11Int(rows[0],"weekly_hours")!=40 || r11Int(rows[0],"shift_hours")!=11 {
        t.Fatalf("partial update lost hour settings: %+v",rows[0])
    }
}
