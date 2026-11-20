package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func importSyntheticFactCSV(t *testing.T, h *controller, content string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	file, err := mw.CreateFormFile("file", "synthetic_fact.csv")
	if err != nil { t.Fatal(err) }
	if _, err := file.Write([]byte(content)); err != nil { t.Fatal(err) }
	if err := mw.Close(); err != nil { t.Fatal(err) }
	req := httptest.NewRequest(http.MethodPost, "/api/import/fact", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.r11ImportFact(rec, req)
	return rec
}

func TestR11CSVImportRejectsBadHoursWithoutPartialWrite(t *testing.T) {
	h := newR11TestController(t)
	r11MustPost(t, h, "/api/employee/save", `{"tab_no":"SAFE-CSV","full_name":"Тест факта"}`)
	r11MustPost(t, h, "/api/day/save", `{"TabNo":"SAFE-CSV","Date":"2026-11-20","Plan":"1/8","PlanHours":8,"Fact":"1/8","FactHours":8}`)
	before, err := os.ReadFile(h.mainDBPath())
	if err != nil { t.Fatal(err) }
	csv := "Табельный номер;Дата;Факт;Часы\nSAFE-CSV;21.11.2026;1/8;8,5\nSAFE-CSV;22.11.2026;1/8;not-a-number\n"
	rec := importSyntheticFactCSV(t, h, csv)
	if rec.Code != http.StatusBadRequest { t.Fatalf("expected HTTP 400, got %d: %s", rec.Code, rec.Body.String()) }
	if !strings.Contains(rec.Body.String(), "строке 3") { t.Fatalf("missing row detail: %s", rec.Body.String()) }
	after, err := os.ReadFile(h.mainDBPath())
	if err != nil { t.Fatal(err) }
	if !bytes.Equal(after, before) { t.Fatal("invalid CSV changed database despite a rejected import") }
}

func TestR11CSVImportParsesCommaDecimalHours(t *testing.T) {
	h := newR11TestController(t)
	r11MustPost(t, h, "/api/employee/save", `{"tab_no":"SAFE-DEC","full_name":"Тест десятичных часов"}`)
	csv := "Табельный номер;Дата;Факт;Часы\nSAFE-DEC;21.11.2026;1/8;8,5\n"
	rec := importSyntheticFactCSV(t, h, csv)
	if rec.Code != http.StatusOK { t.Fatalf("valid decimal CSV failed: %d %s", rec.Code, rec.Body.String()) }
	data := readR11CoreState(t, h)
	days := r11Rows(t, data, "days")
	if len(days) != 1 || r11Float(days[0], "fact_hours") != 8.5 {
		t.Fatalf("CSV fractional hours were not preserved: %+v", days)
	}
}

func TestR11VersionRestoreRejectsInvalidSnapshotsWithoutWrite(t *testing.T) {
	cases := []struct {
		name string
		corrupt func(map[string]json.RawMessage, []map[string]json.RawMessage, int64, int64)
	}{
		{"missing days", func(v map[string]json.RawMessage, _ []map[string]json.RawMessage, _, _ int64) { delete(v, "days") }},
		{"null days", func(v map[string]json.RawMessage, _ []map[string]json.RawMessage, _, _ int64) { v["days"] = json.RawMessage("null") }},
		{"foreign employee", func(v map[string]json.RawMessage, days []map[string]json.RawMessage, _, otherID int64) { r11Put(days[0], "employee_id", otherID); r11Put(v, "days", days) }},
		{"other year", func(v map[string]json.RawMessage, days []map[string]json.RawMessage, _, _ int64) { r11Put(days[0], "date", "2027-11-20"); r11Put(v, "days", days) }},
		{"duplicate day", func(v map[string]json.RawMessage, days []map[string]json.RawMessage, _, _ int64) { r11Put(v, "days", append(days, days[0])) }},
		{"foreign day ID", func(v map[string]json.RawMessage, days []map[string]json.RawMessage, otherDayID, _ int64) { r11Put(days[0], "id", otherDayID); r11Put(v, "days", days) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newR11TestController(t)
			r11MustPost(t, h, "/api/employee/save", `{"tab_no":"SAFE-A","full_name":"Первый"}`)
			r11MustPost(t, h, "/api/employee/save", `{"tab_no":"SAFE-B","full_name":"Второй"}`)
			r11MustPost(t, h, "/api/day/save", `{"TabNo":"SAFE-A","Date":"2026-11-20","Plan":"1/8","PlanHours":8}`)
			r11MustPost(t, h, "/api/day/save", `{"TabNo":"SAFE-B","Date":"2026-11-20","Plan":"2/8","PlanHours":8}`)
			r11MustPost(t, h, "/api/autograf/version/create", `{"tab_no":"SAFE-A","year":2026,"label":"Резервная версия"}`)
			r11MustPost(t, h, "/api/day/quick-save", `{"TabNo":"SAFE-A","Date":"2026-11-20","Plan":"1/10","PlanHours":10}`)
			root := readR11CoreState(t, h)
			employees := r11Rows(t, root, "employees")
			currentDays := r11Rows(t, root, "days")
			versions := r11Rows(t, root, "graph_versions")
			if len(versions) != 1 || len(currentDays) != 2 || len(employees) != 2 { t.Fatal("invalid test setup") }
			otherEmployeeID := r11Int(employees[1], "id")
			otherDayID := r11Int(currentDays[1], "id")
			snapshot := r11Rows(t, versions[0], "days")
			tc.corrupt(versions[0], snapshot, otherDayID, otherEmployeeID)
			r11Put(root, "graph_versions", versions)
			encoded, err := json.MarshalIndent(root, "", "  ")
			if err != nil { t.Fatal(err) }
			if err := os.WriteFile(h.mainDBPath(), encoded, 0600); err != nil { t.Fatal(err) }
			before, err := os.ReadFile(h.mainDBPath())
			if err != nil { t.Fatal(err) }
			rec := callR11Core(h, http.MethodPost, "/api/autograf/version/restore", fmt.Sprintf(`{"id":%d}`, r11Int(versions[0], "id")))
			if rec.Code != http.StatusBadRequest { t.Fatalf("unsafe restore accepted HTTP %d: %s", rec.Code, rec.Body.String()) }
			after, err := os.ReadFile(h.mainDBPath())
			if err != nil { t.Fatal(err) }
			if !bytes.Equal(after, before) { t.Fatal("rejected version restore wrote to database") }
		})
	}
}

func TestR11VersionRestoreKeepsOtherEmployeeAndYear(t *testing.T) {
	h := newR11TestController(t)
	r11MustPost(t, h, "/api/employee/save", `{"tab_no":"KEEP-A","full_name":"Первый"}`)
	r11MustPost(t, h, "/api/employee/save", `{"tab_no":"KEEP-B","full_name":"Второй"}`)
	r11MustPost(t, h, "/api/day/save", `{"TabNo":"KEEP-A","Date":"2026-11-20","Plan":"1/8","PlanHours":8}`)
	r11MustPost(t, h, "/api/day/save", `{"TabNo":"KEEP-A","Date":"2027-11-20","Plan":"1/9","PlanHours":9}`)
	r11MustPost(t, h, "/api/day/save", `{"TabNo":"KEEP-B","Date":"2026-11-20","Plan":"2/8","PlanHours":8}`)
	r11MustPost(t, h, "/api/autograf/version/create", `{"tab_no":"KEEP-A","year":2026,"label":"Версия 2026"}`)
	versions := r11Rows(t, readR11CoreState(t, h), "graph_versions")
	r11MustPost(t, h, "/api/day/quick-save", `{"TabNo":"KEEP-A","Date":"2026-11-20","Plan":"1/10","PlanHours":10}`)
	rec := callR11Core(h, http.MethodPost, "/api/autograf/version/restore", fmt.Sprintf(`{"id":%d}`, r11Int(versions[0], "id")))
	if rec.Code != http.StatusOK { t.Fatalf("valid restore rejected HTTP %d: %s", rec.Code, rec.Body.String()) }
	root := readR11CoreState(t, h)
	days := r11Rows(t, root, "days")
	employees := r11Rows(t, root, "employees")
	if len(days) != 3 { t.Fatalf("expected all three days, got %d", len(days)) }
	plans := map[string]string{}
	for _, day := range days { plans[fmt.Sprintf("%d|%s", r11Int(day, "employee_id"), r11String(day, "date"))] = r11String(day, "plan") }
	for _, check := range []struct{ id int64; date, plan string }{
		{r11Int(employees[0], "id"), "2026-11-20", "1/8"},
		{r11Int(employees[0], "id"), "2027-11-20", "1/9"},
		{r11Int(employees[1], "id"), "2026-11-20", "2/8"},
	} {
		key := fmt.Sprintf("%d|%s", check.id, check.date)
		if plans[key] != check.plan { t.Fatalf("plan %s=%q, want %q", key, plans[key], check.plan) }
	}
}
