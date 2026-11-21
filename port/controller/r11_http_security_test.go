package main

import (
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestR11DeepAuditLocalHTTPDoesNotExposePrivateStateToOtherOrigins(t *testing.T) {
    h := newR11TestController(t)
    h.standalone = true
    mux := http.NewServeMux()
    mux.HandleFunc("/api/state", h.wrap(h.r11CoreState))
    mux.HandleFunc("/api/shutdown", h.wrap(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusNoContent) // no real shutdown in a unit test
    }))
    handler := r11LocalOnlyHandler(mux)

    cases := []struct {
        name,host,origin,site,path,method string
        code int
    }{
        {"same origin state", "127.0.0.1:8779", "http://127.0.0.1:8779", "same-origin", "/api/state", "GET", 200},
        {"desktop health style host", "127.0.0.1", "", "", "/api/state", "GET", 200},
        {"localhost alias", "localhost:8779", "http://localhost:8779", "same-origin", "/api/state", "GET", 200},
        {"foreign website reads state", "127.0.0.1:8779", "https://attacker.example", "cross-site", "/api/state", "GET", 403},
        {"foreign website with forged same site", "127.0.0.1:8779", "https://attacker.example", "same-origin", "/api/state", "GET", 403},
        {"foreign website posts shutdown", "127.0.0.1:8779", "https://attacker.example", "cross-site", "/api/shutdown", "POST", 403},
        {"cross-site header without Origin", "127.0.0.1:8779", "", "cross-site", "/api/state", "GET", 403},
        {"DNS rebinding Host", "attacker.example:8779", "", "", "/api/state", "GET", 403},
        {"userinfo Host bypass", "127.0.0.1:8779@attacker.example", "", "", "/api/state", "GET", 403},
        {"wrong loopback port", "127.0.0.1:8888", "", "", "/api/state", "GET", 403},
        {"null Origin", "127.0.0.1:8779", "null", "", "/api/state", "GET", 403},
    }
    for _, tc := range cases {
        t.Run(tc.name,func(t *testing.T){
            req := httptest.NewRequest(tc.method, "http://127.0.0.1:8779"+tc.path,nil)
            req.Host = tc.host
            if tc.origin!="" {req.Header.Set("Origin",tc.origin)}
            if tc.site!="" {req.Header.Set("Sec-Fetch-Site",tc.site)}
            rec := httptest.NewRecorder()
            handler.ServeHTTP(rec,req)
            if rec.Code!=tc.code {
                t.Fatalf("HTTP %d want %d: %s",rec.Code,tc.code,rec.Body.String())
            }
            if got:=rec.Header().Get("Access-Control-Allow-Origin");got!=""{
                t.Fatalf("private API advertised wildcard CORS %q",got)
            }
        })
    }
}

func TestR11DeepAuditRejectsCrossOriginPreflight(t *testing.T) {
    h:=newR11TestController(t)
    handler:=r11LocalOnlyHandler(h.wrap(func(w http.ResponseWriter,r *http.Request){
        w.WriteHeader(http.StatusNoContent)
    }))
    req:=httptest.NewRequest(http.MethodOptions,"http://127.0.0.1:8779/api/day/save",nil)
    req.Header.Set("Origin","https://other.example")
    req.Header.Set("Access-Control-Request-Method","POST")
    rec:=httptest.NewRecorder();handler.ServeHTTP(rec,req)
    if rec.Code!=http.StatusForbidden {
        t.Fatalf("foreign preflight must be denied: %d",rec.Code)
    }
    if rec.Header().Get("Access-Control-Allow-Origin")!="" {
        t.Fatal("preflight still exposes wildcard CORS")
    }
}
