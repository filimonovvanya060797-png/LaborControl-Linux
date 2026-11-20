package main

import _ "embed"

//go:embed r11_original.html
var r11OriginalHTML []byte

const r11WindowsImportPanel = `<details id="r11-windows-import" class="panel" style="margin-bottom:14px">
<summary style="cursor:pointer;font-weight:700">Перенос базы из Windows R11</summary>
<div class="info-note" style="margin-top:12px">Импортируйте полный JSON-ответ Windows R11 из <code>/api/state</code>. Файл схемы <code>LaborControl_R11_schema.json</code> для этого не подходит. Импорт заменяет текущее локальное состояние; перед заменой существующей базы автоматически создаётся резервная копия. Файл остаётся на этом компьютере.</div>
<div class="filebox"><div><label for="r11-windows-file">Полный JSON-экспорт Windows R11</label><input id="r11-windows-file" type="file" accept=".json,application/json"></div><button class="primary" type="button" onclick="r11ImportWindowsState()">Импортировать базу</button></div>
<div id="r11-windows-import-status" class="small muted" style="margin-top:8px">Пока не выбран файл.</div>
</details>
<script>
async function r11ImportWindowsState(){
  const input=document.getElementById('r11-windows-file');
  const status=document.getElementById('r11-windows-import-status');
  const file=input?.files?.[0];
  if(!file){if(status)status.textContent='Сначала выберите JSON-файл.';return}
  if(!confirm('Импорт заменить текущее локальное состояние LaborControl? Если база уже существует, перед заменой будет создана резервная копия.'))return;
  if(status)status.textContent='Проверка и импорт файла…';
  try{
    const response=await fetch('/api/state/import',{method:'POST',headers:{'Content-Type':'application/json'},body:await file.text()});
    const raw=await response.text();
    let data={};try{data=JSON.parse(raw)}catch(_){}
    if(!response.ok)throw new Error(data.error||raw||('HTTP '+response.status));
    if(typeof load==='function')await load();
    if(status)status.textContent='Импортировано сотрудников: '+(data.employees??'?')+'; дней: '+(data.days??'?')+'. Резервная копия: '+(data.backup||'не требовалась')+'.';
  }catch(error){if(status)status.textContent='Ошибка импорта: '+(error?.message||String(error))}
}
</script>`;
