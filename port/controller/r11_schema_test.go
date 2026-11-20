package main

import (
	"encoding/json"
	"testing"
)

const syntheticR11StateJSON = `{
  "data": {
    "calendar": [{"date":"2026-01-01","day_type":"holiday","norm_hours":0,"note":"synthetic","source":"test"}],
    "days": [{"comment":"","date":"2026-01-01","employee_id":1,"fact":"0","fact_hours":0,"id":2,"plan":"11","plan_hours":11,"shift":"DAY","state":"planned","updated_at":"2026-01-01T00:00:00Z"}],
    "employee_flights": [{"id":3,"future_nested_field":{"preserved":true}}],
    "employees": [{"collection_point":"TEST","fire_date":"","full_name":"Synthetic Employee","hire_date":"2020-01-01","id":1,"is_quarter":false,"main_schedule_ready":true,"notes":"","position":"Driver","probation":false,"probation_end":"","schedule_type":"TEST-CYCLE","tab_no":"TEST-001","unit":"TEST"}],
    "events": [{"created_at":"2026-01-01T00:00:00Z","date_from":"2026-01-02","date_to":"2026-01-03","employee_id":1,"event_type":"TEST","id":4,"note":"synthetic","source":"test","source_code":"T"}],
    "flight_dates": [],
    "flight_routes": [],
    "graph_revisions": [
      {"created_at":"2026-01-01T00:00:00Z","effective_date":"2026-01-01","employee_id":1,"fixed_hours":0,"id":5,"label":"base","number":1,"parent_id":0,"reason":"test","schedule_type":"TEST-CYCLE","status":"draft","updated_at":"2026-01-01T00:00:00Z"},
      {"activated_at":"2026-01-02T00:00:00Z","control_date":"2026-01-02","created_at":"2026-01-01T00:00:00Z","effective_date":"2026-01-02","employee_id":1,"fixed_hours":0,"id":6,"label":"activated","number":2,"parent_id":5,"reason":"test","schedule_type":"TEST-CYCLE","status":"active","updated_at":"2026-01-02T00:00:00Z"}
    ],
    "graph_versions": [{"created_at":"2026-01-01T00:00:00Z","days":[{"date":"2026-01-01","future_day_key":"preserved"}],"employee_id":1,"id":7,"label":"2026","reason":"test","year":2026}],
    "import_history": [],
    "journal": [],
    "manual_colors": [],
    "next_id": 100,
    "notes": [],
    "rvd": [{"created_at":"2026-01-01T00:00:00Z","date_from":"2026-01-04","date_to":"2026-01-04","employee_id":1,"id":8,"note":"test","rest_date":"2026-01-05","type":1}],
    "schedule_changes": [{"future_schedule_change_field":"preserved"}],
    "schedule_types": [{"active":true,"collection_point":"TEST","id":9,"name":"TEST-CYCLE","note":"","pair_name":"","phase":"A"}],
    "schema_version": 2,
    "future_data_field": {"preserved": true}
  },
  "data_path": "synthetic-path",
  "discrepancies": [{"synthetic_discrepancy": true}],
  "version": "synthetic-version",
  "future_root_field": {"preserved": true}
}`

func TestR11StateRoundTripPreservesSchemaAndUnknownFields(t *testing.T) {
	var state R11StateEnvelope
	if err := json.Unmarshal([]byte(syntheticR11StateJSON), &state); err != nil {
		t.Fatalf("unmarshal synthetic R11 state: %v", err)
	}

	if len(state.Data.Calendar) != 1 || state.Data.Calendar[0].NormHours != 0 {
		t.Fatalf("calendar schema not decoded: %+v", state.Data.Calendar)
	}
	if len(state.Data.Days) != 1 || state.Data.Days[0].PlanHours != 11 || state.Data.Days[0].EmployeeID != 1 {
		t.Fatalf("day schema not decoded: %+v", state.Data.Days)
	}
	if len(state.Data.Employees) != 1 || state.Data.Employees[0].TabNo != "TEST-001" || !state.Data.Employees[0].MainScheduleReady {
		t.Fatalf("employee schema not decoded: %+v", state.Data.Employees)
	}
	if len(state.Data.GraphRevisions) != 2 {
		t.Fatalf("graph revisions count = %d, want 2", len(state.Data.GraphRevisions))
	}
	if state.Data.GraphRevisions[0].ActivatedAt != nil || state.Data.GraphRevisions[0].ControlDate != nil {
		t.Fatal("optional graph revision fields should remain absent when not supplied")
	}
	if state.Data.GraphRevisions[1].ActivatedAt == nil || *state.Data.GraphRevisions[1].ActivatedAt != "2026-01-02T00:00:00Z" {
		t.Fatalf("activated_at optional field not decoded: %+v", state.Data.GraphRevisions[1])
	}
	if len(state.Data.GraphVersions) != 1 || len(state.Data.GraphVersions[0].Days) != 1 {
		t.Fatalf("nested graph version days not retained: %+v", state.Data.GraphVersions)
	}

	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal R11 state: %v", err)
	}
	var roundTrip map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatalf("unmarshal round-trip root: %v", err)
	}
	if _, ok := roundTrip["future_root_field"]; !ok {
		t.Fatal("unknown root field was lost")
	}
	var roundTripData map[string]json.RawMessage
	if err := json.Unmarshal(roundTrip["data"], &roundTripData); err != nil {
		t.Fatalf("unmarshal round-trip data: %v", err)
	}
	if _, ok := roundTripData["future_data_field"]; !ok {
		t.Fatal("unknown data field was lost")
	}
	if _, ok := roundTripData["employee_flights"]; !ok {
		t.Fatal("empty/opaque employee_flights collection was lost")
	}
	if _, ok := roundTripData["schedule_changes"]; !ok {
		t.Fatal("empty/opaque schedule_changes collection was lost")
	}
}

func TestR11StateRejectsInvalidIntegerFieldType(t *testing.T) {
	const invalid = `{"data":{"calendar":[],"days":[{"plan_hours":"eleven"}]}}`
	var state R11StateEnvelope
	if err := json.Unmarshal([]byte(invalid), &state); err == nil {
		t.Fatal("expected type error for string plan_hours")
	}
}
