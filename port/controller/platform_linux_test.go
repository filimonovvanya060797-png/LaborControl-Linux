//go:build linux

package main

import (
    "fmt"
    "net"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestLinuxCoreName(t *testing.T) {
    if platformCoreName() != "LaborControl_Core" { t.Fatal("wrong Linux Core name") }
}
func TestLinuxEnvironmentIsUntouched(t *testing.T) {
    source := []string{"PATH=/bin:/usr/bin", "LANG=ru_RU.UTF-8"}
    got := coreEnvironment(source)
    if strings.Join(source, "|") != strings.Join(got, "|") { t.Fatalf("environment modified: %v", got) }
}
func TestLinuxDataDirXDG(t *testing.T) {
    temp := t.TempDir()
    t.Setenv("XDG_DATA_HOME", temp)
    got, err := localDataDir()
    if err != nil || got != filepath.Join(temp, "LaborControl") { t.Fatalf("dir=%s err=%v", got, err) }
}
func TestLinuxDataDirFallback(t *testing.T) {
    t.Setenv("XDG_DATA_HOME", "")
    got, err := localDataDir()
    home, _ := os.UserHomeDir()
    if err != nil || got != filepath.Join(home, ".local", "share", "LaborControl") { t.Fatalf("dir=%s err=%v", got, err) }
}
func TestFindCoreURLUsesOnlyOwnListeningSocket(t *testing.T) {
    l, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil { t.Fatal(err) }
    defer l.Close()
    actual := fmt.Sprintf("http://127.0.0.1:%d/", l.Addr().(*net.TCPAddr).Port)
    got, err := findCoreURL(os.Getpid())
    if err != nil || got != actual { t.Fatalf("findCoreURL got=%q want=%q err=%v", got, actual, err) }
}

func TestLinuxStandaloneMode(t *testing.T) {
	if !platformStandaloneMode() {
		t.Fatal("Linux must allow the local R11 state workspace without the native Core binary")
	}
}
