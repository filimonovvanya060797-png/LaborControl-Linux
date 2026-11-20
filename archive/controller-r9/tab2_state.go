package main

import (
	"fmt"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type tab2Discrepancy struct {
	Date      string  `json:"date"`
	TabNo     string  `json:"tab_no"`
	FullName  string  `json:"full_name"`
	Plan      string  `json:"plan"`
	Fact      string  `json:"fact"`
	PlanHours float64 `json:"plan_hours"`
	FactHours float64 `json:"fact_hours"`
	ShiftPlan string  `json:"shift_plan"`
	ShiftFact string  `json:"shift_fact"`
	Problem   string  `json:"problem"`
}

type tab2PlanFactBlock struct {
	EmployeeID int       `json:"employee_id"`
	TabNo      string    `json:"tab_no"`
	FullName   string    `json:"full_name"`
	Unit       string    `json:"unit"`
	Plan       []string  `json:"plan"`
	ShiftPlan  []string  `json:"shift_plan"`
	Fact       []string  `json:"fact"`
	FactHours  []float64 `json:"fact_hours"`
	ShiftFact  []string  `json:"shift_fact"`
	Output     []string  `json:"output"`
	Problems   []string  `json:"problems"`
}

type tab2DPRow struct {
	TabNo      string `json:"tab_no"`
	FullName   string `json:"full_name"`
	Unit       string `json:"unit"`
	Date       string `json:"date"`
	FlightKind string `json:"flight_kind"`
	Plan       string `json:"plan"`
	SAPDP      bool   `json:"sap_dp"`
	RVD        bool   `json:"rvd"`
	Status     string `json:"status"`
	Problem    string `json:"problem"`
}

type tab2RVDRow struct {
	Number      int    `json:"number"`
	TabNo       string `json:"tab_no"`
	FullName    string `json:"full_name"`
	Period      string `json:"period"`
	PlannedDays int    `json:"planned_days"`
	Type        string `json:"type"`
	Actual      string `json:"actual"`
	ActualDays  int    `json:"actual_days"`
	Rest        string `json:"rest"`
	Status      string `json:"status"`
	Problem     string `json:"problem"`
}

func expectedPlanHours(plan string) float64 {
	p := strings.ToUpper(strings.TrimSpace(plan))
	if p == "" {
		return 0
	}
	switch p {
	case "В", "ОТ", "ДП":
		return 0
	}
	if i := strings.LastIndex(p, "/"); i >= 0 && i+1 < len(p) {
		p = p[i+1:]
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(p, ",", "."), 64)
	if err != nil {
		return 0
	}
	return f
}
func planShift(plan string) string {
	p := strings.ToUpper(strings.TrimSpace(plan))
	if strings.HasPrefix(p, "2/") {
		return "ночь"
	}
	if strings.HasPrefix(p, "1/") {
		return "день"
	}
	// Plain hour values do not by themselves encode a shift in the original
	// plan_fact sheet. The planned shift is shown only when the graph carries
	// an explicit 1/N or 2/N designation.
	return "—"
}
func factShift(v any) string {
	switch strings.ToUpper(strings.TrimSpace(toString(v))) {
	case "DAY", "ДЕНЬ", "1":
		return "день"
	case "NIGHT", "НОЧЬ", "2":
		return "ночь"
	}
	return "—"
}
func displayFact(d map[string]any) string {
	if d == nil {
		return "0"
	}
	if s := strings.TrimSpace(toString(d["fact"])); s != "" {
		return s
	}
	return fmtNum(toFloat(d["fact_hours"]))
}
func discrepancyFor(plan string, fact float64, sp, sf string) (string, string) {
	exp := expectedPlanHours(plan)
	out := "ИСТИНА"
	if math.Abs(exp-fact) > .009 {
		out = "ЛОЖЬ"
	}
	planStr := strings.ToUpper(strings.TrimSpace(plan))
	hasPlan := planStr != "" && planStr != "0" && planStr != "В" && planStr != "ОТ" && planStr != "ДП"
	isVacation := planStr == "В" || planStr == "ОТ" || planStr == "ДП"
	hasFact := math.Abs(fact) > .009
	hasShiftPlan := sp != "" && sp != "—"
	hasShiftFact := sf != "" && sf != "—"
	var probs []string
	if hasPlan && !hasFact {
		probs = append(probs, "Нет факта")
	}
	if hasPlan && hasFact && exp > 0 && math.Abs(exp-fact) > .009 {
		probs = append(probs, fmt.Sprintf("Расхождение: план %s | факт %s", fmtNum(exp), fmtNum(fact)))
	}
	if hasShiftPlan && hasShiftFact && sp != sf {
		probs = append(probs, fmt.Sprintf("Расхождение смен: свод %s | исат %s", sp, sf))
	}
	if hasFact && !hasShiftPlan && !isVacation {
		probs = append(probs, "Нет смены свод")
	}
	if hasFact && !hasShiftFact && !isVacation {
		probs = append(probs, "Нет смены исат")
	}
	if len(probs) == 0 {
		return out, ""
	}
	return out, strings.Join(probs, "; ") + ";"
}

func (h *controller) buildPlanFact(db *mainDB, year, month int) ([]tab2PlanFactBlock, []tab2Discrepancy) {
	_, end := monthBounds(year, month)
	daysN, _ := strconv.Atoi(end[8:10])
	dm := dayMap(db)
	var blocks []tab2PlanFactBlock
	var problems []tab2Discrepancy
	for _, e := range db.Employees {
		eid := toInt(e["id"])
		tab := trimTab(toString(e["tab_no"]))
		b := tab2PlanFactBlock{EmployeeID: eid, TabNo: tab, FullName: toString(e["full_name"]), Unit: toString(e["unit"]), Plan: make([]string, daysN), ShiftPlan: make([]string, daysN), Fact: make([]string, daysN), FactHours: make([]float64, daysN), ShiftFact: make([]string, daysN), Output: make([]string, daysN), Problems: make([]string, daysN)}
		for day := 1; day <= daysN; day++ {
			date := fmt.Sprintf("%04d-%02d-%02d", year, month, day)
			d := dm[dayKey(eid, date)]
			plan := ""
			factH := 0.0
			sf := "—"
			if d != nil {
				plan = toString(d["plan"])
				factH = toFloat(d["fact_hours"])
				sf = factShift(d["shift"])
			}
			sp := planShift(plan)
			out, prob := discrepancyFor(plan, factH, sp, sf)
			b.Plan[day-1] = plan
			b.ShiftPlan[day-1] = sp
			b.Fact[day-1] = displayFact(d)
			b.FactHours[day-1] = factH
			b.ShiftFact[day-1] = sf
			b.Output[day-1] = out
			b.Problems[day-1] = prob
			if prob != "" {
				problems = append(problems, tab2Discrepancy{Date: date, TabNo: tab, FullName: b.FullName, Plan: plan, Fact: b.Fact[day-1], PlanHours: expectedPlanHours(plan), FactHours: factH, ShiftPlan: sp, ShiftFact: sf, Problem: prob})
			}
		}
		blocks = append(blocks, b)
	}
	sort.SliceStable(problems, func(i, j int) bool {
		if problems[i].Date != problems[j].Date {
			return problems[i].Date < problems[j].Date
		}
		return problems[i].TabNo < problems[j].TabNo
	})
	return blocks, problems
}

func (h *controller) buildDPChecks(db *mainDB, st tab2Store, year, month int) []tab2DPRow {
	start, end := monthBounds(year, month)
	ids := empTabByID(db)
	empMap := map[int]map[string]any{}
	for _, e := range db.Employees {
		empMap[toInt(e["id"])] = e
	}
	dm := dayMap(db)
	sapDP := map[string]bool{}
	for _, r := range st.SAPRecords {
		if r.Infotype != "2002_0020" {
			continue
		}
		for _, d := range dateRange(r.DateFrom, r.DateTo) {
			if dateInRange(d, start, end) {
				sapDP[r.TabNo+"|"+d] = true
			}
		}
	}
	ar := activeRVDMain(db)
	rvdSet := map[string]bool{}
	for _, r := range ar {
		tab := ids[toInt(r["employee_id"])]
		for _, d := range dateRange(toString(r["date_from"]), toString(r["date_to"])) {
			rvdSet[tab+"|"+d] = true
		}
	}
	seen := map[string]bool{}
	var out []tab2DPRow
	for _, f := range db.Flights {
		date := toString(f["date"])
		if !dateInRange(date, start, end) {
			continue
		}
		eid := toInt(f["employee_id"])
		tab := ids[eid]
		if tab == "" {
			continue
		}
		k := tab + "|" + date
		if seen[k] {
			continue
		}
		seen[k] = true
		e := empMap[eid]
		plan := ""
		if d := dm[dayKey(eid, date)]; d != nil {
			plan = toString(d["plan"])
		}
		p := strings.ToUpper(strings.TrimSpace(plan))
		planOK := p == "ДП" || p == "В"
		actual := sapDP[k]
		hasRVD := rvdSet[k]
		lifeEx := false
		lifeMsg := ""
		hire := toString(e["hire_date"])
		fire := toString(e["fire_date"])
		if hire != "" && date < hire {
			lifeEx = true
			lifeMsg = "Дата полёта до приёма: кандидат/прилёт за свой счёт"
		}
		if fire != "" && date > fire {
			lifeEx = true
			lifeMsg = "Дата полёта после увольнения"
		}
		status := "OK"
		problem := ""
		if lifeEx {
			status = "ИСКЛЮЧЕНИЕ"
			problem = lifeMsg
		} else if hasRVD && !actual {
			status = "ИСКЛЮЧЕНИЕ"
			problem = "На дату стоит РВД — ДП в табеле не ставится"
		} else {
			if !planOK {
				status = "ПРОБЛЕМА"
				problem = "В графике на дату полёта нет ДП/В"
			}
			if !actual {
				status = "ПРОБЛЕМА"
				if problem != "" {
					problem += "; "
				}
				problem += "В SAP/табеле нет ДП (2002/0020)"
			}
		}
		out = append(out, tab2DPRow{TabNo: tab, FullName: toString(e["full_name"]), Unit: toString(e["unit"]), Date: date, FlightKind: toString(f["kind"]), Plan: plan, SAPDP: actual, RVD: hasRVD, Status: status, Problem: problem})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date < out[j].Date
		}
		return out[i].TabNo < out[j].TabNo
	})
	return out
}

func shortDateISO(s string) string {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format("02.01")
	}
	return s
}

func factDateRangeText(dates []string) string {
	if len(dates) == 0 {
		return "—"
	}
	cp := append([]string(nil), dates...)
	sort.Strings(cp)
	// unique
	u := cp[:0]
	for _, d := range cp {
		if len(u) == 0 || u[len(u)-1] != d {
			u = append(u, d)
		}
	}
	cp = u
	if len(cp) == 1 {
		return shortDateISO(cp[0])
	}
	span := inclusiveDays(cp[0], cp[len(cp)-1])
	if span == len(cp) {
		return shortDateISO(cp[0]) + "–" + shortDateISO(cp[len(cp)-1])
	}
	return shortDateISO(cp[0]) + "–" + shortDateISO(cp[len(cp)-1]) + fmt.Sprintf(" (%d дн.)", len(cp))
}

func szTypeLabel(t string) string {
	switch strings.TrimSpace(t) {
	case "1", "01":
		return "1 · одинарная за отгулы"
	case "2", "02":
		return "2 · двойная"
	default:
		if strings.TrimSpace(t) == "" {
			return "—"
		}
		return t
	}
}

func (h *controller) buildRVDChecks(db *mainDB, st tab2Store, year, month int) []tab2RVDRow {
	monthStart, monthEnd := monthBounds(year, month)
	idTab := empTabByID(db)
	knownTabs := map[string]bool{}
	nameByTab := map[string]string{}
	for _, e := range db.Employees {
		t := trimTab(toString(e["tab_no"]))
		if t == "" {
			continue
		}
		knownTabs[t] = true
		nameByTab[t] = toString(e["full_name"])
	}

	// «Фактические дни РВД» ровно как в макросе: в плане В или ДП,
	// а в факте за этот день есть положительные часы.
	actualAll := map[string][]string{}
	actualMonth := map[string][]string{}
	for _, d := range db.Days {
		eid := toInt(d["employee_id"])
		tab := idTab[eid]
		if tab == "" {
			continue
		}
		p := strings.ToUpper(strings.TrimSpace(toString(d["plan"])))
		if p != "В" && p != "ДП" {
			continue
		}
		if toFloat(d["fact_hours"]) <= 0 {
			continue
		}
		date := toString(d["date"])
		if date == "" {
			continue
		}
		actualAll[tab] = append(actualAll[tab], date)
		if dateInRange(date, monthStart, monthEnd) {
			actualMonth[tab] = append(actualMonth[tab], date)
		}
	}
	for tab := range actualAll {
		sort.Strings(actualAll[tab])
	}

	// Актуальная СЗ РВД берётся из отдельного импортированного файла.
	szByTab := map[string][]tab2SZRVDRecord{}
	for _, rec := range st.SZRVD {
		if !stringsOverlap(rec.DateFrom, rec.DateTo, monthStart, monthEnd) {
			continue
		}
		szByTab[rec.TabNo] = append(szByTab[rec.TabNo], rec)
		if nameByTab[rec.TabNo] == "" && rec.FullName != "" {
			nameByTab[rec.TabNo] = rec.FullName
		}
	}
	for tab := range szByTab {
		sort.Slice(szByTab[tab], func(i, j int) bool { return szByTab[tab][i].DateFrom < szByTab[tab][j].DateFrom })
	}

	set := map[string]bool{}
	for tab, dates := range actualMonth {
		if len(dates) > 0 {
			set[tab] = true
		}
	}
	for tab := range szByTab {
		set[tab] = true
	}
	tabs := make([]string, 0, len(set))
	for tab := range set {
		tabs = append(tabs, tab)
	}
	sort.Strings(tabs)

	var out []tab2RVDRow
	n := 0
	for _, tab := range tabs {
		szRows := szByTab[tab]
		actualDates := actualAll[tab]
		monthDates := actualMonth[tab]

		if !knownTabs[tab] {
			// Макрос выводит сотрудников, которые есть в СЗ, но отсутствуют в нашем файле, как «Лишний».
			for _, rec := range szRows {
				n++
				rest := "—"
				if rec.RestFrom != "" {
					rest = shortDateISO(rec.RestFrom) + "–" + shortDateISO(nonEmpty(rec.RestTo, rec.RestFrom))
				}
				out = append(out, tab2RVDRow{Number: n, TabNo: tab, FullName: nameByTab[tab], Period: rec.DateFrom + " — " + rec.DateTo, PlannedDays: inclusiveDays(rec.DateFrom, rec.DateTo), Type: szTypeLabel(rec.Type), Actual: "—", ActualDays: 0, Rest: rest, Status: "Лишний", Problem: "сотрудник есть в СЗ РВД, но отсутствует в Опер-табеле"})
			}
			continue
		}

		if len(szRows) == 0 && len(monthDates) > 0 {
			n++
			out = append(out, tab2RVDRow{Number: n, TabNo: tab, FullName: nameByTab[tab], Period: "—", PlannedDays: 0, Type: "—", Actual: factDateRangeText(monthDates), ActualDays: len(monthDates), Rest: "—", Status: "Нет в СЗ", Problem: "по плану В/ДП есть фактически отработанные часы, но сотрудник не найден в СЗ РВД"})
			continue
		}

		used := map[string]bool{}
		for _, rec := range szRows {
			var inPeriod []string
			for _, d := range actualDates {
				if d >= rec.DateFrom && d <= rec.DateTo {
					inPeriod = append(inPeriod, d)
					used[d] = true
				}
			}
			planned := inclusiveDays(rec.DateFrom, rec.DateTo)
			actualN := len(inPeriod)
			restDays := 0
			rest := "—"
			if rec.RestFrom != "" {
				rt := nonEmpty(rec.RestTo, rec.RestFrom)
				restDays = inclusiveDays(rec.RestFrom, rt)
				rest = shortDateISO(rec.RestFrom) + "–" + shortDateISO(rt)
			}
			status := "OK"
			var probs []string
			typ := strings.TrimLeft(strings.TrimSpace(rec.Type), "0")
			if typ == "" && strings.TrimSpace(rec.Type) != "" {
				typ = rec.Type
			}
			switch typ {
			case "1":
				if planned != restDays {
					probs = append(probs, fmt.Sprintf("РВД: %d дн.; отгул: %d дн. — расхождение %d дн.", planned, restDays, absInt(planned-restDays)))
				}
				if actualN == 0 {
					status = "Нет подтверждения"
					probs = append(probs, "не отработано")
				} else if planned == restDays && actualN == planned {
					status = "Отгулы"
					probs = nil
				} else {
					status = "Расхождение"
					probs = append(probs, fmt.Sprintf("отработано %d из %d дн.", actualN, planned))
				}
			case "2":
				if actualN == 0 {
					status = "Нет подтверждения"
					probs = append(probs, "нет факта за период по СЗ")
				} else if actualN >= planned {
					status = "OK"
				} else {
					status = "Частично"
					probs = append(probs, fmt.Sprintf("факт %d из %d дн. по СЗ", actualN, planned))
				}
			default:
				if actualN == 0 {
					status = "Нет подтверждения"
					probs = append(probs, "нет факта за период по СЗ")
				}
			}
			n++
			out = append(out, tab2RVDRow{Number: n, TabNo: tab, FullName: nameByTab[tab], Period: rec.DateFrom + " — " + rec.DateTo, PlannedDays: planned, Type: szTypeLabel(rec.Type), Actual: factDateRangeText(inPeriod), ActualDays: actualN, Rest: rest, Status: status, Problem: strings.Join(probs, "; ")})
		}

		var outside []string
		for _, d := range monthDates {
			if !used[d] {
				outside = append(outside, d)
			}
		}
		if len(outside) > 0 {
			n++
			out = append(out, tab2RVDRow{Number: n, TabNo: tab, FullName: nameByTab[tab], Period: "—", PlannedDays: 0, Type: "—", Actual: factDateRangeText(outside), ActualDays: len(outside), Rest: "—", Status: "Вне СЗ", Problem: "факт вне периода по СЗ"})
		}
	}
	return out
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func paletteStatKey(eventType string) string {
	switch eventType {
	case "SAP_RVD1":
		return "RVD1"
	case "SAP_RVD2":
		return "RVD2"
	case "TRAINING", "REPAIR", "MAINTENANCE", "FIELD":
		return "TECH"
	default:
		return eventType
	}
}

func (h *controller) tab2State(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", 405)
		return
	}
	now := time.Now()
	year := intOr(r.URL.Query().Get("year"), now.Year())
	month := intOr(r.URL.Query().Get("month"), int(now.Month()))
	if month < 1 || month > 12 {
		month = int(now.Month())
	}
	db, err := h.loadMainDB()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	tab2StoreMu.Lock()
	st, err := h.loadTab2Store()
	tab2StoreMu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	blocks, disc := h.buildPlanFact(&db, year, month)
	dp := h.buildDPChecks(&db, st, year, month)
	rvd := h.buildRVDChecks(&db, st, year, month)
	// Color statistics for imported SAP events and actual shifts.
	start, end := monthBounds(year, month)
	stats := map[string]int{}
	for _, rec := range st.SAPRecords {
		if !stringsOverlap(rec.DateFrom, rec.DateTo, start, end) {
			continue
		}
		if et, _ := mapSAPEvent(rec); et != "" {
			stats[paletteStatKey(et)] += len(dateRange(maxDate(rec.DateFrom, start), minDate(nonEmpty(rec.DateTo, rec.DateFrom), end)))
		}
		if sh := recordClockShift(rec); sh != "" {
			stats[sh] += len(dateRange(maxDate(rec.DateFrom, start), minDate(nonEmpty(rec.DateTo, rec.DateFrom), end)))
		}
	}
	facts := make([]tab2FactRecord, 0)
	for _, f := range st.Facts {
		if dateInRange(f.Date, start, end) {
			facts = append(facts, f)
		}
	}
	sort.Slice(facts, func(i, j int) bool {
		if facts[i].Date != facts[j].Date {
			return facts[i].Date < facts[j].Date
		}
		if facts[i].TabNo != facts[j].TabNo {
			return facts[i].TabNo < facts[j].TabNo
		}
		return facts[i].Start < facts[j].Start
	})
	sz := make([]tab2SZRVDRecord, 0)
	for _, rec := range st.SZRVD {
		if stringsOverlap(rec.DateFrom, rec.DateTo, start, end) {
			sz = append(sz, rec)
		}
	}
	sort.Slice(sz, func(i, j int) bool {
		if sz[i].TabNo != sz[j].TabNo {
			return sz[i].TabNo < sz[j].TabNo
		}
		return sz[i].DateFrom < sz[j].DateFrom
	})
	imps := append([]tab2ImportLog(nil), st.Imports...)
	sort.Slice(imps, func(i, j int) bool { return imps[i].ID > imps[j].ID })
	if len(imps) > 30 {
		imps = imps[:30]
	}
	stamp := ""
	if fi, e := os.Stat(h.mainDBPath()); e == nil {
		stamp = fi.ModTime().Format(time.RFC3339Nano)
	}
	if fi, e := os.Stat(h.tab2Path()); e == nil {
		stamp += "|" + fi.ModTime().Format(time.RFC3339Nano)
	}
	daysN := 0
	if len(blocks) > 0 {
		daysN = len(blocks[0].Plan)
	}
	writeJSON(w, map[string]any{"ok": true, "version": "LaborControl 1.0 · Проверка табеля R3", "year": year, "month": month, "days": daysN, "blocks": blocks, "discrepancies": disc, "facts": facts, "sz_rvd": sz, "dp": dp, "rvd": rvd, "palette": tab2Palette, "color_stats": stats, "imports": imps, "stamp": stamp})
}

func maxDate(a, b string) string {
	if a > b {
		return a
	}
	return b
}
func minDate(a, b string) string {
	if a < b {
		return a
	}
	return b
}
func nonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}