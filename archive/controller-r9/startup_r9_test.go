package main

import (
	"os"
	"strings"
	"testing"
)

func TestR9Identity(t *testing.T) {
	if controllerVersion != "v1.0-TIMESHEET-PREVIEW-R9" {
		t.Fatalf("version=%s", controllerVersion)
	}
	if listenAddr != "127.0.0.1:8779" {
		t.Fatalf("listen=%s", listenAddr)
	}
	if coreFileName != "LaborControl_Core_R9.exe" {
		t.Fatalf("core=%s", coreFileName)
	}
}

func TestCoreEnvironmentPrependsProgramDir(t *testing.T) {
	env := coreEnvironment([]string{"PATH=C:\\Windows\\System32", "X=1"})
	var path string
	for _, v := range env {
		if strings.HasPrefix(strings.ToUpper(v), "PATH=") {
			path = v
		}
	}
	if path == "" {
		t.Fatal("PATH missing")
	}
	exe, _ := os.Executable()
	_ = exe
	if !strings.Contains(path, "shim;") || !strings.Contains(path, ";C:\\Windows\\System32") {
		t.Fatalf("PATH not preserved: %s", path)
	}
}