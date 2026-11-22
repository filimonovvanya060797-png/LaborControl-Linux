package main

import (
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestR11StageRejectsInvalidDatesAndHoursWithoutOverwritingPreparedPackage(t *testing.T) {
    base := t.TempDir()
    h := &controller{baseDir: base}
    staged := filepath.Join(base, "bulk_staged.json")
    valid := `{"days":[{"tab_no":"T-901","date":"2026-11-20","plan":"1/8","plan_hours":8}]}`
    for _, tc := range []struct{name, payload string}{
        {"invalid February date", `{"days":[{"tab_no":"T-901","date":"2026-02-31","plan":"1/8","plan_hours":8}]}`},
        {"negative hours", `{"days":[{"tab_no":"T-901","date":"2026-11-20","plan":"1/8","plan_hours":-1}]}`},
        {"hours beyond 24", `{"days":[{"tab_no":"T-901","date":"2026-11-20","plan":"1/30","plan_hours":30}]}`},
        {"legacy bad hours", `{"d":[["T-901","2026-11-20","1/8","not-a-number","WORK"]]}`},
        {"legacy negative hours", `{"d":[["T-901","2026-11-20","1/8",-4,"WORK"]]}`},
        {"legacy invalid date", `{"d":[["T-901","2026-13-20","1/8",8,"WORK"]]}`},
        {"duplicate day", `{"days":[{"tab_no":"T-901","date":"2026-11-20","plan":"1/8","plan_hours":8},{"tab_no":"t-901","date":"2026-11-20","plan":"1/8","plan_hours":8}]}`},
    } {
        t.Run(tc.name,func(t *testing.T){
            if err:=os.WriteFile(staged,[]byte(valid),0600); err!=nil {t.Fatal(err)}
            req:=httptest.NewRequest(http.MethodPost,"/stage",strings.NewReader(tc.payload))
            rec:=httptest.NewRecorder()
            h.stage(rec,req)
            if rec.Code!=http.StatusBadRequest {t.Fatalf("status %d, want 400; body=%s",rec.Code,rec.Body.String())}
            after,err:=os.ReadFile(staged)
            if err!=nil {t.Fatal(err)}
            if string(after)!=valid {t.Fatalf("invalid staging replaced valid prepared package: %s",after)}
        })
    }
}

func TestR11StageAcceptsLegacyDecimalComma(t *testing.T) {
    h:=&controller{baseDir:t.TempDir()}
    req:=httptest.NewRequest(http.MethodPost,"/stage",strings.NewReader(`{"d":[["T-901","2026-11-20","1/8","8,5","WORK"]]}`))
    rec:=httptest.NewRecorder()
    h.stage(rec,req)
    if rec.Code!=http.StatusOK {t.Fatalf("status=%d: %s",rec.Code,rec.Body.String())}
    content,err:=os.ReadFile(filepath.Join(h.baseDir,"bulk_staged.json"))
    if err!=nil {t.Fatal(err)}
    if !strings.Contains(string(content),`"plan_hours":8.5`) {t.Fatalf("decimal hours not preserved: %s",content)}
}

func TestR11ApplyDaysRejectsInvalidPreparedDataBeforeBackupOrWrite(t *testing.T) {
    root:=t.TempDir()
    db:=filepath.Join(root,"labor_control.json")
    original:=[]byte(`{"employees":[],"days":[],"next_id":1}`)
    if err:=os.WriteFile(db,original,0600);err!=nil{t.Fatal(err)}
    backupDir:=filepath.Join(root,"backups")
    for _,bad:=range []dayPatch{
        {TabNo:"T-901",Date:"2026-02-31",PlanHours:8},
        {TabNo:"T-901",Date:"2026-11-20",PlanHours:-1},
        {TabNo:"T-901",Date:"2026-11-20",PlanHours:25},
    }{
        if _,err:=applyDays(db,backupDir,stagePayload{Days:[]dayPatch{bad}});err==nil {t.Fatalf("invalid day accepted: %+v",bad)}
        after,err:=os.ReadFile(db)
        if err!=nil {t.Fatal(err)}
        if string(after)!=string(original) {t.Fatal("database changed after invalid staged day")}
        if _,err:=os.Stat(backupDir);!os.IsNotExist(err) {t.Fatalf("backup made on invalid data: %v",err)}
    }
}
