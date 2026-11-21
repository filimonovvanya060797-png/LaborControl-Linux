package main

import (
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
)

func TestEmbeddedTab2SameOriginInR11Desktop(t *testing.T) {
    h := newR11TestController(t)
    rec := httptest.NewRecorder()
    h.serveEmbeddedTab2(rec, httptest.NewRequest(http.MethodGet, "/tab2?embed=tab2", nil))
    if rec.Code != http.StatusOK {
        t.Fatalf("embedded Tab2 status %d: %s", rec.Code, rec.Body.String())
    }
    if !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
        t.Fatalf("wrong Tab2 MIME type: %q", rec.Header().Get("Content-Type"))
    }
    if !strings.Contains(rec.Body.String(), "/api/tab2/state") ||
       !strings.Contains(rec.Body.String(), "embed") {
        t.Fatal("Tab2 page cannot initialize embedded timesheet API")
    }

    rec = httptest.NewRecorder()
    h.serveEmbeddedTab2(rec, httptest.NewRequest(http.MethodGet, "/another", nil))
    if rec.Code != http.StatusNotFound {
        t.Fatalf("unexpected non-Tab2 route status: %d", rec.Code)
    }

    if !strings.Contains(string(r11OriginalHTML), "src=\"/tab2?embed=tab2\"") {
        t.Fatal("R11 check screen must point to local same-origin Tab2 UI")
    }
    if strings.Contains(string(r11OriginalHTML), "127.0.0.1:8767/?embed=tab2") {
        t.Fatal("R11 check screen still points at the legacy browser port")
    }
}
