//go:build linux

package main

import (
    "bufio"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strconv"
    "strings"
)

// Linux does not have a console window to hide.
func configureHidden(_ *exec.Cmd) {}

func openURL(url string) error {
    cmd := exec.Command("xdg-open", url)
    return cmd.Start()
}

func showMessage(title, message string) {
    fmt.Fprintln(os.Stderr, title+": "+message)
}

// Never terminate unrelated Linux processes based on an executable name.
func terminateStaleCoreProcesses() {}

func platformCoreName() string { return "LaborControl_Core" }

func localDataDir() (string, error) {
    base := os.Getenv("XDG_DATA_HOME")
    if base == "" {
        home, err := os.UserHomeDir()
        if err != nil { return "", err }
        base = filepath.Join(home, ".local", "share")
    }
    return filepath.Join(base, "LaborControl"), nil
}

// No Windows rundll32 shim is needed on Linux. Do not alter PATH.
func coreEnvironment(env []string) []string { return env }

// The child process is discovered by matching socket inode numbers under
// /proc/<pid>/fd to listening sockets in /proc/<pid>/net/tcp. This avoids
// depending on netstat/ss and avoids choosing sockets owned by other processes.
func findCoreURL(pid int) (string, error) {
    proc := filepath.Join("/proc", strconv.Itoa(pid))
    fds, err := os.ReadDir(filepath.Join(proc, "fd"))
    if err != nil { return "", err }
    sockets := map[string]bool{}
    for _, fd := range fds {
        target, err := os.Readlink(filepath.Join(proc, "fd", fd.Name()))
        if err != nil { continue }
        if strings.HasPrefix(target, "socket:[") && strings.HasSuffix(target, "]") {
            sockets[strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")] = true
        }
    }
    data, err := os.Open(filepath.Join(proc, "net", "tcp"))
    if err != nil { return "", err }
    defer data.Close()
    scanner := bufio.NewScanner(data)
    for scanner.Scan() {
        parts := strings.Fields(scanner.Text())
        if len(parts) < 10 || !sockets[parts[9]] || parts[3] != "0A" { continue }
        // Kernel's IPv4 address is encoded in little-endian hex.
        local := strings.Split(parts[1], ":")
        if len(local) != 2 || local[0] != "0100007F" { continue }
        port, err := strconv.ParseUint(local[1], 16, 16)
        if err == nil && port > 0 {
            return fmt.Sprintf("http://127.0.0.1:%d/", port), nil
        }
    }
    if err := scanner.Err(); err != nil { return "", err }
    return "", nil
}

// Linux can start the local R11 state workspace while the native Core is being ported.
func platformStandaloneMode() bool { return true }
