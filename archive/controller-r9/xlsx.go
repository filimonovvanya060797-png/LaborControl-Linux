package main

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type gridSheet struct {
	Name string
	Rows [][]string
}

type workbookXML struct {
	Sheets []struct {
		Name string `xml:"name,attr"`
		RID  string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
	} `xml:"sheets>sheet"`
}

type relationshipsXML struct {
	Rels []struct {
		ID     string `xml:"Id,attr"`
		Target string `xml:"Target,attr"`
	} `xml:"Relationship"`
}

type sharedStringsXML struct {
	Items []struct {
		Texts []string `xml:"t"`
		Runs  []struct {
			Text string `xml:"t"`
		} `xml:"r"`
	} `xml:"si"`
}

type worksheetXML struct {
	Rows []struct {
		R int `xml:"r,attr"`
		C []struct {
			R  string `xml:"r,attr"`
			T  string `xml:"t,attr"`
			V  string `xml:"v"`
			IS struct {
				T string `xml:"t"`
				R []struct {
					T string `xml:"t"`
				} `xml:"r"`
			} `xml:"is"`
		} `xml:"c"`
	} `xml:"sheetData>row"`
}

func readSpreadsheet(data []byte, filename string) ([]gridSheet, error) {
	low := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(low, ".xlsx"), strings.HasSuffix(low, ".xlsm"):
		return parseXLSX(data)
	case strings.HasSuffix(low, ".csv"), strings.HasSuffix(low, ".txt"):
		rows, err := parseCSV(data)
		if err != nil {
			return nil, err
		}
		return []gridSheet{{Name: "CSV", Rows: rows}}, nil
	case strings.HasSuffix(low, ".xlsb"):
		return nil, fmt.Errorf("формат XLSB используется старым макросом. В LaborControl загружайте исходную выгрузку SAP/ИСАТ в XLSX или CSV")
	default:
		return nil, fmt.Errorf("неподдерживаемый формат %q; используйте XLSX, XLSM или CSV", path.Ext(filename))
	}
}

func parseCSV(data []byte) ([][]string, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	sample := string(data)
	comma := ','
	if strings.Count(sample, "\t") > strings.Count(sample, ";") && strings.Count(sample, "\t") > strings.Count(sample, ",") {
		comma = '\t'
	} else if strings.Count(sample, ";") > strings.Count(sample, ",") {
		comma = ';'
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = comma
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.TrimLeadingSpace = true
	var out [][]string
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		for i := range row {
			row[i] = strings.TrimSpace(row[i])
		}
		out = append(out, row)
	}
	return out, nil
}

func parseXLSX(data []byte) ([]gridSheet, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("Excel: %w", err)
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	read := func(name string) ([]byte, error) {
		f := files[name]
		if f == nil {
			return nil, fmt.Errorf("Excel: нет %s", name)
		}
		rc, e := f.Open()
		if e != nil {
			return nil, e
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	wbB, err := read("xl/workbook.xml")
	if err != nil {
		return nil, err
	}
	var wb workbookXML
	if err = xml.Unmarshal(wbB, &wb); err != nil {
		return nil, err
	}
	relB, err := read("xl/_rels/workbook.xml.rels")
	if err != nil {
		return nil, err
	}
	var rel relationshipsXML
	if err = xml.Unmarshal(relB, &rel); err != nil {
		return nil, err
	}
	relMap := map[string]string{}
	for _, x := range rel.Rels {
		relMap[x.ID] = x.Target
	}
	shared := []string{}
	if f := files["xl/sharedStrings.xml"]; f != nil {
		b, _ := read("xl/sharedStrings.xml")
		var ss sharedStringsXML
		if xml.Unmarshal(b, &ss) == nil {
			for _, si := range ss.Items {
				var sb strings.Builder
				for _, t := range si.Texts {
					sb.WriteString(t)
				}
				for _, r := range si.Runs {
					sb.WriteString(r.Text)
				}
				shared = append(shared, sb.String())
			}
		}
	}
	var out []gridSheet
	for _, sh := range wb.Sheets {
		target := relMap[sh.RID]
		if target == "" {
			continue
		}
		target = strings.TrimPrefix(target, "/")
		if !strings.HasPrefix(target, "xl/") {
			target = path.Clean("xl/" + target)
		}
		b, e := read(target)
		if e != nil {
			continue
		}
		var ws worksheetXML
		if xml.Unmarshal(b, &ws) != nil {
			continue
		}
		maxCol := 0
		maxRow := 0
		cellRows := map[int]map[int]string{}
		for _, rr := range ws.Rows {
			if rr.R > maxRow {
				maxRow = rr.R
			}
			m := map[int]string{}
			for _, c := range rr.C {
				col := cellColumn(c.R)
				if col > maxCol {
					maxCol = col
				}
				v := c.V
				switch c.T {
				case "s":
					n, _ := strconv.Atoi(v)
					if n >= 0 && n < len(shared) {
						v = shared[n]
					}
				case "inlineStr":
					if c.IS.T != "" {
						v = c.IS.T
					} else {
						var sb strings.Builder
						for _, q := range c.IS.R {
							sb.WriteString(q.T)
						}
						v = sb.String()
					}
				}
				m[col] = strings.TrimSpace(v)
			}
			cellRows[rr.R] = m
		}
		if maxRow == 0 || maxCol == 0 {
			continue
		}
		rows := make([][]string, maxRow)
		for i := 0; i < maxRow; i++ {
			rows[i] = make([]string, maxCol)
			if m := cellRows[i+1]; m != nil {
				for c, v := range m {
					if c > 0 && c <= maxCol {
						rows[i][c-1] = v
					}
				}
			}
		}
		out = append(out, gridSheet{Name: sh.Name, Rows: rows})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("Excel: не найдены рабочие листы")
	}
	return out, nil
}

var cellRefRE = regexp.MustCompile(`^([A-Z]+)`)

func cellColumn(ref string) int {
	m := cellRefRE.FindStringSubmatch(strings.ToUpper(ref))
	if len(m) < 2 {
		return 0
	}
	n := 0
	for _, ch := range m[1] {
		n = n*26 + int(ch-'A'+1)
	}
	return n
}

func compactRows(rows [][]string) [][]string {
	var out [][]string
	for _, r := range rows {
		ok := false
		for _, v := range r {
			if strings.TrimSpace(v) != "" {
				ok = true
				break
			}
		}
		if ok {
			out = append(out, r)
		}
	}
	return out
}

func normHeader(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "ё", "е")
	repl := strings.NewReplacer(" ", "", "_", "", "-", "", ".", "", "/", "", "\\", "", "№", "номер", "(", "", ")", "", ":", "", ",", "", "\n", "", "\r", "")
	return repl.Replace(s)
}

func headerIndex(row []string) map[string]int {
	m := map[string]int{}
	for i, v := range row {
		n := normHeader(v)
		if n != "" {
			if _, ok := m[n]; !ok {
				m[n] = i
			}
		}
	}
	return m
}
func findCol(h map[string]int, names ...string) int {
	for _, n := range names {
		if i, ok := h[normHeader(n)]; ok {
			return i
		}
	}
	return -1
}
func at(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}
func joinName(parts ...string) string {
	var q []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			q = append(q, p)
		}
	}
	return strings.Join(q, " ")
}
func trimTab(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimLeft(s, "0")
	if s == "" {
		return "0"
	}
	if f, e := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64); e == nil && f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return s
}

func parseNumber(s string) float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, " ", ""))
	s = strings.ReplaceAll(s, ",", ".")
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
func fmtNum(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', 2, 64)
}

func excelSerialToTime(v float64) time.Time {
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.Local)
	days := int(v)
	frac := v - float64(days)
	sec := int64(frac*86400 + 0.5)
	return base.AddDate(0, 0, days).Add(time.Duration(sec) * time.Second)
}
func parseDateTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if f, e := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64); e == nil && f > 1000 {
		return excelSerialToTime(f), true
	}
	layouts := []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "02.01.2006 15:04:05", "02.01.2006 15:04", "02.01.2006", "2006-01-02", "02/01/2006", "2.1.2006"}
	for _, l := range layouts {
		if t, e := time.ParseInLocation(l, s, time.Local); e == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
func parseDate(s string) string {
	if t, ok := parseDateTime(s); ok {
		return t.Format("2006-01-02")
	}
	return ""
}
func parseClock(s string) (int, int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, false
	}
	if f, e := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64); e == nil && f >= 0 && f < 1 {
		mins := int(f*24*60 + 0.5)
		return (mins / 60) % 24, mins % 60, true
	}
	for _, l := range []string{"15:04:05", "15:04"} {
		if t, e := time.Parse(l, s); e == nil {
			return t.Hour(), t.Minute(), true
		}
	}
	if t, ok := parseDateTime(s); ok {
		return t.Hour(), t.Minute(), true
	}
	return 0, 0, false
}
func shiftFromClock(h int) string {
	// Перенос логики макроса «ВставитьФакт_Смену_Сравнение»:
	// 06:00–09:59 — дневная, 18:00–22:59 — ночная, остальное не определяется.
	if h >= 6 && h <= 9 {
		return "DAY"
	}
	if h >= 18 && h <= 22 {
		return "NIGHT"
	}
	return ""
}
func dateRange(a, b string) []string {
	if a == "" {
		return nil
	}
	if b == "" {
		b = a
	}
	ta, e1 := time.Parse("2006-01-02", a)
	tb, e2 := time.Parse("2006-01-02", b)
	if e1 != nil || e2 != nil || tb.Before(ta) {
		return []string{a}
	}
	var out []string
	for d := ta; !d.After(tb); d = d.AddDate(0, 0, 1) {
		out = append(out, d.Format("2006-01-02"))
		if len(out) > 3700 {
			break
		}
	}
	return out
}
func inclusiveDays(a, b string) int { return len(dateRange(a, b)) }
func sortGridRows(rows [][]string, col int) {
	sort.SliceStable(rows, func(i, j int) bool { return at(rows[i], col) < at(rows[j], col) })
}