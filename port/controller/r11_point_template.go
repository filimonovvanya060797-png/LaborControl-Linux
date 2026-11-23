package main

import (
    "archive/zip"
    "bytes"
    "encoding/xml"
    "fmt"
    "net/http"
    "strings"
    "time"
)

// r11CollectionPointXLSX writes an actual .xlsx workbook without external
// dependencies. Cells are inline text (never formulas), including tab numbers.
func r11CollectionPointXLSX(rows [][]string) ([]byte,error) {
    var out bytes.Buffer
    zipOut:=zip.NewWriter(&out)
    entries:=[]struct{name,data string}{
        {"[Content_Types].xml",`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`},
        {"_rels/.rels",`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`},
        {"xl/workbook.xml",`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Пункты сбора" sheetId="1" r:id="rId1"/></sheets></workbook>`},
        {"xl/_rels/workbook.xml.rels",`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`},
    }
    for _,entry:=range entries {
        w,err:=zipOut.Create(entry.name)
        if err!=nil{return nil,err}
        if _,err=w.Write([]byte(entry.data));err!=nil{return nil,err}
    }
    w,err:=zipOut.Create("xl/worksheets/sheet1.xml")
    if err!=nil{return nil,err}
    var sheet bytes.Buffer
    sheet.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
    for i,row:=range rows {
        sheet.WriteString(fmt.Sprintf("<row r=\"%d\">",i+1))
        for col,value:=range row {
            if col>25{return nil,fmt.Errorf("слишком много столбцов")}
            sheet.WriteString(fmt.Sprintf("<c r=\"%c%d\" t=\"inlineStr\"><is><t xml:space=\"preserve\">",'A'+col,i+1))
            if err:=xml.EscapeText(&sheet,[]byte(value));err!=nil{return nil,err}
            sheet.WriteString("</t></is></c>")
        }
        sheet.WriteString("</row>")
    }
    sheet.WriteString("</sheetData></worksheet>")
    if _,err=w.Write(sheet.Bytes());err!=nil{return nil,err}
    if err=zipOut.Close();err!=nil{return nil,err}
    return out.Bytes(),nil
}

// r11CollectionPointTemplate returns only employees without a collection point.
// It never modifies or exposes the private JSON state outside localhost.
func (h *controller) r11CollectionPointTemplate(w http.ResponseWriter,r *http.Request){
    if r.Method!=http.MethodGet {
        http.Error(w,"GET only",http.StatusMethodNotAllowed);return
    }
    h.opMu.Lock()
    root,err:=h.r11LoadRootLocked()
    if err!=nil{h.opMu.Unlock();r11WriteError(w,http.StatusInternalServerError,err);return}
    employees,err:=r11Array(root,"employees")
    h.opMu.Unlock()
    if err!=nil{r11WriteError(w,http.StatusInternalServerError,err);return}
    rows:=[][]string{{"Табельный №","ФИО","Подразделение","Пункт сбора / вылета","Примечание"}}
    for _,e:=range employees{
        if strings.TrimSpace(r11String(e,"collection_point"))!=""{continue}
        rows=append(rows,[]string{r11String(e,"tab_no"),r11String(e,"full_name"),r11String(e,"unit"),"",""})
    }
    data,err:=r11CollectionPointXLSX(rows)
    if err!=nil{r11WriteError(w,http.StatusInternalServerError,err);return}
    w.Header().Set("Content-Type","application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
    w.Header().Set("Content-Disposition",fmt.Sprintf("attachment; filename=\"LaborControl_points_%s.xlsx\"",time.Now().Format("2006-01-02")))
    w.Header().Set("Cache-Control","no-store")
    w.Header().Set("X-Content-Type-Options","nosniff")
    _,_=w.Write(data)
}
