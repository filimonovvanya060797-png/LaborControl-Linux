package main

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

type r11XLSXShared struct {
	Items []struct {
		Text string `xml:"t"`
		Runs []struct { Text string `xml:"t"` } `xml:"r"`
	} `xml:"si"`
}

type r11XLSXSheet struct {
	SheetData struct {
		Rows []struct {
			Cells []struct {
				Ref string `xml:"r,attr"`
				Type string `xml:"t,attr"`
				Value string `xml:"v"`
				Inline struct {
					Text string `xml:"t"`
					Runs []struct { Text string `xml:"t"` } `xml:"r"`
				} `xml:"is"`
			} `xml:"c"`
		} `xml:"row"`
	} `xml:"sheetData"`
}

func r11XLSXColumn(ref string) int {
	n:=0
	for _,r:=range strings.ToUpper(ref) {
		if r<'A'||r>'Z' { break }
		n=n*26+int(r-'A'+1)
	}
	if n<1{return 0}
	return n-1
}

func r11ReadXLSX(data []byte) ([][]string,string,error) {
	z,err:=zip.NewReader(bytes.NewReader(data),int64(len(data)))
	if err!=nil{return nil,"",fmt.Errorf("не удалось открыть XLSX: %w",err)}
	files:=map[string]*zip.File{}
	sheets:=[]string{}
	for _,f:=range z.File{
		files[f.Name]=f
		if strings.HasPrefix(f.Name,"xl/worksheets/sheet")&&strings.HasSuffix(f.Name,".xml"){sheets=append(sheets,f.Name)}
	}
	if len(sheets)==0{return nil,"",fmt.Errorf("в XLSX не найден лист")}
	sort.Strings(sheets)
	read:=func(name string)([]byte,error){
		f:=files[name];if f==nil{return nil,nil}
		rc,e:=f.Open();if e!=nil{return nil,e};defer rc.Close()
		return io.ReadAll(io.LimitReader(rc,64<<20))
	}
	shared:=[]string{}
	if raw,e:=read("xl/sharedStrings.xml");e!=nil{return nil,"",e}else if len(raw)>0{
		var ss r11XLSXShared
		if e:=xml.Unmarshal(raw,&ss);e!=nil{return nil,"",fmt.Errorf("ошибка sharedStrings.xml: %w",e)}
		for _,it:=range ss.Items{
			v:=it.Text
			if len(it.Runs)>0{v="";for _,r:=range it.Runs{v+=r.Text}}
			shared=append(shared,v)
		}
	}
	raw,err:=read(sheets[0]);if err!=nil{return nil,"",err}
	var sheet r11XLSXSheet
	if err:=xml.Unmarshal(raw,&sheet);err!=nil{return nil,"",fmt.Errorf("ошибка листа XLSX: %w",err)}
	rows:=make([][]string,0,len(sheet.SheetData.Rows))
	for _,row:=range sheet.SheetData.Rows{
		cells:=make(map[int]string)
		max:=-1
		for _,c:=range row.Cells{
			idx:=r11XLSXColumn(c.Ref);if idx<0{continue}
			v:=c.Value
			switch c.Type{
			case "s":
				n,e:=strconv.Atoi(strings.TrimSpace(v));if e==nil&&n>=0&&n<len(shared){v=shared[n]}
			case "inlineStr":
				v=c.Inline.Text
				if len(c.Inline.Runs)>0{v="";for _,r:=range c.Inline.Runs{v+=r.Text}}
			}
			cells[idx]=v;if idx>max{max=idx}
		}
		if max<0{rows=append(rows,[]string{});continue}
		out:=make([]string,max+1);for i,v:=range cells{out[i]=v};rows=append(rows,out)
	}
	return rows,strings.TrimSuffix(strings.TrimPrefix(sheets[0],"xl/worksheets/"),".xml"),nil
}

func r11NormalizeDate(value string) string {
	value=strings.TrimSpace(strings.Trim(value,"'"))
	for _,layout:=range []string{"2006-01-02","02.01.2006","02/01/2006","2006/01/02","20060102"}{
		if d,err:=time.Parse(layout,value);err==nil{return d.Format("2006-01-02")}
	}
	if n,err:=strconv.ParseFloat(strings.ReplaceAll(value,",","."),64);err==nil&&n>=20000&&n<=80000{
		base:=time.Date(1899,12,30,0,0,0,0,time.UTC)
		return base.Add(time.Duration(n*float64(24*time.Hour))).Format("2006-01-02")
	}
	return ""
}

func r11NormalizeHeader(value string) string {
	s:=strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value,"\ufeff")))
	s=strings.ReplaceAll(s,"ё","е")
	var b strings.Builder
	for _,r:=range s{if r>='a'&&r<='z'||r>='а'&&r<='я'||r>='0'&&r<='9'{b.WriteRune(r)}}
	return b.String()
}

func r11ReadCSV(data []byte) ([][]string,error) {
	data=bytes.TrimPrefix(data,[]byte{0xef,0xbb,0xbf})
	first:=string(data)
	comma:=','
	if strings.Count(strings.SplitN(first,"\n",2)[0],";")>strings.Count(strings.SplitN(first,"\n",2)[0],","){comma=';'}
	reader:=csv.NewReader(bytes.NewReader(data))
	reader.Comma=comma
	reader.FieldsPerRecord=-1
	reader.LazyQuotes=true
	return reader.ReadAll()
}

func r11ReadImportTable(data []byte, filename string) ([][]string,string,error) {
	if strings.EqualFold(filepathExt(filename),".xlsx") || strings.EqualFold(filepathExt(filename),".xlsm") {
		rows,sheet,err:=r11ReadXLSX(data)
		return rows,sheet,err
	}
	rows,err:=r11ReadCSV(data)
	return rows,"CSV",err
}

func filepathExt(name string) string {
	i:=strings.LastIndex(name,".")
	if i<0{return ""}
	return name[i:]
}
