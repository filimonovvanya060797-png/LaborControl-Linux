package main

import (
 "bytes"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

func TestIllustratedEmployeeGuideIsEmbeddedAndOffline(t *testing.T) {
 if len(employeeGuideV7HTML)<600000 { t.Fatalf("embedded employee guide too small (%d): screenshots may be missing",len(employeeGuideV7HTML)) }
 if n:=bytes.Count(employeeGuideV7HTML,[]byte("data:image/png;base64,"));n!=10 {
   t.Fatalf("expected 10 embedded real screenshots, got %d",n)
 }
 for _,want:=range []string{
  "1. Ориентирование в разделе", "8. Ошибки импорта", "10. Предпросмотр пунктов сбора",
  "Проверить файл", "showImage(", "searchChapters(", "viewport",
 }{
  if !bytes.Contains(employeeGuideV7HTML,[]byte(want)){t.Errorf("missing from full illustrated guide: %q",want)}
 }
 for _,bad:=range []string{"https://fonts.googleapis.com","https://cdn.jsdelivr.net","http://localhost:","<script src="} {
  if bytes.Contains(employeeGuideV7HTML,[]byte(bad)){t.Errorf("remote dependency or insecure resource: %q",bad)}
 }
}

func TestEmployeeGuideLocalRouteGETOnly(t *testing.T) {
 h:=&controller{}
 for _,tc:=range []struct{method,path string;status int}{
  {http.MethodGet,"/help/employees",http.StatusOK},
  {http.MethodPost,"/help/employees",http.StatusNotFound},
  {http.MethodGet,"/help/other",http.StatusNotFound},
 }{
  req:=httptest.NewRequest(tc.method,"http://127.0.0.1:8779"+tc.path,nil)
  res:=httptest.NewRecorder()
  h.serveEmployeeGuide(res,req)
  if res.Code!=tc.status{t.Fatalf("%s %s status %d, want %d",tc.method,tc.path,res.Code,tc.status)}
  if tc.status==http.StatusOK{
   if !strings.Contains(res.Header().Get("Content-Type"),"text/html"){t.Fatal("wrong Content-Type")}
   if res.Header().Get("X-Frame-Options")!="SAMEORIGIN"{t.Fatal("must allow same-origin handbook iframe only")}
   if !strings.Contains(res.Body.String(),"data:image/png;base64"){t.Fatal("screenshots not served")}
  }
 }
}

func TestR11IllustratedHelpEntryPoint(t *testing.T) {
 s:=string(r11OriginalHTML)
 for _,want:=range []string{
  "employees-illustrated",
  "id=\"employeeGuideFrame\"",
  "src=\"/help/employees#",
  "guideIllustratedAnchor",
  "10 глав с реальными скриншотами",
  "Ко всем главам",
 }{
  if !strings.Contains(s,want){t.Errorf("R11 help missing %q",want)}
 }
}
