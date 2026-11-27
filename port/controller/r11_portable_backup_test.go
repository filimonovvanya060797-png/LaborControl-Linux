package main

import (
 "bytes"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestPortableBackupRoundTripAndIsolation(t *testing.T) {
 base:=filepath.Join(t.TempDir(),"LaborControl")
 if err:=os.Mkdir(base,0700);err!=nil{t.Fatal(err)}
 h:=&controller{baseDir:base}
 // Preserve decimal hours, extra fields and unknown metadata, not a typed reserialization.
 var root map[string]json.RawMessage
 if err:=json.Unmarshal(emptyR11DataJSON(),&root);err!=nil{t.Fatal(err)}
 root["days"]=json.RawMessage(`[{"id":1,"employee_id":2,"date":"2026-11-03","plan":"7.3","plan_hours":7.3}]`)
 root["extra_field"]=json.RawMessage(`{"test":"сохранить"}`)
 main,err:=json.Marshal(root);if err!=nil{t.Fatal(err)}
 tab:=[]byte(`{"schema_version":2,"next_id":2,"imports":[],"sap_records":[],"facts":[{"id":1,"hours":9.2}],"sz_rvd":[]}`)
 if err=atomicWrite(h.mainDBPath(),main);err!=nil{t.Fatal(err)}
 if err=atomicWrite(h.tab2Path(),tab);err!=nil{t.Fatal(err)}
 // Ordinary backup directory is not included recursively.
 if err=os.Mkdir(filepath.Join(base,"backups"),0700);err!=nil{t.Fatal(err)}
 download:=httptest.NewRecorder()
 h.r11CoreAPI(download,httptest.NewRequest(http.MethodGet,"/api/backup/portable",nil))
 if download.Code!=200{t.Fatalf("download %d: %s",download.Code,download.Body.String())}
 if !strings.Contains(download.Header().Get("Content-Disposition"),".lcbackup"){t.Fatal("missing download filename")}
 archive:=append([]byte(nil),download.Body.Bytes()...)
 b,err:=decodePortableBackup(bytes.NewReader(archive));if err!=nil{t.Fatal(err)}
 if !bytes.Equal(b.Files[0].Data,main)||!bytes.Equal(b.Files[1].Data,tab){t.Fatal("snapshot changed raw bytes")}
 // Change active data after snapshot. Restoration must not overwrite it.
 changed:=append(append([]byte(nil),main...), '\n')
 if err=atomicWrite(h.mainDBPath(),changed);err!=nil{t.Fatal(err)}
 inspect:=httptest.NewRecorder()
 h.r11CoreAPI(inspect,httptest.NewRequest(http.MethodPost,"/api/backup/inspect",bytes.NewReader(archive)))
 if inspect.Code!=200{t.Fatalf("inspect %d: %s",inspect.Code,inspect.Body.String())}
 before,_:=os.ReadDir(filepath.Dir(base))
 restore:=httptest.NewRecorder()
 h.r11CoreAPI(restore,httptest.NewRequest(http.MethodPost,"/api/backup/prepare-restore",bytes.NewReader(archive)))
 if restore.Code!=200{t.Fatalf("restore %d: %s",restore.Code,restore.Body.String())}
 var result struct{ ProfilePath string `json:"profile_path"` }
 if err=json.Unmarshal(restore.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
 if result.ProfilePath==""||filepath.Dir(result.ProfilePath)!=filepath.Dir(base){t.Fatalf("unsafe profile path %q",result.ProfilePath)}
 restored:=filepath.Join(result.ProfilePath,"LaborControl")
 for i,name:=range portableBackupNames{
  got,err:=os.ReadFile(filepath.Join(restored,name));if err!=nil||!bytes.Equal(got,b.Files[i].Data){t.Fatalf("restored %s differs: %v",name,err)}
 }
 if got,err:=os.ReadFile(h.mainDBPath());err!=nil||!bytes.Equal(got,changed){t.Fatal("active database overwritten")}
 if got,err:=os.ReadFile(h.tab2Path());err!=nil||!bytes.Equal(got,tab){t.Fatal("active tab2 overwritten")}
 after,_:=os.ReadDir(filepath.Dir(base));if len(after)!=len(before)+1{t.Fatal("unexpected profile count")}
 // Corruption, unexpected paths, trailing data and future versions must fail
 // without creating another profile.
 for _,kind:=range []string{"hash","path","future","trailing"}{
  bad,err:=decodePortableBackup(bytes.NewReader(archive));if err!=nil{t.Fatal(err)}
  switch kind{case "hash":bad.Files[0].Data[0]^=1;case "path":bad.Files[0].Name="../outside";case "future":bad.Format="unknown"}
  payload,err:=json.Marshal(bad);if err!=nil{t.Fatal(err)}
  if kind=="trailing"{payload=append(payload,[]byte(" {}")...)}
  w:=httptest.NewRecorder();h.r11CoreAPI(w,httptest.NewRequest(http.MethodPost,"/api/backup/prepare-restore",bytes.NewReader(payload)))
  if w.Code!=400{t.Fatalf("%s accepted: %d",kind,w.Code)}
 }
 final,_:=os.ReadDir(filepath.Dir(base));if len(final)!=len(after){t.Fatal("failed restore created profile")}
}

func TestPortableBackupMissingStoreAndBadJSON(t *testing.T){
 h:=&controller{baseDir:t.TempDir()}
 if _,err:=h.capturePortableBackup();err==nil{t.Fatal("empty profile accepted")}
 if err:=atomicWrite(h.mainDBPath(),emptyR11DataJSON());err!=nil{t.Fatal(err)}
 b,err:=h.capturePortableBackup();if err!=nil{t.Fatal(err)}
 if !b.Files[0].Present||b.Files[1].Present{t.Fatal("absence not recorded")}
 if err:=atomicWrite(h.tab2Path(),[]byte("{broken"));err!=nil{t.Fatal(err)}
 if _,err=h.capturePortableBackup();err==nil{t.Fatal("corrupt Tab2 silently accepted")}
}
