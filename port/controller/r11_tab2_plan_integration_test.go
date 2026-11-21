package main

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
)

// Real Tab2 must read the same R11 database as the AutoGraf screen.
// A navigable iframe alone is not sufficient if plan/fact are out of sync.
func TestEmbeddedTab2ReadsR11PlanFromSameDatabase(t *testing.T) {
    h := newR11TestController(t)
    r11MustPost(t, h, "/api/employee/save", `{"tab_no":"TAB2-990001","full_name":"Тестов Иван Иванович","unit":"Колонна 1"}`)
    r11MustPost(t, h, "/api/day/save", `{"TabNo":"TAB2-990001","Date":"2026-11-20","Plan":"1/8","PlanHours":8}`)
    rec := httptest.NewRecorder()
    h.tab2State(rec, httptest.NewRequest(http.MethodGet, "/api/tab2/state?year=2026&month=11", nil))
    if rec.Code != http.StatusOK {
        t.Fatalf("Tab2 API failed HTTP %d: %s", rec.Code, rec.Body.String())
    }
    var result struct {
        Blocks []tab2PlanFactBlock `json:"blocks"`
    }
    if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
        t.Fatalf("bad Tab2 JSON: %v", err)
    }
    if len(result.Blocks) != 1 {
        t.Fatalf("Tab2 received %d drivers, want one from R11 Core", len(result.Blocks))
    }
    b := result.Blocks[0]
    if b.TabNo != "TAB2-990001" || len(b.Plan) != 30 || b.Plan[19] != "1/8" {
        t.Fatalf("Tab2 plan diverges from R11 Autograf: tab=%q, days=%d, plan=%q", b.TabNo, len(b.Plan), func() string {if len(b.Plan)>19 {return b.Plan[19]};return ""}())
    }
}
