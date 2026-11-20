package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	listenAddress = "127.0.0.1:8788"
	employeeCount = 450
	pageSize      = 25
)

//go:embed index.html
var pageHTML []byte

type app struct {
	mu        sync.Mutex
	dataFile  string
	overrides map[string]string
}

type employee struct {
	TabNo  string            `json:"tab_no"`
	Name   string            `json:"name"`
	Column int               `json:"column"`
	Days   map[string]string `json:"days"`
}

func newApp(dataFile string) (*app, error) {
	a := &app{dataFile: dataFile, overrides: map[string]string{}}
	data, err := os.ReadFile(dataFile)
	if os.IsNotExist(err) {
		return a, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &a.overrides); err != nil {
		return nil, fmt.Errorf("демонстрационные данные повреждены: %w", err)
	}
	if a.overrides == nil {
		a.overrides = map[string]string{}
	}
	return a, nil
}

func defaultDataFile() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	// Deliberately separate from labor_control.json and labor_control_tab2.json.
	return filepath.Join(base, "LaborControl", "demo-schedule-overrides.json"), nil
}

func main() {
	path, err := defaultDataFile()
	if err != nil {
		log.Fatal(err)
	}
	a, err := newApp(path)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Демо графиков: http://%s (450 вымышленных водителей; реальные базы не открываются)", listenAddress)
	log.Fatal(http.ListenAndServe(listenAddress, a.handler()))
}

func (a *app) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if (r.URL.Path != "/" && r.URL.Path != "/index.html") || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(pageHTML)
	})
	mux.HandleFunc("/api/roster", a.roster)
	mux.HandleFunc("/api/plan", a.plan)
	return mux
}

func replyJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func parseMonth(s string) (time.Time, error) {
	t, err := time.Parse("2006-01", s)
	if err != nil || t.Year() < 2000 || t.Year() > 2100 {
		return time.Time{}, fmt.Errorf("месяц должен иметь вид ГГГГ-ММ (2000–2100)")
	}
	return t, nil
}

func (a *app) roster(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	month := r.URL.Query().Get("month")
	first, err := parseMonth(month)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	column := r.URL.Query().Get("column")
	if column != "1" && column != "2" && column != "all" {
		http.Error(w, "колонна: 1, 2 или all", http.StatusBadRequest)
		return
	}
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		http.Error(w, "страница должна быть положительным числом", http.StatusBadRequest)
		return
	}
	filter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	count := 0
	employees := []employee{}
	lastDay := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()

	a.mu.Lock()
	defer a.mu.Unlock()
	for i := 1; i <= employeeCount; i++ {
		c := 1
		if i > employeeCount/2 {
			c = 2
		}
		if column != "all" && column != strconv.Itoa(c) {
			continue
		}
		tab := fmt.Sprintf("D%04d", i)
		name := fmt.Sprintf("Водитель %03d", i)
		if filter != "" && !strings.Contains(strings.ToLower(name), filter) && !strings.Contains(strings.ToLower(tab), filter) {
			continue
		}
		count++
		if count <= (page-1)*pageSize || len(employees) >= pageSize {
			continue
		}
		e := employee{TabNo: tab, Name: name, Column: c, Days: map[string]string{}}
		for d := 1; d <= lastDay; d++ {
			dt := time.Date(first.Year(), first.Month(), d, 0, 0, 0, 0, time.UTC)
			date := dt.Format("2006-01-02")
			// Only the first ten employees have clearly synthetic illustrative values.
			// This is NOT a reproduction of the AutoGraph calculation algorithm.
			v := ""
			if i <= 10 {
				if dt.Weekday() == time.Saturday || dt.Weekday() == time.Sunday {
					v = "В"
				} else {
					v = "8"
				}
			}
			if override, ok := a.overrides[tab+"|"+date]; ok {
				v = override
			}
			e.Days[date] = v
		}
		employees = append(employees, e)
	}
	pages := (count + pageSize - 1) / pageSize
	replyJSON(w, map[string]any{
		"month": month, "column": column, "total": count,
		"page": page, "pages": pages, "days_in_month": lastDay,
		"employees": employees, "demo": true,
	})
}

func (a *app) plan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	var p struct {
		TabNo string `json:"tab_no"`
		Date  string `json:"date"`
		Plan  string `json:"plan"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "некорректный запрос", http.StatusBadRequest)
		return
	}
	n, err := strconv.Atoi(strings.TrimPrefix(p.TabNo, "D"))
	if err != nil || p.TabNo != fmt.Sprintf("D%04d", n) || n < 1 || n > employeeCount {
		http.Error(w, "неизвестный демонстрационный сотрудник", http.StatusBadRequest)
		return
	}
	t, err := time.Parse("2006-01-02", p.Date)
	if err != nil || t.Format("2006-01-02") != p.Date || t.Year() < 2000 || t.Year() > 2100 {
		http.Error(w, "неверная дата", http.StatusBadRequest)
		return
	}
	// Do not invent or impose R11 business rules on currently unknown codes.
	if utf8.RuneCountInString(p.Plan) > 20 || strings.ContainsAny(p.Plan, "\r\n\x00") {
		http.Error(w, "значение плана слишком длинное или недопустимое", http.StatusBadRequest)
		return
	}
	key := p.TabNo + "|" + p.Date
	a.mu.Lock()
	defer a.mu.Unlock()
	previous, hadPrevious := a.overrides[key]
	a.overrides[key] = p.Plan
	if err := a.save(); err != nil {
		if hadPrevious {
			a.overrides[key] = previous
		} else {
			delete(a.overrides, key)
		}
		http.Error(w, "не удалось сохранить демонстрационный план: "+err.Error(), http.StatusInternalServerError)
		return
	}
	replyJSON(w, map[string]any{"ok": true, "demo": true})
}

func (a *app) save() error {
	dir := filepath.Dir(a.dataFile)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "demo-schedules-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		return err
	}
	if err := json.NewEncoder(f).Encode(a.overrides); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), a.dataFile)
}
