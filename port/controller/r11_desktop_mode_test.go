package main

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestR11DesktopModeHealthIncludesInstanceMarker(t *testing.T) {
    t.Setenv("LABORCONTROL_DESKTOP_MODE", "1")
    t.Setenv("LABORCONTROL_DESKTOP_TOKEN", "desktop-synthetic-marker")
    h := newR11TestController(t)
    h.standalone = true
    rec := httptest.NewRecorder()
    h.health(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
    if rec.Code != http.StatusOK {
        t.Fatalf("health status=%d: %s", rec.Code, rec.Body.String())
    }
    var got map[string]any
    if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil { t.Fatal(err) }
    if got["version"] != r11CoreVersion || got["desktop_instance"] != "desktop-synthetic-marker" {
        t.Fatalf("unexpected health response %#v", got)
    }
}

func TestR11BrowserModeHealthDoesNotExposeDesktopMarker(t *testing.T) {
    t.Setenv("LABORCONTROL_DESKTOP_MODE", "")
    t.Setenv("LABORCONTROL_DESKTOP_TOKEN", "should-not-be-present")
    h := newR11TestController(t)
    h.standalone = true
    rec := httptest.NewRecorder()
    h.health(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
    var got map[string]any
    if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil { t.Fatal(err) }
    if _, ok := got["desktop_instance"]; ok {
        t.Fatalf("unexpected desktop marker outside desktop mode: %#v", got)
    }
}
