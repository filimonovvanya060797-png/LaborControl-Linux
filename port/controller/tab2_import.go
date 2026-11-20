package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type uploadFile struct {
	Name string
	Data []byte
}

func readMultipartFile(w http.ResponseWriter, r *http.Request, field string) (uploadFile, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 96<<20)
	if err := r.ParseMultipartForm(96 << 20); err != nil {
		return uploadFile{}, err
	}
	f, h, err := r.FormFile(field)
	if err != nil {
		return uploadFile{}, err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return uploadFile{}, err
	}
	return uploadFile{Name: h.Filename, Data: b}, nil
}
func hashBytes(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:8]) }

type factAgg struct {
	TabNo, Name, Date, Shift string
	Hours                    float64
	Count                    int
}

func factShiftLabel(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DAY", "ДЕНЬ", "1":
		return "день"
	case "NIGHT", "НОЧЬ", "2":
		return "ночь"
	default:
		return "—"
	}
}

func parseFactGrid(rows [][]string, filename string) ([]factAgg, []tab2FactRecord, string, error) {
	rows = compactRows(rows)
	if len(rows) < 2 {
		return nil, nil, "", fmt.Errorf("файл факта пуст")
	}
	hr := -1
	var h map[string]int
	for i := 0; i < len(rows) && i < 25; i++ {
		m := headerIndex(rows[i])
		tab := findCol(m, "Табель", "Табельный номер", "ТабНомер")
		hours := findCol(m, "Часов отработано", "Количество часов", "РсчЧас", "Расчетные часы")
		dt := findCol(m, "Путевой лист С", "Путевой лист с", "Текущая дата", "Дата")
		if tab >= 0 && hours >= 0 && dt >= 0 {
			hr = i
			h = m
			break
		}
	}
	if hr < 0 {
		return nil, nil, "", fmt.Errorf("не найдены обязательные колонки факта: табель, дата и часы")
	}
	tabCol := findCol(h, "Табель", "Табельный номер", "ТабНомер")
	nameCol := findCol(h, "ФИО", "Имя сотрудника или кандидата")
	lastCol := findCol(h, "Фамилия")
	firstCol := findCol(h, "Имя")
	middleCol := findCol(h, "Отчество")
	startCol := findCol(h, "Путевой лист С", "Путевой лист с")
	endCol := findCol(h, "Путевой лист По", "Путевой лист по")
	curDateCol := findCol(h, "Текущая дата", "Дата")
	hoursCol := findCol(h, "Часов отработано", "Количество часов", "РсчЧас", "Расчетные часы")
	typeCol := findCol(h, "Вид времени", "Вид")
	mode := "ИСАТ · путевые листы"
	if startCol < 0 {
		mode = "SAP · основное рабочее время"
	}
	now := time.Now().Format(time.RFC3339)
	m := map[string]*factAgg{}
	var raw []tab2FactRecord
	for _, row := range rows[hr+1:] {
		tab := trimTab(at(row, tabCol))
		if tab == "" || tab == "0" {
			continue
		}
		if typeCol >= 0 {
			typ := strings.TrimSpace(at(row, typeCol))
			if typ != "" && typ != "0003" && typ != "3" {
				continue
			}
		}
		name := at(row, nameCol)
		if name == "" {
			name = joinName(at(row, lastCol), at(row, firstCol), at(row, middleCol))
		}
		rawStart := ""
		rawEnd := ""
		rawDate := ""
		shift := ""
		clock := ""
		if startCol >= 0 {
			rawStart = strings.TrimSpace(at(row, startCol))
			rawDate = rawStart
			if t, ok := parseDateTime(rawStart); ok {
				shift = shiftFromClock(t.Hour())
				clock = t.Format("15:04:05")
			}
			if endCol >= 0 {
				rawEnd = strings.TrimSpace(at(row, endCol))
			}
		}
		if rawDate == "" {
			rawDate = at(row, curDateCol)
		}
		date := parseDate(rawDate)
		if date == "" {
			continue
		}
		hours := parseNumber(at(row, hoursCol))
		if hours < 0 {
			continue
		}
		day := 0
		if t, e := time.Parse("2006-01-02", date); e == nil {
			day = t.Day()
		}
		raw = append(raw, tab2FactRecord{TabNo: tab, FullName: name, Start: rawStart, End: rawEnd, Date: date, Hours: hours, Day: day, Time: clock, Shift: factShiftLabel(shift), SourceFile: filename, ImportedAt: now})
		key := tab + "|" + date
		a := m[key]
		if a == nil {
			a = &factAgg{TabNo: tab, Name: name, Date: date, Shift: shift}
			m[key] = a
		}
		a.Hours += hours
		a.Count++
		if a.Shift == "" {
			a.Shift = shift
		} else if shift != "" && a.Shift != shift {
			a.Shift = ""
		}
	}
	out := make([]factAgg, 0, len(m))
	for _, x := range m {
		out = append(out, *x)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TabNo != out[j].TabNo {
			return out[i].TabNo < out[j].TabNo
		}
		return out[i].Date < out[j].Date
	})
	sort.Slice(raw, func(i, j int) bool {
		if raw[i].Date != raw[j].Date {
			return raw[i].Date < raw[j].Date
		}
		if raw[i].TabNo != raw[j].TabNo {
			return raw[i].TabNo < raw[j].TabNo
		}
		return raw[i].Start < raw[j].Start
	})
	if len(out) == 0 {
		return nil, nil, mode, fmt.Errorf("в файле не найдено строк факта")
	}
	return out, raw, mode, nil
}

func (h *controller) tab2ImportFact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	up, err := readMultipartFile(w, r, "file")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sheets, err := readSpreadsheet(up.Data, up.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var facts []factAgg
	var rawFacts []tab2FactRecord
	mode := ""
	for _, sh := range sheets {
		q, raw, m, e := parseFactGrid(sh.Rows, up.Name)
		if e == nil && len(q) > 0 {
			facts = q
			rawFacts = raw
			mode = m
			break
		}
	}
	if len(facts) == 0 {
		http.Error(w, "не удалось распознать факт в книге", http.StatusBadRequest)
		return
	}

	h.opMu.Lock()
	defer h.opMu.Unlock()
	h.requestCoreShutdown()
	db, err := h.loadMainDB()
	if err != nil {
		_, _ = h.startCore()
		http.Error(w, err.Error(), 500)
		return
	}
	ids := empIDByTab(&db)
	dm := dayMap(&db)
	applied, skipped, created := 0, 0, 0
	for _, f := range facts {
		eid := ids[f.TabNo]
		if eid == 0 {
			skipped++
			continue
		}
		k := dayKey(eid, f.Date)
		d := dm[k]
		if d == nil {
			d = map[string]any{"id": mainAlloc(&db), "employee_id": eid, "date": f.Date, "plan": "", "plan_hours": 0.0, "fact": "", "fact_hours": 0.0, "shift": "", "state": "", "comment": "", "updated_at": time.Now().Format(time.RFC3339)}
			db.Days = append(db.Days, d)
			dm[k] = d
			created++
		}
		d["fact"] = fmtNum(f.Hours)
		d["fact_hours"] = f.Hours
		// Если время начала не попало в диапазоны макроса (06–09 / 18–22),
		// смена специально остаётся пустой: это отдельное несоответствие.
		d["shift"] = f.Shift
		d["updated_at"] = time.Now().Format(time.RFC3339)
		applied++
	}
	sortDays(&db)
	appendJournal(&db, "Проверка табеля: импорт факта", fmt.Sprintf("%s; %s; применено %d, пропущено %d", up.Name, mode, applied, skipped))
	backup, err := h.saveMainDB(db, "импорт факта")
	if err != nil {
		_, _ = h.startCore()
		http.Error(w, err.Error(), 500)
		return
	}

	tab2StoreMu.Lock()
	st, storeErr := h.loadTab2Store()
	if storeErr != nil {
		tab2StoreMu.Unlock()
		_, _ = h.startCore()
		http.Error(w, "не удалось открыть хранилище Проверки табеля: "+storeErr.Error(), 500)
		return
	}
	// Повторная выгрузка за ту же дату заменяет старые строки факта этой даты/сотрудника,
	// чтобы страница «Факт» не копила дубликаты и соответствовала текущему Опер-табелю.
	touched := map[string]bool{}
	for _, f := range rawFacts {
		touched[f.TabNo+"|"+f.Date] = true
	}
	kept := st.Facts[:0]
	for _, old := range st.Facts {
		if !touched[old.TabNo+"|"+old.Date] {
			kept = append(kept, old)
		}
	}
	st.Facts = kept
	for i := range rawFacts {
		rawFacts[i].ID = tab2Alloc(&st)
		st.Facts = append(st.Facts, rawFacts[i])
	}
	sort.Slice(st.Facts, func(i, j int) bool {
		if st.Facts[i].Date != st.Facts[j].Date {
			return st.Facts[i].Date < st.Facts[j].Date
		}
		if st.Facts[i].TabNo != st.Facts[j].TabNo {
			return st.Facts[i].TabNo < st.Facts[j].TabNo
		}
		return st.Facts[i].Start < st.Facts[j].Start
	})
	st.Imports = append(st.Imports, tab2ImportLog{ID: tab2Alloc(&st), Kind: "FACT", FileName: up.Name, ImportedAt: time.Now().Format(time.RFC3339), Rows: len(rawFacts), Applied: applied, Skipped: skipped, Message: mode + "; hash " + hashBytes(up.Data)})
	if len(st.Imports) > 300 {
		st.Imports = st.Imports[len(st.Imports)-300:]
	}
	if err := h.saveTab2Store(st); err != nil {
		tab2StoreMu.Unlock()
		_, _ = h.startCore()
		http.Error(w, "не удалось сохранить факт: "+err.Error(), 500)
		return
	}
	tab2StoreMu.Unlock()
	_, startErr := h.startCore()
	if startErr != nil {
		http.Error(w, "факт сохранён, но внутренний сервер не перезапущен: "+startErr.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "file": up.Name, "mode": mode, "rows": len(rawFacts), "days": len(facts), "applied": applied, "skipped": skipped, "created_days": created, "backup": backup})
}

func parseSZRVDGrid(rows [][]string, filename string) ([]tab2SZRVDRecord, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("лист СЗ РВД пуст")
	}
	now := time.Now().Format(time.RFC3339)
	var out []tab2SZRVDRecord
	// Основной рабочий шаблон СЗ РВД из макроса «НайтиРВД»:
	// K — табельный, L:M — период РВД, P — вид (1/2), Q:R — период отгула.
	for i := 24; i < len(rows); i++ {
		row := rows[i]
		tab := trimTab(at(row, 10))
		if tab == "" || tab == "0" {
			continue
		}
		a := parseDate(at(row, 11))
		b := parseDate(at(row, 12))
		if a == "" || b == "" {
			continue
		}
		if b < a {
			a, b = b, a
		}
		fio := strings.TrimSpace(at(row, 3))
		if fio == "" {
			fio = strings.TrimSpace(at(row, 2))
		}
		typ := strings.TrimSpace(at(row, 15))
		rf := parseDate(at(row, 16))
		rt := parseDate(at(row, 17))
		if rf != "" && rt == "" {
			rt = rf
		}
		if rf != "" && rt != "" && rt < rf {
			rf, rt = rt, rf
		}
		out = append(out, tab2SZRVDRecord{TabNo: tab, FullName: fio, DateFrom: a, DateTo: b, Type: typ, RestFrom: rf, RestTo: rt, SourceFile: filename, ImportedAt: now})
	}
	if len(out) > 0 {
		return out, nil
	}

	// Резервный путь для выгрузок с заголовками вместо исходного шаблона.
	hr := -1
	var h map[string]int
	for i := 0; i < len(rows) && i < 40; i++ {
		m := headerIndex(rows[i])
		if findCol(m, "Таб №", "Табель", "Табельный номер", "ТабНомер") >= 0 {
			hr = i
			h = m
			break
		}
	}
	if hr < 0 {
		return nil, fmt.Errorf("не удалось найти данные СЗ РВД: ожидаются колонки K:R листа «Общий» либо таблица с заголовками")
	}
	tabCol := findCol(h, "Таб №", "Табель", "Табельный номер", "ТабНомер")
	fioCol := findCol(h, "ФИО", "Сотрудник")
	fromCol := findCol(h, "Начало РВД", "Дата с", "Начало", "С")
	toCol := findCol(h, "Конец РВД", "Дата по", "Конец", "По")
	typeCol := findCol(h, "Вид привлечения", "Тип привлечения", "Вид")
	restFromCol := findCol(h, "Отгул с", "Дни отдыха с", "Отдых с")
	restToCol := findCol(h, "Отгул по", "Дни отдыха по", "Отдых по")
	if tabCol < 0 || fromCol < 0 || toCol < 0 {
		return nil, fmt.Errorf("в СЗ РВД не найдены табельный номер и период")
	}
	for _, row := range rows[hr+1:] {
		tab := trimTab(at(row, tabCol))
		a := parseDate(at(row, fromCol))
		b := parseDate(at(row, toCol))
		if tab == "" || a == "" || b == "" {
			continue
		}
		if b < a {
			a, b = b, a
		}
		rf := parseDate(at(row, restFromCol))
		rt := parseDate(at(row, restToCol))
		if rf != "" && rt == "" {
			rt = rf
		}
		out = append(out, tab2SZRVDRecord{TabNo: tab, FullName: strings.TrimSpace(at(row, fioCol)), DateFrom: a, DateTo: b, Type: strings.TrimSpace(at(row, typeCol)), RestFrom: rf, RestTo: rt, SourceFile: filename, ImportedAt: now})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("в СЗ РВД не найдено корректных строк")
	}
	return out, nil
}

func (h *controller) tab2ImportSZRVD(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	up, err := readMultipartFile(w, r, "file")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sheets, err := readSpreadsheet(up.Data, up.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var records []tab2SZRVDRecord
	var parseErr error
	for _, sh := range sheets {
		if !strings.EqualFold(strings.TrimSpace(sh.Name), "Общий") {
			continue
		}
		records, parseErr = parseSZRVDGrid(sh.Rows, up.Name)
		if parseErr == nil && len(records) > 0 {
			break
		}
	}
	if len(records) == 0 {
		for _, sh := range sheets {
			records, parseErr = parseSZRVDGrid(sh.Rows, up.Name)
			if parseErr == nil && len(records) > 0 {
				break
			}
		}
	}
	if len(records) == 0 {
		if parseErr == nil {
			parseErr = fmt.Errorf("не удалось распознать СЗ РВД")
		}
		http.Error(w, parseErr.Error(), http.StatusBadRequest)
		return
	}
	tab2StoreMu.Lock()
	defer tab2StoreMu.Unlock()
	st, err := h.loadTab2Store()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	for i := range records {
		records[i].ID = tab2Alloc(&st)
	}
	// Макрос всегда сверяет с актуальным файлом СЗ РВД. Поэтому новый импорт
	// полностью заменяет предыдущую СЗ, а история самого импорта сохраняется.
	st.SZRVD = records
	sort.Slice(st.SZRVD, func(i, j int) bool {
		if st.SZRVD[i].TabNo != st.SZRVD[j].TabNo {
			return st.SZRVD[i].TabNo < st.SZRVD[j].TabNo
		}
		return st.SZRVD[i].DateFrom < st.SZRVD[j].DateFrom
	})
	st.Imports = append(st.Imports, tab2ImportLog{ID: tab2Alloc(&st), Kind: "SZ_RVD", FileName: up.Name, ImportedAt: time.Now().Format(time.RFC3339), Rows: len(records), Applied: len(records), Message: "лист Общий / колонки K:R; hash " + hashBytes(up.Data)})
	if len(st.Imports) > 300 {
		st.Imports = st.Imports[len(st.Imports)-300:]
	}
	if err := h.saveTab2Store(st); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "file": up.Name, "rows": len(records), "applied": len(records)})
}

func normalizeInfotype(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	r := strings.NewReplacer(".", "_", "-", "_", "/", "_")
	s = r.Replace(s)
	switch s {
	case "2002_020", "2002_0020", "0020", "DP":
		return "2002_0020"
	case "2003_07", "2003_7":
		return "2003_07"
	case "2003_05", "2003_5":
		return "2003_05"
	case "2001":
		return "2001"
	case "2004":
		return "2004"
	case "2002_OTHER", "OTHER", "TECH":
		return "2002_OTHER"
	}
	return s
}

func parseSAPGrid(rows [][]string, infotype, filename string) ([]tab2SAPRecord, error) {
	rows = compactRows(rows)
	if len(rows) < 2 {
		return nil, fmt.Errorf("файл пуст")
	}
	infotype = normalizeInfotype(infotype)
	hr := -1
	var h map[string]int
	for i := 0; i < len(rows) && i < 30; i++ {
		m := headerIndex(rows[i])
		if findCol(m, "ТабНомер", "Табельный номер", "Табель") >= 0 {
			hr = i
			h = m
			break
		}
	}
	if hr < 0 {
		return nil, fmt.Errorf("не найден заголовок с табельным номером")
	}
	tabCol := findCol(h, "ТабНомер", "Табельный номер", "Табель")
	fioCol := findCol(h, "ФИО", "Имя сотрудника или кандидата")
	lastCol := findCol(h, "Фамилия")
	firstCol := findCol(h, "Имя")
	middleCol := findCol(h, "Отчество")
	unitCol := findCol(h, "Вышест.орг.ед.", "КратОргЕд.", "Кр. наим.оргединиц по приказу", "Орг единица", "Автоколонна")
	codeCol := findCol(h, "ВидОтПр", "Вид", "Вид времени")
	textCol := findCol(h, "ТкстВидПрс/Отс", "ТкстВидПрсОтс", "ПдрбТкст/КатегВрм", "Подтип для дежурства", "Тип дня")
	var df, dt, st, et, hrs int = -1, -1, -1, -1, -1
	switch infotype {
	case "2001":
		df = findCol(h, "НачОтсут", "Начало")
		dt = findCol(h, "КонОтсут", "Конец")
		hrs = findCol(h, "РсчЧас", "РасчЧас")
	case "2003_07", "2003_05":
		df = findCol(h, "НачЗам", "Начало")
		dt = findCol(h, "КонЗам", "Конец")
		st = findCol(h, "ВремНач", "Время начала")
		et = findCol(h, "Конец", "Время окончания")
	case "2002_0020", "2002_OTHER":
		df = findCol(h, "Начало", "НачПрис")
		dt = findCol(h, "Конец", "КонПрис")
		hrs = findCol(h, "РсчЧас", "Расчетные часы")
	case "2004":
		df = findCol(h, "НачВых", "Начало")
		dt = findCol(h, "КонВых", "Конец")
		st = findCol(h, "ВремНач", "Время начала")
		et = findCol(h, "Конец", "Время окончания")
	default:
		return nil, fmt.Errorf("неизвестный тип импорта %s", infotype)
	}
	if df < 0 {
		return nil, fmt.Errorf("не найдена дата начала для %s", infotype)
	}
	if dt < 0 {
		dt = df
	}
	now := time.Now().Format(time.RFC3339)
	var out []tab2SAPRecord
	for _, row := range rows[hr+1:] {
		tab := trimTab(at(row, tabCol))
		if tab == "" || tab == "0" {
			continue
		}
		a := parseDate(at(row, df))
		b := parseDate(at(row, dt))
		if a == "" {
			continue
		}
		if b == "" {
			b = a
		}
		name := at(row, fioCol)
		if name == "" {
			name = joinName(at(row, lastCol), at(row, firstCol), at(row, middleCol))
		}
		rec := tab2SAPRecord{Infotype: infotype, TabNo: tab, FullName: name, Unit: at(row, unitCol), DateFrom: a, DateTo: b, Code: strings.TrimSpace(at(row, codeCol)), Text: strings.TrimSpace(at(row, textCol)), StartTime: strings.TrimSpace(at(row, st)), EndTime: strings.TrimSpace(at(row, et)), Hours: parseNumber(at(row, hrs)), SourceFile: filename, ImportedAt: now}
		out = append(out, rec)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("нет распознанных строк %s", infotype)
	}
	return out, nil
}

func mapSAPEvent(r tab2SAPRecord) (string, string) {
	code := strings.TrimSpace(r.Code)
	txt := cleanLower(r.Text)
	switch r.Infotype {
	case "2001":
		switch code {
		case "0102", "0105", "0106", "0170", "0171", "0172", "0173":
			return "VACATION", r.Text
		case "0112", "0113":
			return "UNPAID", r.Text
		case "0160", "0162", "0165", "0167":
			return "SICK", r.Text
		case "0111", "0163":
			return "EXT_VACATION", r.Text
		case "0123", "0124", "0126", "0127":
			return "STUDY", r.Text
		case "0130":
			return "MED", r.Text
		case "1004":
			return "ABSENCE", r.Text
		case "0108":
			return "NO_SHOW", r.Text
		}
		if strings.Contains(txt, "отпуск") && !strings.Contains(txt, "без") {
			return "VACATION", r.Text
		}
		if strings.Contains(txt, "боль") || strings.Contains(txt, "травм") {
			return "SICK", r.Text
		}
		if strings.Contains(txt, "неяв") {
			return "ABSENCE", r.Text
		}
		if strings.Contains(txt, "прогул") {
			return "NO_SHOW", r.Text
		}
		if strings.Contains(txt, "медицин") {
			return "MED", r.Text
		}
		if strings.Contains(txt, "команд") || strings.Contains(txt, "обуч") || strings.Contains(txt, "квал") {
			return "STUDY", r.Text
		}
	case "2002_0020":
		return "DP", r.Text
	case "2004":
		if code == "02" || strings.Contains(txt, "отгул") {
			return "SAP_RVD1", r.Text
		}
		if code == "01" || strings.Contains(txt, "доплат") || strings.Contains(txt, "двойн") {
			return "SAP_RVD2", r.Text
		}
	case "2002_OTHER":
		if code == "0043" || code == "0044" || strings.Contains(txt, "стаж") {
			return "TRAINING", r.Text
		}
		if code == "0045" || strings.Contains(txt, "полев") {
			return "FIELD", r.Text
		}
		if strings.Contains(txt, "ремонт") {
			return "REPAIR", r.Text
		}
		if strings.Contains(txt, "техничес") || strings.Contains(txt, "то ") {
			return "MAINTENANCE", r.Text
		}
	}
	return "", ""
}

func recordClockShift(r tab2SAPRecord) string {
	if r.Infotype != "2003_05" {
		return ""
	}
	h, _, ok := parseClock(r.StartTime)
	if !ok {
		return ""
	}
	return shiftFromClock(h)
}
func recordDurationHours(r tab2SAPRecord) float64 {
	h1, m1, o1 := parseClock(r.StartTime)
	h2, m2, o2 := parseClock(r.EndTime)
	if !o1 || !o2 {
		return r.Hours
	}
	a := h1*60 + m1
	b := h2*60 + m2
	if b <= a {
		b += 24 * 60
	}
	return float64(b-a) / 60
}

func (h *controller) tab2ImportSAP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	up, err := readMultipartFile(w, r, "file")
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	typ := normalizeInfotype(r.FormValue("type"))
	if typ == "" {
		typ = normalizeInfotype(r.URL.Query().Get("type"))
	}
	sheets, err := readSpreadsheet(up.Data, up.Name)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var recs []tab2SAPRecord
	for _, sh := range sheets {
		q, e := parseSAPGrid(sh.Rows, typ, up.Name)
		if e == nil && len(q) > 0 {
			recs = q
			break
		}
	}
	if len(recs) == 0 {
		http.Error(w, "не удалось распознать строки "+typ, 400)
		return
	}
	minD, maxD := recs[0].DateFrom, recs[0].DateTo
	tabs := map[string]bool{}
	for _, x := range recs {
		tabs[x.TabNo] = true
		if x.DateFrom < minD {
			minD = x.DateFrom
		}
		if x.DateTo > maxD {
			maxD = x.DateTo
		}
	}
	h.opMu.Lock()
	defer h.opMu.Unlock()
	h.requestCoreShutdown()
	db, err := h.loadMainDB()
	if err != nil {
		_, _ = h.startCore()
		http.Error(w, err.Error(), 500)
		return
	}
	tab2StoreMu.Lock()
	st, storeErr := h.loadTab2Store()
	if storeErr != nil {
		tab2StoreMu.Unlock()
		_, _ = h.startCore()
		http.Error(w, "не удалось открыть хранилище Табель 2.0: "+storeErr.Error(), 500)
		return
	}
	kept := st.SAPRecords[:0]
	for _, x := range st.SAPRecords {
		if x.Infotype == typ && tabs[x.TabNo] && stringsOverlap(x.DateFrom, x.DateTo, minD, maxD) {
			continue
		}
		kept = append(kept, x)
	}
	st.SAPRecords = kept
	for i := range recs {
		recs[i].ID = tab2Alloc(&st)
		st.SAPRecords = append(st.SAPRecords, recs[i])
	}
	st.Imports = append(st.Imports, tab2ImportLog{ID: tab2Alloc(&st), Kind: typ, FileName: up.Name, ImportedAt: time.Now().Format(time.RFC3339), Rows: len(recs), Applied: len(recs), Message: "hash " + hashBytes(up.Data)})
	if len(st.Imports) > 300 {
		st.Imports = st.Imports[len(st.Imports)-300:]
	}
	if err = h.saveTab2Store(st); err != nil {
		tab2StoreMu.Unlock()
		_, _ = h.startCore()
		http.Error(w, err.Error(), 500)
		return
	}
	tab2StoreMu.Unlock()
	appliedEvents, shiftDays := materializeTab2IntoMain(&db, st, typ, tabs, minD, maxD)
	appendJournal(&db, "Табель 2.0: импорт SAP", fmt.Sprintf("%s; %s; строк %d; событий %d; смен %d", up.Name, typ, len(recs), appliedEvents, shiftDays))
	backup, err := h.saveMainDB(db, "импорт SAP")
	if err != nil {
		_, _ = h.startCore()
		http.Error(w, err.Error(), 500)
		return
	}
	_, startErr := h.startCore()
	if startErr != nil {
		http.Error(w, "данные сохранены, но сервер не перезапущен: "+startErr.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "type": typ, "file": up.Name, "rows": len(recs), "events": appliedEvents, "shift_days": shiftDays, "period_from": minD, "period_to": maxD, "backup": backup})
}

func materializeTab2IntoMain(db *mainDB, st tab2Store, typ string, tabs map[string]bool, minD, maxD string) (int, int) {
	ids := empIDByTab(db)
	dm := dayMap(db)
	// Remove only events created by this integration for the same infotype and overlapping period/tabs.
	prefix := "TAB2:" + typ + ":"
	evOut := db.Events[:0]
	for _, e := range db.Events {
		sc := toString(e["source_code"])
		eid := toInt(e["employee_id"])
		tab := ""
		for t, id := range ids {
			if id == eid {
				tab = t
				break
			}
		}
		if strings.HasPrefix(sc, prefix) && tabs[tab] && stringsOverlap(toString(e["date_from"]), toString(e["date_to"]), minD, maxD) {
			continue
		}
		if strings.HasPrefix(sc, "TAB2:OVERLAP:") && tabs[tab] && stringsOverlap(toString(e["date_from"]), toString(e["date_to"]), minD, maxD) {
			continue
		}
		evOut = append(evOut, e)
	}
	db.Events = evOut
	applied, shiftDays := 0, 0
	for _, r := range st.SAPRecords {
		if r.Infotype != typ || !tabs[r.TabNo] || !stringsOverlap(r.DateFrom, r.DateTo, minD, maxD) {
			continue
		}
		eid := ids[r.TabNo]
		if eid == 0 {
			continue
		}
		if sh := recordClockShift(r); sh != "" {
			for _, date := range dateRange(r.DateFrom, r.DateTo) {
				k := dayKey(eid, date)
				d := dm[k]
				if d == nil {
					d = map[string]any{"id": mainAlloc(db), "employee_id": eid, "date": date, "plan": "", "plan_hours": 0.0, "fact": "", "fact_hours": 0.0, "shift": "", "state": "", "comment": ""}
					db.Days = append(db.Days, d)
					dm[k] = d
				}
				d["shift"] = sh
				d["updated_at"] = time.Now().Format(time.RFC3339)
				shiftDays++
			}
		}
		et, note := mapSAPEvent(r)
		if et == "" {
			continue
		}
		db.Events = append(db.Events, map[string]any{"id": mainAlloc(db), "employee_id": eid, "date_from": r.DateFrom, "date_to": r.DateTo, "event_type": et, "note": note, "source": "TAB2_SAP", "source_code": fmt.Sprintf("TAB2:%s:%d", typ, r.ID), "created_at": time.Now().Format(time.RFC3339)})
		applied++
	}
	materializeOverlaps(db, tabs, minD, maxD)
	sortDays(db)
	return applied, shiftDays
}

func materializeOverlaps(db *mainDB, tabs map[string]bool, minD, maxD string) {
	idTab := empTabByID(db)
	counts := map[string]int{}
	for _, e := range db.Events {
		if toString(e["source"]) != "TAB2_SAP" || toString(e["event_type"]) == "OVERLAP" {
			continue
		}
		tab := idTab[toInt(e["employee_id"])]
		if !tabs[tab] {
			continue
		}
		a, b := toString(e["date_from"]), toString(e["date_to"])
		for _, d := range dateRange(a, b) {
			if dateInRange(d, minD, maxD) {
				counts[tab+"|"+d]++
			}
		}
	}
	ids := empIDByTab(db)
	for k, n := range counts {
		if n < 2 {
			continue
		}
		p := strings.SplitN(k, "|", 2)
		tab, date := p[0], p[1]
		db.Events = append(db.Events, map[string]any{"id": mainAlloc(db), "employee_id": ids[tab], "date_from": date, "date_to": date, "event_type": "OVERLAP", "note": "Наложение событий SAP/ИСАТ", "source": "TAB2_SAP", "source_code": "TAB2:OVERLAP:" + tab + ":" + date, "created_at": time.Now().Format(time.RFC3339)})
	}
}

func (h *controller) tab2ImportHistory(w http.ResponseWriter, r *http.Request) {
	tab2StoreMu.Lock()
	st, err := h.loadTab2Store()
	tab2StoreMu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	q := append([]tab2ImportLog(nil), st.Imports...)
	sort.Slice(q, func(i, j int) bool { return q[i].ID > q[j].ID })
	if len(q) > 100 {
		q = q[:100]
	}
	writeJSON(w, map[string]any{"ok": true, "imports": q})
}

func (h *controller) tab2ExportDebug(w http.ResponseWriter, r *http.Request) { // developer-readable snapshot; no working DB replacement
	tab2StoreMu.Lock()
	st, err := h.loadTab2Store()
	tab2StoreMu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=tab2_debug.json")
	_ = json.NewEncoder(w).Encode(st)
}

func safeBaseName(s string) string { return filepath.Base(strings.ReplaceAll(s, "\\", "/")) }
func intOr(s string, def int) int {
	n, e := strconv.Atoi(strings.TrimSpace(s))
	if e != nil {
		return def
	}
	return n
}