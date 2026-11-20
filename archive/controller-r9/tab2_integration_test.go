package main

import "testing"

func testDB() mainDB {
	return mainDB{
		Employees: []map[string]any{{"id": 1, "tab_no": "100001", "full_name": "Тест Тестов", "unit": "АК-1", "hire_date": "2026-01-01"}},
		Days: []map[string]any{
			{"id": 2, "employee_id": 1, "date": "2026-06-10", "plan": "11", "plan_hours": 11.0, "fact": "8", "fact_hours": 8.0, "shift": "DAY"},
			{"id": 5, "employee_id": 1, "date": "2026-06-15", "plan": "В", "plan_hours": 0.0, "fact": "11", "fact_hours": 11.0, "shift": "DAY"},
		},
		Flights: []map[string]any{{"id": 3, "employee_id": 1, "date": "2026-06-12", "kind": "Прилёт"}},
		RVD:     []map[string]any{{"id": 4, "employee_id": 1, "date_from": "2026-06-15", "date_to": "2026-06-15", "type": 2}},
		Events:  []map[string]any{},
		NextID:  10,
	}
}

func TestBuildPlanFactAndChecks(t *testing.T) {
	h := &controller{}
	db := testDB()
	blocks, probs := h.buildPlanFact(&db, 2026, 6)
	if len(blocks) != 1 || len(probs) == 0 {
		t.Fatalf("plan_fact: blocks=%d probs=%d", len(blocks), len(probs))
	}
	if blocks[0].Output[9] != "ЛОЖЬ" {
		t.Fatalf("June 10 output=%q", blocks[0].Output[9])
	}

	st := tab2Store{
		SAPRecords: []tab2SAPRecord{{Infotype: "2002_0020", TabNo: "100001", DateFrom: "2026-06-12", DateTo: "2026-06-12", Code: "0020"}},
		SZRVD:      []tab2SZRVDRecord{{TabNo: "100001", DateFrom: "2026-06-15", DateTo: "2026-06-15", Type: "2"}},
	}
	dp := h.buildDPChecks(&db, st, 2026, 6)
	if len(dp) != 1 || dp[0].Status != "ПРОБЛЕМА" { // SAP DP exists, but graph is missing ДП/В.
		t.Fatalf("dp=%+v", dp)
	}
	rv := h.buildRVDChecks(&db, st, 2026, 6)
	if len(rv) != 1 || rv[0].Status != "OK" {
		t.Fatalf("rvd=%+v", rv)
	}
}