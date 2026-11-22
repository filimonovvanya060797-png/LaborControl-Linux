package main

import (
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
    "testing"
)

func r11EmployeeTestRows(t *testing.T, h *controller) []map[string]json.RawMessage {
    t.Helper()
    data := readR11CoreState(t,h)
    var rows []map[string]json.RawMessage
    if err:=json.Unmarshal(data["employees"],&rows);err!=nil{t.Fatal(err)}
    return rows
}

func TestR11EmployeeBatchIsAtomicOnInvalidRow(t *testing.T) {
    h:=newR11TestController(t)
    old:=callR11Core(h,http.MethodPost,"/api/employee/save",`{"tab_no":"980001","full_name":"Исходный","unit":"Колонна 1","notes":"сохранить"}`)
    if old.Code!=http.StatusOK{t.Fatal(old.Body.String())}
    current:=r11EmployeeTestRows(t,h)
    id:=r11Int(current[0],"id")
    body:=fmt.Sprintf(`{"employees":[{"id":%d,"tab_no":"980001","full_name":"Изменённый","unit":"Колонна 2"},{"tab_no":"980002","full_name":"Некорректный","hire_date":"2026-02-31"}]}`,id)
    rec:=callR11Core(h,http.MethodPost,"/api/employee/batch",body)
    if rec.Code!=http.StatusBadRequest{t.Fatalf("status=%d body=%s",rec.Code,rec.Body.String())}
    if !strings.Contains(rec.Body.String(),"строка 2"){t.Fatalf("missing row number: %s",rec.Body.String())}
    rows:=r11EmployeeTestRows(t,h)
    if len(rows)!=1||r11String(rows[0],"full_name")!="Исходный"||r11String(rows[0],"unit")!="Колонна 1"||r11String(rows[0],"notes")!="сохранить"{
        t.Fatalf("batch partially changed database: %+v",rows)
    }
}

func TestR11EmployeeBatchCreatesAndUpdatesTogether(t *testing.T){
    h:=newR11TestController(t)
    first:=callR11Core(h,http.MethodPost,"/api/employee/save",`{"tab_no":"980101","full_name":"Первый","position":"Водитель","collection_point":"Уфа","schedule_type":"А","probation":true,"probation_end":"2027-04-14","notes":"важная заметка"}`)
    if first.Code!=http.StatusOK{t.Fatal(first.Body.String())}
    id:=r11Int(r11EmployeeTestRows(t,h)[0],"id")
    payload:=fmt.Sprintf(`{"employees":[{"id":%d,"tab_no":"980101","full_name":"Первый","position":"Водитель","collection_point":"Ленск","schedule_type":"А","probation":true,"probation_end":"2027-04-14","notes":"важная заметка"},{"tab_no":"980102","full_name":"Второй","position":"Водитель","unit":"Колонна 2"}]}`,id)
    rec:=callR11Core(h,http.MethodPost,"/api/employee/batch",payload)
    if rec.Code!=http.StatusOK{t.Fatalf("batch status=%d body=%s",rec.Code,rec.Body.String())}
    var result map[string]any
    if err:=json.Unmarshal(rec.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
    if result["created"]!=float64(1)||result["updated"]!=float64(1){t.Fatalf("counts: %+v",result)}
    rows:=r11EmployeeTestRows(t,h)
    if len(rows)!=2{t.Fatalf("employee count=%d",len(rows))}
    if r11String(rows[0],"collection_point")!="Ленск"||r11String(rows[0],"schedule_type")!="А"||
        r11String(rows[0],"notes")!="важная заметка"||r11String(rows[0],"probation_end")!="2027-04-14"{
        t.Fatalf("employee fields changed unexpectedly: %+v",rows[0])
    }
    if r11String(rows[1],"tab_no")!="980102"{t.Fatalf("new employee: %+v",rows[1])}
}

func TestR11EmployeeBatchRejectsDuplicateTabsAndIDs(t *testing.T){
    for _,tc:=range []struct{name,body string}{
        {"same tab",`{"employees":[{"tab_no":"980200","full_name":"Один"},{"tab_no":"980200","full_name":"Другой"}]}`},
        {"tab case",`{"employees":[{"tab_no":"T-a","full_name":"Один"},{"tab_no":"t-A","full_name":"Другой"}]}`},
        {"bad date",`{"employees":[{"tab_no":"980201","full_name":"Один"},{"tab_no":"980202","full_name":"Другой","fire_date":"2026-02-31"}]}`},
        {"empty card",`{"employees":[{"tab_no":"980201","full_name":"Один"},null]}`},
    }{
        t.Run(tc.name,func(t *testing.T){
            h:=newR11TestController(t)
            rec:=callR11Core(h,http.MethodPost,"/api/employee/batch",tc.body)
            if rec.Code!=http.StatusBadRequest{t.Fatalf("status=%d body=%s",rec.Code,rec.Body.String())}
            if got:=len(r11EmployeeTestRows(t,h));got!=0{t.Fatalf("partial import: %d employees",got)}
        })
    }
    h:=newR11TestController(t)
    old:=callR11Core(h,http.MethodPost,"/api/employee/save",`{"tab_no":"980211","full_name":"Исходный"}`)
    if old.Code!=http.StatusOK{t.Fatal(old.Body.String())}
    id:=r11Int(r11EmployeeTestRows(t,h)[0],"id")
    body:=fmt.Sprintf(`{"employees":[{"id":%d,"tab_no":"980211","full_name":"Один"},{"id":%d,"tab_no":"980211","full_name":"Два"}]}`,id,id)
    rec:=callR11Core(h,http.MethodPost,"/api/employee/batch",body)
    if rec.Code!=http.StatusBadRequest{t.Fatalf("duplicate ID status=%d body=%s",rec.Code,rec.Body.String())}
    if name:=r11String(r11EmployeeTestRows(t,h)[0],"full_name");name!="Исходный"{t.Fatalf("partial change: %q",name)}
}

func TestR11EmployeeSaveValidatesMergedRecord(t *testing.T){
    h:=newR11TestController(t)
    created:=callR11Core(h,http.MethodPost,"/api/employee/save",`{"tab_no":"980300","full_name":"Пробный","probation":true,"probation_end":"2027-04-14"}`)
    if created.Code!=http.StatusOK{t.Fatal(created.Body.String())}
    id:=r11Int(r11EmployeeTestRows(t,h)[0],"id")
    // A partial client must not disable probation while leaving its old date.
    rec:=callR11Core(h,http.MethodPost,"/api/employee/save",
        fmt.Sprintf(`{"id":%d,"tab_no":"980300","full_name":"Пробный","probation":false}`,id))
    if rec.Code!=http.StatusBadRequest{t.Fatalf("partial invalid update status=%d body=%s",rec.Code,rec.Body.String())}
    rows:=r11EmployeeTestRows(t,h)
    if r11String(rows[0],"probation_end")!="2027-04-14"{
        t.Fatalf("rejected request changed record: %+v",rows[0])
    }
}

func TestR11EmployeeBatchAccepts450Drivers(t *testing.T) {
    h:=newR11TestController(t)
    rows:=make([]map[string]any,450)
    for i:=0;i<450;i++ {
        rows[i]=map[string]any{"tab_no":fmt.Sprintf("99%04d",i+1),"full_name":fmt.Sprintf("Тестовый водитель %d",i+1),"unit":fmt.Sprintf("Колонна %d",1+i/225)}
    }
    payload,err:=json.Marshal(map[string]any{"employees":rows})
    if err!=nil{t.Fatal(err)}
    rec:=callR11Core(h,http.MethodPost,"/api/employee/batch",string(payload))
    if rec.Code!=http.StatusOK{t.Fatalf("450 employee import status=%d body=%s",rec.Code,rec.Body.String())}
    if got:=len(r11EmployeeTestRows(t,h));got!=450{t.Fatalf("employee count=%d",got)}
}
