package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestRosterTwoColumnsAndPagination(t *testing.T) {
	a, err := newApp(filepath.Join(t.TempDir(), "demo.json"))
	if err != nil { t.Fatal(err) }
	h := a.handler()
	for _, test := range []struct {
		url string
		wantTotal int
		wantFirst string
		wantLen int
	}{
		{"/api/roster?month=2026-11&column=all&page=1", 450, "D0001", 25},
		{"/api/roster?month=2026-11&column=1&page=1", 225, "D0001", 25},
		{"/api/roster?month=2026-11&column=2&page=1", 225, "D0226", 25},
		{"/api/roster?month=2026-11&column=all&page=1&q=D0001", 1, "D0001", 1},
	} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, test.url, nil))
		if r.Code != 200 { t.Fatalf("%s: status %d: %s", test.url, r.Code, r.Body.String()) }
		var out struct {
			Total int `json:"total"`
			Employees []employee `json:"employees"`
		}
		if err := json.Unmarshal(r.Body.Bytes(), &out); err != nil { t.Fatal(err) }
		if out.Total != test.wantTotal || len(out.Employees) != test.wantLen || out.Employees[0].TabNo != test.wantFirst {
			t.Fatalf("%s: got total=%d len=%d first=%s", test.url, out.Total, len(out.Employees), out.Employees[0].TabNo)
		}
	}
}

func TestEditSurvivesRestartAndStaysInDemoFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo-schedule-overrides.json")
	a, err := newApp(path)
	if err != nil { t.Fatal(err) }
	req := httptest.NewRequest(http.MethodPost, "/api/plan", strings.NewReader(
		`{"tab_no":"D0001","date":"2026-11-23","plan":"2/11"}`))
	w := httptest.NewRecorder()
	a.handler().ServeHTTP(w, req)
	if w.Code != 200 { t.Fatalf("save returned %d: %s", w.Code, w.Body.String()) }
	again, err := newApp(path)
	if err != nil { t.Fatal(err) }
	w = httptest.NewRecorder()
	again.handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/roster?month=2026-11&column=1&page=1", nil))
	if w.Code != 200 { t.Fatal(w.Code, w.Body.String()) }
	var out struct { Employees []employee `json:"employees"` }
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil { t.Fatal(err) }
	if got := out.Employees[0].Days["2026-11-23"]; got != "2/11" {
		t.Fatalf("edit lost after reload: got %q", got)
	}
}

func TestRejectInvalidEdit(t *testing.T) {
	a, _ := newApp(filepath.Join(t.TempDir(), "demo.json"))
	for _, body := range []string{
		`{"tab_no":"D9999","date":"2026-11-23","plan":"8"}`,
		`{"tab_no":"D0001","date":"2026-02-30","plan":"8"}`,
		`{"tab_no":"D0001","date":"2026-11-23","plan":"a\\nb"}`,
	} {
		w := httptest.NewRecorder()
		a.handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/plan", strings.NewReader(body)))
		if w.Code != 400 { t.Fatalf("invalid edit accepted: %s -> %d", body, w.Code) }
	}
}

func TestGetRejectsInvalidMonth(t *testing.T) {
	a, _ := newApp(filepath.Join(t.TempDir(), "demo.json"))
	w := httptest.NewRecorder()
	a.handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/roster?month=2026-13&column=1&page=1", nil))
	if w.Code != 400 { t.Fatal(w.Code, w.Body.String()) }
}
