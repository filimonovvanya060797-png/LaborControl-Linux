package main

import (
    "encoding/json"
    "fmt"
    "net/http"
    "testing"
)

func TestR11GenderImportDefaultsAndPreservation(t *testing.T) {
    h:=newR11TestController(t)
    create:=callR11Core(h,http.MethodPost,"/api/employee/batch",
      `{"employees":[{"tab_no":"M-01","full_name":"Тестовый мужчина","gender":"M","hire_date":"2026-11-01"},
                    {"tab_no":"F-01","full_name":"Тестовая женщина","gender":"F","hire_date":"2026-11-01"},
                    {"tab_no":"U-01","full_name":"Не указан пол","hire_date":"2026-11-01"}]}`)
    if create.Code!=http.StatusOK {t.Fatalf("batch failed: %s",create.Body.String())}
    rows:=r11EmployeeTestRows(t,h)
    if len(rows)!=3 {t.Fatalf("got %d rows",len(rows))}
    if r11String(rows[0],"gender")!="M"||r11Float(rows[0],"week_hours")!=40 {t.Fatalf("male default: %+v",rows[0])}
    if r11String(rows[1],"gender")!="F"||r11Float(rows[1],"week_hours")!=36 {t.Fatalf("female default: %+v",rows[1])}
    if r11String(rows[2],"gender")!=""||len(r11Raw(rows[2],"week_hours"))!=0 {t.Fatalf("unknown gender silently received a norm: %+v",rows[2])}
    oldID:=r11Int(rows[1],"id")
    update:=callR11Core(h,http.MethodPost,"/api/employee/batch",
      fmt.Sprintf(`{"employees":[{"id":%d,"tab_no":"F-01","full_name":"Тестовая женщина обновлена","unit":"Колонна 2"}]}`,oldID))
    if update.Code!=http.StatusOK {t.Fatalf("repeat batch status=%d: %s",update.Code,update.Body.String())}
    rows=r11EmployeeTestRows(t,h)
    if r11String(rows[1],"gender")!="F"||r11Float(rows[1],"week_hours")!=36 {t.Fatalf("repeat import erased gender/norm: %+v",rows[1])}
    before:=r11String(rows[1],"full_name")
    invalid:=callR11Core(h,http.MethodPost,"/api/employee/save",
      fmt.Sprintf(`{"id":%d,"tab_no":"F-01","full_name":"Искажённый","gender":"unknown"}`,oldID))
    if invalid.Code!=http.StatusBadRequest {t.Fatalf("invalid gender accepted: %d",invalid.Code)}
    if got:=r11String(r11EmployeeTestRows(t,h)[1],"full_name");got!=before {t.Fatalf("invalid update mutated employee: %q",got)}
}

func TestR11FractionalDayHoursPersist(t *testing.T) {
    h:=newR11TestController(t)
    create:=callR11Core(h,http.MethodPost,"/api/employee/save",
      `{"tab_no":"D-01","full_name":"Дробные смены","gender":"F","week_hours":36}`)
    if create.Code!=http.StatusOK {t.Fatal(create.Body.String())}
    for _,tc:=range []struct{date,plan string;hours float64}{
        {"2026-11-03","7.3",7.3},
        {"2026-11-04","1/9.2",9.2},
        {"2026-11-05","2/7.25",7.25},
        {"2026-11-06","10.5",10.5},
    }{
        body:=fmt.Sprintf(`{"TabNo":"D-01","Date":%q,"Plan":%q,"PlanHours":%v,"State":"WORK"}`,tc.date,tc.plan,tc.hours)
        r:=callR11Core(h,http.MethodPost,"/api/day/quick-save",body)
        if r.Code!=http.StatusOK {t.Fatalf("%s rejected: %d %s",tc.date,r.Code,r.Body.String())}
    }
    data:=readR11CoreState(t,h)
    var days []map[string]json.RawMessage
    if err:=json.Unmarshal(data["days"],&days);err!=nil {t.Fatal(err)}
    if len(days)!=4 {t.Fatalf("saved %d days, expected 4",len(days))}
    total:=float64(0)
    for _,d:=range days{total+=r11Float(d,"plan_hours")}
    if total<34.249999||total>34.250001 {t.Fatalf("fractional hours not preserved, total=%v",total)}
}
