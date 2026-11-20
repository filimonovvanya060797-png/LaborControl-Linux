package main

import (
	"strings"
	"testing"
)

func TestExpectedPlanHoursLikePlanFact(t *testing.T) {
	cases := map[string]float64{
		"В": 0, "ОТ": 0, "ДП": 0,
		"11": 11, "8": 8, "1/11": 11, "2/10": 10,
	}
	for in, want := range cases {
		if got := expectedPlanHours(in); got != want {
			t.Fatalf("expectedPlanHours(%q)=%v, want %v", in, got, want)
		}
	}
}

func TestPlanFactDiscrepancy(t *testing.T) {
	out, p := discrepancyFor("11", 8, "день", "день")
	if out != "ЛОЖЬ" || p == "" {
		t.Fatalf("11 vs 8 must be discrepancy: out=%q problem=%q", out, p)
	}
	out, p = discrepancyFor("В", 0, "—", "—")
	if out != "ИСТИНА" || p != "" {
		t.Fatalf("В vs 0 must match: out=%q problem=%q", out, p)
	}
}

func TestSAPRVDMapping(t *testing.T) {
	if typ, _ := mapSAPEvent(tab2SAPRecord{Infotype: "2004", Code: "01"}); typ != "SAP_RVD2" {
		t.Fatalf("2004/01 = %q, want SAP_RVD2", typ)
	}
	if typ, _ := mapSAPEvent(tab2SAPRecord{Infotype: "2004", Code: "02"}); typ != "SAP_RVD1" {
		t.Fatalf("2004/02 = %q, want SAP_RVD1", typ)
	}
	if typ, _ := mapSAPEvent(tab2SAPRecord{Infotype: "2002_0020", Code: "0020"}); typ != "DP" {
		t.Fatalf("2002/0020 = %q, want DP", typ)
	}
}

func TestPaletteStatKey(t *testing.T) {
	cases := map[string]string{
		"SAP_RVD1":    "RVD1",
		"SAP_RVD2":    "RVD2",
		"TRAINING":    "TECH",
		"REPAIR":      "TECH",
		"MAINTENANCE": "TECH",
		"FIELD":       "TECH",
		"VACATION":    "VACATION",
	}
	for in, want := range cases {
		if got := paletteStatKey(in); got != want {
			t.Fatalf("paletteStatKey(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestShiftFromClockMatchesTimesheetMacro(t *testing.T) {
	cases := map[int]string{5: "", 6: "DAY", 9: "DAY", 10: "", 17: "", 18: "NIGHT", 22: "NIGHT", 23: ""}
	for h, want := range cases {
		if got := shiftFromClock(h); got != want {
			t.Fatalf("shiftFromClock(%d)=%q, want %q", h, got, want)
		}
	}
}

func TestPlanFactShiftChecks(t *testing.T) {
	_, p := discrepancyFor("1/11", 11, "день", "ночь")
	if p == "" || !strings.Contains(p, "Расхождение смен") {
		t.Fatalf("shift mismatch not detected: %q", p)
	}
	_, p = discrepancyFor("11", 11, "—", "день")
	if p == "" || !strings.Contains(p, "Нет смены свод") {
		t.Fatalf("missing planned shift not detected: %q", p)
	}
	_, p = discrepancyFor("1/11", 11, "день", "—")
	if p == "" || !strings.Contains(p, "Нет смены исат") {
		t.Fatalf("missing fact shift not detected: %q", p)
	}
}

func TestParseSZRVDFixedColumns(t *testing.T) {
	rows := make([][]string, 26)
	rows[24] = make([]string, 18)
	rows[24][3] = "Тест Тестов"
	rows[24][10] = "100001"
	rows[24][11] = "15.06.2026"
	rows[24][12] = "16.06.2026"
	rows[24][15] = "1"
	rows[24][16] = "20.06.2026"
	rows[24][17] = "21.06.2026"
	recs, err := parseSZRVDGrid(rows, "СЗ РВД.xlsx")
	if err != nil || len(recs) != 1 {
		t.Fatalf("parseSZRVDGrid err=%v recs=%+v", err, recs)
	}
	r := recs[0]
	if r.TabNo != "100001" || r.DateFrom != "2026-06-15" || r.DateTo != "2026-06-16" || r.Type != "1" || r.RestFrom != "2026-06-20" || r.RestTo != "2026-06-21" {
		t.Fatalf("unexpected record: %+v", r)
	}
}

func TestRVDCheckCoreStatuses(t *testing.T) {
	h := &controller{}
	base := mainDB{Employees: []map[string]any{{"id": 1, "tab_no": "100001", "full_name": "Тест Тестов"}}, NextID: 10}
	// Actual RVD candidate without SZ.
	db := base
	db.Days = []map[string]any{{"id": 2, "employee_id": 1, "date": "2026-06-15", "plan": "В", "fact_hours": 11.0}}
	r := h.buildRVDChecks(&db, tab2Store{}, 2026, 6)
	if len(r) != 1 || r[0].Status != "Нет в СЗ" {
		t.Fatalf("Нет в СЗ: %+v", r)
	}

	// Exact RVD-2 period is OK.
	st := tab2Store{SZRVD: []tab2SZRVDRecord{{TabNo: "100001", DateFrom: "2026-06-15", DateTo: "2026-06-15", Type: "2"}}}
	r = h.buildRVDChecks(&db, st, 2026, 6)
	if len(r) != 1 || r[0].Status != "OK" {
		t.Fatalf("RVD-2 OK: %+v", r)
	}

	// SZ exists, no actual work -> no confirmation.
	db2 := base
	r = h.buildRVDChecks(&db2, st, 2026, 6)
	if len(r) != 1 || r[0].Status != "Нет подтверждения" {
		t.Fatalf("no confirmation: %+v", r)
	}

	// RVD-1 with matching two-day rest and actual period -> Отгулы.
	db3 := base
	db3.Days = []map[string]any{
		{"id": 2, "employee_id": 1, "date": "2026-06-15", "plan": "В", "fact_hours": 11.0},
		{"id": 3, "employee_id": 1, "date": "2026-06-16", "plan": "ДП", "fact_hours": 11.0},
	}
	st1 := tab2Store{SZRVD: []tab2SZRVDRecord{{TabNo: "100001", DateFrom: "2026-06-15", DateTo: "2026-06-16", Type: "1", RestFrom: "2026-06-20", RestTo: "2026-06-21"}}}
	r = h.buildRVDChecks(&db3, st1, 2026, 6)
	if len(r) != 1 || r[0].Status != "Отгулы" {
		t.Fatalf("RVD-1 rests: %+v", r)
	}
}