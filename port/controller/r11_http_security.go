package main

import (
    "net"
    "net/http"
    "net/url"
    "strings"
)

// The controller serves private employee/timekeeping information over
// loopback. Protect both the browser UI and every API, including routes not
// individually wrapped by controller.wrap.
func r11AllowedLocalHost(hostPort string) bool {
    if hostPort == "" || strings.Contains(hostPort, "@") {
        return false
    }
    host, port, err := net.SplitHostPort(hostPort)
    if err != nil {
        // Host without port is emitted by the native GTK health/shutdown probe.
        host, port = hostPort, ""
    }
    if port != "" && port != "8779" { return false }
    host = strings.TrimSuffix(strings.ToLower(host), ".")
    return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

func r11AllowedOrigin(raw string) bool {
    parsed, err := url.Parse(raw)
    if err != nil || parsed == nil { return false }
    if parsed.Scheme != "http" || parsed.User != nil ||
        parsed.RawQuery != "" || parsed.Path != "" || parsed.Fragment != "" {
        return false
    }
    return r11AllowedLocalHost(parsed.Host)
}

func r11LocalOnlyHandler(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("Referrer-Policy", "no-referrer")
        if !r11AllowedLocalHost(r.Host) {
            http.Error(w, "Запрос к LaborControl должен использовать локальный адрес 127.0.0.1", http.StatusForbidden)
            return
        }
        if origin := r.Header.Get("Origin"); origin != "" && !r11AllowedOrigin(origin) {
            http.Error(w, "Запрос со стороннего сайта запрещён", http.StatusForbidden)
            return
        }
        site := strings.ToLower(r.Header.Get("Sec-Fetch-Site"))
        if site == "cross-site" {
            http.Error(w, "Межсайтовый запрос запрещён", http.StatusForbidden)
            return
        }
        next.ServeHTTP(w, r)
    })
}
