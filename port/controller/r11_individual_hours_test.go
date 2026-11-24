package main

import (
    "encoding/json"
    "net/http"
    "strings"
    "testing"
)

func TestR11EmployeeWeekAndShiftPersistAndProtectHireDate(t *testing.T) {
    h := newR11TestController(t)
    create := callR11Core(h, http.MethodPost, "/api/employee/save",
        `{"tab_no":"W-36","full_name":"Учебная сотрудница","collection_point":"Сургут","hire_date":"2026-11-01","fire_date":"2026-11-30","week_hours":36,"shift_hours":10,"norm_reason":"36 часов по установленному режиму"}`)
    if create.Code != http.StatusOK {
        t.Fatalf("employee create status=%d: %s", create.Code, create.Body.String())
    }
    data := readR11CoreState(t,h)
    var employees []map[string]json.RawMessage
    if err:=json.Unmarshal(data["employees"], &employees);err!=nil {t.Fatal(err)}
    if len(employees)!=1 || r11Float(employees[0],"week_hours")!=36 ||
       r11Int(employees[0],"shift_hours")!=10 ||
       r11String(employees[0],"norm_reason")!="36 часов по установленному режиму" {
        t.Fatalf("individual hours were not persisted: %+v",employees)
    }
    for _, tc := range []struct{date string; want int}{
       {"2026-10-31",http.StatusBadRequest},
       {"2026-11-01",http.StatusOK},
       {"2026-11-30",http.StatusOK},
       {"2026-12-01",http.StatusBadRequest},
    }{
       resp:=callR11Core(h,http.MethodPost,"/api/day/quick-save",
         `{"TabNo":"W-36","Date":"`+tc.date+`","Plan":"10","PlanHours":10,"State":"WORK"}`)
       if resp.Code!=tc.want {t.Fatalf("date %s: got=%d, want=%d; %s",tc.date,resp.Code,tc.want,resp.Body.String())}
    }
    data=readR11CoreState(t,h)
    var days []map[string]json.RawMessage
    if err:=json.Unmarshal(data["days"],&days);err!=nil{t.Fatal(err)}
    if len(days)!=2 {t.Fatalf("out-of-employment dates were stored: %+v",days)}
    change:=callR11Core(h,http.MethodPost,"/api/employee/save",
      `{"id":1,"tab_no":"W-36","full_name":"Учебная сотрудница","collection_point":"Сургут","schedule_type":"Сург-Тал-Сург 7","hire_date":"2026-11-01","fire_date":"2026-11-30","week_hours":36,"shift_hours":10,"norm_reason":"36 часов по установленному режиму"}`)
    if change.Code!=http.StatusOK {t.Fatalf("update status=%d %s",change.Code,change.Body.String())}
    data=readR11CoreState(t,h)
    if err:=json.Unmarshal(data["employees"],&employees);err!=nil{t.Fatal(err)}
    if r11Float(employees[0],"week_hours")!=36 || r11Int(employees[0],"shift_hours")!=10 {
       t.Fatalf("schedule change cleared week or shift hours: %+v",employees[0])
    }
}

func TestR11EmployeeRejectInvalidWeekOrShiftWithoutMutatingData(t *testing.T) {
    h:=newR11TestController(t)
    bad:=[]string{
      `{"tab_no":"W-X","full_name":"Тест","week_hours":41,"shift_hours":10}`,
      `{"tab_no":"W-X","full_name":"Тест","week_hours":"36","shift_hours":10}`,
      `{"tab_no":"W-X","full_name":"Тест","week_hours":36,"shift_hours":12}`,
      `{"tab_no":"W-X","full_name":"Тест","week_hours":36,"shift_hours":"10"}`,
      `{"tab_no":"W-X","full_name":"Тест","week_hours":36.123,"shift_hours":10}`,
    }
    for i,body:=range bad{
      r:=callR11Core(h,http.MethodPost,"/api/employee/save",body)
      if r.Code!=http.StatusBadRequest {
         t.Fatalf("bad employee #%d accepted (%d): %s",i,r.Code,r.Body.String())
      }
    }
    state:=readR11CoreState(t,h)
    if !strings.Contains(string(state["employees"]), "[]") {
      t.Fatalf("invalid employee mutated state: %s",state["employees"])
    }
}
