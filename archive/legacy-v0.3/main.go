package main

import (
	_ "embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const appVersion = "0.3 АвтоГраф"

type Employee struct {
	ID          int    `json:"id"`
	TabNo       string `json:"tab_no"`
	FullName    string `json:"full_name"`
	Position    string `json:"position"`
	Unit        string `json:"unit"`
	AccountType string `json:"account_type"`
	Route       string `json:"route"`
	Notes       string `json:"notes"`
}

type WorkDay struct {
	EmployeeID int     `json:"employee_id"`
	Date       string  `json:"date"`
	Plan       string  `json:"plan"`
	PlanHours  float64 `json:"plan_hours"`
	Fact       string  `json:"fact"`
	FactHours  float64 `json:"fact_hours"`
	Shift      string  `json:"shift"`
	State      string  `json:"state"`
	Comment    string  `json:"comment"`
}

type RVD struct {
	ID         int    `json:"id"`
	EmployeeID int    `json:"employee_id"`
	DateFrom   string `json:"date_from"`
	DateTo     string `json:"date_to"`
	Type       int    `json:"type"`
	RestDate   string `json:"rest_date"`
	Note       string `json:"note"`
	CreatedAt  string `json:"created_at"`
}

type Event struct {
	ID         int    `json:"id"`
	EmployeeID int    `json:"employee_id"`
	DateFrom   string `json:"date_from"`
	DateTo     string `json:"date_to"`
	EventType  string `json:"event_type"`
	SourceCode string `json:"source_code"`
	Source     string `json:"source"`
	Note       string `json:"note"`
	CreatedAt  string `json:"created_at"`
}

type JournalEntry struct {
	ID        int    `json:"id"`
	Time      string `json:"time"`
	Operation string `json:"operation"`
	Details   string `json:"details"`
}

type AppData struct {
	Version   string         `json:"version"`
	Employees []Employee     `json:"employees"`
	Days      []WorkDay      `json:"days"`
	RVD       []RVD          `json:"rvd"`
	Events    []Event        `json:"events"`
	Journal   []JournalEntry `json:"journal"`
	NextID    int            `json:"next_id"`
}

type Store struct {
	mu       sync.Mutex
	data     AppData
	dataPath string
	bakDir   string
}

func newStore() (*Store, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		exe, _ := os.Executable()
		base = filepath.Join(filepath.Dir(exe), "data")
	} else {
		base = filepath.Join(base, "LaborControl")
	}
	if err := os.MkdirAll(base, 0755); err != nil {
		return nil, err
	}
	bak := filepath.Join(base, "backups")
	if err := os.MkdirAll(bak, 0755); err != nil {
		return nil, err
	}
	s := &Store{dataPath: filepath.Join(base, "labor_control.json"), bakDir: bak}
	s.data = AppData{Version: appVersion, NextID: 1}
	if b, err := os.ReadFile(s.dataPath); err == nil {
		if err := json.Unmarshal(b, &s.data); err != nil {
			return nil, fmt.Errorf("не удалось открыть локальную базу: %w", err)
		}
		if s.data.NextID < 1 {
			s.data.NextID = 1
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	s.data.Version = appVersion
	return s, s.saveLocked()
}

func (s *Store) nextIDLocked() int { id := s.data.NextID; s.data.NextID++; return id }
func (s *Store) addLogLocked(op, details string) {
	s.data.Journal = append(s.data.Journal, JournalEntry{ID: s.nextIDLocked(), Time: time.Now().Format("02.01.2006 15:04:05"), Operation: op, Details: details})
	if len(s.data.Journal) > 1000 {
		s.data.Journal = s.data.Journal[len(s.data.Journal)-1000:]
	}
}
func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.dataPath + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.dataPath)
}
func (s *Store) backupLocked(reason string) error {
	if _, err := os.Stat(s.dataPath); err != nil {
		return nil
	}
	name := fmt.Sprintf("labor_control_%s_%s.json", time.Now().Format("20060102_150405.000"), safeName(reason))
	in, err := os.Open(s.dataPath)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(filepath.Join(s.bakDir, name))
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, s)
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}
func (s *Store) mutate(reason string, fn func(*AppData) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.backupLocked(reason)
	if err := fn(&s.data); err != nil {
		return err
	}
	return s.saveLocked()
}
func (s *Store) snapshot() AppData {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(s.data)
	var cp AppData
	_ = json.Unmarshal(b, &cp)
	return cp
}

func findEmployeeByTab(d *AppData, tab string) *Employee {
	tab = strings.TrimSpace(tab)
	for i := range d.Employees {
		if d.Employees[i].TabNo == tab {
			return &d.Employees[i]
		}
	}
	return nil
}
func empName(d AppData, id int) string {
	for _, e := range d.Employees {
		if e.ID == id {
			return e.FullName
		}
	}
	return ""
}
func empTab(d AppData, id int) string {
	for _, e := range d.Employees {
		if e.ID == id {
			return e.TabNo
		}
	}
	return ""
}

func main() {
	store, err := newStore()
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, indexHTML)
	})
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		d := store.snapshot()
		disc := computeDiscrepancies(d)
		writeJSON(w, map[string]any{"data": d, "discrepancies": disc, "data_path": store.dataPath, "version": appVersion})
	})
	mux.HandleFunc("/api/employee/save", func(w http.ResponseWriter, r *http.Request) {
		var e Employee
		if !decodeJSON(w, r, &e) {
			return
		}
		e.TabNo = strings.TrimSpace(e.TabNo)
		e.FullName = strings.TrimSpace(e.FullName)
		if e.TabNo == "" || e.FullName == "" {
			http.Error(w, "Табельный номер и ФИО обязательны", 400)
			return
		}
		err := store.mutate("employee", func(d *AppData) error {
			if old := findEmployeeByTab(d, e.TabNo); old != nil && old.ID != e.ID {
				return fmt.Errorf("табельный номер %s уже существует", e.TabNo)
			}
			if e.ID == 0 {
				e.ID = store.nextIDLocked()
				d.Employees = append(d.Employees, e)
				store.addLogLocked("Сотрудник добавлен", e.TabNo+" — "+e.FullName)
			} else {
				found := false
				for i := range d.Employees {
					if d.Employees[i].ID == e.ID {
						d.Employees[i] = e
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("сотрудник не найден")
				}
				store.addLogLocked("Сотрудник изменён", e.TabNo+" — "+e.FullName)
			}
			return nil
		})
		respondErr(w, err)
	})
	mux.HandleFunc("/api/employee/delete", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			ID int `json:"id"`
		}
		if !decodeJSON(w, r, &q) {
			return
		}
		err := store.mutate("delete_employee", func(d *AppData) error {
			idx := -1
			var label string
			for i, e := range d.Employees {
				if e.ID == q.ID {
					idx = i
					label = e.TabNo + " — " + e.FullName
					break
				}
			}
			if idx < 0 {
				return fmt.Errorf("сотрудник не найден")
			}
			d.Employees = append(d.Employees[:idx], d.Employees[idx+1:]...)
			nd := d.Days[:0]
			for _, x := range d.Days {
				if x.EmployeeID != q.ID {
					nd = append(nd, x)
				}
			}
			d.Days = nd
			nr := d.RVD[:0]
			for _, x := range d.RVD {
				if x.EmployeeID != q.ID {
					nr = append(nr, x)
				}
			}
			d.RVD = nr
			ne := d.Events[:0]
			for _, x := range d.Events {
				if x.EmployeeID != q.ID {
					ne = append(ne, x)
				}
			}
			d.Events = ne
			store.addLogLocked("Сотрудник удалён", label)
			return nil
		})
		respondErr(w, err)
	})
	mux.HandleFunc("/api/day/save", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			TabNo, Date, Plan, Fact, Shift, State, Comment string
			PlanHours, FactHours                           float64
		}
		if !decodeJSON(w, r, &q) {
			return
		}
		if q.TabNo == "" || q.Date == "" {
			http.Error(w, "Табельный номер и дата обязательны", 400)
			return
		}
		err := store.mutate("day", func(d *AppData) error {
			e := findEmployeeByTab(d, q.TabNo)
			if e == nil {
				return fmt.Errorf("сотрудник с табельным %s не найден", q.TabNo)
			}
			pos := -1
			for i, x := range d.Days {
				if x.EmployeeID == e.ID && x.Date == q.Date {
					pos = i
					break
				}
			}
			x := WorkDay{EmployeeID: e.ID, Date: q.Date, Plan: q.Plan, PlanHours: q.PlanHours, Fact: q.Fact, FactHours: q.FactHours, Shift: q.Shift, State: q.State, Comment: q.Comment}
			if pos >= 0 {
				d.Days[pos] = x
			} else {
				d.Days = append(d.Days, x)
			}
			store.addLogLocked("План/факт сохранён", q.TabNo+"; "+q.Date+"; план="+q.Plan+"; факт="+q.Fact)
			return nil
		})
		respondErr(w, err)
	})
	mux.HandleFunc("/api/day/delete", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			TabNo string `json:"tab_no"`
			Date  string `json:"date"`
		}
		if !decodeJSON(w, r, &q) {
			return
		}
		err := store.mutate("day_delete", func(d *AppData) error {
			e := findEmployeeByTab(d, q.TabNo)
			if e == nil {
				return fmt.Errorf("сотрудник с табельным %s не найден", q.TabNo)
			}
			nd := d.Days[:0]
			deleted := false
			for _, x := range d.Days {
				if x.EmployeeID == e.ID && x.Date == q.Date {
					deleted = true
					continue
				}
				nd = append(nd, x)
			}
			d.Days = nd
			if deleted {
				store.addLogLocked("День АвтоГрафа удалён", q.TabNo+"; "+q.Date)
			}
			return nil
		})
		respondErr(w, err)
	})
	mux.HandleFunc("/api/rvd/save", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			TabNo, DateFrom, DateTo, RestDate, Note string
			Type                                    int
		}
		if !decodeJSON(w, r, &q) {
			return
		}
		if q.Type != 1 && q.Type != 2 {
			http.Error(w, "Тип РВД должен быть 1 или 2", 400)
			return
		}
		if q.Type == 1 && q.RestDate == "" {
			http.Error(w, "Для РВД-1 обязательно укажите день отдыха", 400)
			return
		}
		err := store.mutate("rvd", func(d *AppData) error {
			e := findEmployeeByTab(d, q.TabNo)
			if e == nil {
				return fmt.Errorf("сотрудник с табельным %s не найден", q.TabNo)
			}
			id := store.nextIDLocked()
			d.RVD = append(d.RVD, RVD{ID: id, EmployeeID: e.ID, DateFrom: q.DateFrom, DateTo: q.DateTo, Type: q.Type, RestDate: q.RestDate, Note: q.Note, CreatedAt: time.Now().Format(time.RFC3339)})
			evType := "RVD2"
			if q.Type == 1 {
				evType = "RVD1"
			}
			d.Events = append(d.Events, Event{ID: store.nextIDLocked(), EmployeeID: e.ID, DateFrom: q.DateFrom, DateTo: q.DateTo, EventType: evType, SourceCode: "2004", Source: "РВД", Note: q.Note, CreatedAt: time.Now().Format(time.RFC3339)})
			if q.Type == 1 {
				d.Events = append(d.Events, Event{ID: store.nextIDLocked(), EmployeeID: e.ID, DateFrom: q.RestDate, DateTo: q.RestDate, EventType: "REST_RVD1", Source: "РВД", Note: "День отдыха за РВД-1", CreatedAt: time.Now().Format(time.RFC3339)})
			}
			store.addLogLocked("РВД добавлено", q.TabNo+"; тип="+strconv.Itoa(q.Type)+"; "+q.DateFrom+" — "+q.DateTo)
			return nil
		})
		respondErr(w, err)
	})
	mux.HandleFunc("/api/event/save", func(w http.ResponseWriter, r *http.Request) {
		var q struct{ TabNo, DateFrom, DateTo, EventType, SourceCode, Source, Note string }
		if !decodeJSON(w, r, &q) {
			return
		}
		err := store.mutate("event", func(d *AppData) error {
			e := findEmployeeByTab(d, q.TabNo)
			if e == nil {
				return fmt.Errorf("сотрудник с табельным %s не найден", q.TabNo)
			}
			d.Events = append(d.Events, Event{ID: store.nextIDLocked(), EmployeeID: e.ID, DateFrom: q.DateFrom, DateTo: q.DateTo, EventType: q.EventType, SourceCode: q.SourceCode, Source: q.Source, Note: q.Note, CreatedAt: time.Now().Format(time.RFC3339)})
			store.addLogLocked("Событие добавлено", q.TabNo+"; "+q.EventType+"; "+q.DateFrom+" — "+q.DateTo)
			return nil
		})
		respondErr(w, err)
	})
	mux.HandleFunc("/api/event/delete", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			ID int `json:"id"`
		}
		if !decodeJSON(w, r, &q) {
			return
		}
		err := store.mutate("event_delete", func(d *AppData) error {
			ne := d.Events[:0]
			deleted := ""
			for _, x := range d.Events {
				if x.ID == q.ID {
					deleted = fmt.Sprintf("%s; %s — %s", x.EventType, x.DateFrom, x.DateTo)
					continue
				}
				ne = append(ne, x)
			}
			d.Events = ne
			if deleted != "" {
				store.addLogLocked("Событие удалено", deleted)
			}
			return nil
		})
		respondErr(w, err)
	})
	mux.HandleFunc("/api/demo", func(w http.ResponseWriter, r *http.Request) {
		err := store.mutate("demo", func(d *AppData) error {
			if findEmployeeByTab(d, "311267") != nil {
				return nil
			}
			d.Employees = append(d.Employees, Employee{ID: store.nextIDLocked(), TabNo: "311267", FullName: "Филимонов Иван Сергеевич", AccountType: "NORMAL", Notes: "Демонстрационная запись"})
			store.addLogLocked("Добавлен демонстрационный сотрудник", "311267 — Филимонов Иван Сергеевич")
			return nil
		})
		respondErr(w, err)
	})
	mux.HandleFunc("/api/demo/autograf", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Year int `json:"year"`
		}
		if !decodeJSON(w, r, &q) {
			return
		}
		if q.Year < 2020 || q.Year > 2100 {
			q.Year = time.Now().Year()
		}
		err := store.mutate("demo_autograf", func(d *AppData) error {
			e := findEmployeeByTab(d, "311267")
			if e == nil {
				d.Employees = append(d.Employees, Employee{ID: store.nextIDLocked(), TabNo: "311267", FullName: "Филимонов Иван Сергеевич", AccountType: "NORMAL", Route: "ДЕМО", Notes: "Демонстрационная запись"})
				e = findEmployeeByTab(d, "311267")
			}
			plans := []struct {
				Day, Plan, Shift, State, Comment string
				Hours                            float64
			}{
				{"03", "11", "DAY", "WORK", "ДЕМО", 11},
				{"04", "11", "DAY", "WORK", "ДЕМО", 11},
				{"05", "В", "", "FREE", "ДЕМО", 0},
				{"06", "2/11", "NIGHT", "WORK", "ДЕМО", 11},
				{"07", "В", "", "FREE", "ДЕМО", 0},
				{"09", "ДП", "", "TRAVEL", "ДЕМО", 0},
				{"10", "11", "DAY", "WORK", "ДЕМО", 11},
				{"11", "1/11", "DAY", "WORK", "ДЕМО", 11},
				{"12", "В", "", "FREE", "ДЕМО", 0},
				{"13", "11", "DAY", "WORK", "ДЕМО", 11},
				{"14", "11", "DAY", "WORK", "ДЕМО", 11},
				{"15", "В", "", "FREE", "ДЕМО", 0},
			}
			for _, p := range plans {
				date := fmt.Sprintf("%04d-01-%s", q.Year, p.Day)
				pos := -1
				for i, x := range d.Days {
					if x.EmployeeID == e.ID && x.Date == date {
						pos = i
						break
					}
				}
				x := WorkDay{EmployeeID: e.ID, Date: date, Plan: p.Plan, PlanHours: p.Hours, Shift: p.Shift, State: p.State, Comment: p.Comment}
				if pos >= 0 {
					d.Days[pos] = x
				} else {
					d.Days = append(d.Days, x)
				}
			}
			baseEvents := []Event{
				{EmployeeID: e.ID, DateFrom: fmt.Sprintf("%04d-01-09", q.Year), DateTo: fmt.Sprintf("%04d-01-09", q.Year), EventType: "DP", Source: "ДЕМО", Note: "Демонстрационный ДП"},
				{EmployeeID: e.ID, DateFrom: fmt.Sprintf("%04d-01-13", q.Year), DateTo: fmt.Sprintf("%04d-01-13", q.Year), EventType: "RVD2", SourceCode: "2004", Source: "ДЕМО", Note: "Демонстрационная РВД-2"},
				{EmployeeID: e.ID, DateFrom: fmt.Sprintf("%04d-01-18", q.Year), DateTo: fmt.Sprintf("%04d-01-20", q.Year), EventType: "VACATION", SourceCode: "2001", Source: "ДЕМО", Note: "Демонстрационный отпуск"},
				{EmployeeID: e.ID, DateFrom: fmt.Sprintf("%04d-01-25", q.Year), DateTo: fmt.Sprintf("%04d-01-26", q.Year), EventType: "SICK", SourceCode: "2001", Source: "ДЕМО", Note: "Демонстрационный больничный"},
			}
			for _, ev := range baseEvents {
				exists := false
				for _, old := range d.Events {
					if old.EmployeeID == ev.EmployeeID && old.DateFrom == ev.DateFrom && old.DateTo == ev.DateTo && old.EventType == ev.EventType && old.Source == "ДЕМО" {
						exists = true
						break
					}
				}
				if !exists {
					ev.ID = store.nextIDLocked()
					ev.CreatedAt = time.Now().Format(time.RFC3339)
					d.Events = append(d.Events, ev)
				}
			}
			store.addLogLocked("Демо АвтоГраф", fmt.Sprintf("311267; январь %d; данные только для демонстрации интерфейса", q.Year))
			return nil
		})
		respondErr(w, err)
	})
	mux.HandleFunc("/api/backup", func(w http.ResponseWriter, r *http.Request) {
		store.mu.Lock()
		defer store.mu.Unlock()
		err := store.backupLocked("manual")
		if err == nil {
			store.addLogLocked("Резервная копия", "Создана вручную")
			err = store.saveLocked()
		}
		respondErr(w, err)
	})
	mux.HandleFunc("/api/import/fact", func(w http.ResponseWriter, r *http.Request) { importFact(w, r, store) })
	mux.HandleFunc("/api/import/events", func(w http.ResponseWriter, r *http.Request) { importEvents(w, r, store) })
	mux.HandleFunc("/api/shutdown", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true})
		go func() { time.Sleep(300 * time.Millisecond); os.Exit(0) }()
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	addr := "http://" + ln.Addr().String() + "/"
	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Print(err)
		}
	}()
	if err := openBrowser(addr); err != nil {
		fmt.Println("Откройте в браузере:", addr)
	}
	select {}
}

type discrepancy struct {
	TabNo, FullName, Date, Plan, Fact, Kind string
	PlanHours, FactHours                    float64
}

func computeDiscrepancies(d AppData) []discrepancy {
	var out []discrepancy
	for _, x := range d.Days {
		plan := strings.TrimSpace(x.Plan)
		fact := strings.TrimSpace(x.Fact)
		kind := ""
		if plan != "" && fact == "" {
			kind = "Нет факта"
		} else if plan == "" && fact != "" {
			kind = "Нет плана"
		} else if plan != fact || (x.PlanHours != 0 || x.FactHours != 0) && x.PlanHours != x.FactHours {
			kind = "План ≠ факт"
		}
		if kind != "" {
			out = append(out, discrepancy{TabNo: empTab(d, x.EmployeeID), FullName: empName(d, x.EmployeeID), Date: x.Date, Plan: x.Plan, Fact: x.Fact, Kind: kind, PlanHours: x.PlanHours, FactHours: x.FactHours})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date == out[j].Date {
			return out[i].TabNo < out[j].TabNo
		}
		return out[i].Date < out[j].Date
	})
	return out
}
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Method != "POST" {
		http.Error(w, "POST only", 405)
		return false
	}
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, err.Error(), 400)
		return false
	}
	return true
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
func respondErr(w http.ResponseWriter, err error) {
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", "."), 64)
	return f
}

func importFact(w http.ResponseWriter, r *http.Request, store *Store) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	defer f.Close()
	rd := csv.NewReader(f)
	rd.FieldsPerRecord = -1
	rows, err := rd.ReadAll()
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if len(rows) < 2 {
		http.Error(w, "CSV пуст", 400)
		return
	}
	header := map[string]int{}
	for i, h := range rows[0] {
		header[strings.TrimSpace(strings.ToLower(h))] = i
	}
	req := []string{"tab_no", "date", "value"}
	for _, k := range req {
		if _, ok := header[k]; !ok {
			http.Error(w, "Нет колонки "+k, 400)
			return
		}
	}
	count := 0
	err = store.mutate("import_fact", func(d *AppData) error {
		for _, row := range rows[1:] {
			get := func(k string) string {
				i, ok := header[k]
				if !ok || i >= len(row) {
					return ""
				}
				return strings.TrimSpace(row[i])
			}
			tab := get("tab_no")
			e := findEmployeeByTab(d, tab)
			if e == nil {
				continue
			}
			date := get("date")
			pos := -1
			for i, x := range d.Days {
				if x.EmployeeID == e.ID && x.Date == date {
					pos = i
					break
				}
			}
			var x WorkDay
			if pos >= 0 {
				x = d.Days[pos]
			} else {
				x = WorkDay{EmployeeID: e.ID, Date: date}
			}
			x.Fact = get("value")
			x.FactHours = parseFloat(get("hours"))
			x.Shift = get("shift")
			if pos >= 0 {
				d.Days[pos] = x
			} else {
				d.Days = append(d.Days, x)
			}
			count++
		}
		store.addLogLocked("Импорт факта", fmt.Sprintf("Загружено %d строк", count))
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "count": count})
}
func importEvents(w http.ResponseWriter, r *http.Request, store *Store) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	defer f.Close()
	rd := csv.NewReader(f)
	rd.FieldsPerRecord = -1
	rows, err := rd.ReadAll()
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if len(rows) < 2 {
		http.Error(w, "CSV пуст", 400)
		return
	}
	header := map[string]int{}
	for i, h := range rows[0] {
		header[strings.TrimSpace(strings.ToLower(h))] = i
	}
	count := 0
	err = store.mutate("import_events", func(d *AppData) error {
		for _, row := range rows[1:] {
			get := func(k string) string {
				i, ok := header[k]
				if !ok || i >= len(row) {
					return ""
				}
				return strings.TrimSpace(row[i])
			}
			e := findEmployeeByTab(d, get("tab_no"))
			if e == nil {
				continue
			}
			d.Events = append(d.Events, Event{ID: store.nextIDLocked(), EmployeeID: e.ID, DateFrom: get("date_from"), DateTo: get("date_to"), EventType: get("event_type"), SourceCode: get("source_code"), Source: get("source"), Note: get("note"), CreatedAt: time.Now().Format(time.RFC3339)})
			count++
		}
		store.addLogLocked("Импорт событий", fmt.Sprintf("Загружено %d строк", count))
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "count": count})
}
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

//go:embed index.html
var indexHTML string