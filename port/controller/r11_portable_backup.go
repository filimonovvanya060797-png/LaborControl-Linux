package main

import (
 "bytes"
 "crypto/sha256"
 "encoding/json"
 "fmt"
 "io"
 "net/http"
 "os"
 "path/filepath"
 "time"
)

// A portable snapshot contains the two live data stores, not recursively
// nested history copies or browser storage. It never overwrites a live profile.
const portableBackupFormat = "LaborControl-R11-portable/1"
const portableBackupFileLimit = 64 << 20
const portableBackupBodyLimit = 180 << 20

var portableBackupNames = []string{"labor_control.json", "labor_control_tab2.json"}

type portableBackupEntry struct {
 Name string `json:"name"`
 Present bool `json:"present"`
 SHA256 string `json:"sha256"`
 Data []byte `json:"data"`
}
type portableBackup struct {
 Format string `json:"format"`
 CreatedAt string `json:"created_at"`
 Files []portableBackupEntry `json:"files"`
}
func portableHash(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

func readPortableEntry(base, name string) (portableBackupEntry, error) {
 e := portableBackupEntry{Name:name}
 path := filepath.Join(base,name)
 info, err := os.Lstat(path)
 if os.IsNotExist(err) { return e,nil }
 if err != nil { return e,err }
 if !info.Mode().IsRegular() || info.Size() > portableBackupFileLimit {
  return e,fmt.Errorf("неподдерживаемый тип или размер файла %s",name)
 }
 f,err:=os.Open(path); if err!=nil{return e,err}; defer f.Close()
 e.Data,err=io.ReadAll(io.LimitReader(f,portableBackupFileLimit+1))
 if err!=nil{return e,err}
 if len(e.Data)>portableBackupFileLimit{return e,fmt.Errorf("файл %s слишком большой",name)}
 e.Present=true; e.SHA256=portableHash(e.Data)
 return e,nil
}

// Validate raw JSON without decoding integer-only legacy day-hour structs;
// decimal hours and unknown fields must survive byte-for-byte.
func validatePortableEntry(e portableBackupEntry) error {
 if !e.Present {
  if len(e.Data)!=0 || e.SHA256!="" {return fmt.Errorf("противоречивое описание отсутствующего файла")}
  return nil
 }
 if len(e.Data)==0 || len(e.Data)>portableBackupFileLimit || portableHash(e.Data)!=e.SHA256 {
  return fmt.Errorf("контрольная сумма или размер файла %s неверны",e.Name)
 }
 var root map[string]json.RawMessage
 if err:=json.Unmarshal(e.Data,&root);err!=nil||root==nil{return fmt.Errorf("повреждён JSON файла %s",e.Name)}
 if e.Name==portableBackupNames[0] {
  if wrapped,ok:=root["data"];ok {
   if err:=json.Unmarshal(wrapped,&root);err!=nil||root==nil{return fmt.Errorf("повреждена структура основной базы")}
  }
 }
 var version,next int
 if json.Unmarshal(root["schema_version"],&version)!=nil || json.Unmarshal(root["next_id"],&next)!=nil || next<1 {
  return fmt.Errorf("не распознана версия или счётчик базы %s",e.Name)
 }
 var keys []string
 switch e.Name {
 case "labor_control.json":
  if version!=8 {return fmt.Errorf("неподдерживаемая версия основной базы: %d",version)}
  keys=r11RequiredDataKeys
 case "labor_control_tab2.json":
  if version!=2 {return fmt.Errorf("неподдерживаемая версия Табеля 2.0: %d",version)}
  keys=[]string{"imports","sap_records","facts","szrvd"}
 default:return fmt.Errorf("неизвестный файл")
 }
 for _,key:=range keys {
  if key=="schema_version"||key=="next_id"{continue}
  var rows []json.RawMessage
  v,ok:=root[key]; if !ok || json.Unmarshal(v,&rows)!=nil || bytes.Equal(bytes.TrimSpace(v),[]byte("null")) {
   return fmt.Errorf("не распознано поле %s в %s",key,e.Name)
  }
 }
 return nil
}

func validatePortableBackup(b portableBackup) error {
 if b.Format!=portableBackupFormat || len(b.Files)!=len(portableBackupNames) {return fmt.Errorf("неподдерживаемый формат резервной копии")}
 if _,err:=time.Parse(time.RFC3339Nano,b.CreatedAt);err!=nil{return fmt.Errorf("неверная дата копии")}
 for i,name:=range portableBackupNames{
  if b.Files[i].Name!=name{return fmt.Errorf("неверный перечень файлов резервной копии")}
  if err:=validatePortableEntry(b.Files[i]);err!=nil{return err}
 }
 if !b.Files[0].Present && !b.Files[1].Present{return fmt.Errorf("база ещё не создана: копировать нечего")}
 return nil
}
func decodePortableBackup(r io.Reader)(portableBackup,error){
 var b portableBackup
 d:=json.NewDecoder(r);d.DisallowUnknownFields()
 if err:=d.Decode(&b);err!=nil{return b,fmt.Errorf("не удалось прочитать резервную копию: %w",err)}
 var extra any
 if err:=d.Decode(&extra);err!=io.EOF{return b,fmt.Errorf("лишние данные после резервной копии")}
 return b,validatePortableBackup(b)
}

func (h *controller) capturePortableBackup()(portableBackup,error){
 // Same lock order as fact/SAP imports. No live writes during capture.
 h.opMu.Lock();defer h.opMu.Unlock()
 tab2StoreMu.Lock();defer tab2StoreMu.Unlock()
 b:=portableBackup{Format:portableBackupFormat,CreatedAt:time.Now().UTC().Format(time.RFC3339Nano)}
 for _,name:=range portableBackupNames{
  e,err:=readPortableEntry(h.baseDir,name);if err!=nil{return b,err}
  b.Files=append(b.Files,e)
 }
 return b,validatePortableBackup(b)
}

func (h *controller) portableBackupDownload(w http.ResponseWriter,r *http.Request){
 if r.Method!=http.MethodGet{http.Error(w,"GET only",405);return}
 b,err:=h.capturePortableBackup();if err!=nil{http.Error(w,err.Error(),400);return}
 // No success claim before the user saves and reopens the downloaded file.
 w.Header().Set("Cache-Control","no-store")
 w.Header().Set("Content-Type","application/octet-stream")
 w.Header().Set("Content-Disposition",`attachment; filename="LaborControl_`+time.Now().Format("20060102_150405")+`.lcbackup"`)
 if err:=json.NewEncoder(w).Encode(b);err!=nil{return}
}

// Stage into a new private profile only. No archive paths are interpreted,
// no live database is replaced, and no previous profile is removed.
func (h *controller) preparePortableRestore(b portableBackup)(string,error){
 if err:=validatePortableBackup(b);err!=nil{return "",err}
 parent:=filepath.Dir(h.baseDir)
 profile,err:=os.MkdirTemp(parent,"LaborControl-R11-Restored-");if err!=nil{return "",err}
 success:=false;defer func(){if !success{_ = os.RemoveAll(profile)}}()
 dir:=filepath.Join(profile,"LaborControl")
 if err=os.Mkdir(dir,0700);err!=nil{return "",err}
 for _,e:=range b.Files{
  if !e.Present{continue}
  if err=atomicWrite(filepath.Join(dir,e.Name),e.Data);err!=nil{return "",err}
  check,err:=readPortableEntry(dir,e.Name)
  if err!=nil{return "",err}
  if check.SHA256!=e.SHA256 || !bytes.Equal(check.Data,e.Data){return "",fmt.Errorf("проверка восстановленного файла не пройдена")}
 }
 success=true
 return profile,nil
}

func (h *controller) portableBackupInspect(w http.ResponseWriter,r *http.Request){
 if r.Method!=http.MethodPost{http.Error(w,"POST only",405);return}
 r.Body=http.MaxBytesReader(w,r.Body,portableBackupBodyLimit)
 b,err:=decodePortableBackup(r.Body);if err!=nil{http.Error(w,err.Error(),400);return}
 result:=map[string]any{"ok":true,"created_at":b.CreatedAt,"main_present":b.Files[0].Present,"tab2_present":b.Files[1].Present}
 if r.URL.Path=="/api/backup/prepare-restore"{
  path,err:=h.preparePortableRestore(b);if err!=nil{http.Error(w,err.Error(),500);return}
  result["profile_path"]=path
 }
 w.Header().Set("Cache-Control","no-store")
 writeJSON(w,result)
}
