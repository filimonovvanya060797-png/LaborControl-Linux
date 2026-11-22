package main

import (
    "encoding/json"
    "strings"
    "testing"
)

func TestEmployeeDateAndProbationValidation(t *testing.T) {
    cases:=[]struct{
        name string
        fields map[string]any
        wantError string
    }{
        {"valid no probation",map[string]any{"hire_date":"2026-11-20","fire_date":"","probation":false,"probation_end":""},""},
        {"valid probation",map[string]any{"hire_date":"2026-11-20","probation":true,"probation_end":"2027-02-20"},""},
        {"impossible hire date",map[string]any{"hire_date":"2026-02-31","probation":false},"корректной датой"},
        {"fire before hire",map[string]any{"hire_date":"2026-11-20","fire_date":"2026-11-19","probation":false},"раньше даты приёма"},
        {"probation missing end",map[string]any{"hire_date":"2026-11-20","probation":true,"probation_end":""},"укажите дату окончания"},
        {"end without probation",map[string]any{"probation":false,"probation_end":"2027-02-20"},"галочка не установлена"},
        {"probation end before hire",map[string]any{"hire_date":"2026-11-20","probation":true,"probation_end":"2026-11-19"},"раньше даты приёма"},
    }
    for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
        body:=map[string]json.RawMessage{}
        for k,v:=range tc.fields{ raw,err:=json.Marshal(v);if err!=nil{t.Fatal(err)};body[k]=raw}
        err:=r11StrictEmployeePayload(body)
        if tc.wantError=="" {if err!=nil{t.Fatalf("unexpected validation error: %v",err)};return}
        if err==nil||!strings.Contains(err.Error(),tc.wantError){
            t.Fatalf("want error containing %q, got %v",tc.wantError,err)
        }
    })}
}

func TestR11UIIncludesReadableGraphNotesAndLocalConfirm(t *testing.T){
    s:=string(r11OriginalHTML)
    for _,v:=range []string{
        "Примечание сотрудника",
        "<b>Испытательный срок</b>",
        "replace(/^Испытательный срок\\s+/",
        "Подтверждение удаления",
        "Да, удалить",
        "Подтверждение импорта сотрудников",
        "ФИО не совпадает с карточкой",
        "Неверная дата приёма",
        "/api/employee/points/template.xlsx",
    }{
        if !strings.Contains(s,v){t.Fatalf("R11 UI missing %q",v)}
    }
    if strings.Contains(s, "Испытательный срок: ") {
        t.Fatal("R11 UI still contains the duplicate static probation line")
    }
}
