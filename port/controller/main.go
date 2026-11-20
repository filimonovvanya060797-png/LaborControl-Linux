package main

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	controllerVersion = "v1.0-TIMESHEET-PREVIEW-R9"
	listenAddr        = "127.0.0.1:8779"
	coreFileName      = "LaborControl_Core_R9.exe"
)

//go:embed frontend.html
var frontendHTML []byte

type dayPatch struct {
	TabNo     string  `json:"tab_no"`
	Date      string  `json:"date"`
	Plan      string  `json:"plan"`
	PlanHours float64 `json:"plan_hours"`
	Fact      string  `json:"fact"`
	FactHours float64 `json:"fact_hours"`
	Shift     string  `json:"shift"`
	State     string  `json:"state"`
	Comment   string  `json:"comment"`
	Reason    string  `json:"reason"`
}

type stagePayload struct {
	Days      []dayPatch `json:"days,omitempty"`
	Rows      [][]any    `json:"d,omitempty"`
	Schedules [][]any    `json:"s,omitempty"`
}

type schedulePatch struct {
	TabNo         string
	NewSchedule   string
	EffectiveDate string
	Reason        string
	HoursBefore   float64
}

type controller struct {
	baseDir  string
	exeDir   string
	corePath string
	logf     *os.File

	opMu sync.Mutex

	coreMu   sync.Mutex
	coreCmd  *exec.Cmd
	coreDone chan error
	coreURL  string
	standalone bool
}

func main() {
	// Reuse only the same controller build. If an older preview is still
	// listening on 8767, stop it first so a newly launched ZIP cannot silently
	// fall back to yesterday's controller/frontend.
	if v, err := existingControllerVersion(); err == nil && v != "" {
		if v == controllerVersion || v == r11StateCompatVersion {
			if u, e := requestExistingStart(); e == nil && u != "" {
				if e = openURL(u); e != nil {
					showMessage("LaborControl", "Программа уже запущена, но не удалось открыть браузер: "+e.Error())
				}
				return
			}
		}
		_ = stopExistingController()
		_ = waitControllerDown(5 * time.Second)
	}

	// Previous preview builds could leave their legacy Core running on a random port.
	// Kill only that legacy child process before starting this build, otherwise the browser
	// can keep showing the old v0.9.11 page forever even though the new controller started.
	terminateStaleCoreProcesses()

	exe, err := os.Executable()
	if err != nil {
		showMessage("LaborControl", "Не удалось определить папку программы: "+err.Error())
		return
	}
	exeDir := filepath.Dir(exe)
	base, err := localDataDir()
	if err != nil {
		showMessage("LaborControl", "Не удалось определить папку данных: "+err.Error())
		return
	}
	if err := os.MkdirAll(base, 0755); err != nil {
		showMessage("LaborControl", "Не удалось открыть локальную папку данных: "+err.Error())
		return
	}
	lf, _ := os.OpenFile(filepath.Join(base, "laborcontrol_controller.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if lf != nil {
		log.SetOutput(lf)
	}

	h := &controller{
		baseDir:  base,
		exeDir:   exeDir,
		corePath: filepath.Join(exeDir, platformCoreName()),
		logf:     lf,
	}
	if _, err := os.Stat(h.corePath); err != nil {
		if platformStandaloneMode() {
			if runErr := h.runStandaloneR11(); runErr != nil {
				showMessage("LaborControl", runErr.Error())
			}
			return
		}
		showMessage("LaborControl", "Не найден внутренний компонент Core ("+filepath.Base(h.corePath)+"). Для полноценного запуска Windows-версии требуется восстановить Core.")
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.wrap(h.health))
	mux.HandleFunc("/start", h.wrap(h.startEndpoint))
	mux.HandleFunc("/stage", h.wrap(h.stage))
	mux.HandleFunc("/apply", h.wrap(h.apply))
	mux.HandleFunc("/core-shutdown", h.wrap(h.coreShutdownOnly))
	h.registerTab2(mux)
	mux.HandleFunc("/api/shutdown", h.wrap(h.shutdownAll))
	mux.HandleFunc("/api/", h.proxyAPI)
	mux.HandleFunc("/", h.serveFrontend)

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		// Race-safe fallback: another controller may have appeared after the first probe.
		if u, e := requestExistingStart(); e == nil && u != "" {
			_ = openURL(u)
			return
		}
		showMessage("LaborControl", "Не удалось запустить ускоритель: "+err.Error())
		return
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("controller server stopped: %v", err)
		}
	}()

	coreURL, err := h.startCore()
	if err != nil {
		_ = ln.Close()
		showMessage("LaborControl", "Не удалось запустить LaborControl: "+err.Error())
		return
	}
	// R9 suppresses only the legacy Core browser auto-open by placing a tiny
	// local shim/rundll32.exe first in PATH. The Core HTTP server still starts
	// normally, while the user sees only the controller UI below.
	uiURL := "http://" + listenAddr + "/?shell=timesheet-r9"
	log.Printf("%s started, core=%s, coreUI=%s, controllerUI=%s", controllerVersion, h.corePath, coreURL, uiURL)
	if err := openURL(uiURL); err != nil {
		showMessage("LaborControl", "Программа запущена, но браузер не открылся. Адрес: "+uiURL)
	}

	select {}
}

func existingControllerVersion() (string, error) {
	c := &http.Client{Timeout: 800 * time.Millisecond}
	r, err := c.Get("http://" + listenAddr + "/health")
	if err != nil {
		return "", err
	}
	defer r.Body.Close()
	if r.StatusCode/100 != 2 {
		return "", fmt.Errorf("health: %s", r.Status)
	}
	var v struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		return "", err
	}
	return strings.TrimSpace(v.Version), nil
}

func stopExistingController() error {
	c := &http.Client{Timeout: 1200 * time.Millisecond}
	req, _ := http.NewRequest(http.MethodPost, "http://"+listenAddr+"/api/shutdown", nil)
	r, err := c.Do(req)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	return nil
}

func waitControllerDown(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", listenAddr, 150*time.Millisecond)
		if err != nil {
			return nil
		}
		_ = c.Close()
		time.Sleep(120 * time.Millisecond)
	}
	return fmt.Errorf("старый контроллер не освободил %s", listenAddr)
}

func requestExistingStart() (string, error) {
	c := &http.Client{Timeout: 900 * time.Millisecond}
	req, _ := http.NewRequest(http.MethodGet, "http://"+listenAddr+"/start", nil)
	r, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer r.Body.Close()
	if r.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
		return "", fmt.Errorf("controller: %s", strings.TrimSpace(string(b)))
	}
	var v struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		return "", err
	}
	return v.URL, nil
}

func (h *controller) wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func (h *controller) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	version := controllerVersion
	if h.standalone {
		version = r11StateCompatVersion
	}
	writeJSON(w, map[string]any{"ok": true, "version": version})
}

func (h *controller) startEndpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "GET/POST only", http.StatusMethodNotAllowed)
		return
	}
	if _, err := h.startCore(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "url": "http://" + listenAddr + "/"})
}

func (h *controller) startCore() (string, error) {
	h.coreMu.Lock()
	if h.coreCmd != nil {
		cmd, done, u := h.coreCmd, h.coreDone, h.coreURL
		pid := cmd.Process.Pid
		h.coreMu.Unlock()
		select {
		case <-done:
			// Process ended between the check and here; start a fresh one below.
		default:
			if u != "" {
				return u, nil
			}
			u, err := discoverCoreURL(pid, 8*time.Second)
			if err != nil {
				return "", err
			}
			h.coreMu.Lock()
			if h.coreCmd == cmd {
				h.coreURL = u
			}
			h.coreMu.Unlock()
			return u, nil
		}
		h.coreMu.Lock()
		if h.coreCmd == cmd {
			h.coreCmd, h.coreDone, h.coreURL = nil, nil, ""
		}
	}

	cmd := exec.Command(h.corePath)
	cmd.Dir = h.exeDir
	cmd.Env = coreEnvironment(os.Environ())
	configureHidden(cmd)
	if h.logf != nil {
		cmd.Stdout = h.logf
		cmd.Stderr = h.logf
	}
	if err := cmd.Start(); err != nil {
		h.coreMu.Unlock()
		return "", err
	}
	done := make(chan error, 1)
	h.coreCmd = cmd
	h.coreDone = done
	h.coreURL = ""
	pid := cmd.Process.Pid
	h.coreMu.Unlock()

	go func(c *exec.Cmd, d chan error) {
		err := c.Wait()
		d <- err
		close(d)
		h.coreMu.Lock()
		if h.coreCmd == c {
			h.coreCmd, h.coreDone, h.coreURL = nil, nil, ""
		}
		h.coreMu.Unlock()
		log.Printf("core pid=%d exited: %v", c.Process.Pid, err)
	}(cmd, done)

	u, err := discoverCoreURL(pid, 10*time.Second)
	if err != nil {
		_ = cmd.Process.Kill()
		return "", err
	}
	h.coreMu.Lock()
	if h.coreCmd == cmd {
		h.coreURL = u
	}
	h.coreMu.Unlock()
	log.Printf("core pid=%d url=%s", pid, u)
	return u, nil
}

func (h *controller) requestCoreShutdown() {
	h.coreMu.Lock()
	u := h.coreURL
	h.coreMu.Unlock()
	if u != "" {
		c := &http.Client{Timeout: 1200 * time.Millisecond}
		req, _ := http.NewRequest(http.MethodPost, strings.TrimRight(u, "/")+"/api/shutdown", nil)
		if resp, err := c.Do(req); err == nil && resp != nil {
			_ = resp.Body.Close()
		}
	}
	h.stopCore(1800 * time.Millisecond)
}

func (h *controller) coreShutdownOnly(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	h.requestCoreShutdown()
	writeJSON(w, map[string]any{"ok": true})
}

func (h *controller) shutdownAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	h.requestCoreShutdown()
	writeJSON(w, map[string]any{"ok": true})
	go func() {
		time.Sleep(250 * time.Millisecond)
		if h.logf != nil {
			_ = h.logf.Close()
		}
		os.Exit(0)
	}()
}

func (h *controller) stopCore(wait time.Duration) {
	h.coreMu.Lock()
	cmd, done := h.coreCmd, h.coreDone
	h.coreMu.Unlock()
	if cmd == nil || done == nil {
		return
	}
	select {
	case <-done:
		return
	case <-time.After(wait):
	}
	log.Printf("core pid=%d did not stop in time; terminating", cmd.Process.Pid)
	_ = cmd.Process.Kill()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

func discoverCoreURL(pid int, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		if u, err := findCoreURL(pid); err == nil && u != "" {
			return u, nil
		} else if err != nil {
			last = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	if last == nil { last = fmt.Errorf("порт процесса не найден") }
	return "", fmt.Errorf("не удалось определить локальный адрес LaborControl (PID %d): %w", pid, last)
}

func parseNetstatForPID(s string, pid int) string {
	want := strconv.Itoa(pid)
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		f := strings.Fields(scanner.Text())
		if len(f) < 4 || !strings.EqualFold(f[0], "TCP") || f[len(f)-1] != want {
			continue
		}
		local, remote := f[1], f[2]
		if !strings.HasPrefix(local, "127.0.0.1:") {
			continue
		}
		// Listening rows use an all-zero remote endpoint. This avoids choosing an established client connection.
		if remote != "0.0.0.0:0" && remote != "[::]:0" && remote != "*:0" && remote != "*:*" {
			continue
		}
		i := strings.LastIndexByte(local, ':')
		if i < 0 || i+1 >= len(local) {
			continue
		}
		p, err := strconv.Atoi(local[i+1:])
		if err != nil || p <= 0 || p > 65535 {
			continue
		}
		return fmt.Sprintf("http://127.0.0.1:%d/", p)
	}
	return ""
}

func (h *controller) stage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	var p stagePayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := normalizeStage(&p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(p.Days) == 0 {
		http.Error(w, "пустой пакет дней", http.StatusBadRequest)
		return
	}
	if len(p.Days) > 100000 {
		http.Error(w, "слишком большой пакет", http.StatusBadRequest)
		return
	}
	for i := range p.Days {
		p.Days[i].TabNo = strings.TrimSpace(p.Days[i].TabNo)
		p.Days[i].Date = strings.TrimSpace(p.Days[i].Date)
		if p.Days[i].TabNo == "" || p.Days[i].Date == "" {
			http.Error(w, fmt.Sprintf("строка %d: нет табельного номера или даты", i+1), http.StatusBadRequest)
			return
		}
	}
	b, err := json.Marshal(p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.opMu.Lock()
	defer h.opMu.Unlock()
	if err := atomicWrite(filepath.Join(h.baseDir, "bulk_staged.json"), b); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "days": len(p.Days)})
}

func (h *controller) apply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	h.opMu.Lock()
	defer h.opMu.Unlock()

	// The page has already asked the core to shut down. Wait briefly, then terminate it if necessary.
	h.stopCore(1800 * time.Millisecond)

	staged := filepath.Join(h.baseDir, "bulk_staged.json")
	sb, err := os.ReadFile(staged)
	if err != nil {
		u, _ := h.startCore()
		_ = u
		http.Error(w, "нет подготовленного пакета: "+err.Error(), http.StatusBadRequest)
		return
	}
	var p stagePayload
	if err := json.Unmarshal(sb, &p); err != nil {
		_, _ = h.startCore()
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	db := filepath.Join(h.baseDir, "labor_control.json")
	result, err := applyDays(db, filepath.Join(h.baseDir, "backups"), p)
	if err != nil {
		log.Printf("apply error: %v", err)
		_, _ = h.startCore()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = os.Remove(staged)

	u, err := h.startCore()
	if err != nil {
		http.Error(w, "график сохранён, но не удалось перезапустить LaborControl: "+err.Error(), http.StatusInternalServerError)
		return
	}
	_ = u
	result["url"] = "http://" + listenAddr + "/"
	writeJSON(w, result)
}

func applyDays(dbPath, backupDir string, p stagePayload) (map[string]any, error) {
	b, err := os.ReadFile(dbPath)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть базу %s: %w", dbPath, err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(b, &root); err != nil {
		return nil, fmt.Errorf("ошибка JSON базы: %w", err)
	}

	var employees []map[string]any
	var days []map[string]any
	var journal []map[string]any
	if err := json.Unmarshal(root["employees"], &employees); err != nil {
		return nil, fmt.Errorf("employees: %w", err)
	}
	if raw := root["days"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &days); err != nil {
			return nil, fmt.Errorf("days: %w", err)
		}
	}
	if raw := root["journal"]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &journal)
	}

	nextID := 1
	if raw := root["next_id"]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &nextID)
	}
	if nextID < 1 {
		nextID = 1
	}
	alloc := func() int { id := nextID; nextID++; return id }

	empByTab := map[string]int{}
	for _, e := range employees {
		tab := strings.TrimSpace(toString(e["tab_no"]))
		id := toInt(e["id"])
		if tab != "" && id != 0 {
			empByTab[tab] = id
		}
	}

	// IMPORTANT UX6 RULE:
	// AutoGraph may update the employee's current schedule type, but it must not create
	// graph_revisions or schedule_changes. Official revisions are created only by the technician.
	schedulePatches, err := parseSchedulePatches(p.Schedules)
	if err != nil {
		return nil, err
	}
	scheduleUpdated := map[string]string{}
	for _, sp := range schedulePatches {
		eid := empByTab[sp.TabNo]
		if eid == 0 {
			return nil, fmt.Errorf("сотрудник %s не найден в базе", sp.TabNo)
		}
		for _, e := range employees {
			if toInt(e["id"]) != eid {
				continue
			}
			old := toString(e["schedule_type"])
			if old != sp.NewSchedule {
				e["schedule_type"] = sp.NewSchedule
				scheduleUpdated[sp.TabNo] = old + " → " + sp.NewSchedule
			}
			break
		}
	}

	idx := make(map[string]int, len(days))
	for i, d := range days {
		key := strconv.Itoa(toInt(d["employee_id"])) + "|" + toString(d["date"])
		idx[key] = i
	}

	perEmp := map[string]int{}
	created, updated := 0, 0
	for _, q := range p.Days {
		eid := empByTab[q.TabNo]
		if eid == 0 {
			return nil, fmt.Errorf("сотрудник %s не найден в базе", q.TabNo)
		}
		key := strconv.Itoa(eid) + "|" + q.Date
		if pos, ok := idx[key]; ok {
			d := days[pos]
			d["plan"] = q.Plan
			d["plan_hours"] = q.PlanHours
			d["state"] = q.State
			// Facts, shifts and user comments are deliberately preserved.
			updated++
		} else {
			d := map[string]any{
				"id": alloc(), "employee_id": eid, "date": q.Date,
				"plan": q.Plan, "plan_hours": q.PlanHours,
				"fact": q.Fact, "fact_hours": q.FactHours,
				"shift": q.Shift, "state": q.State, "comment": q.Comment,
			}
			days = append(days, d)
			idx[key] = len(days) - 1
			created++
		}
		perEmp[q.TabNo]++
	}

	sort.SliceStable(days, func(i, j int) bool {
		ei, ej := toInt(days[i]["employee_id"]), toInt(days[j]["employee_id"])
		if ei != ej {
			return ei < ej
		}
		return toString(days[i]["date"]) < toString(days[j]["date"])
	})

	tabs := make([]string, 0, len(perEmp))
	for tab := range perEmp {
		tabs = append(tabs, tab)
	}
	sort.Strings(tabs)
	now := time.Now().Format("02.01.2006 15:04:05")
	for _, tab := range tabs {
		details := fmt.Sprintf("%s; записано дней: %d", tab, perEmp[tab])
		if s := scheduleUpdated[tab]; s != "" {
			details += "; вид графика: " + s
		}
		journal = append(journal, map[string]any{
			"id": alloc(), "time": now,
			"operation": "АвтоГраф FAST: построение без создания редакции",
			"details":   details,
		})
	}

	dbak, err := makeBackup(dbPath, backupDir)
	if err != nil {
		return nil, fmt.Errorf("не удалось создать резервную копию: %w", err)
	}

	employeesJSON, _ := json.Marshal(employees)
	daysJSON, _ := json.Marshal(days)
	journalJSON, _ := json.Marshal(journal)
	nextJSON, _ := json.Marshal(nextID)
	root["employees"] = employeesJSON
	root["days"] = daysJSON
	root["journal"] = journalJSON
	root["next_id"] = nextJSON
	// graph_revisions and schedule_changes intentionally remain untouched in root.

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, err
	}
	out = append(out, '\n')
	if err := atomicWrite(dbPath, out); err != nil {
		return nil, err
	}
	return map[string]any{
		"ok": true, "employees": len(perEmp), "days": len(p.Days),
		"created": created, "updated": updated, "schedule_updated": len(scheduleUpdated),
		"backup": dbak,
	}, nil
}

func normalizeStage(p *stagePayload) error {
	if len(p.Rows) > 0 {
		p.Days = make([]dayPatch, 0, len(p.Rows))
		for i, r := range p.Rows {
			if len(r) < 5 {
				return fmt.Errorf("строка d[%d]: ожидалось 5 значений", i)
			}
			p.Days = append(p.Days, dayPatch{
				TabNo: toString(r[0]), Date: toString(r[1]), Plan: toString(r[2]),
				PlanHours: toFloat(r[3]), State: toString(r[4]),
			})
		}
	}
	return nil
}

func parseSchedulePatches(rows [][]any) ([]schedulePatch, error) {
	out := make([]schedulePatch, 0, len(rows))
	for i, r := range rows {
		if len(r) < 5 {
			return nil, fmt.Errorf("строка s[%d]: ожидалось 5 значений", i)
		}
		out = append(out, schedulePatch{
			TabNo: toString(r[0]), NewSchedule: toString(r[1]), EffectiveDate: toString(r[2]),
			Reason: toString(r[3]), HoursBefore: toFloat(r[4]),
		})
	}
	return out, nil
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.ReplaceAll(x, ",", "."), 64)
		return f
	default:
		return 0
	}
}

func makeBackup(dbPath, backupDir string) (string, error) {
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", err
	}
	src, err := os.Open(dbPath)
	if err != nil {
		return "", err
	}
	defer src.Close()
	name := "labor_control_" + time.Now().Format("20060102_150405.000") + "_ux6_fast.json"
	dstPath := filepath.Join(backupDir, name)
	dst, err := os.Create(dstPath)
	if err != nil {
		return "", err
	}
	_, cpErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if cpErr != nil {
		return "", cpErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return dstPath, nil
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp := path + ".ux6.tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err == nil {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(tmp, path)
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func toInt(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case json.Number:
		n, _ := strconv.Atoi(x.String())
		return n
	case string:
		n, _ := strconv.Atoi(x)
		return n
	default:
		return 0
	}
}

func (h *controller) serveFrontend(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(frontendHTML)
}

func (h *controller) proxyAPI(w http.ResponseWriter, r *http.Request) {
	coreURL, err := h.startCore()
	if err != nil {
		http.Error(w, "внутренний LaborControl недоступен: "+err.Error(), http.StatusBadGateway)
		return
	}
	target, err := url.Parse(coreURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	p := httputil.NewSingleHostReverseProxy(target)
	orig := p.Director
	p.Director = func(req *http.Request) {
		orig(req)
		req.Host = target.Host
	}
	p.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, e error) {
		http.Error(rw, "внутренний LaborControl: "+e.Error(), http.StatusBadGateway)
	}
	p.ServeHTTP(w, r)
}