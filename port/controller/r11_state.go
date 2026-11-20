package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const r11StateCompatVersion = "R11-LINUX-STATE-COMPAT/0.1"

//go:embed r11_state.html
var r11StateHTML []byte

var r11RequiredDataKeys = []string{
	"calendar", "days", "employee_flights", "employees", "events",
	"flight_dates", "flight_routes", "graph_revisions", "graph_versions",
	"import_history", "journal", "manual_colors", "next_id", "notes",
	"rvd", "schedule_changes", "schedule_types", "schema_version",
}

type r11DayChange struct {
	EmployeeID int64   `json:"employee_id"`
	Date       string  `json:"date"`
	Plan       string  `json:"plan"`
	PlanHours  int32   `json:"plan_hours"`
	State      *string `json:"state,omitempty"`
}

type r11DayChangesRequest struct {
	Days []r11DayChange `json:"days"`
}

// runStandaloneR11 starts the local state workspace when the Linux Core binary
// is not yet available. It deliberately does not pretend to run the Windows
// Core or its schedule-generation algorithms.
func (h *controller) runStandaloneR11() error {
	h.standalone = true
	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.wrap(h.health))
	mux.HandleFunc("/start", h.wrap(h.standaloneStart))
	mux.HandleFunc("/api/state", h.wrap(h.r11State))
	mux.HandleFunc("/api/state/import", h.wrap(h.r11ImportState))
	mux.HandleFunc("/api/state/days", h.wrap(h.r11UpdateDays))
	mux.HandleFunc("/api/shutdown", h.wrap(h.shutdownAll))
	mux.HandleFunc("/r11", h.serveR11UI)
	mux.HandleFunc("/", h.serveR11UI)

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("не удалось открыть локальный адрес %s: %w", listenAddr, err)
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, MaxHeaderBytes: 1 << 20}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("R11 state workspace stopped: %v", err)
		}
	}()

	uiURL := "http://" + listenAddr + "/r11"
	log.Printf("%s started in local state mode; database=%s; UI=%s", r11StateCompatVersion, h.mainDBPath(), uiURL)
	if err := openURL(uiURL); err != nil {
		showMessage("LaborControl", "Локальный редактор запущен. Откройте в браузере: "+uiURL)
	}
	select {}
}

func (h *controller) standaloneStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "GET/POST only", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "url": "http://" + listenAddr + "/r11"})
}

func (h *controller) serveR11UI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/r11" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(r11StateHTML)
}

func (h *controller) r11State(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	h.opMu.Lock()
	data, err := h.readR11DataLocked()
	h.opMu.Unlock()
	if err != nil {
		http.Error(w, "не удалось прочитать локальное состояние: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"data":          json.RawMessage(data),
		"data_path":     filepath.Base(h.mainDBPath()),
		"discrepancies": []any{},
		"version":       r11StateCompatVersion,
	})
}

func (h *controller) r11ImportState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "не удалось прочитать файл состояния: "+err.Error(), http.StatusBadRequest)
		return
	}
	data, counts, err := decodeR11Import(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.opMu.Lock()
	defer h.opMu.Unlock()
	backup, err := h.backupR11StateLocked()
	if err != nil {
		http.Error(w, "не удалось создать резервную копию: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := atomicWrite(h.mainDBPath(), data); err != nil {
		http.Error(w, "не удалось сохранить состояние: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"ok": true, "employees": counts.Employees, "days": counts.Days,
		"backup": r11BackupLabel(backup),
	})
}

type r11ImportCounts struct {
	Employees int
	Days      int
}

func decodeR11Import(body []byte) ([]byte, r11ImportCounts, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var incoming map[string]json.RawMessage
	if err := dec.Decode(&incoming); err != nil {
		return nil, r11ImportCounts{}, fmt.Errorf("неверный JSON: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, r11ImportCounts{}, fmt.Errorf("после JSON обнаружены лишние данные")
	}
	if incoming == nil {
		return nil, r11ImportCounts{}, fmt.Errorf("ожидался JSON-объект")
	}

	data := json.RawMessage(body)
	if wrapped, ok := incoming["data"]; ok {
		data = wrapped
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return nil, r11ImportCounts{}, fmt.Errorf("поле data должно быть JSON-объектом")
	}
	for _, key := range r11RequiredDataKeys {
		if _, ok := fields[key]; !ok {
			return nil, r11ImportCounts{}, fmt.Errorf("в состоянии отсутствует обязательное поле %q", key)
		}
	}
	var typed R11Data
	if err := json.Unmarshal(data, &typed); err != nil {
		return nil, r11ImportCounts{}, fmt.Errorf("структура состояния R11 не распознана: %w", err)
	}
	if typed.NextID < 1 {
		return nil, r11ImportCounts{}, fmt.Errorf("next_id должен быть положительным")
	}
	out, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return nil, r11ImportCounts{}, err
	}
	return append(out, '\n'), r11ImportCounts{Employees: len(typed.Employees), Days: len(typed.Days)}, nil
}

func (h *controller) readR11DataLocked() ([]byte, error) {
	path := h.mainDBPath()
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return emptyR11DataJSON(), nil
	}
	if err != nil {
		return nil, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("JSON базы повреждён: %w", err)
	}
	if wrapped, ok := root["data"]; ok {
		if err := json.Unmarshal(wrapped, &root); err != nil {
			return nil, fmt.Errorf("поле data повреждено: %w", err)
		}
	}
	for _, key := range r11RequiredDataKeys {
		if _, ok := root[key]; !ok {
			switch key {
			case "next_id":
				root[key] = json.RawMessage("1")
			case "schema_version":
				root[key] = json.RawMessage("0")
			default:
				root[key] = json.RawMessage("[]")
			}
		}
	}
	return json.Marshal(root)
}

func emptyR11DataJSON() []byte {
	return []byte(`{
		"calendar": [],
		"days": [],
		"employee_flights": [],
		"employees": [],
		"events": [],
		"flight_dates": [],
		"flight_routes": [],
		"graph_revisions": [],
		"graph_versions": [],
		"import_history": [],
		"journal": [],
		"manual_colors": [],
		"next_id": 1,
		"notes": [],
		"rvd": [],
		"schedule_changes": [],
		"schedule_types": [],
		"schema_version": 0
	}`)
}

func (h *controller) r11UpdateDays(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req r11DayChangesRequest
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "неверный пакет изменений: "+err.Error(), http.StatusBadRequest)
		return
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		http.Error(w, "после пакета изменений обнаружены лишние данные", http.StatusBadRequest)
		return
	}
	if len(req.Days) == 0 {
		http.Error(w, "пакет изменений пуст", http.StatusBadRequest)
		return
	}
	if len(req.Days) > 10000 {
		http.Error(w, "за один раз можно изменить не более 10000 дней", http.StatusRequestEntityTooLarge)
		return
	}
	requestKeys := make(map[string]bool, len(req.Days))
	for i, item := range req.Days {
		if item.EmployeeID < 1 {
			http.Error(w, fmt.Sprintf("days[%d]: employee_id должен быть положительным", i), http.StatusBadRequest)
			return
		}
		if parsed, err := time.Parse("2006-01-02", item.Date); err != nil || parsed.Format("2006-01-02") != item.Date {
			http.Error(w, fmt.Sprintf("days[%d]: дата должна иметь формат YYYY-MM-DD", i), http.StatusBadRequest)
			return
		}
		if len(item.Plan) > 32 || strings.ContainsAny(item.Plan, "\r\n\x00") {
			http.Error(w, fmt.Sprintf("days[%d]: недопустимое обозначение плана", i), http.StatusBadRequest)
			return
		}
		if item.PlanHours < 0 || item.PlanHours > 24 {
			http.Error(w, fmt.Sprintf("days[%d]: plan_hours должен быть от 0 до 24", i), http.StatusBadRequest)
			return
		}
		key := r11DayKey(item.EmployeeID, item.Date)
		if requestKeys[key] {
			http.Error(w, fmt.Sprintf("день %s указан в пакете более одного раза", key), http.StatusBadRequest)
			return
		}
		requestKeys[key] = true
	}

	h.opMu.Lock()
	defer h.opMu.Unlock()
	data, err := h.readR11DataLocked()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var employees []json.RawMessage
	if err := json.Unmarshal(root["employees"], &employees); err != nil {
		http.Error(w, "коллекция employees повреждена: "+err.Error(), http.StatusInternalServerError)
		return
	}
	knownEmployees := make(map[int64]bool, len(employees))
	for _, raw := range employees {
		var employee struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(raw, &employee); err != nil {
			http.Error(w, "запись employees повреждена: "+err.Error(), http.StatusInternalServerError)
			return
		}
		knownEmployees[employee.ID] = true
	}
	for _, item := range req.Days {
		if !knownEmployees[item.EmployeeID] {
			http.Error(w, fmt.Sprintf("сотрудник с employee_id=%d не найден", item.EmployeeID), http.StatusBadRequest)
			return
		}
	}

	var days []map[string]json.RawMessage
	if err := json.Unmarshal(root["days"], &days); err != nil {
		http.Error(w, "коллекция days повреждена: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if days == nil {
		days = []map[string]json.RawMessage{}
	}
	positions := make(map[string]int, len(days))
	for i, day := range days {
		var employeeID int64
		var date string
		if err := json.Unmarshal(day["employee_id"], &employeeID); err != nil {
			http.Error(w, fmt.Sprintf("days[%d].employee_id повреждён", i), http.StatusInternalServerError)
			return
		}
		if err := json.Unmarshal(day["date"], &date); err != nil {
			http.Error(w, fmt.Sprintf("days[%d].date повреждена", i), http.StatusInternalServerError)
			return
		}
		key := r11DayKey(employeeID, date)
		if _, exists := positions[key]; exists {
			http.Error(w, "в базе обнаружены повторяющиеся записи дня: "+key, http.StatusConflict)
			return
		}
		positions[key] = i
	}

	var nextID int64
	if err := json.Unmarshal(root["next_id"], &nextID); err != nil {
		http.Error(w, "next_id повреждён", http.StatusInternalServerError)
		return
	}
	if nextID < 1 {
		nextID = 1
	}
	created, updated := 0, 0
	employeeSet := map[int64]bool{}
	for _, item := range req.Days {
		key := r11DayKey(item.EmployeeID, item.Date)
		if pos, exists := positions[key]; exists {
			day := days[pos]
			day["plan"], _ = json.Marshal(item.Plan)
			day["plan_hours"], _ = json.Marshal(item.PlanHours)
			if item.State != nil {
				day["state"], _ = json.Marshal(*item.State)
			}
			updated++
		} else {
			day := map[string]json.RawMessage{}
			values := map[string]any{
				"id": nextID, "employee_id": item.EmployeeID, "date": item.Date,
				"plan": item.Plan, "plan_hours": item.PlanHours,
				"fact": "", "fact_hours": 0, "shift": "", "state": "", "comment": "",
			}
			for name, value := range values {
				day[name], _ = json.Marshal(value)
			}
			positions[key] = len(days)
			days = append(days, day)
			nextID++
			created++
		}
		employeeSet[item.EmployeeID] = true
	}

	sort.SliceStable(days, func(i, j int) bool {
		var leftID, rightID int64
		var leftDate, rightDate string
		_ = json.Unmarshal(days[i]["employee_id"], &leftID)
		_ = json.Unmarshal(days[j]["employee_id"], &rightID)
		_ = json.Unmarshal(days[i]["date"], &leftDate)
		_ = json.Unmarshal(days[j]["date"], &rightDate)
		if leftID != rightID {
			return leftID < rightID
		}
		return leftDate < rightDate
	})
	root["days"], _ = json.Marshal(days)
	if created > 0 {
		root["next_id"], _ = json.Marshal(nextID)
	}

	var journal []map[string]json.RawMessage
	if raw := root["journal"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &journal); err != nil {
			http.Error(w, "коллекция journal повреждена: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if journal == nil {
		journal = []map[string]json.RawMessage{}
	}
	entry := map[string]any{
		"time": time.Now().Format("02.01.2006 15:04:05"),
		"operation": "Изменение плановых дней графика (Linux)",
		"details": fmt.Sprintf("Изменено дней: %d; сотрудников: %d", len(req.Days), len(employeeSet)),
	}
	entryRaw, _ := json.Marshal(entry)
	var entryMap map[string]json.RawMessage
	_ = json.Unmarshal(entryRaw, &entryMap)
	journal = append(journal, entryMap)
	root["journal"], _ = json.Marshal(journal)

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	backup, err := h.backupR11StateLocked()
	if err != nil {
		http.Error(w, "не удалось создать резервную копию: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := atomicWrite(h.mainDBPath(), append(out, '\n')); err != nil {
		http.Error(w, "не удалось сохранить изменения: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"ok": true, "created": created, "updated": updated,
		"employees": len(employeeSet), "backup": r11BackupLabel(backup),
	})
}

func (h *controller) backupR11StateLocked() (string, error) {
	source := h.mainDBPath()
	if _, err := os.Stat(source); os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	body, err := os.ReadFile(source)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(h.baseDir, "backups")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	name := "labor_control_" + time.Now().Format("20060102_150405.000") + "_before_r11_change.json"
	path := filepath.Join(dir, name)
	if err := atomicWrite(path, body); err != nil {
		return "", err
	}
	return path, nil
}

func r11DayKey(employeeID int64, date string) string {
	return strconv.FormatInt(employeeID, 10) + "|" + date
}

func r11BackupLabel(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}
