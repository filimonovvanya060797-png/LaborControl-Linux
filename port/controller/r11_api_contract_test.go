package main

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The frontend extracted from Windows R11 is our API inventory. Make sure a
// future edit cannot accidentally leave one of its routes unmapped. A GET on
// write-only endpoints must fail with 405, not silently change local data.
func TestR11OriginalFrontendHasNoUnmappedAPIEndpoints(t *testing.T) {
	matches:=regexp.MustCompile(`/api/[a-z0-9/_.-]+`).FindAllString(string(r11OriginalHTML),-1)
	if len(matches)<25{t.Fatalf("incomplete original R11 API inventory: %d matches",len(matches))}
	unique:=map[string]bool{}
	for _,p:=range matches{unique[p]=true}
	paths:=make([]string,0,len(unique))
	for p:=range unique{paths=append(paths,p)}
	sort.Strings(paths)
	h:=newR11TestController(t)
	for _,path:=range paths{
		t.Run(strings.TrimPrefix(path,"/api/"),func(t *testing.T){
			req:=httptest.NewRequest(http.MethodGet,path,nil)
			rec:=httptest.NewRecorder()
			h.r11CoreAPI(rec,req)
			// State, revision/get, XLSX template and portable snapshot download are read-only. Write routes return 405.
			if rec.Code==http.StatusNotFound{
				t.Fatalf("the original frontend uses %s, but the Linux Core has no handler",path)
			}
			if path!="/api/state"&&path!="/api/autograf/revision/get"&&path!="/api/employee/points/template.xlsx"&&path!="/api/backup/portable"&&rec.Code!=http.StatusMethodNotAllowed{
				t.Fatalf("write operation %s accepted a GET request: HTTP %d",path,rec.Code)
			}
		})
	}
}
