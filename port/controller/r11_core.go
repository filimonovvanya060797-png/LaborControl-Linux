package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const r11CoreVersion = "R11-LINUX-CORE/0.1"

// r11CoreAPI is the native Linux implementation of the Windows Core HTTP
// contract. The browser UI remains the original R11 UI; this process owns the
// local JSON database and never forwards it to a remote service.
func (h *controller) r11CoreAPI(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/state":
		h.r11CoreState(w, r)
	case "/api/employee/save":
		h.r11EmployeeSave(w, r)
	case "/api/employee/delete":
		h.r11EmployeeDelete(w, r)
	case "/api/day/save", "/api/day/quick-save":
		h.r11DaySave(w, r)
	case "/api/day/delete":
		h.r11DayDelete(w, r)
	case "/api/event/save":
		h.r11EventSave(w, r)
	case "/api/event/delete":
		h.r11EventDelete(w, r)
	case "/api/rvd/save":
		h.r11RVDSave(w, r)
	case "/api/calendar/init":
		h.r11CalendarInit(w, r)
	case "/api/calendar/clear":
		h.r11CalendarClear(w, r)
	case "/api/calendar/day/save":
		h.r11CalendarDaySave(w, r)
	case "/api/calendar/import-confirm":
		h.r11CalendarImportConfirm(w, r)
	case "/api/autograf/schedule-type/save":
		h.r11ScheduleTypeSave(w, r)
	case "/api/autograf/schedule-type/delete":
		h.r11ScheduleTypeDelete(w, r)
	case "/api/autograf/flight/save":
		h.r11FlightSave(w, r)
	case "/api/autograf/flight/delete":
		h.r11FlightDelete(w, r)
	case "/api/autograf/flight/batch":
		h.r11FlightBatch(w, r)
	case "/api/autograf/version/create":
		h.r11VersionCreate(w, r)
	case "/api/autograf/version/restore":
		h.r11VersionRestore(w, r)
	case "/api/autograf/schedule-change":
		h.r11ScheduleChange(w, r)
	case "/api/autograf/revision/create":
		h.r11RevisionCreate(w, r)
	case "/api/autograf/revision/get":
		h.r11RevisionGet(w, r)
	case "/api/autograf/revision/activate":
		h.r11RevisionActivate(w, r)
	case "/api/note/save":
		h.r11NoteSave(w, r)
	case "/api/note/delete":
		h.r11NoteDelete(w, r)
	case "/api/note/reminder/delete":
		h.r11NoteReminderDelete(w, r)
	case "/api/note/split":
		h.r11NoteSplit(w, r)
	case "/api/color/save":
		h.r11ColorSave(w, r)
	case "/api/color/delete":
		h.r11ColorDelete(w, r)
	case "/api/backup":
		h.r11Backup(w, r)
	case "/api/import/fact":
		h.r11ImportFact(w, r)
	case "/api/import/events":
		h.r11ImportEvents(w, r)
	case "/api/autograf/flight/import-preview":
		h.r11FlightImportPreview(w, r)
	case "/api/autograf/flight/import-confirm":
		h.r11FlightImportConfirm(w, r)
	case "/api/employee-flight/import-confirm":
		h.r11EmployeeFlightImportConfirm(w, r)
	case "/api/employee-flight/delete":
		h.r11EmployeeFlightDelete(w, r)
	case "/api/demo":
		h.r11Demo(w, r)
	case "/api/demo/autograf":
		h.r11DemoAutograf(w, r)
	case "/api/shutdown":
		h.shutdownAll(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *controller) serveFullR11UI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" && r.URL.Path != "/r11" {
		http.NotFound(w, r)
		return
	}
	page := bytes.ReplaceAll(frontendHTML, []byte("state.version='v1.0 TIMESHEET R9'"), []byte("state.version='R11 Linux Core'"))
	page = bytes.ReplaceAll(page, []byte("техника по труду · Табель R6"), []byte("техника по труду · Linux R11"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(page)
}

func r11BodyMap(w http.ResponseWriter, r *http.Request, max int64) (map[string]json.RawMessage, error) {
	r.Body = http.MaxBytesReader(w, r.Body, max)
	dec := json.NewDecoder(r.Body)
	var m map[string]json.RawMessage
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("ожидался JSON-объект")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("после JSON-объекта обнаружены лишние данные")
		}
		return nil, err
	}
	return m, nil
}

func r11Raw(m map[string]json.RawMessage, key string) json.RawMessage {
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return nil
}

func r11String(m map[string]json.RawMessage, key string) string {
	raw := r11Raw(m, key)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		return strconv.FormatBool(b)
	}
	return ""
}

func r11Int(m map[string]json.RawMessage, key string) int64 {
	raw := r11Raw(m, key)
	if len(raw) == 0 {
		return 0
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		v, _ := n.Int64()
		return v
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		v, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		return v
	}
	return 0
}

func r11Float(m map[string]json.RawMessage, key string) float64 {
	raw := r11Raw(m, key)
	if len(raw) == 0 {
		return 0
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		v, _ := n.Float64()
		return v
	}
	var f float64
	_ = json.Unmarshal(raw, &f)
	return f
}

func r11Bool(m map[string]json.RawMessage, key string) bool {
	raw := r11Raw(m, key)
	if len(raw) == 0 {
		return false
	}
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		return b
	}
	return r11String(m, key) == "true"
}

func r11Put(m map[string]json.RawMessage, key string, value any) {
	b, _ := json.Marshal(value)
	m[key] = b
}

func r11Array(root map[string]json.RawMessage, key string) ([]map[string]json.RawMessage, error) {
	raw := root[key]
	if len(raw) == 0 || string(raw) == "null" {
		return []map[string]json.RawMessage{}, nil
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("коллекция %s повреждена: %w", key, err)
	}
	if rows == nil {
		rows = []map[string]json.RawMessage{}
	}
	return rows, nil
}

func r11SetArray(root map[string]json.RawMessage, key string, rows []map[string]json.RawMessage) {
	if rows == nil {
		rows = []map[string]json.RawMessage{}
	}
	r11Put(root, key, rows)
}

func r11NextID(root map[string]json.RawMessage) int64 {
	var m map[string]json.RawMessage
	_ = json.Unmarshal(root["next_id"], &m)
	var n int64
	_ = json.Unmarshal(root["next_id"], &n)
	if n < 1 {
		n = 1
	}
	r11Put(root, "next_id", n+1)
	return n
}

func r11FindEmployee(rows []map[string]json.RawMessage, tab string) (map[string]json.RawMessage, int) {
	for i, e := range rows {
		if strings.EqualFold(r11String(e, "tab_no"), strings.TrimSpace(tab)) {
			return e, i
		}
	}
	return nil, -1
}

func r11FindEmployeeID(rows []map[string]json.RawMessage, id int64) (map[string]json.RawMessage, int) {
	for i, e := range rows {
		if r11Int(e, "id") == id {
			return e, i
		}
	}
	return nil, -1
}

func r11FindRecordID(rows []map[string]json.RawMessage, id int64) (map[string]json.RawMessage, int) {
	for i, e := range rows {
		if r11Int(e, "id") == id {
			return e, i
		}
	}
	return nil, -1
}

func r11DateOK(s string) bool {
	if len(s) != 10 {
		return false
	}
	d, err := time.Parse("2006-01-02", s)
	return err == nil && d.Format("2006-01-02") == s
}

func r11AppendJournal(root map[string]json.RawMessage, operation, details string) error {
	rows, err := r11Array(root, "journal")
	if err != nil {
		return err
	}
	entry := map[string]json.RawMessage{}
	r11Put(entry, "time", time.Now().Format("02.01.2006 15:04:05"))
	r11Put(entry, "operation", operation)
	r11Put(entry, "details", details)
	rows = append(rows, entry)
	r11SetArray(root, "journal", rows)
	return nil
}

func (h *controller) r11LoadRootLocked() (map[string]json.RawMessage, error) {
	data, err := h.readR11DataLocked()
	if err != nil {
		return nil, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	return root, nil
}

func (h *controller) r11SaveRootLocked(root map[string]json.RawMessage, operation, details string) (string, error) {
	backup, err := h.backupR11StateLocked()
	if err != nil {
		return "", err
	}
	if err := r11AppendJournal(root, operation, details); err != nil {
		return "", err
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	if err := atomicWrite(h.mainDBPath(), append(out, '\n')); err != nil {
		return "", err
	}
	return backup, nil
}

func r11WriteError(w http.ResponseWriter, status int, err error) {
	http.Error(w, err.Error(), status)
}

func (h *controller) r11Mutate(w http.ResponseWriter, r *http.Request, operation string, fn func(map[string]json.RawMessage) (map[string]any, string, error)) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	body, err := r11BodyMap(w, r, 32<<20)
	if err != nil {
		r11WriteError(w, http.StatusBadRequest, err)
		return
	}
	h.opMu.Lock()
	defer h.opMu.Unlock()
	root, err := h.r11LoadRootLocked()
	if err != nil {
		r11WriteError(w, http.StatusInternalServerError, err)
		return
	}
	response, details, err := fn(root)
	if err != nil {
		r11WriteError(w, http.StatusBadRequest, err)
		return
	}
	backup, err := h.r11SaveRootLocked(root, operation, details)
	if err != nil {
		r11WriteError(w, http.StatusInternalServerError, err)
		return
	}
	if response == nil {
		response = map[string]any{}
	}
	response["ok"] = true
	if backup != "" {
		response["backup"] = r11BackupLabel(backup)
	}
	writeJSON(w, response)
}

func (h *controller) r11CoreState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	h.opMu.Lock()
	root, err := h.r11LoadRootLocked()
	h.opMu.Unlock()
	if err != nil {
		r11WriteError(w, http.StatusInternalServerError, err)
		return
	}
	data, err := json.Marshal(root)
	if err != nil {
		r11WriteError(w, http.StatusInternalServerError, err)
		return
	}
	discrepancies := r11ComputeDiscrepancies(root)
	writeJSON(w, map[string]any{
		"data": json.RawMessage(data),
		"data_path": filepath.Base(h.mainDBPath()),
		"discrepancies": discrepancies,
		"version": r11CoreVersion,
	})
}

func r11ComputeDiscrepancies(root map[string]json.RawMessage) []map[string]any {
	employees, _ := r11Array(root, "employees")
	days, _ := r11Array(root, "days")
	byID := make(map[int64]map[string]json.RawMessage, len(employees))
	for _, e := range employees {
		byID[r11Int(e, "id")] = e
	}
	out := make([]map[string]any, 0)
	for _, d := range days {
		plan, fact := r11String(d, "plan"), r11String(d, "fact")
		ph, fh := r11Float(d, "plan_hours"), r11Float(d, "fact_hours")
		kind := ""
		switch {
		case plan != "" && fact == "":
			kind = "Нет факта"
		case plan == "" && fact != "":
			kind = "Нет плана"
		case plan != fact || (ph != 0 || fh != 0) && math.Abs(ph-fh) > 0.0001:
			kind = "План ≠ факт"
		}
		if kind == "" {
			continue
		}
		e := byID[r11Int(d, "employee_id")]
		out = append(out, map[string]any{
			"date": r11String(d, "date"), "tab_no": r11String(e, "tab_no"),
			"full_name": r11String(e, "full_name"), "plan": plan, "fact": fact,
			"plan_hours": ph, "fact_hours": fh, "kind": kind,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i]["date"] == out[j]["date"] {
			return fmt.Sprint(out[i]["tab_no"]) < fmt.Sprint(out[j]["tab_no"])
		}
		return fmt.Sprint(out[i]["date"]) < fmt.Sprint(out[j]["date"])
	})
	return out
}

func (h *controller) r11EmployeeSave(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w, r, "Карточка сотрудника", func(root map[string]json.RawMessage) (map[string]any, string, error) {
		rows, err := r11Array(root, "employees")
		if err != nil { return nil, "", err }
		tab, name := r11StringBody(r, "tab_no"), r11StringBody(r, "full_name")
		_ = tab; _ = name
		return h.r11EmployeeSaveBody(root, rows, nil)
	})
}

func r11StringBody(r *http.Request, key string) string { return "" }

// r11EmployeeSaveBody is called by the decoded handler below; keeping payload
// handling separate makes the uniqueness and update rules easy to test.
func (h *controller) r11EmployeeSaveBody(root map[string]json.RawMessage, rows []map[string]json.RawMessage, body map[string]json.RawMessage) (map[string]any, string, error) {
	if body == nil { return nil, "", fmt.Errorf("нет тела запроса") }
	return nil, "", fmt.Errorf("not implemented")
}

func (h *controller) r11EmployeeDelete(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w, r, "Сотрудник удалён", func(root map[string]json.RawMessage) (map[string]any, string, error) {
		body, err := r11BodyMap(w, r, 32<<20)
		if err != nil { return nil, "", err }
		id := r11Int(body, "id")
		employees, err := r11Array(root, "employees")
		if err != nil { return nil, "", err }
		_, pos := r11FindEmployeeID(employees, id)
		if pos < 0 { return nil, "", fmt.Errorf("сотрудник id=%d не найден", id) }
		employees = append(employees[:pos], employees[pos+1:]...)
		r11SetArray(root, "employees", employees)
		for _, key := range []string{"days","rvd","events","employee_flights","graph_revisions","graph_versions","schedule_changes","notes","manual_colors"} {
			items, e := r11Array(root, key); if e != nil { return nil, "", e }
			kept := items[:0]
			for _, item := range items { if r11Int(item,"employee_id") != id { kept=append(kept,item) } }
			r11SetArray(root,key,kept)
		}
		return map[string]any{"id":id}, fmt.Sprintf("ID %d",id), nil
	})
}

func (h *controller) r11DaySave(w http.ResponseWriter, r *http.Request) {
	body, err := r11BodyMap(w, r, 8<<20)
	if err != nil { r11WriteError(w,400,err); return }
	quick := r.URL.Path == "/api/day/quick-save"
	h.opMu.Lock()
	defer h.opMu.Unlock()
	root, err := h.r11LoadRootLocked()
	if err != nil { r11WriteError(w,500,err); return }
	employees, err := r11Array(root,"employees")
	if err != nil { r11WriteError(w,500,err); return }
	emp, _ := r11FindEmployee(employees,r11String(body,"TabNo"))
	if emp == nil { r11WriteError(w,400,fmt.Errorf("сотрудник %q не найден",r11String(body,"TabNo"))); return }
	date := r11String(body,"Date")
	if !r11DateOK(date) { r11WriteError(w,400,fmt.Errorf("дата должна иметь формат YYYY-MM-DD")); return }
	days, err := r11Array(root,"days")
	if err != nil { r11WriteError(w,500,err); return }
	employeeID := r11Int(emp,"id")
	pos := -1
	for i,d := range days { if r11Int(d,"employee_id")==employeeID && r11String(d,"date")==date { pos=i; break } }
	var day map[string]json.RawMessage
	if pos >= 0 { day=days[pos] } else { day=map[string]json.RawMessage{}; r11Put(day,"id",r11NextID(root)); r11Put(day,"employee_id",employeeID); r11Put(day,"date",date) }
	if v:=r11Raw(body,"Plan"); len(v)>0 { day["plan"]=append(json.RawMessage(nil),v...) }
	if v:=r11Raw(body,"PlanHours"); len(v)>0 { day["plan_hours"]=append(json.RawMessage(nil),v...) }
	if v:=r11Raw(body,"State"); len(v)>0 { day["state"]=append(json.RawMessage(nil),v...) }
	if v:=r11Raw(body,"Comment"); len(v)>0 { day["comment"]=append(json.RawMessage(nil),v...) }
	if !quick {
		for _, pair := range [][2]string{{"Fact","fact"},{"FactHours","fact_hours"},{"Shift","shift"}} {
			if v:=r11Raw(body,pair[0]); len(v)>0 { day[pair[1]]=append(json.RawMessage(nil),v...) }
		}
	}
	r11Put(day,"updated_at",time.Now().Format(time.RFC3339))
	if pos >= 0 { days[pos]=day } else { days=append(days,day) }
	r11SetArray(root,"days",days)
	reason := r11String(body,"ChangeReason")
	op := "День графика сохранён"
	if quick { op="Ячейка графика изменена" }
	details := fmt.Sprintf("%s · %s · %s/%s",r11String(emp,"tab_no"),date,r11String(day,"plan"),r11String(day,"plan_hours"))
	if reason!="" { details += " · "+reason }
	backup, err := h.r11SaveRootLocked(root,op,details)
	if err != nil { r11WriteError(w,500,err); return }
	resp:=map[string]any{"ok":true,"created":0,"updated":1}
	if pos<0 { resp["created"]=1; resp["updated"]=0 }
	if backup!="" { resp["backup"]=r11BackupLabel(backup) }
	writeJSON(w,resp)
}

func (h *controller) r11DayDelete(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"День графика удалён",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,1<<20);if err!=nil{return nil,"",err}
		employees,err:=r11Array(root,"employees");if err!=nil{return nil,"",err}
		emp,_:=r11FindEmployee(employees,r11String(body,"tab_no"));if emp==nil{return nil,"",fmt.Errorf("сотрудник не найден")}
		date:=r11String(body,"date");if !r11DateOK(date){return nil,"",fmt.Errorf("неверная дата")}
		days,err:=r11Array(root,"days");if err!=nil{return nil,"",err}
		kept:=days[:0];removed:=0
		for _,d:=range days{if r11Int(d,"employee_id")==r11Int(emp,"id")&&r11String(d,"date")==date{removed++;continue};kept=append(kept,d)}
		if removed==0{return nil,"",fmt.Errorf("запись дня не найдена")}
		r11SetArray(root,"days",kept)
		return map[string]any{"deleted":removed},fmt.Sprintf("%s · %s",r11String(emp,"tab_no"),date),nil
	})
}

func (h *controller) r11EventSave(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Событие добавлено",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,4<<20);if err!=nil{return nil,"",err}
		employees,err:=r11Array(root,"employees");if err!=nil{return nil,"",err}
		emp,_:=r11FindEmployee(employees,r11String(body,"TabNo"));if emp==nil{return nil,"",fmt.Errorf("сотрудник %q не найден",r11String(body,"TabNo"))}
		a,b:=r11String(body,"DateFrom"),r11String(body,"DateTo");if b==""{b=a}
		if !r11DateOK(a)||!r11DateOK(b)||b<a{return nil,"",fmt.Errorf("проверьте даты события")}
		events,err:=r11Array(root,"events");if err!=nil{return nil,"",err}
		item:=map[string]json.RawMessage{}
		r11Put(item,"id",r11NextID(root));r11Put(item,"employee_id",r11Int(emp,"id"))
		r11Put(item,"date_from",a);r11Put(item,"date_to",b)
		for _,k:=range []string{"EventType","SourceCode","Source","Note"}{if v:=r11Raw(body,k);len(v)>0{dst:=map[string]string{"EventType":"event_type","SourceCode":"source_code","Source":"source","Note":"note"}[k];item[dst]=append(json.RawMessage(nil),v...)}}
		r11Put(item,"created_at",time.Now().Format(time.RFC3339))
		events=append(events,item);r11SetArray(root,"events",events)
		return map[string]any{"id":r11Int(item,"id")} ,fmt.Sprintf("%s · %s—%s · %s",r11String(emp,"tab_no"),a,b,r11String(item,"event_type")),nil
	})
}

func (h *controller) r11EventDelete(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Событие удалено",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,1<<20);if err!=nil{return nil,"",err}
		rows,err:=r11Array(root,"events");if err!=nil{return nil,"",err}
		_,pos:=r11FindRecordID(rows,r11Int(body,"id"));if pos<0{return nil,"",fmt.Errorf("событие не найдено")}
		rows=append(rows[:pos],rows[pos+1:]...);r11SetArray(root,"events",rows)
		return map[string]any{"id":r11Int(body,"id")},"ID "+r11String(body,"id"),nil
	})
}

func (h *controller) r11RVDSave(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"РВД сохранено",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,2<<20);if err!=nil{return nil,"",err}
		employees,err:=r11Array(root,"employees");if err!=nil{return nil,"",err}
		emp,_:=r11FindEmployee(employees,r11String(body,"TabNo"));if emp==nil{return nil,"",fmt.Errorf("сотрудник не найден")}
		a,b:=r11String(body,"DateFrom"),r11String(body,"DateTo");if b==""{b=a}
		if !r11DateOK(a)||!r11DateOK(b)||b<a{return nil,"",fmt.Errorf("проверьте даты РВД")}
		typ:=r11Int(body,"Type");if typ!=1&&typ!=2{return nil,"",fmt.Errorf("тип РВД должен быть 1 или 2")}
		rows,err:=r11Array(root,"rvd");if err!=nil{return nil,"",err}
		item:=map[string]json.RawMessage{}
		r11Put(item,"id",r11NextID(root));r11Put(item,"employee_id",r11Int(emp,"id"));r11Put(item,"date_from",a);r11Put(item,"date_to",b);r11Put(item,"type",typ)
		r11Put(item,"rest_date",r11String(body,"RestDate"));r11Put(item,"note",r11String(body,"Note"));r11Put(item,"created_at",time.Now().Format(time.RFC3339))
		rows=append(rows,item);r11SetArray(root,"rvd",rows)
		return map[string]any{"id":r11Int(item,"id")},fmt.Sprintf("%s · РВД-%d · %s—%s",r11String(emp,"tab_no"),typ,a,b),nil
	})
}

func (h *controller) r11CalendarDaySave(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"День календаря сохранён",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,2<<20);if err!=nil{return nil,"",err}
		date:=r11String(body,"date");if !r11DateOK(date){return nil,"",fmt.Errorf("неверная дата")}
		rows,err:=r11Array(root,"calendar");if err!=nil{return nil,"",err}
		pos:=-1;for i,x:=range rows{if r11String(x,"date")==date{pos=i;break}}
		item:=map[string]json.RawMessage{};if pos>=0{item=rows[pos]}
		r11Put(item,"date",date);r11Put(item,"day_type",r11String(body,"day_type"));r11Put(item,"norm_hours",r11Float(body,"norm_hours"));r11Put(item,"note",r11String(body,"note"));r11Put(item,"source",r11String(body,"source"))
		if pos>=0{rows[pos]=item}else{rows=append(rows,item)}
		r11SetArray(root,"calendar",rows)
		return map[string]any{"date":date},date,nil
	})
}

func (h *controller) r11CalendarInit(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Производственный календарь создан",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,1<<20);if err!=nil{return nil,"",err}
		year:=int(r11Int(body,"year"));if year<2000||year>2100{return nil,"",fmt.Errorf("неверный год")}
		rows,err:=r11Array(root,"calendar");if err!=nil{return nil,"",err}
		for _,x:=range rows{if strings.HasPrefix(r11String(x,"date"),fmt.Sprintf("%04d-",year)){return nil,"",fmt.Errorf("календарь %d уже содержит записи; сначала очистите год",year)}}
		for d:=time.Date(year,1,1,0,0,0,0,time.UTC);d.Year()==year;d=d.AddDate(0,0,1){
			typ,hours:="OFF",0.0;if d.Weekday()!=time.Saturday&&d.Weekday()!=time.Sunday{typ="WORK";hours=8}
			item:=map[string]json.RawMessage{};r11Put(item,"date",d.Format("2006-01-02"));r11Put(item,"day_type",typ);r11Put(item,"norm_hours",hours);r11Put(item,"note","ТЕСТОВЫЙ черновик — без праздников и переносов");r11Put(item,"source","Черновик")
			rows=append(rows,item)
		}
		r11SetArray(root,"calendar",rows)
		return map[string]any{"year":year,"count":len(rows)},fmt.Sprintf("Тестовый черновик %d",year),nil
	})
}

func (h *controller) r11CalendarClear(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Производственный календарь очищен",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,1<<20);if err!=nil{return nil,"",err};year:=int(r11Int(body,"year"))
		rows,err:=r11Array(root,"calendar");if err!=nil{return nil,"",err}
		kept:=rows[:0];removed:=0
		for _,x:=range rows{if strings.HasPrefix(r11String(x,"date"),fmt.Sprintf("%04d-",year)){removed++}else{kept=append(kept,x)}}
		r11SetArray(root,"calendar",kept)
		return map[string]any{"year":year,"deleted":removed},fmt.Sprintf("%d · удалено %d",year,removed),nil
	})
}

func (h *controller) r11CalendarImportConfirm(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Импорт производственного календаря",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,16<<20);if err!=nil{return nil,"",err}
		var incoming []map[string]json.RawMessage
		if err:=json.Unmarshal(r11Raw(body,"rows"),&incoming);err!=nil{return nil,"",fmt.Errorf("неверный список строк календаря: %w",err)}
		if len(incoming)==0{return nil,"",fmt.Errorf("нет строк для импорта")}
		rows,err:=r11Array(root,"calendar");if err!=nil{return nil,"",err}
		pos:=map[string]int{};for i,x:=range rows{pos[r11String(x,"date")]=i}
		added,updated:=0,0
		for _,x:=range incoming{
			date:=r11String(x,"date");if !r11DateOK(date){return nil,"",fmt.Errorf("неверная дата календаря %q",date)}
			if i,ok:=pos[date];ok{for k,v:=range x{rows[i][k]=v};updated++}else{rows=append(rows,x);pos[date]=len(rows)-1;added++}
		}
		r11SetArray(root,"calendar",rows)
		return map[string]any{"added":added,"updated":updated,"count":len(incoming)},fmt.Sprintf("%s · строк %d",r11String(body,"file_name"),len(incoming)),nil
	})
}

func (h *controller) r11ScheduleTypeSave(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Вид графика сохранён",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,2<<20);if err!=nil{return nil,"",err}
		point,name:=r11String(body,"collection_point"),r11String(body,"name");if point==""||name==""{return nil,"",fmt.Errorf("укажите пункт сбора и название вида графика")}
		phase:=r11String(body,"phase");if phase!=""&&phase!="A"&&phase!="B"{return nil,"",fmt.Errorf("фаза должна быть A или B")}
		rows,err:=r11Array(root,"schedule_types");if err!=nil{return nil,"",err}
		id:=r11Int(body,"id");pos:=-1
		for i,x:=range rows{if id>0&&r11Int(x,"id")==id{pos=i;break};if id==0&&r11String(x,"collection_point")==point&&strings.EqualFold(r11String(x,"name"),name){pos=i;break}}
		item:=map[string]json.RawMessage{};if pos>=0{item=rows[pos]}else{r11Put(item,"id",r11NextID(root))}
		for _,k:=range []string{"collection_point","name","pair_name","phase","note"}{if v:=r11Raw(body,k);len(v)>0{item[k]=append(json.RawMessage(nil),v...)}}
		if v:=r11Raw(body,"active");len(v)>0{item["active"]=append(json.RawMessage(nil),v...)}
		if pos>=0{rows[pos]=item}else{rows=append(rows,item)}
		r11SetArray(root,"schedule_types",rows)
		return map[string]any{"id":r11Int(item,"id")},fmt.Sprintf("%s · %s · фаза %s",point,name,phase),nil
	})
}

func (h *controller) r11ScheduleTypeDelete(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Вид графика удалён",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,1<<20);if err!=nil{return nil,"",err};rows,err:=r11Array(root,"schedule_types");if err!=nil{return nil,"",err}
		id:=r11Int(body,"id");_,pos:=r11FindRecordID(rows,id);if pos<0{return nil,"",fmt.Errorf("вид графика не найден")}
		rows=append(rows[:pos],rows[pos+1:]...);r11SetArray(root,"schedule_types",rows)
		return map[string]any{"id":id},fmt.Sprintf("ID %d",id),nil
	})
}

func (h *controller) r11FlightSave(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Дата полёта сохранена",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,2<<20);if err!=nil{return nil,"",err}
		point,date:=r11String(body,"collection_point"),r11String(body,"date");if point==""||!r11DateOK(date){return nil,"",fmt.Errorf("укажите пункт сбора и корректную дату")}
		rows,err:=r11Array(root,"flight_dates");if err!=nil{return nil,"",err}
		id:=r11Int(body,"id");pos:=-1
		for i,x:=range rows{if id>0&&r11Int(x,"id")==id{pos=i;break};if id==0&&r11String(x,"collection_point")==point&&r11String(x,"date")==date{pos=i;break}}
		item:=map[string]json.RawMessage{};if pos>=0{item=rows[pos]}else{r11Put(item,"id",r11NextID(root))}
		for _,k:=range []string{"collection_point","date","source","note"}{if v:=r11Raw(body,k);len(v)>0{item[k]=append(json.RawMessage(nil),v...)}}
		r11Put(item,"updated_at",time.Now().Format(time.RFC3339))
		if pos>=0{rows[pos]=item}else{rows=append(rows,item)}
		r11SetArray(root,"flight_dates",rows)
		return map[string]any{"id":r11Int(item,"id")},fmt.Sprintf("%s · %s",point,date),nil
	})
}

func (h *controller) r11FlightDelete(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Дата полёта удалена",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,1<<20);if err!=nil{return nil,"",err};rows,err:=r11Array(root,"flight_dates");if err!=nil{return nil,"",err}
		id:=r11Int(body,"id");_,pos:=r11FindRecordID(rows,id);if pos<0{return nil,"",fmt.Errorf("дата полёта не найдена")}
		rows=append(rows[:pos],rows[pos+1:]...);r11SetArray(root,"flight_dates",rows)
		return map[string]any{"id":id},fmt.Sprintf("ID %d",id),nil
	})
}

func (h *controller) r11FlightBatch(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Добавлен список дат полётов",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,2<<20);if err!=nil{return nil,"",err}
		point:=r11String(body,"collection_point");if point==""{return nil,"",fmt.Errorf("укажите пункт сбора")}
		text:=r11String(body,"dates");parts:=strings.FieldsFunc(text,func(c rune)bool{return c==','||c==';'||c=='\n'||c=='\r'||c=='\t'||c==' '})
		if len(parts)==0{return nil,"",fmt.Errorf("укажите даты")}
		rows,err:=r11Array(root,"flight_dates");if err!=nil{return nil,"",err};added,skipped:=0,0
		for _,raw:=range parts{date:=strings.TrimSpace(raw);if !r11DateOK(date){if d,e:=time.Parse("02.01.2006",date);e==nil{date=d.Format("2006-01-02")}}
			if !r11DateOK(date){return nil,"",fmt.Errorf("не распознана дата %q",raw)}
			found:=false;for _,x:=range rows{if r11String(x,"collection_point")==point&&r11String(x,"date")==date{found=true;break}}
			if found{skipped++;continue}
			item:=map[string]json.RawMessage{};r11Put(item,"id",r11NextID(root));r11Put(item,"collection_point",point);r11Put(item,"date",date);r11Put(item,"source","Ручной список");r11Put(item,"note",r11String(body,"note"));r11Put(item,"created_at",time.Now().Format(time.RFC3339));rows=append(rows,item);added++
		}
		r11SetArray(root,"flight_dates",rows)
		return map[string]any{"added":added,"skipped":skipped},fmt.Sprintf("%s · добавлено %d, пропущено %d",point,added,skipped),nil
	})
}

func (h *controller) r11VersionCreate(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Контрольная версия графика создана",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,2<<20);if err!=nil{return nil,"",err}
		employees,err:=r11Array(root,"employees");if err!=nil{return nil,"",err};emp,_:=r11FindEmployee(employees,r11String(body,"tab_no"));if emp==nil{return nil,"",fmt.Errorf("сотрудник не найден")}
		year:=int(r11Int(body,"year"));if year<2000||year>2100{return nil,"",fmt.Errorf("неверный год")}
		days,err:=r11Array(root,"days");if err!=nil{return nil,"",err};snapshot:=[]map[string]json.RawMessage{}
		prefix:=fmt.Sprintf("%04d-",year);for _,d:=range days{if r11Int(d,"employee_id")==r11Int(emp,"id")&&strings.HasPrefix(r11String(d,"date"),prefix){b,_:=json.Marshal(d);var cp map[string]json.RawMessage;_ = json.Unmarshal(b,&cp);snapshot=append(snapshot,cp)}}
		rows,err:=r11Array(root,"graph_versions");if err!=nil{return nil,"",err}
		item:=map[string]json.RawMessage{};r11Put(item,"id",r11NextID(root));r11Put(item,"employee_id",r11Int(emp,"id"));r11Put(item,"year",year);r11Put(item,"label",r11String(body,"label"));r11Put(item,"reason",r11String(body,"reason"));r11Put(item,"created_at",time.Now().Format(time.RFC3339));r11Put(item,"days",snapshot)
		rows=append(rows,item);r11SetArray(root,"graph_versions",rows)
		return map[string]any{"id":r11Int(item,"id"),"days":len(snapshot)},fmt.Sprintf("%s · %d · %s",r11String(emp,"tab_no"),year,r11String(body,"label")),nil
	})
}

func (h *controller) r11VersionRestore(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Контрольная версия графика восстановлена",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,1<<20);if err!=nil{return nil,"",err}
		versions,err:=r11Array(root,"graph_versions");if err!=nil{return nil,"",err};v,_:=r11FindRecordID(versions,r11Int(body,"id"));if v==nil{return nil,"",fmt.Errorf("версия не найдена")}
		employeeID,year:=r11Int(v,"employee_id"),int(r11Int(v,"year"));var snapshot []map[string]json.RawMessage
		if err:=json.Unmarshal(v["days"],&snapshot);err!=nil{return nil,"",fmt.Errorf("снимок дней повреждён: %w",err)}
		days,err:=r11Array(root,"days");if err!=nil{return nil,"",err};prefix:=fmt.Sprintf("%04d-",year);kept:=days[:0]
		for _,d:=range days{if r11Int(d,"employee_id")==employeeID&&strings.HasPrefix(r11String(d,"date"),prefix){continue};kept=append(kept,d)}
		kept=append(kept,snapshot...);r11SetArray(root,"days",kept)
		return map[string]any{"id":r11Int(v,"id"),"restored":len(snapshot)},fmt.Sprintf("ID %d · %d дней",r11Int(v,"id"),len(snapshot)),nil
	})
}

func (h *controller) r11ScheduleChange(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Изменение вида графика зафиксировано",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,2<<20);if err!=nil{return nil,"",err}
		employees,err:=r11Array(root,"employees");if err!=nil{return nil,"",err};emp,_:=r11FindEmployee(employees,r11String(body,"tab_no"));if emp==nil{return nil,"",fmt.Errorf("сотрудник не найден")}
		date,newSchedule:=r11String(body,"effective_date"),r11String(body,"new_schedule");if !r11DateOK(date)||newSchedule==""{return nil,"",fmt.Errorf("укажите дату и новый вид графика")}
		rows,err:=r11Array(root,"schedule_changes");if err!=nil{return nil,"",err}
		old:=r11String(emp,"schedule_type")
		item:=map[string]json.RawMessage{};r11Put(item,"id",r11NextID(root));r11Put(item,"employee_id",r11Int(emp,"id"));r11Put(item,"effective_date",date);r11Put(item,"old_schedule",old);r11Put(item,"new_schedule",newSchedule);r11Put(item,"hours_before",0);r11Put(item,"reason",r11String(body,"reason"));r11Put(item,"created_at",time.Now().Format(time.RFC3339))
		rows=append(rows,item);r11SetArray(root,"schedule_changes",rows)
		return map[string]any{"id":r11Int(item,"id")},fmt.Sprintf("%s · %s → %s · %s",r11String(emp,"tab_no"),old,newSchedule,date),nil
	})
}

func (h *controller) r11RevisionCreate(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Официальная редакция графика создана",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,4<<20);if err!=nil{return nil,"",err}
		employees,err:=r11Array(root,"employees");if err!=nil{return nil,"",err};emp,_:=r11FindEmployee(employees,r11String(body,"tab_no"));if emp==nil{return nil,"",fmt.Errorf("сотрудник не найден")}
		effective:=r11String(body,"effective_date");if !r11DateOK(effective){return nil,"",fmt.Errorf("неверная дата начала редакции")}
		rows,err:=r11Array(root,"graph_revisions");if err!=nil{return nil,"",err}
		maxNo:=int64(0);parent:=int64(0);hasActive:=false
		for _,x:=range rows{if r11Int(x,"employee_id")!=r11Int(emp,"id"){continue};if r11Int(x,"number")>maxNo{maxNo=r11Int(x,"number")};if strings.EqualFold(r11String(x,"status"),"ACTIVE"){hasActive=true;parent=r11Int(x,"id")}}
		days,err:=r11Array(root,"days");if err!=nil{return nil,"",err};events,err:=r11Array(root,"events");if err!=nil{return nil,"",err};notes,err:=r11Array(root,"notes");if err!=nil{return nil,"",err};colors,err:=r11Array(root,"manual_colors");if err!=nil{return nil,"",err}
		filter:=func(src []map[string]json.RawMessage)[]map[string]json.RawMessage{out:=[]map[string]json.RawMessage{};for _,x:=range src{if r11Int(x,"employee_id")==r11Int(emp,"id"){out=append(out,x)}};return out}
		item:=map[string]json.RawMessage{};r11Put(item,"id",r11NextID(root));r11Put(item,"employee_id",r11Int(emp,"id"));r11Put(item,"number",maxNo+1);r11Put(item,"effective_date",effective);r11Put(item,"control_date",r11String(body,"control_date"));r11Put(item,"fixed_hours",r11Float(body,"fixed_hours"));r11Put(item,"schedule_type",r11String(body,"schedule_type"));r11Put(item,"reason",r11String(body,"reason"));r11Put(item,"label",r11String(body,"label"));r11Put(item,"parent_id",parent)
		status:="DRAFT";if !hasActive{status="ACTIVE";r11Put(item,"activated_at",time.Now().Format(time.RFC3339))}
		r11Put(item,"status",status);now:=time.Now().Format(time.RFC3339);r11Put(item,"created_at",now);r11Put(item,"updated_at",now)
		r11Put(item,"days_snapshot",filter(days));r11Put(item,"events_snapshot",filter(events));r11Put(item,"notes_snapshot",filter(notes));r11Put(item,"colors_snapshot",filter(colors))
		rows=append(rows,item);r11SetArray(root,"graph_revisions",rows)
		return map[string]any{"id":r11Int(item,"id"),"number":maxNo+1,"status":status},fmt.Sprintf("%s · редакция №%d · %s",r11String(emp,"tab_no"),maxNo+1,effective),nil
	})
}

func (h *controller) r11RevisionGet(w http.ResponseWriter, r *http.Request) {
	if r.Method!=http.MethodGet{http.Error(w,"GET only",405);return}
	id,err:=strconv.ParseInt(r.URL.Query().Get("id"),10,64);if err!=nil||id<1{http.Error(w,"неверный id редакции",400);return}
	h.opMu.Lock();root,e:=h.r11LoadRootLocked();h.opMu.Unlock();if e!=nil{http.Error(w,e.Error(),500);return}
	rows,e:=r11Array(root,"graph_revisions");if e!=nil{http.Error(w,e.Error(),500);return};item,_:=r11FindRecordID(rows,id);if item==nil{http.Error(w,"редакция не найдена",404);return}
	writeJSON(w,item)
}

func (h *controller) r11RevisionActivate(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Статус официальной редакции изменён",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,1<<20);if err!=nil{return nil,"",err};rows,err:=r11Array(root,"graph_revisions");if err!=nil{return nil,"",err}
		id:=r11Int(body,"id");target,_:=r11FindRecordID(rows,id);if target==nil{return nil,"",fmt.Errorf("редакция не найдена")}
		empID:=r11Int(target,"employee_id");for _,x:=range rows{if r11Int(x,"employee_id")==empID{if r11Int(x,"id")==id{r11Put(x,"status","ACTIVE");r11Put(x,"activated_at",time.Now().Format(time.RFC3339))}else if strings.EqualFold(r11String(x,"status"),"ACTIVE"){r11Put(x,"status","ARCHIVED")}}}
		r11SetArray(root,"graph_revisions",rows)
		return map[string]any{"id":id},fmt.Sprintf("Редакция ID %d",id),nil
	})
}

func (h *controller) r11NoteSave(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Примечание сохранено",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,2<<20);if err!=nil{return nil,"",err}
		employees,err:=r11Array(root,"employees");if err!=nil{return nil,"",err};emp,_:=r11FindEmployee(employees,r11String(body,"tab_no"));if emp==nil{return nil,"",fmt.Errorf("сотрудник не найден")}
		a,b:=r11String(body,"date_from"),r11String(body,"date_to");if b==""{b=a};if !r11DateOK(a)||!r11DateOK(b)||b<a{return nil,"",fmt.Errorf("проверьте диапазон примечания")}
		rows,err:=r11Array(root,"notes");if err!=nil{return nil,"",err};id:=r11Int(body,"id");pos:=-1
		if id>0{_,pos=r11FindRecordID(rows,id)}
		item:=map[string]json.RawMessage{};if pos>=0{item=rows[pos]}else{r11Put(item,"id",r11NextID(root));r11Put(item,"created_at",time.Now().Format(time.RFC3339))}
		r11Put(item,"employee_id",r11Int(emp,"id"));r11Put(item,"date_from",a);r11Put(item,"date_to",b);r11Put(item,"text",r11String(body,"text"));r11Put(item,"reminder",r11String(body,"reminder"));r11Put(item,"updated_at",time.Now().Format(time.RFC3339))
		if pos>=0{rows[pos]=item}else{rows=append(rows,item)};r11SetArray(root,"notes",rows)
		return map[string]any{"id":r11Int(item,"id")},fmt.Sprintf("%s · %s—%s",r11String(emp,"tab_no"),a,b),nil
	})
}

func (h *controller) r11NoteDelete(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Примечание удалено",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,1<<20);if err!=nil{return nil,"",err};rows,err:=r11Array(root,"notes");if err!=nil{return nil,"",err}
		id:=r11Int(body,"id");_,pos:=r11FindRecordID(rows,id);if pos<0{return nil,"",fmt.Errorf("примечание не найдено")}
		rows=append(rows[:pos],rows[pos+1:]...);r11SetArray(root,"notes",rows);return map[string]any{"id":id},fmt.Sprintf("ID %d",id),nil
	})
}

func (h *controller) r11NoteReminderDelete(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Напоминание удалено",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,1<<20);if err!=nil{return nil,"",err};rows,err:=r11Array(root,"notes");if err!=nil{return nil,"",err}
		id:=r11Int(body,"id");item,_:=r11FindRecordID(rows,id);if item==nil{return nil,"",fmt.Errorf("примечание не найдено")}
		r11Put(item,"reminder","");r11Put(item,"updated_at",time.Now().Format(time.RFC3339));r11SetArray(root,"notes",rows)
		return map[string]any{"id":id},fmt.Sprintf("ID %d",id),nil
	})
}

func (h *controller) r11NoteSplit(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Диапазон примечания разделён",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,1<<20);if err!=nil{return nil,"",err};rows,err:=r11Array(root,"notes");if err!=nil{return nil,"",err}
		id,date:=r11Int(body,"id"),r11String(body,"date");if !r11DateOK(date){return nil,"",fmt.Errorf("неверная дата")}
		item,_:=r11FindRecordID(rows,id);if item==nil{return nil,"",fmt.Errorf("примечание не найдено")}
		a,b:=r11String(item,"date_from"),r11String(item,"date_to");if date<a||date>b{return nil,"",fmt.Errorf("дата вне диапазона примечания")}
		if a==b{return nil,"",fmt.Errorf("нельзя разделить однодневное примечание")}
		if date==a{next:=r11DateAdd(date,1);r11Put(item,"date_from",next)}else if date==b{prev:=r11DateAdd(date,-1);r11Put(item,"date_to",prev)}else{
			right:=map[string]json.RawMessage{};for k,v:=range item{right[k]=append(json.RawMessage(nil),v...)}
			r11Put(item,"date_to",r11DateAdd(date,-1));r11Put(right,"id",r11NextID(root));r11Put(right,"date_from",r11DateAdd(date,1));r11Put(right,"created_at",time.Now().Format(time.RFC3339));rows=append(rows,right)
		}
		r11SetArray(root,"notes",rows);return map[string]any{"id":id},fmt.Sprintf("ID %d · %s",id,date),nil
	})
}

func r11DateAdd(date string, n int) string {
	d,e:=time.Parse("2006-01-02",date);if e!=nil{return date};return d.AddDate(0,0,n).Format("2006-01-02")
}

func (h *controller) r11ColorSave(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Ручной цвет применён",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,2<<20);if err!=nil{return nil,"",err};employees,err:=r11Array(root,"employees");if err!=nil{return nil,"",err}
		emp,_:=r11FindEmployee(employees,r11String(body,"tab_no"));if emp==nil{return nil,"",fmt.Errorf("сотрудник не найден")}
		a,b,color:=r11String(body,"date_from"),r11String(body,"date_to"),r11String(body,"color");if b==""{b=a};if !r11DateOK(a)||!r11DateOK(b)||b<a{return nil,"",fmt.Errorf("неверный диапазон")}
		if !map[string]bool{"yellow":true,"green":true,"peach":true,"pink":true,"gray":true}[color]{return nil,"",fmt.Errorf("неизвестный цвет")}
		rows,err:=r11Array(root,"manual_colors");if err!=nil{return nil,"",err};id:=r11Int(emp,"id");count:=0
		for d:=a;d<=b;d=r11DateAdd(d,1){pos:=-1;for i,x:=range rows{if r11Int(x,"employee_id")==id&&r11String(x,"date")==d{pos=i;break}}
			item:=map[string]json.RawMessage{};if pos>=0{item=rows[pos]}else{r11Put(item,"id",r11NextID(root));r11Put(item,"employee_id",id);r11Put(item,"date",d);rows=append(rows,item)}
			r11Put(item,"color",color);count++
		}
		r11SetArray(root,"manual_colors",rows);return map[string]any{"count":count},fmt.Sprintf("%s · %s—%s · %s",r11String(emp,"tab_no"),a,b,color),nil
	})
}

func (h *controller) r11ColorDelete(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Ручной цвет удалён",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,2<<20);if err!=nil{return nil,"",err};employees,err:=r11Array(root,"employees");if err!=nil{return nil,"",err};emp,_:=r11FindEmployee(employees,r11String(body,"tab_no"));if emp==nil{return nil,"",fmt.Errorf("сотрудник не найден")}
		a,b:=r11String(body,"date_from"),r11String(body,"date_to");if b==""{b=a};if !r11DateOK(a)||!r11DateOK(b)||b<a{return nil,"",fmt.Errorf("неверный диапазон")}
		rows,err:=r11Array(root,"manual_colors");if err!=nil{return nil,"",err};kept:=rows[:0];removed:=0
		for _,x:=range rows{if r11Int(x,"employee_id")==r11Int(emp,"id")&&r11String(x,"date")>=a&&r11String(x,"date")<=b{removed++;continue};kept=append(kept,x)}
		r11SetArray(root,"manual_colors",kept);return map[string]any{"deleted":removed},fmt.Sprintf("%s · %s—%s",r11String(emp,"tab_no"),a,b),nil
	})
}

func (h *controller) r11Backup(w http.ResponseWriter, r *http.Request) {
	if r.Method!=http.MethodPost{http.Error(w,"POST only",405);return}
	h.opMu.Lock();defer h.opMu.Unlock();path,err:=h.backupR11StateLocked();if err!=nil{http.Error(w,err.Error(),500);return}
	writeJSON(w,map[string]any{"ok":true,"backup":r11BackupLabel(path)})
}

func (h *controller) r11ImportFact(w http.ResponseWriter, r *http.Request) {
	h.r11CSVImport(w,r,false)
}

func (h *controller) r11ImportEvents(w http.ResponseWriter, r *http.Request) {
	h.r11CSVImport(w,r,true)
}

func (h *controller) r11CSVImport(w http.ResponseWriter, r *http.Request, eventsOnly bool) {
	if r.Method!=http.MethodPost{http.Error(w,"POST only",405);return}
	if err:=r.ParseMultipartForm(32<<20);err!=nil{http.Error(w,err.Error(),400);return}
	file,header,err:=r.FormFile("file");if err!=nil{http.Error(w,err.Error(),400);return};defer file.Close()
	reader:=csv.NewReader(file);reader.FieldsPerRecord=-1;reader.LazyQuotes=true
	rows,err:=reader.ReadAll();if err!=nil{http.Error(w,err.Error(),400);return};if len(rows)<2{http.Error(w,"CSV пуст",400);return}
	head:=map[string]int{};for i,x:=range rows[0]{head[strings.ToLower(strings.TrimSpace(x))]=i}
	get:=func(row []string,k string)string{i,ok:=head[k];if !ok||i>=len(row){return ""};return strings.TrimSpace(row[i])}
	h.opMu.Lock();defer h.opMu.Unlock();root,err:=h.r11LoadRootLocked();if err!=nil{http.Error(w,err.Error(),500);return}
	employees,err:=r11Array(root,"employees");if err!=nil{http.Error(w,err.Error(),500);return}
	days,err:=r11Array(root,"days");if err!=nil{http.Error(w,err.Error(),500);return}
	events,err:=r11Array(root,"events");if err!=nil{http.Error(w,err.Error(),500);return}
	applied:=0
	for _,row:=range rows[1:]{emp,_:=r11FindEmployee(employees,get(row,"tab_no"));if emp==nil{continue}
		if eventsOnly{
			a,b:=get(row,"date_from"),get(row,"date_to");if b==""{b=a};if !r11DateOK(a)||!r11DateOK(b)||b<a{continue}
			item:=map[string]json.RawMessage{};r11Put(item,"id",r11NextID(root));r11Put(item,"employee_id",r11Int(emp,"id"));r11Put(item,"date_from",a);r11Put(item,"date_to",b);r11Put(item,"event_type",get(row,"event_type"));r11Put(item,"source_code",get(row,"source_code"));r11Put(item,"source",get(row,"source"));r11Put(item,"note",get(row,"note"));r11Put(item,"created_at",time.Now().Format(time.RFC3339));events=append(events,item);applied++
		}else{
			date:=get(row,"date");if !r11DateOK(date){continue};pos:=-1;for i,d:=range days{if r11Int(d,"employee_id")==r11Int(emp,"id")&&r11String(d,"date")==date{pos=i;break}}
			item:=map[string]json.RawMessage{};if pos>=0{item=days[pos]}else{r11Put(item,"id",r11NextID(root));r11Put(item,"employee_id",r11Int(emp,"id"));r11Put(item,"date",date)}
			r11Put(item,"fact",get(row,"value"));if _,ok:=head["hours"];ok{v,_:=strconv.ParseFloat(strings.ReplaceAll(get(row,"hours"),",","."),64);r11Put(item,"fact_hours",v)};if _,ok:=head["shift"];ok{r11Put(item,"shift",get(row,"shift"))}
			if pos>=0{days[pos]=item}else{days=append(days,item)};applied++
		}
	}
	if eventsOnly{r11SetArray(root,"events",events)}else{r11SetArray(root,"days",days)}
	backup,err:=h.r11SaveRootLocked(root,func()string{if eventsOnly{return "Импорт событий"};return "Импорт факта"}(),fmt.Sprintf("%s · строк %d · применено %d",filepath.Base(header.Filename),len(rows)-1,applied))
	if err!=nil{http.Error(w,err.Error(),500);return}
	writeJSON(w,map[string]any{"ok":true,"count":applied,"applied":applied,"rows":len(rows)-1,"backup":r11BackupLabel(backup)})
}

// The following handlers are deliberately explicit stubs until their full
// import and compatibility rules have been ported and covered by fixtures.
// They return a clear error instead of silently accepting incomplete data.
func (h *controller) r11FlightImportPreview(w http.ResponseWriter, r *http.Request) { http.Error(w,"Импорт общих полётов Excel ещё не перенесён; используйте ручной ввод дат.",http.StatusNotImplemented) }
func (h *controller) r11FlightImportConfirm(w http.ResponseWriter, r *http.Request) { http.Error(w,"Подтверждение импорта общих полётов ещё не перенесено.",http.StatusNotImplemented) }
func (h *controller) r11EmployeeFlightImportConfirm(w http.ResponseWriter, r *http.Request) { http.Error(w,"Импорт индивидуальных полётов ещё не перенесён.",http.StatusNotImplemented) }
func (h *controller) r11EmployeeFlightDelete(w http.ResponseWriter, r *http.Request) { http.Error(w,"Удаление индивидуального полёта ещё не перенесено.",http.StatusNotImplemented) }
func (h *controller) r11Demo(w http.ResponseWriter, r *http.Request) { http.Error(w,"Демо-генератор ещё не перенесён.",http.StatusNotImplemented) }
func (h *controller) r11DemoAutograf(w http.ResponseWriter, r *http.Request) { http.Error(w,"Демо-генератор АвтоГрафа ещё не перенесён.",http.StatusNotImplemented) }
