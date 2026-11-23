package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
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
	case "/api/employee/batch":
		h.r11EmployeeBatch(w, r)
	case "/api/employee/points/template.xlsx":
		h.r11CollectionPointTemplate(w, r)
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
	case "/api/autograf/schedule-type/pair":
		h.r11SchedulePairSave(w, r)
	case "/api/autograf/schedule-type/delete":
		h.r11ScheduleTypeDelete(w, r)
	case "/api/autograf/cycle/save":
		h.r11CycleSave(w, r)
	case "/api/autograf/cycle/flight/move":
		h.r11CycleFlightMove(w, r)
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
	page := bytes.ReplaceAll(r11OriginalHTML, []byte("state.version='v0.9.11'"), []byte("state.version=state.version||'R11 Linux Core'"))
	page = bytes.ReplaceAll(page, []byte("техника по труду · Табель R6"), []byte("техника по труду · Linux R11"))
	page = bytes.ReplaceAll(page, []byte(`<div id="page"></div>`), []byte(r11WindowsImportPanel+`<div id="page"></div>`))
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


func r11EmployeeUpsert(root map[string]json.RawMessage, rows []map[string]json.RawMessage, body map[string]json.RawMessage) ([]map[string]json.RawMessage, int64, bool, error) {
    if err := r11StrictEmployeePayload(body); err != nil { return nil, 0, false, err }
    tab, name := r11String(body,"tab_no"),r11String(body,"full_name")
    if tab==""||name=="" { return nil,0,false,fmt.Errorf("табельный номер и ФИО обязательны") }
    id:=r11Int(body,"id")
    pos:=-1
    if id>0 {
        _,pos=r11FindEmployeeID(rows,id)
        if pos<0 { return nil,0,false,fmt.Errorf("сотрудник id=%d не найден; создание карточки с новым ID отменено",id) }
    }
    if pos<0 {
        _,dup:=r11FindEmployee(rows,tab)
        if dup>=0 { return nil,0,false,fmt.Errorf("табельный номер %s уже существует",tab) }
    } else {
        for i,other:=range rows {
            if i!=pos&&strings.EqualFold(r11String(other,"tab_no"),tab) {
                return nil,0,false,fmt.Errorf("табельный номер %s уже существует",tab)
            }
        }
    }
    item:=map[string]json.RawMessage{}
    created:=pos<0
    if !created {
        for k,v:=range rows[pos] { item[k]=append(json.RawMessage(nil),v...) }
    }
    for _,key:=range []string{"tab_no","full_name","position","unit","collection_point","schedule_type","hire_date","fire_date","probation_end","notes","is_quarter","probation","main_schedule_ready"} {
        if v:=r11Raw(body,key);len(v)>0 { item[key]=append(json.RawMessage(nil),v...) }
    }
    // Validate the complete record after merging, not only the partial request.
    if err:=r11StrictEmployeePayload(item);err!=nil { return nil,0,false,err }
    if created {
        r11Put(item,"id",r11NextID(root))
        r11Put(item,"created_at",time.Now().Format(time.RFC3339))
    }
    r11Put(item,"updated_at",time.Now().Format(time.RFC3339))
    if created { rows=append(rows,item) }else{ rows[pos]=item }
    return rows,r11Int(item,"id"),created,nil
}

func (h *controller) r11EmployeeSave(w http.ResponseWriter,r *http.Request) {
    h.r11Mutate(w,r,"Карточка сотрудника",func(root map[string]json.RawMessage)(map[string]any,string,error){
        body,err:=r11BodyMap(w,r,2<<20);if err!=nil{return nil,"",err}
        rows,err:=r11Array(root,"employees");if err!=nil{return nil,"",err}
        rows,id,created,err:=r11EmployeeUpsert(root,rows,body);if err!=nil{return nil,"",err}
        r11SetArray(root,"employees",rows)
        verb:="изменён";if created{verb="создан"}
        return map[string]any{"id":id,"created":created},fmt.Sprintf("%s · %s · %s",r11String(body,"tab_no"),r11String(body,"full_name"),verb),nil
    })
}

// Apply an entire employee or collection-point spreadsheet as one transaction.
// On a bad row, nothing is written and the existing database remains unchanged.
func (h *controller) r11EmployeeBatch(w http.ResponseWriter,r *http.Request) {
    h.r11Mutate(w,r,"Массовый импорт сотрудников",func(root map[string]json.RawMessage)(map[string]any,string,error){
        body,err:=r11BodyMap(w,r,32<<20);if err!=nil{return nil,"",err}
        raw:=r11Raw(body,"employees")
        var batch []map[string]json.RawMessage
        if len(raw)==0||json.Unmarshal(raw,&batch)!=nil||len(batch)<1||len(batch)>2000 {
            return nil,"",fmt.Errorf("передайте от 1 до 2000 корректных карточек в массиве employees")
        }
        rows,err:=r11Array(root,"employees");if err!=nil{return nil,"",err}
        touched:=map[string]bool{}
        createdCount,updatedCount:=0,0
        for i,emp:=range batch {
            if emp==nil {return nil,"",fmt.Errorf("строка %d: карточка отсутствует",i+1)}
            id:=r11Int(emp,"id")
            tab:=strings.ToLower(strings.TrimSpace(r11String(emp,"tab_no")))
            key:=fmt.Sprintf("tab:%s",tab)
            if id>0 { key=fmt.Sprintf("id:%d",id) }
            if touched[key] {return nil,"",fmt.Errorf("строка %d: повтор сотрудника в пакете",i+1)}
            touched[key]=true
            var newID int64
            var created bool
            rows,newID,created,err=r11EmployeeUpsert(root,rows,emp)
            if err!=nil{return nil,"",fmt.Errorf("строка %d: %w",i+1,err)}
            _=newID
            if created {createdCount++}else{updatedCount++}
        }
        r11SetArray(root,"employees",rows)
        return map[string]any{"created":createdCount,"updated":updatedCount,"applied":len(batch)},
            fmt.Sprintf("%d записей (новых: %d; изменений: %d)",len(batch),createdCount,updatedCount),nil
    })
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

// r11PlanDateAllowed is a hard server-side boundary for editing a person's plan.
// The date of termination itself remains writable. Historical records and
// SAP/ISAT fact imports are not deleted or rewritten by this check.
func r11PlanDateAllowed(emp map[string]json.RawMessage, date string) error {
    if fire := r11String(emp, "fire_date"); fire != "" && date > fire {
        formatted := fire
        if d, err := time.Parse("2006-01-02", fire); err == nil { formatted = d.Format("02.01.2006") }
        return fmt.Errorf("после увольнения нельзя изменять план: последний день %s включительно", formatted)
    }
    return nil
}

// A restored version may contain obsolete plan entries past the end of employment.
// Keep any existing post-termination rows unchanged; restoring cannot add, change,
// or erase them. Other pre-termination days may be restored normally.
func r11SafeRestoreDays(emp map[string]json.RawMessage, employeeID int64, current, snapshot []map[string]json.RawMessage) ([]map[string]json.RawMessage, error) {
    fire := r11String(emp,"fire_date")
    if fire=="" { return snapshot,nil }
    historical := map[string]map[string]json.RawMessage{}
    for _,d := range current {
        date:=r11String(d,"date")
        if r11Int(d,"employee_id")==employeeID && date>fire { historical[date]=d }
    }
    safe:=make([]map[string]json.RawMessage,0,len(snapshot)+len(historical))
    for _,d := range snapshot {
        date:=r11String(d,"date")
        if r11Int(d,"employee_id")!=employeeID || date<=fire { safe=append(safe,d);continue }
        original,exists:=historical[date]
        if !exists { return nil,fmt.Errorf("версия содержит план за %s после увольнения: восстановление запрещено",date) }
        before,_:=json.Marshal(original)
        after,_:=json.Marshal(d)
        if !bytes.Equal(before,after) { return nil,fmt.Errorf("версия изменяет исторический день %s после увольнения: восстановление запрещено",date) }
    }
    for _,d:=range historical { safe=append(safe,d) }
    return safe,nil
}

func (h *controller) r11DaySave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	body, err := r11BodyMap(w, r, 8<<20)
	if err != nil { r11WriteError(w,400,err); return }
	if err := r11StrictDayPayload(body); err != nil { r11WriteError(w,400,err); return }
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
    if err := r11PlanDateAllowed(emp,date); err != nil { r11WriteError(w,400,err); return }
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
        if err:=r11PlanDateAllowed(emp,date);err!=nil{return nil,"",err}
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
        if err:=r11PlanDateAllowed(emp,b);err!=nil{return nil,"",err}
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
		if err:=r11StrictCalendarPayload(body);err!=nil{return nil,"",err}
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
		seenIncoming:=map[string]bool{}
		for _,x:=range incoming{
			date:=r11String(x,"date");if !r11DateOK(date){return nil,"",fmt.Errorf("неверная дата календаря %q",date)}
			if seenIncoming[date]{return nil,"",fmt.Errorf("повтор даты календаря %s в одном импорте",date)}
			seenIncoming[date]=true
			if err:=r11StrictCalendarPayload(x);err!=nil{return nil,"",fmt.Errorf("дата %s: %w",date,err)}
			if len(r11Raw(x,"norm_hours"))==0{return nil,"",fmt.Errorf("дата %s: отсутствует норма часов",date)}
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

// r11SchedulePairSave creates a mutually linked A/B pair in a single protected mutation.
// id > 0 completes a previously saved first half; id == 0 creates two new halves.
// r11SchedulePairSave creates or links two mutually opposite types in a single transaction.
// A custom partner_name may differ from the source name (e.g. 7 vs 16); the dotted
// shorthand is only a default and must never be forced on manually selected pairs.
func (h *controller) r11SchedulePairSave(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w, r, "Пара видов графиков сохранена", func(root map[string]json.RawMessage) (map[string]any, string, error) {
		body, err := r11BodyMap(w, r, 2<<20)
		if err != nil { return nil, "", err }
		rows, err := r11Array(root, "schedule_types")
		if err != nil { return nil, "", err }
		id := r11Int(body, "id")
		if id < 0 { return nil, "", fmt.Errorf("неверный идентификатор графика") }
		point, name, phase := r11String(body, "collection_point"), r11String(body, "name"), r11String(body, "phase")
		var source map[string]json.RawMessage
		if id > 0 {
			source, _ = r11FindRecordID(rows, id)
			if source == nil { return nil, "", fmt.Errorf("исходный график не найден") }
			point, name, phase = r11String(source, "collection_point"), r11String(source, "name"), r11String(source, "phase")
		}
		if point == "" || name == "" || len(point) > 120 || len(name) > 150 {
			return nil, "", fmt.Errorf("укажите корректные пункт сбора и название первого графика")
		}
		if phase != "A" && phase != "B" {
			return nil, "", fmt.Errorf("выберите рабочую или свободную первую последовательность")
		}
		custom := r11String(body, "partner_name")
		partner := custom
		if partner == "" && id > 0 { partner = r11String(source, "pair_name") }
		if partner == "" {
			partner = name + "."
			if strings.HasSuffix(name, ".") { partner = strings.TrimSuffix(name, ".") }
		}
		if id == 0 && custom == "" && strings.HasSuffix(name, ".") {
			return nil, "", fmt.Errorf("первое название в автоматическом режиме должно быть без точки")
		}
		if partner == "" || strings.EqualFold(partner, name) || len(partner) > 150 {
			return nil, "", fmt.Errorf("выберите другое, отличающееся название парного графика")
		}
		otherPhase := "A"
		if phase == "A" { otherPhase = "B" }

		var target map[string]json.RawMessage
		for _, x := range rows {
			if r11String(x, "collection_point") != point { continue }
			title := r11String(x, "name")
			if strings.EqualFold(title, partner) { target = x }
			if id == 0 && strings.EqualFold(title, name) {
				return nil, "", fmt.Errorf("вид %q уже существует; выберите парный в его строке", name)
			}
		}
		// A previously linked real pair must not be broken by relinking one half.
		if id > 0 {
			old := r11String(source, "pair_name")
			if old != "" && !strings.EqualFold(old, partner) {
				for _, x := range rows {
					if r11String(x, "collection_point") == point &&
					   strings.EqualFold(r11String(x, "name"), old) &&
					   strings.EqualFold(r11String(x, "pair_name"), name) {
						return nil, "", fmt.Errorf("старый партнёр %q уже связан с этим видом; сначала разберите существующую пару", old)
					}
				}
				if custom == "" { return nil, "", fmt.Errorf("для смены пары явно выберите другой график") }
			}
		}
		if target != nil {
			if r11String(target, "phase") != otherPhase {
				return nil, "", fmt.Errorf("у графика %q уже задана неподходящая первая последовательность; данные не изменены", partner)
			}
			old := r11String(target, "pair_name")
			if old != "" && !strings.EqualFold(old, name) {
				return nil, "", fmt.Errorf("график %q уже связан с другим графиком %q", partner, old)
			}
		}
		if id > 0 && target != nil &&
			strings.EqualFold(r11String(source, "pair_name"), partner) &&
			strings.EqualFold(r11String(target, "pair_name"), name) {
			return nil, "", fmt.Errorf("эта пара уже создана")
		}
		active, note := true, ""
		if id > 0 {
			active, note = r11Bool(source, "active"), r11String(source, "note")
		}
		create := func(title, paired, graphPhase string) map[string]json.RawMessage {
			item := map[string]json.RawMessage{}
			r11Put(item, "id", r11NextID(root))
			r11Put(item, "collection_point", point)
			r11Put(item, "name", title)
			r11Put(item, "pair_name", paired)
			r11Put(item, "phase", graphPhase)
			r11Put(item, "active", active)
			r11Put(item, "note", note)
			return item
		}
		added, linked := 0, 0
		if source != nil {
			r11Put(source, "pair_name", partner)
		} else {
			rows = append(rows, create(name, partner, phase))
			added++
		}
		if target != nil {
			r11Put(target, "pair_name", name)
			linked++
		} else {
			rows = append(rows, create(partner, name, otherPhase))
			added++
		}
		r11SetArray(root, "schedule_types", rows)
		return map[string]any{"added": added, "linked": linked, "names": []string{name, partner}},
			fmt.Sprintf("%s · %s ↔ %s", point, name, partner), nil
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

// r11CycleKey reads the legacy R11 note without replacing or erasing it.
func r11CycleKey(point, first, second string) string {
	return point + "|" + first + "|" + second
}
func r11CycleLegacyLabel(first, second string) string {
	a, _ := time.Parse("2006-01-02", first)
	b, _ := time.Parse("2006-01-02", second)
	return "Цикл от " + a.Format("02.01.2006") + " / " + b.Format("02.01.2006")
}
func r11CycleSequence(note string) int {
	if strings.Contains(note, "Первая последовательность") || strings.Contains(note, "LCSEQ:1") { return 1 }
	if strings.Contains(note, "Вторая последовательность") || strings.Contains(note, "LCSEQ:2") { return 2 }
	return 0
}

// r11CycleSave creates/adopts a complete flight cycle with an optional schedule pair
// in one journaled, backed-up transaction. Legacy note-marked flights are preserved,
// never matched to another cycle merely by collection_point and date.
func (h *controller) r11CycleSave(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w, r, "Сохранён цикл полётов и виды графиков", func(root map[string]json.RawMessage) (map[string]any, string, error) {
		body, err := r11BodyMap(w, r, 2<<20)
		if err != nil { return nil, "", err }
		point, first, second := r11String(body,"collection_point"), r11String(body,"first"), r11String(body,"second")
		if point=="" || len(point)>120 || !r11DateOK(first) || !r11DateOK(second) || second<=first {
			return nil,"",fmt.Errorf("укажите пункт сбора и две последовательные опорные даты")
		}
		fst,_:=time.Parse("2006-01-02",first); snd,_:=time.Parse("2006-01-02",second)
		year:=int(r11Int(body,"year"))
		if year!=fst.Year() || snd.Year()!=year || year<2000 || year>2100 || snd.Sub(fst)>28*24*time.Hour {
			return nil,"",fmt.Errorf("первые ДП должны быть в одном году, с интервалом до 28 дней")
		}
		wantTypes:=r11Bool(body,"create_types")
		name,phase:=r11String(body,"name"),r11String(body,"phase")
		if wantTypes && (name==""||len(name)>145||strings.HasSuffix(name,".")||(phase!="A"&&phase!="B")) {
			return nil,"",fmt.Errorf("укажите название графика без точки и первую рабочую/свободную фазу")
		}
		cycles,err:=r11Array(root,"flight_cycles");if err!=nil{return nil,"",err}
		flights,err:=r11Array(root,"flight_dates");if err!=nil{return nil,"",err}
		types,err:=r11Array(root,"schedule_types");if err!=nil{return nil,"",err}
		var cycle map[string]json.RawMessage
		for _,c:=range cycles {
			if r11String(c,"collection_point")==point && r11String(c,"first_anchor")==first && r11String(c,"second_anchor")==second {
				cycle=c;break
			}
		}
		newCycle:=cycle==nil
		if cycle==nil{
			cycle=map[string]json.RawMessage{}
			r11Put(cycle,"id",r11NextID(root))
			r11Put(cycle,"collection_point",point);r11Put(cycle,"first_anchor",first)
			r11Put(cycle,"second_anchor",second);r11Put(cycle,"year",year)
			r11Put(cycle,"created_at",time.Now().Format(time.RFC3339))
			cycles=append(cycles,cycle)
		}
		cid:=r11Int(cycle,"id")
		legacyLabel:=r11CycleLegacyLabel(first,second)
		expected:=[]struct{date string; seq int}{}
		expected=append(expected,struct{date string;seq int}{first,1},struct{date string;seq int}{second,2})
		end:=fmt.Sprintf("%04d-12-31",year)
		for d,q:=snd,2;;{
			d=d.AddDate(0,0,28)
			if d.Format("2006-01-02")>end {break}
			if q==1 {q=2}else{q=1}
			expected=append(expected,struct{date string;seq int}{d.Format("2006-01-02"),q})
		}
		added,linked:=0,0
		for _,v:=range expected {
			var match map[string]json.RawMessage
			for _,x:=range flights {
				if r11String(x,"collection_point")!=point {continue}
				if r11Int(x,"cycle_id")==cid && r11String(x,"date")==v.date {match=x;break}
				// Adopt only records *explicitly* tagged with exactly these anchors.
				if r11Int(x,"cycle_id")==0 && r11String(x,"date")==v.date &&
					strings.Contains(r11String(x,"note"),legacyLabel) {
					if match!=nil {return nil,"",fmt.Errorf("дубли ДП %s при переносе цикла — требуется проверка",v.date)}
					match=x
				}
			}
			if match!=nil {
				oldSeq:=r11Int(match,"sequence")
				if oldSeq>0 && oldSeq!=int64(v.seq) {return nil,"",fmt.Errorf("ДП %s имеет иную последовательность",v.date)}
				if oldSeq==0 && seq:=r11CycleSequence(r11String(match,"note")); seq!=0 && seq!=v.seq {
					return nil,"",fmt.Errorf("ДП %s принадлежит другой последовательности",v.date)
				}
				r11Put(match,"cycle_id",cid);r11Put(match,"sequence",v.seq);linked++
			} else {
				x:=map[string]json.RawMessage{}
				r11Put(x,"id",r11NextID(root));r11Put(x,"collection_point",point)
				r11Put(x,"cycle_id",cid);r11Put(x,"sequence",v.seq);r11Put(x,"date",v.date)
				r11Put(x,"source","Расчёт общего цикла полётов")
				seqText:="Первая последовательность рейса"
				if v.seq==2 {seqText="Вторая последовательность рейса"}
				r11Put(x,"note",seqText+". Фаза определяется видом графика. "+legacyLabel)
				r11Put(x,"created_at",time.Now().Format(time.RFC3339))
				flights=append(flights,x);added++
			}
		}
		// Adopt explicitly labelled corrected legacy dates, too; never mix cycles by city.
		for _,x:=range flights {
			if r11Int(x,"cycle_id")==0 && r11String(x,"collection_point")==point &&
				strings.Contains(r11String(x,"note"),legacyLabel) {
				seq:=r11CycleSequence(r11String(x,"note"))
				if seq!=1&&seq!=2{return nil,"",fmt.Errorf("у даты %s не определена последовательность",r11String(x,"date"))}
				r11Put(x,"cycle_id",cid);r11Put(x,"sequence",seq);linked++
			}
		}
		if wantTypes {
			partner:=name+"."
			var one,two map[string]json.RawMessage
			for _,x:=range types {
				if r11String(x,"collection_point")!=point {continue}
				if strings.EqualFold(r11String(x,"name"),name) {one=x}
				if strings.EqualFold(r11String(x,"name"),partner) {two=x}
			}
			other:="A";if phase=="A" {other="B"}
			for _,check:=range []struct{row map[string]json.RawMessage;title,pair,ph string}{
				{one,name,partner,phase},{two,partner,name,other},
			}{
				if check.row==nil {continue}
				if ex:=r11Int(check.row,"cycle_id");ex!=0&&ex!=cid{
					return nil,"",fmt.Errorf("вид %q уже связан с другим циклом",check.title)
				}
				if v:=r11String(check.row,"phase");v!=""&&v!=check.ph{
					return nil,"",fmt.Errorf("у вида %q другая фаза; ничего не изменено",check.title)
				}
				if v:=r11String(check.row,"pair_name");v!=""&&v!=check.pair{
					return nil,"",fmt.Errorf("вид %q уже состоит в другой паре",check.title)
				}
			}
			makeType:=func(title,paired,ph string,old map[string]json.RawMessage) {
				x:=old
				if x==nil {
					x=map[string]json.RawMessage{}
					r11Put(x,"id",r11NextID(root));r11Put(x,"collection_point",point)
					r11Put(x,"name",title);r11Put(x,"active",true);types=append(types,x)
				}
				r11Put(x,"pair_name",paired);r11Put(x,"phase",ph);r11Put(x,"cycle_id",cid)
			}
			makeType(name,partner,phase,one);makeType(partner,name,other,two)
		}
		r11SetArray(root,"flight_cycles",cycles)
		r11SetArray(root,"flight_dates",flights)
		r11SetArray(root,"schedule_types",types)
		return map[string]any{"cycle_id":cid,"new_cycle":newCycle,"flights_added":added,"flights_linked":linked,"schedule_pair":wantTypes,"flights":len(expected)},
			fmt.Sprintf("%s · %s / %s · ДП %d · новых %d · пара %v",point,first,second,len(expected),added,wantTypes),nil
	})
}

// r11CycleFlightMove changes exactly one persisted flight, never the other dates,
// the stable cycle ID, or employee plans. The original anchors remain audit metadata.
func (h *controller) r11CycleFlightMove(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Исправлена дата одного рейса",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,2<<20);if err!=nil{return nil,"",err}
		id:=r11Int(body,"id");newDate:=r11String(body,"date");reason:=r11String(body,"reason")
		if id<=0||!r11DateOK(newDate)||len(reason)<3||len(reason)>300 {
			return nil,"",fmt.Errorf("укажите рейс, корректную новую дату и причину")
		}
		rows,err:=r11Array(root,"flight_dates");if err!=nil{return nil,"",err}
		row,_:=r11FindRecordID(rows,id)
		if row==nil||r11Int(row,"cycle_id")<=0{
			return nil,"",fmt.Errorf("рейс не привязан к конкретному циклу; сначала привяжите цикл")
		}
		old:=r11String(row,"date");cid:=r11Int(row,"cycle_id")
		if newDate==old{return nil,"",fmt.Errorf("дата не изменилась")}
		cycles,err:=r11Array(root,"flight_cycles");if err!=nil{return nil,"",err}
		cycle,_:=r11FindRecordID(cycles,cid)
		if cycle==nil{return nil,"",fmt.Errorf("цикл рейса не найден")}
		if newDate[:4]!=old[:4] {
			return nil,"",fmt.Errorf("перенос через границу года требует отдельного согласования")
		}
		var previous,next string
		for _,x:=range rows{
			if r11Int(x,"cycle_id")!=cid||r11Int(x,"id")==id{continue}
			d:=r11String(x,"date")
			if d==newDate{return nil,"",fmt.Errorf("в этом цикле дата %s уже существует",newDate)}
			if d<old && d>previous {previous=d}
			if d>old && (next==""||d<next) {next=d}
		}
		if (previous!=""&&newDate<=previous)||(next!=""&&newDate>=next){
			return nil,"",fmt.Errorf("перенос нарушает последовательность рейсов: соседние даты %s и %s",previous,next)
		}
		original:=r11String(row,"adjusted_from")
		if original==""{r11Put(row,"adjusted_from",old)}
		r11Put(row,"date",newDate)
		r11Put(row,"adjustment_reason",reason)
		r11Put(row,"updated_at",time.Now().Format(time.RFC3339))
		r11SetArray(root,"flight_dates",rows)
		return map[string]any{"id":id,"cycle_id":cid,"old_date":old,"new_date":newDate,"plans_modified":false},
			fmt.Sprintf("Цикл #%d · %s → %s · причина: %s · планы сотрудников не изменены",cid,old,newDate,reason),nil
	})
}

func (h *controller) r11FlightSave(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Дата полёта сохранена",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,2<<20);if err!=nil{return nil,"",err}
		point,date:=r11String(body,"collection_point"),r11String(body,"date");if point==""||!r11DateOK(date){return nil,"",fmt.Errorf("укажите пункт сбора и корректную дату")}
		rows,err:=r11Array(root,"flight_dates");if err!=nil{return nil,"",err}
		id:=r11Int(body,"id");pos:=-1
		for i,x:=range rows{if id>0&&r11Int(x,"id")==id{pos=i;break};if id==0&&r11String(x,"collection_point")==point&&r11String(x,"date")==date{pos=i;break}}
		if pos>=0 {old:=rows[pos];if date!=r11String(old,"date")&&(r11Int(old,"cycle_id")>0||strings.Contains(r11String(old,"note"),"Цикл от ")) {return nil,"",fmt.Errorf("перенос циклового рейса выполняется через «Изменить рейс» внутри цикла")}};item:=map[string]json.RawMessage{};if pos>=0{item=rows[pos]}else{r11Put(item,"id",r11NextID(root))}
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
	h.r11Mutate(w, r, "Контрольная версия графика восстановлена", func(root map[string]json.RawMessage) (map[string]any, string, error) {
		body, err := r11BodyMap(w, r, 1<<20)
		if err != nil { return nil, "", err }
		versions, err := r11Array(root, "graph_versions")
		if err != nil { return nil, "", err }
		v, _ := r11FindRecordID(versions, r11Int(body, "id"))
		if v == nil { return nil, "", fmt.Errorf("версия не найдена") }
		employeeID, year := r11Int(v, "employee_id"), int(r11Int(v, "year"))
		if employeeID < 1 || year < 2000 || year > 2100 {
			return nil, "", fmt.Errorf("у версии неверный сотрудник или год")
		}
		raw, exists := v["days"]
		if !exists || len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
			return nil, "", fmt.Errorf("у версии отсутствует снимок дней; восстановление отменено")
		}
		var snapshot []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &snapshot); err != nil || snapshot == nil {
			return nil, "", fmt.Errorf("снимок дней повреждён или не является массивом: %v", err)
		}
		prefix := fmt.Sprintf("%04d-", year)
		seenDates := map[string]bool{}
		seenIDs := map[int64]bool{}
		for _, d := range snapshot {
			date, id := r11String(d, "date"), r11Int(d, "id")
			if r11Int(d, "employee_id") != employeeID || !r11DateOK(date) || !strings.HasPrefix(date, prefix) {
				return nil, "", fmt.Errorf("снимок содержит день другого сотрудника или года")
			}
			if id < 1 || seenDates[date] || seenIDs[id] {
				return nil, "", fmt.Errorf("снимок содержит отсутствующий ID или повторяющийся день")
			}
			seenDates[date], seenIDs[id] = true, true
		}
		days, err := r11Array(root, "days")
		if err != nil { return nil, "", err }
        employees,err:=r11Array(root,"employees");if err!=nil{return nil,"",err}
        emp,_:=r11FindEmployeeID(employees,employeeID);if emp==nil{return nil,"",fmt.Errorf("сотрудник восстановленной версии не найден")}
        snapshot,err=r11SafeRestoreDays(emp,employeeID,days,snapshot)
        if err!=nil{return nil,"",err}
		kept := make([]map[string]json.RawMessage, 0, len(days)+len(snapshot))
		keptIDs := map[int64]bool{}
		for _, d := range days {
			if r11Int(d, "employee_id") == employeeID && strings.HasPrefix(r11String(d, "date"), prefix) {
                // After termination history is immutable; it is supplied unmodified by r11SafeRestoreDays.
                if fire:=r11String(emp,"fire_date");fire!=""&&r11String(d,"date")>fire { continue }
                continue
            }
			kept = append(kept, d)
			if id := r11Int(d, "id"); id > 0 { keptIDs[id] = true }
		}
		for _, d := range snapshot {
			if keptIDs[r11Int(d, "id")] {
				return nil, "", fmt.Errorf("ID дня из снимка занят другой записью; восстановление отменено")
			}
		}
		kept = append(kept, snapshot...)
		r11SetArray(root, "days", kept)
		return map[string]any{"id": r11Int(v, "id"), "restored": len(snapshot)}, fmt.Sprintf("ID %d · %d дней", r11Int(v, "id"), len(snapshot)), nil
	})
}

func (h *controller) r11ScheduleChange(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w, r, "Изменение вида графика зафиксировано", func(root map[string]json.RawMessage) (map[string]any, string, error) {
		body, err := r11BodyMap(w, r, 2<<20); if err != nil { return nil, "", err }
		employees, err := r11Array(root, "employees"); if err != nil { return nil, "", err }
		emp, _ := r11FindEmployee(employees, r11String(body, "tab_no")); if emp == nil { return nil, "", fmt.Errorf("сотрудник не найден") }
		date, newSchedule := r11String(body, "effective_date"), r11String(body, "new_schedule")
		if !r11DateOK(date) || newSchedule == "" { return nil, "", fmt.Errorf("укажите дату и новый вид графика") }
        if err:=r11PlanDateAllowed(emp,date);err!=nil{return nil,"",err}
		old := r11String(emp, "schedule_type")
		if old == newSchedule {
			revisions, e := r11Array(root, "graph_revisions")
			if e == nil {
				for _, rev := range revisions { if r11Int(rev, "employee_id") == r11Int(emp, "id") && strings.EqualFold(r11String(rev, "status"), "ACTIVE") && r11String(rev, "schedule_type") != "" { old = r11String(rev, "schedule_type"); break } }
			}
		}
		days, err := r11Array(root, "days"); if err != nil { return nil, "", err }
		monthStart := date[:7] + "-01"; hoursBefore := 0.0
		for _, d := range days { dayDate := r11String(d, "date"); if r11Int(d, "employee_id") == r11Int(emp, "id") && dayDate >= monthStart && dayDate < date { hoursBefore += r11Float(d, "plan_hours") } }
		rows, err := r11Array(root, "schedule_changes"); if err != nil { return nil, "", err }
		item := map[string]json.RawMessage{}
		r11Put(item, "id", r11NextID(root)); r11Put(item, "employee_id", r11Int(emp, "id"))
		r11Put(item, "effective_date", date); r11Put(item, "old_schedule", old); r11Put(item, "new_schedule", newSchedule)
		r11Put(item, "hours_before", hoursBefore); r11Put(item, "reason", r11String(body, "reason")); r11Put(item, "created_at", time.Now().Format(time.RFC3339))
		rows = append(rows, item); r11SetArray(root, "schedule_changes", rows)
		r11Put(emp, "schedule_type", newSchedule); r11Put(emp, "updated_at", time.Now().Format(time.RFC3339)); r11SetArray(root, "employees", employees)
		return map[string]any{"id": r11Int(item, "id"), "hours_before": hoursBefore}, fmt.Sprintf("%s · %s → %s · %s", r11String(emp, "tab_no"), old, newSchedule, date), nil
	})
}

func (h *controller) r11RevisionCreate(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w, r, "Официальная редакция графика создана", func(root map[string]json.RawMessage) (map[string]any, string, error) {
		body, err := r11BodyMap(w, r, 4<<20)
		if err != nil { return nil, "", err }
		employees, err := r11Array(root, "employees"); if err != nil { return nil, "", err }
		emp, _ := r11FindEmployee(employees, r11String(body, "tab_no")); if emp == nil { return nil, "", fmt.Errorf("сотрудник не найден") }
		effective := r11String(body, "effective_date"); if !r11DateOK(effective) { return nil, "", fmt.Errorf("неверная дата начала редакции") }
        if err:=r11PlanDateAllowed(emp,effective);err!=nil{return nil,"",err}
		rows, err := r11Array(root, "graph_revisions"); if err != nil { return nil, "", err }
		days, err := r11Array(root, "days"); if err != nil { return nil, "", err }
		events, err := r11Array(root, "events"); if err != nil { return nil, "", err }
		notes, err := r11Array(root, "notes"); if err != nil { return nil, "", err }
		colors, err := r11Array(root, "manual_colors"); if err != nil { return nil, "", err }
		employeeID := r11Int(emp, "id")
		filter := func(src []map[string]json.RawMessage) []map[string]json.RawMessage {
			out := []map[string]json.RawMessage{}
			for _, x := range src { if r11Int(x, "employee_id") == employeeID { out = append(out, x) } }
			return out
		}
		maxNo, parent := int64(0), int64(0)
		for _, old := range rows {
			if r11Int(old, "employee_id") != employeeID { continue }
			if n := r11Int(old, "number"); n > maxNo { maxNo = n }
			if strings.EqualFold(r11String(old, "status"), "ACTIVE") {
				parent = r11Int(old, "id")
				// The archived revision is a historical snapshot. Do not replace
				// it with the currently edited days when a new revision is created.
				r11Put(old, "status", "ARCHIVED")
				r11Put(old, "updated_at", time.Now().Format(time.RFC3339))
			}
		}
		item := map[string]json.RawMessage{}
		r11Put(item, "id", r11NextID(root)); r11Put(item, "employee_id", employeeID)
		r11Put(item, "number", maxNo+1); r11Put(item, "effective_date", effective)
		r11Put(item, "control_date", r11String(body, "control_date"))
		r11Put(item, "fixed_hours", r11Float(body, "fixed_hours"))
		r11Put(item, "schedule_type", r11String(body, "schedule_type"))
		r11Put(item, "reason", r11String(body, "reason")); r11Put(item, "label", r11String(body, "label"))
		r11Put(item, "parent_id", parent); r11Put(item, "status", "ACTIVE")
		now := time.Now().Format(time.RFC3339)
		r11Put(item, "activated_at", now); r11Put(item, "created_at", now); r11Put(item, "updated_at", now)
		r11Put(item, "days_snapshot", filter(days)); r11Put(item, "events_snapshot", filter(events))
		r11Put(item, "notes_snapshot", filter(notes)); r11Put(item, "colors_snapshot", filter(colors))
		rows = append(rows, item); r11SetArray(root, "graph_revisions", rows)
		return map[string]any{"id": r11Int(item, "id"), "number": maxNo+1, "status": "ACTIVE"}, fmt.Sprintf("%s · редакция №%d · %s", r11String(emp, "tab_no"), maxNo+1, effective), nil
	})
}

func (h *controller) r11RevisionGet(w http.ResponseWriter, r *http.Request) {
	if r.Method!=http.MethodGet{http.Error(w,"GET only",405);return}
	id,err:=strconv.ParseInt(r.URL.Query().Get("id"),10,64);if err!=nil||id<1{http.Error(w,"неверный id редакции",400);return}
	h.opMu.Lock();root,e:=h.r11LoadRootLocked();h.opMu.Unlock();if e!=nil{http.Error(w,e.Error(),500);return}
	rows,e:=r11Array(root,"graph_revisions");if e!=nil{http.Error(w,e.Error(),500);return};item,_:=r11FindRecordID(rows,id);if item==nil{http.Error(w,"редакция не найдена",404);return}
	writeJSON(w,item)
}

// r11RevisionActivate restores the archived employee-specific collections.
// Updating only the revision status would make a cancelled revision appear
// active while leaving the later plan in place. It must be an atomic restore.
func (h *controller) r11RevisionActivate(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w, r, "Восстановлена официальная редакция графика", func(root map[string]json.RawMessage) (map[string]any, string, error) {
		body, err := r11BodyMap(w, r, 1<<20)
		if err != nil { return nil, "", err }
		revisions, err := r11Array(root, "graph_revisions")
		if err != nil { return nil, "", err }
		id := r11Int(body, "id")
		target, _ := r11FindRecordID(revisions, id)
		if target == nil { return nil, "", fmt.Errorf("редакция с id=%d не найдена", id) }
		employeeID := r11Int(target, "employee_id")
		if employeeID < 1 { return nil, "", fmt.Errorf("у редакции нет сотрудника") }
		if strings.EqualFold(r11String(target, "status"), "ACTIVE") {
			return map[string]any{"id":id,"already_active":true},fmt.Sprintf("Редакция ID %d уже действующая",id),nil
		}

		collections := []struct{ Key, Snapshot string }{
			{"days","days_snapshot"},
			{"events","events_snapshot"},
			{"notes","notes_snapshot"},
			{"manual_colors","colors_snapshot"},
		}
		current := map[string][]map[string]json.RawMessage{}
		snapshot := map[string][]map[string]json.RawMessage{}
		for _, col := range collections {
			rows, e := r11Array(root, col.Key)
			if e != nil { return nil, "", e }
			current[col.Key] = rows
			raw, ok := target[col.Snapshot]
			if !ok || string(raw) == "null" { return nil, "", fmt.Errorf("у редакции ID %d отсутствует снимок %s; восстановление отменено",id,col.Snapshot) }
			var snap []map[string]json.RawMessage
			if e := json.Unmarshal(raw, &snap); e != nil { return nil, "", fmt.Errorf("снимок %s повреждён: %w",col.Snapshot,e) }
			if err := r11ValidateRevisionSnapshotRows(col.Key, snap, rows, employeeID); err != nil {
				return nil, "", err
			}
			if snap==nil { snap=[]map[string]json.RawMessage{} }
			snapshot[col.Key]=snap
		}
		// Retired employees may have historic rows after their termination date.
        // Preserve them unchanged when activating an archived revision.
        employeesForCutoff,err:=r11Array(root,"employees");if err!=nil{return nil,"",err}
        empForCutoff,_:=r11FindEmployeeID(employeesForCutoff,employeeID)
        if empForCutoff==nil{return nil,"",fmt.Errorf("сотрудник восстановленной редакции не найден")}
        snapshot["days"],err=r11SafeRestoreDays(empForCutoff,employeeID,current["days"],snapshot["days"])
        if err!=nil{return nil,"",err}
		now:=time.Now().Format(time.RFC3339)
		for _, rev := range revisions {
			if r11Int(rev,"employee_id")!=employeeID || r11Int(rev,"id")==id { continue }
			if !strings.EqualFold(r11String(rev,"status"),"ACTIVE") { continue }
			for _, col := range collections {
				owned:=[]map[string]json.RawMessage{}
				for _,item:=range current[col.Key] { if r11Int(item,"employee_id")==employeeID { owned=append(owned,item) } }
				r11Put(rev,col.Snapshot,owned)
			}
			r11Put(rev,"status","ARCHIVED")
			r11Put(rev,"updated_at",now)
		}
		counts:=map[string]int{}
		for _, col := range collections {
			kept:=make([]map[string]json.RawMessage,0,len(current[col.Key])+len(snapshot[col.Key]))
			for _,item:=range current[col.Key] { if r11Int(item,"employee_id")!=employeeID { kept=append(kept,item) } }
			kept=append(kept,snapshot[col.Key]...)
			r11SetArray(root,col.Key,kept)
			counts[col.Key]=len(snapshot[col.Key])
		}
		r11Put(target,"status","ACTIVE")
		r11Put(target,"activated_at",now)
		r11Put(target,"updated_at",now)
		r11SetArray(root,"graph_revisions",revisions)
		employees,err:=r11Array(root,"employees");if err!=nil{return nil,"",err}
		emp,_:=r11FindEmployeeID(employees,employeeID)
		if emp!=nil {
			if schedule:=r11String(target,"schedule_type");schedule!=""{r11Put(emp,"schedule_type",schedule)}
			r11Put(emp,"updated_at",now);r11SetArray(root,"employees",employees)
		}
		return map[string]any{"id":id,"employee_id":employeeID,"restored":counts},
			fmt.Sprintf("ID %d · сотрудник %d · восстановлено дней %d",id,employeeID,counts["days"]),nil
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
	h.r11Mutate(w, r, "Диапазон примечания разделён", func(root map[string]json.RawMessage) (map[string]any, string, error) {
		body, err := r11BodyMap(w, r, 1<<20); if err != nil { return nil, "", err }
		rows, err := r11Array(root, "notes"); if err != nil { return nil, "", err }
		id, date := r11Int(body, "id"), r11String(body, "date")
		if !r11DateOK(date) { return nil, "", fmt.Errorf("неверная дата") }
		item, _ := r11FindRecordID(rows, id); if item == nil { return nil, "", fmt.Errorf("примечание не найдено") }
		a, b := r11String(item, "date_from"), r11String(item, "date_to")
		if date < a || date > b { return nil, "", fmt.Errorf("дата вне диапазона примечания") }
		if a == b { return nil, "", fmt.Errorf("нельзя разделить однодневное примечание") }
		standalone := map[string]json.RawMessage{}
		for k, v := range item { standalone[k] = append(json.RawMessage(nil), v...) }
		r11Put(standalone, "id", r11NextID(root)); r11Put(standalone, "date_from", date); r11Put(standalone, "date_to", date)
		r11Put(standalone, "created_at", time.Now().Format(time.RFC3339)); r11Put(standalone, "updated_at", time.Now().Format(time.RFC3339))
		if date == a {
			r11Put(item, "date_from", r11DateAdd(date, 1))
		} else if date == b {
			r11Put(item, "date_to", r11DateAdd(date, -1))
		} else {
			right := map[string]json.RawMessage{}
			for k, v := range item { right[k] = append(json.RawMessage(nil), v...) }
			r11Put(item, "date_to", r11DateAdd(date, -1))
			r11Put(right, "id", r11NextID(root)); r11Put(right, "date_from", r11DateAdd(date, 1))
			r11Put(right, "created_at", time.Now().Format(time.RFC3339)); r11Put(right, "updated_at", time.Now().Format(time.RFC3339))
			rows = append(rows, right)
		}
		r11Put(item, "updated_at", time.Now().Format(time.RFC3339))
		rows = append(rows, standalone); r11SetArray(root, "notes", rows)
		return map[string]any{"id": id, "standalone_id": r11Int(standalone, "id")}, fmt.Sprintf("ID %d · %s", id, date), nil
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
		validColors:=map[string]bool{"yellow":true,"green":true,"peach":true,"pink":true,"gray":true}
		if !validColors[color]{return nil,"",fmt.Errorf("неизвестный цвет")}
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
	if r.Method != http.MethodPost { http.Error(w,"POST only",405); return }
	if err:=r.ParseMultipartForm(32<<20);err!=nil{http.Error(w,err.Error(),400);return}
	file,header,err:=r.FormFile("file");if err!=nil{http.Error(w,err.Error(),400);return};defer file.Close()
	raw,err:=io.ReadAll(io.LimitReader(file,32<<20));if err!=nil{http.Error(w,err.Error(),400);return}
	rows,err:=r11ReadCSV(raw);if err!=nil{http.Error(w,err.Error(),400);return};rows=filterNonEmptyRows(rows);if len(rows)<2{http.Error(w,"CSV пуст",400);return}
	headers:=rows[0]
	get:=func(row []string,k string)string{
		aliases:=map[string][]string{
			"tab_no":{"tab_no","tabno","табельный номер","табельный","таб №"},
			"date":{"date","дата"},
			"date_from":{"date_from","дата с","начало периода","дата начала"},
			"date_to":{"date_to","дата по","окончание периода","дата окончания"},
			"value":{"value","факт","значение","код факта"},
			"hours":{"hours","часы","факт часы","количество часов"},
			"shift":{"shift","смена"},
			"event_type":{"event_type","вид события","тип события","событие"},
			"source_code":{"source_code","код события","код"},
			"source":{"source","источник"},
			"note":{"note","примечание","комментарий"},
		}
		return r11Cell(row,r11HeaderIndex(headers,aliases[k]...))
	}
	if r11HeaderIndex(headers,"tab_no","tabno","табельный номер","табельный","таб №")<0 {http.Error(w,"Не найдена колонка табельного номера",400);return}
	if eventsOnly {
		if r11HeaderIndex(headers,"date_from","дата с","начало периода","дата начала","date","дата")<0 || r11HeaderIndex(headers,"event_type","вид события","тип события","событие")<0 {http.Error(w,"Для импорта событий нужны дата и вид события",400);return}
	} else if r11HeaderIndex(headers,"date","дата")<0 || r11HeaderIndex(headers,"value","факт","значение","код факта")<0 {http.Error(w,"Для импорта факта нужны дата и значение факта",400);return}
	h.opMu.Lock();defer h.opMu.Unlock()
	root,err:=h.r11LoadRootLocked();if err!=nil{http.Error(w,err.Error(),500);return}
	employees,err:=r11Array(root,"employees");if err!=nil{http.Error(w,err.Error(),500);return}
	days,err:=r11Array(root,"days");if err!=nil{http.Error(w,err.Error(),500);return}
	events,err:=r11Array(root,"events");if err!=nil{http.Error(w,err.Error(),500);return}
	// Repeated import of the same SAP/ISAT report must not duplicate an event.
	csvEventKey:=func(e map[string]json.RawMessage)string{
		return strings.Join([]string{
			strconv.FormatInt(r11Int(e,"employee_id"),10),r11String(e,"date_from"),
			r11String(e,"date_to"),r11String(e,"event_type"),
			r11String(e,"source_code"),r11String(e,"source"),r11String(e,"note"),
		},"\\x00")
	}
	seenCSVEvents:=map[string]bool{}
	for _,e:=range events{seenCSVEvents[csvEventKey(e)]=true}
	applied:=0
	for rowIndex,row:=range rows[1:]{
		emp,_:=r11FindEmployee(employees,get(row,"tab_no"));if emp==nil{continue}
		if eventsOnly {
			a:=r11NormalizeDate(get(row,"date_from"));if a==""{a=r11NormalizeDate(get(row,"date"))}
			b:=r11NormalizeDate(get(row,"date_to"));if b==""{b=a};if !r11DateOK(a)||!r11DateOK(b)||b<a{continue}
			item:=map[string]json.RawMessage{};r11Put(item,"employee_id",r11Int(emp,"id"));r11Put(item,"date_from",a);r11Put(item,"date_to",b);r11Put(item,"event_type",get(row,"event_type"));r11Put(item,"source_code",get(row,"source_code"));r11Put(item,"source",get(row,"source"));r11Put(item,"note",get(row,"note"))
			key:=csvEventKey(item);if seenCSVEvents[key]{continue}
			r11Put(item,"id",r11NextID(root));r11Put(item,"created_at",time.Now().Format(time.RFC3339));events=append(events,item);seenCSVEvents[key]=true;applied++
		} else {
			date:=r11NormalizeDate(get(row,"date"));if !r11DateOK(date){continue};pos:=-1
			for i,d:=range days{if r11Int(d,"employee_id")==r11Int(emp,"id")&&r11String(d,"date")==date{pos=i;break}}
			item:=map[string]json.RawMessage{};if pos>=0{item=days[pos]}else{r11Put(item,"id",r11NextID(root));r11Put(item,"employee_id",r11Int(emp,"id"));r11Put(item,"date",date)}
			r11Put(item,"fact",get(row,"value"));if v:=strings.TrimSpace(get(row,"hours"));v!=""{
				n,parseErr:=strconv.ParseFloat(strings.ReplaceAll(v,",","."),64)
				if parseErr!=nil||math.IsNaN(n)||math.IsInf(n,0)||n<0||n>24{
					http.Error(w,fmt.Sprintf("Некорректные часы в строке %d: %q",rowIndex+2,v),http.StatusBadRequest)
					return
				}
				r11Put(item,"fact_hours",n)
			};if v:=get(row,"shift");v!=""{r11Put(item,"shift",v)}
			if pos>=0{days[pos]=item}else{days=append(days,item)};applied++
		}
	}
	if eventsOnly{r11SetArray(root,"events",events)}else{r11SetArray(root,"days",days)}
	skipped:=len(rows)-1-applied
	kind,operation:="FACT","Импорт факта";if eventsOnly{kind="EVENTS";operation="Импорт событий"}
	if err:=r11AppendImportHistory(root,kind,filepath.Base(header.Filename),len(rows)-1,applied,skipped,"");err!=nil{http.Error(w,err.Error(),500);return}
	backup,err:=h.r11SaveRootLocked(root,operation,fmt.Sprintf("%s · строк %d · применено %d · пропущено %d",filepath.Base(header.Filename),len(rows)-1,applied,skipped))
	if err!=nil{http.Error(w,err.Error(),500);return}
	writeJSON(w,map[string]any{"ok":true,"count":applied,"applied":applied,"skipped":skipped,"rows":len(rows)-1,"backup":r11BackupLabel(backup)})
}

func r11AppendImportHistory(root map[string]json.RawMessage,kind,fileName string,rows,applied,skipped int,message string) error {
	items,err:=r11Array(root,"import_history");if err!=nil{return err}
	item:=map[string]json.RawMessage{};r11Put(item,"id",r11NextID(root));r11Put(item,"kind",kind);r11Put(item,"file_name",fileName);r11Put(item,"imported_at",time.Now().Format(time.RFC3339));r11Put(item,"rows",rows);r11Put(item,"applied",applied);r11Put(item,"skipped",skipped);r11Put(item,"message",message)
	items=append(items,item);r11SetArray(root,"import_history",items);return nil
}

func r11HeaderIndex(headers []string, aliases ...string) int {
	wanted:=map[string]bool{}
	for _,a:=range aliases{wanted[r11NormalizeHeader(a)]=true}
	for i,h:=range headers{if wanted[r11NormalizeHeader(h)]{return i}}
	for i,h:=range headers{n:=r11NormalizeHeader(h);for a:=range wanted{if a!=""&&strings.Contains(n,a){return i}}}
	return -1
}

func r11Cell(row []string, index int) string {
	if index<0||index>=len(row){return ""}
	return strings.TrimSpace(row[index])
}

func (h *controller) r11FlightImportPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method!=http.MethodPost{http.Error(w,"POST only",405);return}
	if err:=r.ParseMultipartForm(64<<20);err!=nil{http.Error(w,err.Error(),400);return}
	file,header,err:=r.FormFile("file");if err!=nil{http.Error(w,err.Error(),400);return};defer file.Close()
	raw,err:=io.ReadAll(io.LimitReader(file,64<<20));if err!=nil{http.Error(w,err.Error(),400);return}
	table,sheet,err:=r11ReadImportTable(raw,header.Filename);if err!=nil{http.Error(w,err.Error(),400);return}
	table=filterNonEmptyRows(table);if len(table)<2{http.Error(w,"В файле нет строк с датами полётов",400);return}
	headerRow:=-1;pointCol,dateCol:=-1,-1
	for i:=0;i<len(table)&&i<6;i++{p:=r11HeaderIndex(table[i],"пункт сбора","пункт сбора вылета","пункт","город","collection point","collection_point");d:=r11HeaderIndex(table[i],"дата дп","дата полёта","дата полета","дата","date");if p>=0&&d>=0{headerRow=i;pointCol=p;dateCol=d;break}}
	if headerRow<0{http.Error(w,"Не найдены колонки «Пункт сбора» и «Дата ДП»",400);return}
	headers:=table[headerRow];sourceCol:=r11HeaderIndex(headers,"источник","source");noteCol:=r11HeaderIndex(headers,"примечание","комментарий","note")
	year,_:=strconv.Atoi(r.FormValue("year"))
	h.opMu.Lock();root,loadErr:=h.r11LoadRootLocked();h.opMu.Unlock();if loadErr!=nil{http.Error(w,loadErr.Error(),500);return}
	existing,loadErr:=r11Array(root,"flight_dates");if loadErr!=nil{http.Error(w,loadErr.Error(),500);return}
	existingSet:=map[string]bool{};for _,x:=range existing{existingSet[r11String(x,"collection_point")+"|"+r11String(x,"date")]=true}
	seen:=map[string]bool{};yearsSet:=map[int]bool{};rows:=[]map[string]any{};counts:=map[string]int{"new":0,"existing":0,"duplicate_file":0,"other_year":0,"error":0}
	for i:=headerRow+1;i<len(table);i++{row:=table[i];if len(row)==0{continue};point:=r11Cell(row,pointCol);date:=r11NormalizeDate(r11Cell(row,dateCol));source:=r11Cell(row,sourceCol);if source==""{source="Импорт Excel"};note:=r11Cell(row,noteCol)
		entry:=map[string]any{"row":i+1,"collection_point":point,"date":date,"source":source,"note":note,"status":"new","selectable":true,"default_selected":true,"message":""};status:="new"
		switch{case point==""||date=="":status="error";entry["selectable"]=false;entry["default_selected"]=false;entry["message"]="Не заполнен пункт сбора или дата"
		case year>0&&strings.HasPrefix(date,fmt.Sprintf("%04d-",year))==false:status="other_year";entry["selectable"]=false;entry["default_selected"]=false;entry["message"]="Дата не относится к выбранному году"
		case seen[point+"|"+date]:status="duplicate_file";entry["selectable"]=false;entry["default_selected"]=false;entry["message"]="Такая дата повторяется в файле"
		case existingSet[point+"|"+date]:status="existing";entry["selectable"]=false;entry["default_selected"]=false;entry["message"]="Дата уже есть в базе"}
		if status=="new"{seen[point+"|"+date]=true};if date!=""{if d,e:=time.Parse("2006-01-02",date);e==nil{yearsSet[d.Year()]=true}}
		entry["status"]=status;counts[status]++;rows=append(rows,entry)
	}
	years:=make([]int,0,len(yearsSet));for y:=range yearsSet{years=append(years,y)};sort.Ints(years)
	writeJSON(w,map[string]any{"ok":true,"file_name":filepath.Base(header.Filename),"sheet_name":sheet,"rows":rows,"counts":counts,"years":years})
}

func filterNonEmptyRows(rows [][]string) [][]string {out:=make([][]string,0,len(rows));for _,row:=range rows{nonempty:=false;for _,v:=range row{if strings.TrimSpace(v)!=""{nonempty=true;break}};if nonempty{out=append(out,row)}};return out}

func (h *controller) r11FlightImportConfirm(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Импорт согласованных полётов",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,16<<20);if err!=nil{return nil,"",err};var incoming []map[string]json.RawMessage
		if err:=json.Unmarshal(r11Raw(body,"rows"),&incoming);err!=nil{return nil,"",fmt.Errorf("неверный список полётов: %w",err)};if len(incoming)==0{return nil,"",fmt.Errorf("не выбраны строки для импорта")}
		rows,err:=r11Array(root,"flight_dates");if err!=nil{return nil,"",err};set:=map[string]bool{};for _,x:=range rows{set[r11String(x,"collection_point")+"|"+r11String(x,"date")]=true};added,skipped:=0,0
		for _,x:=range incoming{point,date:=r11String(x,"collection_point"),r11NormalizeDate(r11String(x,"date"));if point==""||date==""{skipped++;continue};key:=point+"|"+date;if set[key]{skipped++;continue}
			item:=map[string]json.RawMessage{};r11Put(item,"id",r11NextID(root));r11Put(item,"collection_point",point);r11Put(item,"date",date);source:=r11String(x,"source");if source==""{source="Импорт Excel"};r11Put(item,"source",source);r11Put(item,"note",r11String(x,"note"));r11Put(item,"created_at",time.Now().Format(time.RFC3339));rows=append(rows,item);set[key]=true;added++}
		r11SetArray(root,"flight_dates",rows);return map[string]any{"added":added,"skipped":skipped},fmt.Sprintf("добавлено %d, пропущено %d",added,skipped),nil
	})
}

func (h *controller) r11EmployeeFlightImportConfirm(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Импорт индивидуальных полётов",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,32<<20);if err!=nil{return nil,"",err};var incoming []map[string]json.RawMessage
		if err:=json.Unmarshal(r11Raw(body,"rows"),&incoming);err!=nil{return nil,"",fmt.Errorf("неверный список индивидуальных полётов: %w",err)};if len(incoming)==0{return nil,"",fmt.Errorf("нет строк для импорта")}
		employees,err:=r11Array(root,"employees");if err!=nil{return nil,"",err};rows,err:=r11Array(root,"employee_flights");if err!=nil{return nil,"",err};added,updated,skipped:=0,0,0
		for _,x:=range incoming{employeeID:=r11Int(x,"employee_id");if employeeID==0&&r11String(x,"tab_no")!=""{e,_:=r11FindEmployee(employees,r11String(x,"tab_no"));if e!=nil{employeeID=r11Int(e,"id")}}
			if _,pos:=r11FindEmployeeID(employees,employeeID);pos<0{skipped++;continue};date:=r11NormalizeDate(r11String(x,"date"));kind:=r11String(x,"kind");if date==""||kind==""{skipped++;continue}
			replaceID:=r11Int(x,"replace_id");pos:=-1
			if replaceID>0{
				existing,idx:=r11FindRecordID(rows,replaceID)
				if idx<0{return nil,"",fmt.Errorf("полёт id=%d для замены не найден",replaceID)}
				if r11Int(existing,"employee_id")!=employeeID{return nil,"",fmt.Errorf("полёт id=%d принадлежит другому сотруднику; замена запрещена",replaceID)}
				pos=idx
			}
			if pos<0{for i,old:=range rows{if r11Int(old,"employee_id")==employeeID&&r11String(old,"date")==date&&strings.EqualFold(r11String(old,"kind"),kind){pos=i;break}}}
			item:=map[string]json.RawMessage{};if pos>=0{item=rows[pos]}else{r11Put(item,"id",r11NextID(root));r11Put(item,"created_at",time.Now().Format(time.RFC3339))}
			r11Put(item,"employee_id",employeeID);r11Put(item,"date",date);r11Put(item,"kind",kind);r11Put(item,"collection_point",r11String(x,"collection_point"));r11Put(item,"note",r11String(x,"note"));r11Put(item,"source",r11String(x,"source"));r11Put(item,"updated_at",time.Now().Format(time.RFC3339))
			if pos>=0{rows[pos]=item;updated++}else{rows=append(rows,item);added++}}
		r11SetArray(root,"employee_flights",rows);return map[string]any{"added":added,"updated":updated,"skipped":skipped},fmt.Sprintf("%s · добавлено %d, обновлено %d, пропущено %d",r11String(body,"file_name"),added,updated,skipped),nil
	})
}

func (h *controller) r11EmployeeFlightDelete(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Индивидуальный полёт удалён",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,1<<20);if err!=nil{return nil,"",err};rows,err:=r11Array(root,"employee_flights");if err!=nil{return nil,"",err};id:=r11Int(body,"id");_,pos:=r11FindRecordID(rows,id);if pos<0{return nil,"",fmt.Errorf("индивидуальный полёт не найден")}
		rows=append(rows[:pos],rows[pos+1:]...);r11SetArray(root,"employee_flights",rows);return map[string]any{"id":id},fmt.Sprintf("ID %d",id),nil
	})
}

func (h *controller) r11Demo(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Демо-сотрудник создан",func(root map[string]json.RawMessage)(map[string]any,string,error){
		employees,err:=r11Array(root,"employees");if err!=nil{return nil,"",err};if e,_:=r11FindEmployee(employees,"311267");e!=nil{return map[string]any{"id":r11Int(e,"id"),"exists":true},"Демо-сотрудник 311267 уже существует",nil}
		item:=map[string]json.RawMessage{};r11Put(item,"id",r11NextID(root));r11Put(item,"tab_no","311267");r11Put(item,"full_name","ДЕМО · Водитель 311267");r11Put(item,"position","Водитель");r11Put(item,"unit","Демо");r11Put(item,"collection_point","Тюмень");r11Put(item,"schedule_type","");r11Put(item,"is_quarter",false);r11Put(item,"hire_date","2026-01-01");r11Put(item,"fire_date","");r11Put(item,"probation",false);r11Put(item,"probation_end","");r11Put(item,"main_schedule_ready",false);r11Put(item,"notes","СИНТЕТИЧЕСКИЕ ДЕМО-ДАННЫЕ");r11Put(item,"created_at",time.Now().Format(time.RFC3339))
		employees=append(employees,item);r11SetArray(root,"employees",employees);return map[string]any{"id":r11Int(item,"id")},"Синтетический сотрудник 311267",nil
	})
}

func (h *controller) r11DemoAutograf(w http.ResponseWriter, r *http.Request) {
	h.r11Mutate(w,r,"Демо-график АвтоГраф создан",func(root map[string]json.RawMessage)(map[string]any,string,error){
		body,err:=r11BodyMap(w,r,1<<20);if err!=nil{return nil,"",err};year:=int(r11Int(body,"year"));if year<2000||year>2100{return nil,"",fmt.Errorf("неверный год")}
		employees,err:=r11Array(root,"employees");if err!=nil{return nil,"",err};emp,_:=r11FindEmployee(employees,"311267");if emp==nil{return nil,"",fmt.Errorf("сначала создайте демо-сотрудника")};id:=r11Int(emp,"id")
		days,err:=r11Array(root,"days");if err!=nil{return nil,"",err};for _,d:=range days{if r11Int(d,"employee_id")==id&&strings.HasPrefix(r11String(d,"date"),fmt.Sprintf("%04d-",year)){return nil,"",fmt.Errorf("у демо-сотрудника уже есть график за %d год",year)}}
		types,err:=r11Array(root,"schedule_types");if err!=nil{return nil,"",err};hasType:=false;for _,x:=range types{if r11String(x,"collection_point")=="Тюмень"&&r11String(x,"name")=="Демо A"{hasType=true}}
		if !hasType{for _,phase:=range []string{"A","B"}{x:=map[string]json.RawMessage{};r11Put(x,"id",r11NextID(root));r11Put(x,"collection_point","Тюмень");r11Put(x,"name","Демо "+phase);pair:="Демо B";if phase=="B"{pair="Демо A"};r11Put(x,"pair_name",pair);r11Put(x,"phase",phase);r11Put(x,"active",true);r11Put(x,"note","Синтетический пример");types=append(types,x)};r11SetArray(root,"schedule_types",types)}
		r11Put(emp,"schedule_type","Демо A");r11SetArray(root,"employees",employees)
		flights,err:=r11Array(root,"flight_dates");if err!=nil{return nil,"",err};start:=time.Date(year,1,2,0,0,0,0,time.UTC);second:=start.AddDate(0,0,14);seq:=1
		for d:=start;d.Year()==year;{x:=map[string]json.RawMessage{};r11Put(x,"id",r11NextID(root));r11Put(x,"collection_point","Тюмень");r11Put(x,"date",d.Format("2006-01-02"));r11Put(x,"source","Демо-цикл");note:="Первая последовательность · LCSEQ:1";if seq==2{note="Вторая последовательность · LCSEQ:2"};r11Put(x,"note",note);flights=append(flights,x);if seq==1{d=second}else{d=d.AddDate(0,0,28)};seq=3-seq}
		r11SetArray(root,"flight_dates",flights);seq=1;cursor:=time.Date(year,1,2,0,0,0,0,time.UTC)
		for cursor.Year()==year{phase,plan,hours:="WORK","11",11.0;if seq==2{phase="FREE";plan="В";hours=0};next:=cursor.AddDate(0,0,14);if seq==2{next=cursor.AddDate(0,0,28)}
			for d:=cursor;d.Before(next)&&d.Year()==year;d=d.AddDate(0,0,1){p,h:=plan,hours;if d.Equal(cursor){p="ДП";h=0};x:=map[string]json.RawMessage{};r11Put(x,"id",r11NextID(root));r11Put(x,"employee_id",id);r11Put(x,"date",d.Format("2006-01-02"));r11Put(x,"plan",p);r11Put(x,"plan_hours",h);r11Put(x,"fact","");r11Put(x,"fact_hours",0);r11Put(x,"shift","");r11Put(x,"state",phase);r11Put(x,"comment","СИНТЕТИЧЕСКИЙ ДЕМО-ГРАФИК");days=append(days,x)}
			cursor=next;seq=3-seq}
		r11SetArray(root,"days",days);return map[string]any{"employee_id":id,"year":year},fmt.Sprintf("Демо АвтоГраф · 311267 · %d",year),nil
	})
}
