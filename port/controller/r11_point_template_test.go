package main

import (
    "archive/zip"
    "bytes"
    "strings"
    "testing"
)

func TestCollectionPointTemplateXLSXRoundTrip(t *testing.T){
    rows:=[][]string{
        {"Табельный №","ФИО","Подразделение","Пункт сбора / вылета","Примечание"},
        {"0002","Тестов Алексей Иванович","Колонна 1","",""},
        {"22","Ручнов Сергей & Олегович","Колонна 2","",""},
        {"23","<Тест>. Запрещено \"формулы\"","Колонна 2","",""},
    }
    out,err:=r11CollectionPointXLSX(rows)
    if err!=nil{t.Fatal(err)}
    if len(out)<300||!bytes.HasPrefix(out,[]byte("PK")){t.Fatal("not XLSX ZIP")}
    zipr,err:=zip.NewReader(bytes.NewReader(out),int64(len(out)))
    if err!=nil{t.Fatal(err)}
    if len(zipr.File)<5{t.Fatalf("incomplete XLSX: %d entries",len(zipr.File))}
    got,sheet,err:=r11ReadXLSX(out)
    if err!=nil{t.Fatal(err)}
    if sheet!="sheet1"{t.Fatalf("unexpected worksheet %q",sheet)}
    if len(got)!=len(rows){t.Fatalf("rows got %d, want %d",len(got),len(rows))}
    for i:=range rows{for j:=range rows[i]{
        if got[i][j]!=rows[i][j]{t.Errorf("cell %d,%d: got %q want %q",i,j,got[i][j],rows[i][j])}
    }}
    if !strings.Contains(string(out[:2]),"PK"){t.Fatal("invalid archive")}
}

func TestCollectionPointTemplateNoFormulaExecution(t *testing.T){
    rows:=[][]string{{"Табельный №","ФИО","Подразделение","Пункт сбора / вылета","Примечание"},{"=1+2","+danger","@test","",""}}
    out,err:=r11CollectionPointXLSX(rows)
    if err!=nil{t.Fatal(err)}
    got,_,err:=r11ReadXLSX(out)
    if err!=nil{t.Fatal(err)}
    if got[1][0]!="=1+2"||got[1][1]!="+danger"{t.Fatalf("text was changed: %v",got[1])}
}
