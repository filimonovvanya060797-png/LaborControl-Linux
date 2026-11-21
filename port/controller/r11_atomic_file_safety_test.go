package main

import (
    "bytes"
    "os"
    "path/filepath"
    "testing"
)

func TestR11DeepAuditAtomicWriteNeverDeletesExistingDirectory(t *testing.T) {
    root := t.TempDir()
    path := filepath.Join(root, "reserved-database-path")
    if err := os.Mkdir(path, 0700); err != nil { t.Fatal(err) }
    err := atomicWrite(path, []byte("not safe"))
    if err == nil { t.Fatal("atomicWrite replaced a user directory with a database file") }
    info, statErr := os.Stat(path)
    if statErr != nil || !info.IsDir() {
        t.Fatalf("failed atomic write deleted a preexisting directory: %v %#v", statErr, info)
    }
}

func TestR11DeepAuditDatabaseAndBackupArePrivate(t *testing.T) {
    root := t.TempDir()
    dataDir := filepath.Join(root, "LaborControl-R11-Audit")
    path := filepath.Join(dataDir, "labor_control.json")
    body := []byte(`{"employees":[],"days":[]}`)
    if err := atomicWrite(path, body); err != nil { t.Fatal(err) }
    dirInfo, err := os.Stat(dataDir)
    if err != nil { t.Fatal(err) }
    if got := dirInfo.Mode().Perm(); got & 0077 != 0 {
        t.Errorf("database directory exposes private data to other users: %04o", got)
    }
    fileInfo, err := os.Stat(path)
    if err != nil { t.Fatal(err) }
    if got := fileInfo.Mode().Perm(); got & 0077 != 0 {
        t.Errorf("database file exposes private data to other users: %04o", got)
    }

    name, err := makeBackup(path, filepath.Join(dataDir, "backups"))
    if err != nil { t.Fatal(err) }
    copy, err := os.ReadFile(name)
    if err != nil { t.Fatal(err) }
    if !bytes.Equal(copy, body) { t.Fatal("backup does not match source database") }
    info, err := os.Stat(name)
    if err != nil { t.Fatal(err) }
    if got := info.Mode().Perm(); got & 0077 != 0 {
        t.Errorf("backup file exposes private data to other users: %04o", got)
    }
    backupDir, err := os.Stat(filepath.Dir(name))
    if err != nil { t.Fatal(err) }
    if got := backupDir.Mode().Perm(); got & 0077 != 0 {
        t.Errorf("backup directory exposes private data to other users: %04o", got)
    }
}
