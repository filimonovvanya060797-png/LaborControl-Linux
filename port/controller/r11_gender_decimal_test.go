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


func TestR11FiveDayAtomicSaveAndModePreservation(t *testing.T) {
    h:=newR11TestController(t)
    create:=callR11Core(h,http.MethodPost,"/api/employee/batch",
      `{"employees":[{"tab_no":"FIVE","full_name":"Учебная пятидневка","gender":"F","week_hours":36,"work_mode":"FIVE_DAY","hire_date":"2026-11-02","fire_date":"2026-11-04","notes":"Сохранить"},
                    {"tab_no":"OLD","full_name":"Старый вахтовый","gender":"M"}]}`)
    if create.Code!=http.StatusOK {t.Fatal(create.Body.String())}
    rows:=r11EmployeeTestRows(t,h)
    id:=r11Int(rows[0],"id")
    if r11EmployeeMode(rows[0])!="FIVE_DAY"||r11EmployeeMode(rows[1])!="SHIFT" {t.Fatal("work modes not preserved")}
    update:=callR11Core(h,http.MethodPost,"/api/employee/save",
      fmt.Sprintf(`{"id":%d,"tab_no":"FIVE","full_name":"Учебная пятидневка","position":"Техник"}`,id))
    if update.Code!=http.StatusOK {t.Fatal(update.Body.String())}
    if row:=r11EmployeeTestRows(t,h)[0];r11EmployeeMode(row)!="FIVE_DAY"||r11Float(row,"week_hours")!=36||r11String(row,"notes")!="Сохранить"{t.Fatal("partial employee update erased data")}
    old:=callR11Core(h,http.MethodPost,"/api/day/save",`{"TabNo":"FIVE","Date":"2026-11-02","Plan":"8","PlanHours":8,"Fact":"8","FactHours":8,"Comment":"Не стирать"}`)
    if old.Code!=http.StatusOK {t.Fatal(old.Body.String())}
    other:=callR11Core(h,http.MethodPost,"/api/day/save",`{"TabNo":"OLD","Date":"2026-11-02","Plan":"11","PlanHours":11}`)
    if other.Code!=http.StatusOK {t.Fatal(other.Body.String())}
    invalid:=callR11Core(h,http.MethodPost,"/api/autograf/five-day/save",
      fmt.Sprintf(`{"employee_id":%d,"week_hours":36,"days":[{"date":"2026-11-02","plan":"7.2","hours":7.2,"state":"WORK"},{"date":"2026-11-03","plan":"7","hours":7,"state":"WORK"}]}`,id))
    if invalid.Code!=http.StatusBadRequest {t.Fatalf("invalid preview accepted %d %s",invalid.Code,invalid.Body.String())}
    before:=readR11CoreState(t,h)
    var prior []map[string]json.RawMessage
    if err:=json.Unmarshal(before["days"],&prior);err!=nil{t.Fatal(err)}
    if len(prior)!=2||r11Float(prior[0],"plan_hours")!=8{t.Fatal("partial write after invalid preview")}
    valid:=callR11Core(h,http.MethodPost,"/api/autograf/five-day/save",
      fmt.Sprintf(`{"employee_id":%d,"week_hours":36,"days":[{"date":"2026-11-02","plan":"7.2","hours":7.2,"state":"WORK"},{"date":"2026-11-03","plan":"6.2","hours":6.2,"state":"WORK"},{"date":"2026-11-04","plan":"В","hours":0,"state":"FREE"}]}`,id))
    if valid.Code!=http.StatusOK {t.Fatalf("valid preview rejected: %d %s",valid.Code,valid.Body.String())}
    data:=readR11CoreState(t,h)
    var days,versions []map[string]json.RawMessage
    if err:=json.Unmarshal(data["days"],&days);err!=nil{t.Fatal(err)}
    if err:=json.Unmarshal(data["graph_versions"],&versions);err!=nil{t.Fatal(err)}
    if len(days)!=4||len(versions)!=1{t.Fatalf("days=%d versions=%d",len(days),len(versions))}
    if r11Float(days[0],"plan_hours")!=7.2||r11Float(days[0],"fact_hours")!=8||r11String(days[0],"comment")!="Не стирать"||r11Float(days[1],"plan_hours")!=11{t.Fatal("fact/comment or other employee changed")}
    badMode:=callR11Core(h,http.MethodPost,"/api/employee/save",fmt.Sprintf(`{"id":%d,"tab_no":"FIVE","full_name":"Учебная пятидневка","work_mode":"SHIFT"}`,id))
    if badMode.Code!=http.StatusBadRequest{t.Fatal("historical work mode silently changed")}
    after:=r11EmployeeTestRows(t,h)
    if r11EmployeeMode(after[0])!="FIVE_DAY"{t.Fatal("mode mutated after refusal")}
}
func TestR11FiveDayNorms(t *testing.T) {
    for _,tc:=range []struct{week,normal,short float64}{{40,8,7},{36,7.2,6.2},{35,7,6},{24,4.8,3.8}} {
        if r11FiveDayNorm(8,tc.week)!=tc.normal||r11FiveDayNorm(7,tc.week)!=tc.short||r11FiveDayNorm(0,tc.week)!=0 {t.Fatalf("week %v failed",tc.week)}
    }
}
