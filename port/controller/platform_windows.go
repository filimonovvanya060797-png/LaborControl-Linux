//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

func configureHidden(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

func openURL(url string) error {
	shell32 := syscall.NewLazyDLL("shell32.dll")
	proc := shell32.NewProc("ShellExecuteW")
	verb, _ := syscall.UTF16PtrFromString("open")
	target, _ := syscall.UTF16PtrFromString(url)
	r, _, e := proc.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(target)), 0, 0, 1)
	if r <= 32 {
		return fmt.Errorf("ShellExecuteW: %v (код %d)", e, r)
	}
	return nil
}

func showMessage(title, text string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	proc := user32.NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	proc.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x10)
}

func terminateStaleCoreProcesses() {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	taskkill := filepath.Join(root, "System32", "taskkill.exe")
	for _, name := range []string{"LaborControl_Core.exe", "LaborControl_Core_R9.exe"} {
		cmd := exec.Command(taskkill, "/F", "/T", "/IM", name)
		configureHidden(cmd)
		_ = cmd.Run()
	}
}
func localDataDir() (string, error) {
    base := os.Getenv("LOCALAPPDATA")
    if base == "" {
        home, err := os.UserHomeDir()
        if err != nil { return "", err }
        base = filepath.Join(home, "AppData", "Local")
    }
    return filepath.Join(base, "LaborControl"), nil
}

func platformCoreName() string { return coreFileName }

func coreEnvironment(env []string) []string {
	// Keep the normal Windows environment, but put the program directory first
	// in PATH. R9 ships a harmless shim/rundll32.exe there, so the legacy Core's
	// openBrowser() call does not create a second/old browser tab.
	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	out := make([]string, 0, len(env)+1)
	pathDone := false
	for _, kv := range env {
		if strings.HasPrefix(strings.ToUpper(kv), "PATH=") {
			out = append(out, "PATH="+filepath.Join(dir, "shim")+";"+kv[5:])
			pathDone = true
		} else {
			out = append(out, kv)
		}
	}
	if !pathDone {
		out = append(out, "PATH="+filepath.Join(dir, "shim"))
	}
	return out
}


func findCoreURL(pid int) (string, error) {
    cmd := exec.Command("netstat", "-ano", "-p", "tcp")
    configureHidden(cmd)
    out, err := cmd.Output()
    if err != nil { return "", err }
    return parseNetstatForPID(string(out), pid), nil
}
