package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func makeTestXLSX(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	shared, err := z.Create("xl/sharedStrings.xml")
	if err != nil { t.Fatal(err) }
	_, _ = io.WriteString(shared, `<?xml version="1.0" encoding="UTF-8"?><sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><si><t>Пункт сбора</t></si><si><t>Дата ДП</t></si><si><t>Источник</t></si><si><t>Тюмень</t></si><si><t>Импорт тест</t></si></sst>`)
	sheet, err := z.Create("xl/worksheets/sheet1.xml")
	if err != nil { t.Fatal(err) }
	_, _ = io.WriteString(sheet, `<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="s"><v>2</v></c></row><row r="2"><c r="A2" t="s"><v>3</v></c><c r="B2" t="inlineStr"><is><t>20.11.2026</t></is></c><c r="C2" t="s"><v>4</v></c></row></sheetData></worksheet>`)
	if err := z.Close(); err != nil { t.Fatal(err) }
	return buf.Bytes()
}

func TestR11ReadXLSXAndExcelDateSerial(t *testing.T) {
	rows, sheet, err := r11ReadXLSX(makeTestXLSX(t))
	if err != nil { t.Fatal(err) }
	if sheet != "sheet1" { t.Fatalf("sheet=%q", sheet) }
	if len(rows) != 2 || len(rows[0]) != 3 || rows[1][0] != "Тюмень" || rows[1][1] != "20.11.2026" { t.Fatalf("rows=%v", rows) }
	if got := r11NormalizeDate("20.11.2026"); got != "2026-11-20" { t.Fatalf("date=%q", got) }
	if got := r11NormalizeDate("46346"); got == "" { t.Fatal("Excel serial date was not converted") }
}

func TestR11ReadCSVSupportsSemicolonAndBOM(t *testing.T) {
	rows, err := r11ReadCSV([]byte("\xef\xbb\xbfТабельный номер;Дата;Факт;Часы\nT-01;20.11.2026;1/8;8\n"))
	if err != nil { t.Fatal(err) }
	if len(rows) != 2 || len(rows[0]) != 4 || rows[1][0] != "T-01" { t.Fatalf("rows=%v", rows) }
	if r11HeaderIndex(rows[0], "tab_no", "табельный номер") != 0 { t.Fatal("tab_no header not recognized") }
}

func TestR11FlightImportPreviewAndConfirm(t *testing.T) {
	h := newR11TestController(t)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "flights.xlsx")
	if err != nil { t.Fatal(err) }
	if _, err := fw.Write(makeTestXLSX(t)); err != nil { t.Fatal(err) }
	_ = mw.WriteField("year", "2026")
	if err := mw.Close(); err != nil { t.Fatal(err) }
	req := httptest.NewRequest(http.MethodPost, "/api/autograf/flight/import-preview", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.r11FlightImportPreview(rec, req)
	if rec.Code != http.StatusOK { t.Fatalf("preview status=%d body=%s", rec.Code, rec.Body.String()) }
	var preview struct {
		Rows []struct {
			Point string `json:"collection_point"`
			Date string `json:"date"`
			Status string `json:"status"`
			Selectable bool `json:"selectable"`
		} `json:"rows"`
		Counts map[string]int `json:"counts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil { t.Fatal(err) }
	if len(preview.Rows) != 1 || preview.Rows[0].Point != "Тюмень" || preview.Rows[0].Date != "2026-11-20" || preview.Rows[0].Status != "new" || !preview.Rows[0].Selectable { t.Fatalf("preview=%+v", preview) }
	confirmBody, _ := json.Marshal(map[string]any{"rows": []map[string]any{
		{"collection_point":"Тюмень","date":"2026-11-20","source":"Импорт тест","note":""},
		{"collection_point":"Тюмень","date":"2026-11-20","source":"Дубль","note":""},
	}})
	confirm := callR11Core(h, http.MethodPost, "/api/autograf/flight/import-confirm", string(confirmBody))
	if confirm.Code != http.StatusOK { t.Fatalf("confirm=%d %s", confirm.Code, confirm.Body.String()) }
	var result map[string]any
	if err := json.Unmarshal(confirm.Body.Bytes(), &result); err != nil { t.Fatal(err) }
	if result["added"] != float64(1) || result["skipped"] != float64(1) { t.Fatalf("result=%v", result) }
	data := readR11CoreState(t, h)
	var flights []map[string]json.RawMessage
	if err := json.Unmarshal(data["flight_dates"], &flights); err != nil { t.Fatal(err) }
	if len(flights) != 1 { t.Fatalf("flights=%d", len(flights)) }
}

func TestR11EmployeeFlightImportAndDelete(t *testing.T) {
	h := newR11TestController(t)
	emp := callR11Core(h, http.MethodPost, "/api/employee/save", `{"tab_no":"T-11","full_name":"Индивидуальный тест","collection_point":"Тюмень"}`)
	if emp.Code != http.StatusOK { t.Fatal(emp.Body.String()) }
	data := readR11CoreState(t, h)
	var employees []map[string]json.RawMessage
	if err := json.Unmarshal(data["employees"], &employees); err != nil { t.Fatal(err) }
	id := r11Int(employees[0], "id")
	payload, _ := json.Marshal(map[string]any{"file_name":"individual.csv","rows":[]map[string]any{{"employee_id":id,"date":"2026-11-20","kind":"Прилёт","collection_point":"Тюмень","note":"Тест"}}})
	rec := callR11Core(h, http.MethodPost, "/api/employee-flight/import-confirm", string(payload))
	if rec.Code != http.StatusOK { t.Fatalf("import=%d %s", rec.Code, rec.Body.String()) }
	data = readR11CoreState(t, h)
	var flights []map[string]json.RawMessage
	if err := json.Unmarshal(data["employee_flights"], &flights); err != nil { t.Fatal(err) }
	if len(flights) != 1 { t.Fatalf("employee_flights=%d", len(flights)) }
	delBody, _ := json.Marshal(map[string]any{"id":r11Int(flights[0],"id")})
	del := callR11Core(h, http.MethodPost, "/api/employee-flight/delete", string(delBody))
	if del.Code != http.StatusOK { t.Fatalf("delete=%d %s", del.Code, del.Body.String()) }
	data = readR11CoreState(t, h)
	if err := json.Unmarshal(data["employee_flights"], &flights); err != nil { t.Fatal(err) }
	if len(flights) != 0 { t.Fatalf("employee_flights after delete=%d", len(flights)) }
}
