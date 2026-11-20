package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newR11TestController(t *testing.T) *controller {
	t.Helper()
	return &controller{baseDir: t.TempDir()}
}

func callR11Handler(handler http.HandlerFunc, method, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func TestR11StateGetReturnsEmptyCompatibleEnvelope(t *testing.T) {
	h := newR11TestController(t)
	rec := callR11Handler(h.r11State, http.MethodGet, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET state status=%d body=%s", rec.Code, rec.Body.String())
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"data", "data_path", "discrepancies", "version"} {
		if _, ok := envelope[key]; !ok {
			t.Fatalf("state envelope is missing %q", key)
		}
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(envelope["data"], &data); err != nil {
		t.Fatal(err)
	}
	for _, key := range r11RequiredDataKeys {
		if _, ok := data[key]; !ok {
			t.Fatalf("empty state is missing %q", key)
		}
	}
	var employees []json.RawMessage
	if err := json.Unmarshal(data["employees"], &employees); err != nil || len(employees) != 0 {
		t.Fatalf("employees=%v err=%v", employees, err)
	}
	if _, err := os.Stat(h.mainDBPath()); !os.IsNotExist(err) {
		t.Fatalf("GET must not create a database file, stat err=%v", err)
	}
}

func TestDecodeR11ImportAcceptsWindowsEnvelope(t *testing.T) {
	data, counts, err := decodeR11Import([]byte(syntheticR11StateJSON))
	if err != nil {
		t.Fatal(err)
	}
	if counts.Employees != 1 || counts.Days != 1 {
		t.Fatalf("counts=%+v, want one employee and one day", counts)
	}
	var stored map[string]json.RawMessage
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	if _, ok := stored["future_data_field"]; !ok {
		t.Fatal("unknown data field was not preserved")
	}
	if _, ok := stored["data"]; ok {
		t.Fatal("Windows response envelope must be unwrapped before storage")
	}
}

func TestR11ImportStateStoresDataAndReturnsCounts(t *testing.T) {
	h := newR11TestController(t)
	rec := callR11Handler(h.r11ImportState, http.MethodPost, syntheticR11StateJSON)
	if rec.Code != http.StatusOK {
		t.Fatalf("import status=%d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		OK        bool   `json:"ok"`
		Employees int    `json:"employees"`
		Days      int    `json:"days"`
		Backup    string `json:"backup"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Employees != 1 || result.Days != 1 || result.Backup != "" {
		t.Fatalf("unexpected import result: %+v", result)
	}
	stored, err := os.ReadFile(h.mainDBPath())
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(stored, &root); err != nil {
		t.Fatal(err)
	}
	if _, ok := root["employees"]; !ok {
		t.Fatal("stored DB does not contain employees at the root")
	}
	if _, ok := root["data"]; ok {
		t.Fatal("stored DB should contain data object, not API envelope")
	}
}

func TestR11UpdateDaysPreservesFactAndUnknownFields(t *testing.T) {
	h := newR11TestController(t)
	fixture := strings.Replace(
		syntheticR11StateJSON,
		`"updated_at":"2026-01-01T00:00:00Z"}`,
		`"updated_at":"2026-01-01T00:00:00Z","future_day_key":"keep-me"}`,
		1,
	)
	data, _, err := decodeR11Import([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(h.mainDBPath(), data); err != nil {
		t.Fatal(err)
	}

	request := `{"days":[{"employee_id":1,"date":"2026-01-01","plan":"2/10","plan_hours":10}]}`
	rec := callR11Handler(h.r11UpdateDays, http.MethodPost, request)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		OK      bool   `json:"ok"`
		Created int    `json:"created"`
		Updated int    `json:"updated"`
		Backup  string `json:"backup"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Created != 0 || result.Updated != 1 || result.Backup == "" {
		t.Fatalf("unexpected update result: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(h.baseDir, "backups", result.Backup)); err != nil {
		t.Fatalf("backup not found: %v", err)
	}

	stored, err := os.ReadFile(h.mainDBPath())
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(stored, &root); err != nil {
		t.Fatal(err)
	}
	var days []map[string]json.RawMessage
	if err := json.Unmarshal(root["days"], &days); err != nil || len(days) != 1 {
		t.Fatalf("days=%v err=%v", days, err)
	}
	checks := map[string]string{
		"plan": "2/10", "fact": "0", "shift": "DAY",
		"comment": "", "future_day_key": "keep-me",
	}
	for key, want := range checks {
		var got string
		if err := json.Unmarshal(days[0][key], &got); err != nil || got != want {
			t.Fatalf("%s=%q want=%q err=%v", key, got, want, err)
		}
	}
	var hours int
	if err := json.Unmarshal(days[0]["plan_hours"], &hours); err != nil || hours != 10 {
		t.Fatalf("plan_hours=%d err=%v", hours, err)
	}
}

func TestR11UpdateDaysCreatesRecordAndJournal(t *testing.T) {
	h := newR11TestController(t)
	data, _, err := decodeR11Import([]byte(syntheticR11StateJSON))
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(h.mainDBPath(), data); err != nil {
		t.Fatal(err)
	}
	request := `{"days":[{"employee_id":1,"date":"2026-01-02","plan":"В","plan_hours":0}]}`
	rec := callR11Handler(h.r11UpdateDays, http.MethodPost, request)
	if rec.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	stored, err := os.ReadFile(h.mainDBPath())
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(stored, &root); err != nil {
		t.Fatal(err)
	}
	var nextID int
	if err := json.Unmarshal(root["next_id"], &nextID); err != nil || nextID != 101 {
		t.Fatalf("next_id=%d err=%v, want 101", nextID, err)
	}
	var days []map[string]json.RawMessage
	if err := json.Unmarshal(root["days"], &days); err != nil || len(days) != 2 {
		t.Fatalf("days count=%d err=%v, want 2", len(days), err)
	}
	var journal []map[string]json.RawMessage
	if err := json.Unmarshal(root["journal"], &journal); err != nil || len(journal) != 1 {
		t.Fatalf("journal count=%d err=%v, want 1", len(journal), err)
	}
}

func TestR11UpdateDaysRejectsUnknownEmployeeWithoutWriting(t *testing.T) {
	h := newR11TestController(t)
	data, _, err := decodeR11Import([]byte(syntheticR11StateJSON))
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(h.mainDBPath(), data); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(h.mainDBPath())
	if err != nil {
		t.Fatal(err)
	}
	request := `{"days":[{"employee_id":1,"date":"2026-01-02","plan":"8","plan_hours":8},{"employee_id":999,"date":"2026-01-02","plan":"8","plan_hours":8}]}`
	rec := callR11Handler(h.r11UpdateDays, http.MethodPost, request)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s, want 400", rec.Code, rec.Body.String())
	}
	after, err := os.ReadFile(h.mainDBPath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("database changed despite rejected batch")
	}
}

func TestDecodeR11ImportRejectsMissingSchemaFields(t *testing.T) {
	_, _, err := decodeR11Import([]byte(`{"data":{"employees":[]}}`))
	if err == nil || !strings.Contains(err.Error(), "calendar") {
		t.Fatalf("expected missing calendar error, got %v", err)
	}
}

func TestR11UpdateDaysHandles450EmployeesAndMonthlyGrid(t *testing.T) {
	h := newR11TestController(t)
	var root map[string]json.RawMessage
	if err := json.Unmarshal(emptyR11DataJSON(), &root); err != nil {
		t.Fatal(err)
	}
	const employeeCount = 450
	const daysPerEmployee = 30
	employees := make([]map[string]any, 0, employeeCount)
	days := make([]map[string]any, 0, employeeCount*daysPerEmployee)
	nextID := 1
	for employee := 1; employee <= employeeCount; employee++ {
		employees = append(employees, map[string]any{
			"id": employee, "tab_no": fmt.Sprintf("T%04d", employee),
			"full_name": fmt.Sprintf("Synthetic driver %04d", employee),
		})
		for day := 1; day <= daysPerEmployee; day++ {
			days = append(days, map[string]any{
				"id": nextID, "employee_id": employee,
				"date": fmt.Sprintf("2026-01-%02d", day),
				"plan": "8", "plan_hours": 8,
				"fact": "8", "fact_hours": 8,
				"shift": "DAY", "state": "planned", "comment": "",
			})
			nextID++
		}
	}
	root["employees"], _ = json.Marshal(employees)
	root["days"], _ = json.Marshal(days)
	root["next_id"], _ = json.Marshal(nextID)
	initial, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(h.mainDBPath(), initial); err != nil {
		t.Fatal(err)
	}

	changes := make([]r11DayChange, 0, employeeCount)
	for employee := 1; employee <= employeeCount; employee++ {
		plan := "2/10"
		hours := int32(10)
		changes = append(changes, r11DayChange{
			EmployeeID: int64(employee), Date: "2026-01-01",
			Plan: &plan, PlanHours: &hours,
		})
	}
	requestBody, err := json.Marshal(r11DayChangesRequest{Days: changes})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/state/days", bytes.NewReader(requestBody))
	rec := httptest.NewRecorder()
	h.r11UpdateDays(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("450-employee update status=%d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Updated   int `json:"updated"`
		Created   int `json:"created"`
		Employees int `json:"employees"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Updated != employeeCount || result.Created != 0 || result.Employees != employeeCount {
		t.Fatalf("unexpected 450-employee result: %+v", result)
	}
	stored, err := os.ReadFile(h.mainDBPath())
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]json.RawMessage
	if err := json.Unmarshal(stored, &saved); err != nil {
		t.Fatal(err)
	}
	var savedDays []map[string]json.RawMessage
	if err := json.Unmarshal(saved["days"], &savedDays); err != nil {
		t.Fatal(err)
	}
	if len(savedDays) != employeeCount*daysPerEmployee {
		t.Fatalf("stored day count=%d, want %d", len(savedDays), employeeCount*daysPerEmployee)
	}
	var plan, fact string
	var hours int
	if err := json.Unmarshal(savedDays[0]["plan"], &plan); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(savedDays[0]["fact"], &fact); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(savedDays[0]["plan_hours"], &hours); err != nil {
		t.Fatal(err)
	}
	if plan != "2/10" || fact != "8" || hours != 10 {
		t.Fatalf("plan/fact/hours = %q/%q/%d; expected 2/10/8/10", plan, fact, hours)
	}
}
