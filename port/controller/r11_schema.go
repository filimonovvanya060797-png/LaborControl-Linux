package main

import (
	"encoding/json"
)

// R11StateEnvelope describes the state response returned by the Windows
// LaborControl_Core_R11 API. DataPath and Version remain RawMessage because
// the sanitized schema export intentionally did not capture their values or
// scalar types. Unknown root fields are retained during round trips.
type R11StateEnvelope struct {
	Data          R11Data           `json:"data"`
	DataPath      json.RawMessage   `json:"data_path"`
	Discrepancies []json.RawMessage `json:"discrepancies"`
	Version       json.RawMessage   `json:"version"`
	Extra         map[string]json.RawMessage `json:"-"`
}

// R11Data mirrors the 18 top-level data keys observed in the Windows R11
// /api/state response. Unknown data keys are retained so a future R11 schema
// extension is not silently discarded by a read/write cycle.
type R11Data struct {
	Calendar        []R11CalendarDay  `json:"calendar"`
	Days            []R11Day          `json:"days"`
	EmployeeFlights []json.RawMessage `json:"employee_flights"`
	Employees       []R11Employee     `json:"employees"`
	Events          []R11Event        `json:"events"`
	FlightDates     []R11FlightDate   `json:"flight_dates"`
	FlightRoutes    []R11FlightRoute  `json:"flight_routes"`
	GraphRevisions  []R11GraphRevision `json:"graph_revisions"`
	GraphVersions   []R11GraphVersion `json:"graph_versions"`
	ImportHistory   []R11ImportHistory `json:"import_history"`
	Journal         []R11JournalEntry `json:"journal"`
	ManualColors    []R11ManualColor  `json:"manual_colors"`
	NextID          int32             `json:"next_id"`
	Notes           []R11Note         `json:"notes"`
	RVD             []R11RVDRecord    `json:"rvd"`
	ScheduleChanges []json.RawMessage `json:"schedule_changes"`
	ScheduleTypes   []R11ScheduleType `json:"schedule_types"`
	SchemaVersion   int32             `json:"schema_version"`
	Extra           map[string]json.RawMessage `json:"-"`
}

type R11CalendarDay struct {
	Date      string `json:"date"`
	DayType   string `json:"day_type"`
	NormHours int32  `json:"norm_hours"`
	Note      string `json:"note"`
	Source    string `json:"source"`
}

type R11Day struct {
	Comment   string `json:"comment"`
	Date      string `json:"date"`
	EmployeeID int32 `json:"employee_id"`
	Fact      string `json:"fact"`
	FactHours int32  `json:"fact_hours"`
	ID        int32  `json:"id"`
	Plan      string `json:"plan"`
	PlanHours int32  `json:"plan_hours"`
	Shift     string `json:"shift"`
	State     string `json:"state"`
	UpdatedAt string `json:"updated_at"`
}

type R11Employee struct {
	CollectionPoint  string `json:"collection_point"`
	FireDate         string `json:"fire_date"`
	FullName         string `json:"full_name"`
	HireDate         string `json:"hire_date"`
	ID               int32  `json:"id"`
	IsQuarter        bool   `json:"is_quarter"`
	MainScheduleReady bool  `json:"main_schedule_ready"`
	Notes            string `json:"notes"`
	Position         string `json:"position"`
	Probation        bool   `json:"probation"`
	ProbationEnd     string `json:"probation_end"`
	ScheduleType     string `json:"schedule_type"`
	TabNo            string `json:"tab_no"`
	Unit             string `json:"unit"`
}

type R11Event struct {
	CreatedAt string `json:"created_at"`
	DateFrom  string `json:"date_from"`
	DateTo    string `json:"date_to"`
	EmployeeID int32 `json:"employee_id"`
	EventType string `json:"event_type"`
	ID        int32  `json:"id"`
	Note      string `json:"note"`
	Source    string `json:"source"`
	SourceCode string `json:"source_code"`
}

type R11FlightDate struct {
	CollectionPoint string `json:"collection_point"`
	Date            string `json:"date"`
	ID              int32  `json:"id"`
	Note            string `json:"note"`
	Source          string `json:"source"`
}

type R11FlightRoute struct {
	Active          bool   `json:"active"`
	CollectionPoint string `json:"collection_point"`
	ID              int32  `json:"id"`
	Name            string `json:"name"`
}

type R11GraphRevision struct {
	ActivatedAt  *string `json:"activated_at,omitempty"`
	ControlDate  *string `json:"control_date,omitempty"`
	CreatedAt    string  `json:"created_at"`
	EffectiveDate string `json:"effective_date"`
	EmployeeID   int32   `json:"employee_id"`
	FixedHours   int32   `json:"fixed_hours"`
	ID           int32   `json:"id"`
	Label        string  `json:"label"`
	Number       int32   `json:"number"`
	ParentID     int32   `json:"parent_id"`
	Reason       string  `json:"reason"`
	ScheduleType string  `json:"schedule_type"`
	Status       string  `json:"status"`
	UpdatedAt    string  `json:"updated_at"`
}

type R11GraphVersion struct {
	CreatedAt  string            `json:"created_at"`
	Days       []json.RawMessage `json:"days"`
	EmployeeID int32             `json:"employee_id"`
	ID         int32             `json:"id"`
	Label      string            `json:"label"`
	Reason     string            `json:"reason"`
	Year       int32             `json:"year"`
}

type R11ImportHistory struct {
	Added     int32  `json:"added"`
	CreatedAt string `json:"created_at"`
	FileName  string `json:"file_name"`
	ID        int32  `json:"id"`
	Moved     int32  `json:"moved"`
	Skipped   int32  `json:"skipped"`
	Type      string `json:"type"`
	Updated   int32  `json:"updated"`
}

type R11JournalEntry struct {
	Details   string `json:"details"`
	Operation string `json:"operation"`
	Time      string `json:"time"`
}

type R11ManualColor struct {
	Color     string `json:"color"`
	Date      string `json:"date"`
	EmployeeID int32 `json:"employee_id"`
	ID        int32  `json:"id"`
	UpdatedAt string `json:"updated_at"`
}

type R11Note struct {
	CreatedAt string `json:"created_at"`
	DateFrom  string `json:"date_from"`
	DateTo    string `json:"date_to"`
	EmployeeID int32 `json:"employee_id"`
	ID        int32  `json:"id"`
	Reminder  string `json:"reminder"`
	Text      string `json:"text"`
	UpdatedAt string `json:"updated_at"`
}

type R11RVDRecord struct {
	CreatedAt string `json:"created_at"`
	DateFrom  string `json:"date_from"`
	DateTo    string `json:"date_to"`
	EmployeeID int32 `json:"employee_id"`
	ID        int32  `json:"id"`
	Note      string `json:"note"`
	RestDate  string `json:"rest_date"`
	Type      int32  `json:"type"`
}

type R11ScheduleType struct {
	Active          bool   `json:"active"`
	CollectionPoint string `json:"collection_point"`
	ID              int32  `json:"id"`
	Name            string `json:"name"`
	Note            string `json:"note"`
	PairName        string `json:"pair_name"`
	Phase           string `json:"phase"`
}

func (s *R11StateEnvelope) UnmarshalJSON(b []byte) error {
	type plain R11StateEnvelope
	var decoded plain
	if err := json.Unmarshal(b, &decoded); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	for _, key := range []string{"data", "data_path", "discrepancies", "version"} {
		delete(raw, key)
	}
	*s = R11StateEnvelope(decoded)
	s.Extra = raw
	return nil
}

func (s R11StateEnvelope) MarshalJSON() ([]byte, error) {
	type plain R11StateEnvelope
	b, err := json.Marshal(plain(s))
	if err != nil {
		return nil, err
	}
	return mergeR11Extra(b, s.Extra)
}

func (d *R11Data) UnmarshalJSON(b []byte) error {
	type plain R11Data
	var decoded plain
	if err := json.Unmarshal(b, &decoded); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	for _, key := range []string{
		"calendar", "days", "employee_flights", "employees", "events",
		"flight_dates", "flight_routes", "graph_revisions", "graph_versions",
		"import_history", "journal", "manual_colors", "next_id", "notes",
		"rvd", "schedule_changes", "schedule_types", "schema_version",
	} {
		delete(raw, key)
	}
	*d = R11Data(decoded)
	d.Extra = raw
	return nil
}

func (d R11Data) MarshalJSON() ([]byte, error) {
	type plain R11Data
	b, err := json.Marshal(plain(d))
	if err != nil {
		return nil, err
	}
	return mergeR11Extra(b, d.Extra)
}

func mergeR11Extra(knownJSON []byte, extra map[string]json.RawMessage) ([]byte, error) {
	if len(extra) == 0 {
		return knownJSON, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(knownJSON, &fields); err != nil {
		return nil, err
	}
	for key, value := range extra {
		if _, exists := fields[key]; !exists {
			fields[key] = value
		}
	}
	return json.Marshal(fields)
}
