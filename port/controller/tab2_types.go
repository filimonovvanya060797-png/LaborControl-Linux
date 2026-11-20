package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type tab2ImportLog struct {
	ID         int    `json:"id"`
	Kind       string `json:"kind"`
	FileName   string `json:"file_name"`
	ImportedAt string `json:"imported_at"`
	Rows       int    `json:"rows"`
	Applied    int    `json:"applied"`
	Skipped    int    `json:"skipped"`
	Message    string `json:"message,omitempty"`
}

type tab2FactRecord struct {
	ID         int     `json:"id"`
	TabNo      string  `json:"tab_no"`
	FullName   string  `json:"full_name"`
	Start      string  `json:"start"`
	End        string  `json:"end"`
	Date       string  `json:"date"`
	Hours      float64 `json:"hours"`
	Day        int     `json:"day"`
	Time       string  `json:"time"`
	Shift      string  `json:"shift"`
	SourceFile string  `json:"source_file"`
	ImportedAt string  `json:"imported_at"`
}

type tab2SZRVDRecord struct {
	ID         int    `json:"id"`
	TabNo      string `json:"tab_no"`
	FullName   string `json:"full_name"`
	DateFrom   string `json:"date_from"`
	DateTo     string `json:"date_to"`
	Type       string `json:"type"`
	RestFrom   string `json:"rest_from,omitempty"`
	RestTo     string `json:"rest_to,omitempty"`
	SourceFile string `json:"source_file"`
	ImportedAt string `json:"imported_at"`
}

type tab2SAPRecord struct {
	ID         int     `json:"id"`
	Infotype   string  `json:"infotype"`
	TabNo      string  `json:"tab_no"`
	FullName   string  `json:"full_name"`
	Unit       string  `json:"unit,omitempty"`
	DateFrom   string  `json:"date_from"`
	DateTo     string  `json:"date_to"`
	Code       string  `json:"code,omitempty"`
	Text       string  `json:"text,omitempty"`
	StartTime  string  `json:"start_time,omitempty"`
	EndTime    string  `json:"end_time,omitempty"`
	Hours      float64 `json:"hours,omitempty"`
	SourceFile string  `json:"source_file"`
	ImportedAt string  `json:"imported_at"`
}

type tab2Store struct {
	SchemaVersion int               `json:"schema_version"`
	NextID        int               `json:"next_id"`
	Imports       []tab2ImportLog   `json:"imports"`
	SAPRecords    []tab2SAPRecord   `json:"sap_records"`
	Facts         []tab2FactRecord  `json:"facts"`
	SZRVD         []tab2SZRVDRecord `json:"sz_rvd"`
}

type tab2PaletteItem struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Color string `json:"color"`
	Note  string `json:"note,omitempty"`
}

var tab2Palette = []tab2PaletteItem{
	{"RVD1", "РВД одинарная", "#92D050", "работа в выходной с предоставлением дней отдыха"},
	{"RVD2", "РВД двойная", "#00B0F0", "работа в выходной с двойной оплатой"},
	{"REST_RVD1", "Отгулы", "#FFFF00", "дни отдыха за РВД-1"},
	{"VACATION", "ОТ", "#B7DEE8", "ежегодный оплачиваемый отпуск"},
	{"MED", "МО", "#FFC000", "медосмотр"},
	{"SICK", "Б", "#948A54", "больничный"},
	{"EXT_VACATION", "ОД", "#CCC0DA", "донорские/продлённые дни"},
	{"UNPAID", "ОБС", "#60497A", "отпуск без сохранения"},
	{"STUDY", "ПК, К, У", "#E26B0A", "обучение / командировка / учебный отпуск"},
	{"ABSENCE", "НН", "#FF0000", "неявка"},
	{"NO_SHOW", "Прогул", "#808080", "прогул"},
	{"DAY", "День", "#FDE9D9", "дневная смена"},
	{"NIGHT", "Ночь", "#31869B", "ночная смена"},
	{"DP", "ДП", "#FF66FF", "день прилёта / вылета"},
	{"TECH", "ТО, ремонт, стажировка", "#66FFFF", "технические и обучающие события"},
	{"OVERLAP", "Наложение", "#D9D9D9", "одновременно несколько событий"},
}

type mainDB struct {
	Root      map[string]json.RawMessage
	Employees []map[string]any
	Days      []map[string]any
	RVD       []map[string]any
	Events    []map[string]any
	Flights   []map[string]any
	Notes     []map[string]any
	Journal   []map[string]any
	NextID    int
}

func (h *controller) tab2Path() string   { return filepath.Join(h.baseDir, "labor_control_tab2.json") }
func (h *controller) mainDBPath() string { return filepath.Join(h.baseDir, "labor_control.json") }

var tab2StoreMu sync.Mutex

func (h *controller) loadTab2Store() (tab2Store, error) {
	p := h.tab2Path()
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return tab2Store{SchemaVersion: 2, NextID: 1, Imports: []tab2ImportLog{}, SAPRecords: []tab2SAPRecord{}, Facts: []tab2FactRecord{}, SZRVD: []tab2SZRVDRecord{}}, nil
	}
	if err != nil {
		return tab2Store{}, err
	}
	var s tab2Store
	if err = json.Unmarshal(b, &s); err != nil {
		return tab2Store{}, err
	}
	if s.SchemaVersion == 0 {
		s.SchemaVersion = 2
	}
	if s.NextID < 1 {
		s.NextID = 1
	}
	if s.Imports == nil {
		s.Imports = []tab2ImportLog{}
	}
	if s.SAPRecords == nil {
		s.SAPRecords = []tab2SAPRecord{}
	}
	if s.Facts == nil {
		s.Facts = []tab2FactRecord{}
	}
	if s.SZRVD == nil {
		s.SZRVD = []tab2SZRVDRecord{}
	}
	if s.SchemaVersion < 2 {
		s.SchemaVersion = 2
	}
	return s, nil
}
func (h *controller) saveTab2Store(s tab2Store) error {
	s.SchemaVersion = 2
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return atomicWrite(h.tab2Path(), b)
}
func tab2Alloc(s *tab2Store) int {
	if s.NextID < 1 {
		s.NextID = 1
	}
	id := s.NextID
	s.NextID++
	return id
}

func (h *controller) loadMainDB() (mainDB, error) {
	b, err := os.ReadFile(h.mainDBPath())
	if err != nil {
		return mainDB{}, err
	}
	var root map[string]json.RawMessage
	if err = json.Unmarshal(b, &root); err != nil {
		return mainDB{}, err
	}
	d := mainDB{Root: root, NextID: 1}
	dec := func(k string, dst any) {
		if raw := root[k]; len(raw) > 0 {
			_ = json.Unmarshal(raw, dst)
		}
	}
	dec("employees", &d.Employees)
	dec("days", &d.Days)
	dec("rvd", &d.RVD)
	dec("events", &d.Events)
	dec("employee_flights", &d.Flights)
	dec("notes", &d.Notes)
	dec("journal", &d.Journal)
	dec("next_id", &d.NextID)
	if d.NextID < 1 {
		d.NextID = 1
	}
	return d, nil
}
func (h *controller) saveMainDB(d mainDB, reason string) (string, error) {
	backup, err := makeBackup(h.mainDBPath(), filepath.Join(h.baseDir, "backups"))
	if err != nil {
		return "", err
	}
	put := func(k string, v any) error {
		b, e := json.Marshal(v)
		if e != nil {
			return e
		}
		d.Root[k] = b
		return nil
	}
	if err = put("employees", d.Employees); err != nil {
		return "", err
	}
	if err = put("days", d.Days); err != nil {
		return "", err
	}
	if err = put("rvd", d.RVD); err != nil {
		return "", err
	}
	if err = put("events", d.Events); err != nil {
		return "", err
	}
	if err = put("employee_flights", d.Flights); err != nil {
		return "", err
	}
	if err = put("notes", d.Notes); err != nil {
		return "", err
	}
	if err = put("journal", d.Journal); err != nil {
		return "", err
	}
	if err = put("next_id", d.NextID); err != nil {
		return "", err
	}
	out, err := json.MarshalIndent(d.Root, "", "  ")
	if err != nil {
		return "", err
	}
	out = append(out, '\n')
	if err = atomicWrite(h.mainDBPath(), out); err != nil {
		return "", err
	}
	logDetails := reason
	if logDetails == "" {
		logDetails = "Табель 2.0"
	}
	_ = logDetails
	return backup, nil
}
func mainAlloc(d *mainDB) int { id := d.NextID; d.NextID++; return id }

func empByTab(d *mainDB) map[string]map[string]any {
	m := map[string]map[string]any{}
	for _, e := range d.Employees {
		t := trimTab(toString(e["tab_no"]))
		if t != "" {
			m[t] = e
		}
	}
	return m
}
func empIDByTab(d *mainDB) map[string]int {
	m := map[string]int{}
	for _, e := range d.Employees {
		t := trimTab(toString(e["tab_no"]))
		if t != "" {
			m[t] = toInt(e["id"])
		}
	}
	return m
}
func empNameByID(d *mainDB) map[int]string {
	m := map[int]string{}
	for _, e := range d.Employees {
		m[toInt(e["id"])] = toString(e["full_name"])
	}
	return m
}
func empTabByID(d *mainDB) map[int]string {
	m := map[int]string{}
	for _, e := range d.Employees {
		m[toInt(e["id"])] = trimTab(toString(e["tab_no"]))
	}
	return m
}
func dayKey(eid int, date string) string { return fmt.Sprintf("%d|%s", eid, date) }
func dayMap(d *mainDB) map[string]map[string]any {
	m := map[string]map[string]any{}
	for _, x := range d.Days {
		m[dayKey(toInt(x["employee_id"]), toString(x["date"]))] = x
	}
	return m
}
func rvdCancelledMain(d *mainDB, r map[string]any) bool {
	key := "RVD_CANCEL:" + toString(r["id"])
	for _, e := range d.Events {
		if toString(e["event_type"]) == "RVD_CANCEL" && toString(e["source_code"]) == key {
			return true
		}
	}
	return false
}
func activeRVDMain(d *mainDB) []map[string]any {
	var out []map[string]any
	for _, r := range d.RVD {
		if !rvdCancelledMain(d, r) {
			out = append(out, r)
		}
	}
	return out
}

func appendJournal(d *mainDB, op, details string) {
	d.Journal = append(d.Journal, map[string]any{"id": mainAlloc(d), "time": time.Now().Format("02.01.2006 15:04:05"), "operation": op, "details": details})
}

func sortDays(d *mainDB) {
	sort.SliceStable(d.Days, func(i, j int) bool {
		ei, ej := toInt(d.Days[i]["employee_id"]), toInt(d.Days[j]["employee_id"])
		if ei != ej {
			return ei < ej
		}
		return toString(d.Days[i]["date"]) < toString(d.Days[j]["date"])
	})
}
func stringsOverlap(a1, a2, b1, b2 string) bool {
	if a2 == "" {
		a2 = a1
	}
	if b2 == "" {
		b2 = b1
	}
	return a1 <= b2 && b1 <= a2
}
func monthBounds(year, month int) (string, string) {
	if month < 1 || month > 12 {
		month = 1
	}
	start := fmt.Sprintf("%04d-%02d-01", year, month)
	t, _ := time.Parse("2006-01-02", start)
	end := t.AddDate(0, 1, -1).Format("2006-01-02")
	return start, end
}
func dateInRange(d, a, b string) bool { return d != "" && d >= a && d <= b }
func cleanLower(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), "ё", "е"))
}