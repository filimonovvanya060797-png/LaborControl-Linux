#!/usr/bin/env node
'use strict';

// Smoke-test the *actual embedded R11 JavaScript* without external packages or a browser.
// This catches ReferenceError / TypeError failures in screen rendering that Go tests
// and a successful GTK compilation cannot see.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const html = fs.readFileSync(path.resolve(__dirname, '../r11_original.html'), 'utf8');
const match = html.match(/<script>([\s\S]*?)<\/script>/);
assert.ok(match, 'R11 inline JavaScript script is missing');
const original = match[1];
const bootless = original.replace(/\bload\(\);\s*$/, '');
assert.notEqual(bootless, original, 'Could not isolate asynchronous load() from R11 UI');

function fixture(withEmployee) {
  const names = [
    'employees','days','rvd','events','journal','calendar','graph_versions',
    'graph_revisions','schedule_changes','schedule_types','flight_routes',
    'flight_dates','employee_flights','notes','manual_colors'
  ];
  const data = Object.fromEntries(names.map(name => [name, []]));
  if (withEmployee) {
    data.employees.push({
      id: 1, tab_no: '990001', full_name: 'Тестов Иван Иванович',
      position: 'Водитель', unit: 'Колонна 1',
      collection_point: 'Тюмень', schedule_type: '', is_quarter: false,
      hire_date: '', fire_date: '', probation: false,
      probation_end: '', main_schedule_ready: false, notes: ''
    });
    data.days.push({
      id: 2, employee_id: 1, date: '2026-11-20',
      plan: '1/8', plan_hours: 8, fact: '', fact_hours: 0,
      shift: 'DAY', state: 'WORK', comment: ''
    });
  }
  return {version:'test',data_path:'/tmp/r11-ui-smoke.json',data,discrepancies:[]};
}

function execute(config, state) {
  const elements = new Map();
  const errors = [];
  function getElement(id) {
    if (!elements.has(id)) {
      elements.set(id,{
        innerHTML: '', textContent:'', value:'', checked:false, disabled:false,
        style:{}, dataset:{}, files:[],
        focus() {}, setAttribute() {}, getAttribute() {return null},
        querySelector() {return null}, querySelectorAll(){return []}
      });
    }
    return elements.get(id);
  }
  const ctx = vm.createContext({
    console:{log(){},warn(){},error(...args){errors.push(args.map(String).join(' '))}},
    document:{getElementById:getElement,querySelector(){return null},querySelectorAll(){return []}},
    window:{scrollTo(){}},
    localStorage:{}, 
    setTimeout(){return 1},clearTimeout(){}, 
    alert(){}, confirm(){return false},prompt(){return ''},
    fetch(){throw new Error('UI rendering must not access a live backend')},
    URL,TextDecoder,TextEncoder,Blob
  });
  vm.runInContext(bootless,ctx,{filename:'r11_original.js',timeout:15000});
  vm.runInContext(
    'state='+JSON.stringify(state)+';normalize();agYear=2026;agMonth=11;calMonth=11;selectedDate="2026-11-20";agTab='+JSON.stringify(state.data.employees[0]?.tab_no||'')+';',
    ctx,{timeout:15000}
  );
  for (const step of config) {
    errors.length=0;
    const prepare=(step.archived ? 'graphRevisionPreviewId=10;graphRevisionPreviewData=state.data.graph_revisions[0];' : '')+(step.staleTab ? 'agTab='+JSON.stringify(step.staleTab)+';' : '')+(step.invalidEmployeeImport ? 'empImport={name:"errors.xlsx",head:["Табельный №","ФИО"],rows:[["","Без номера"]]};empImportMap={tab_no:"0",full_name:"1",position:"",unit:"",hire_date:"",fire_date:""};' : '')+(step.illustratedHelp ? 'guideSelected="employees-illustrated";guideIllustratedAnchor="excelload";' : '')+(step.incompatiblePoint ? 'empPointImport={name:"points.xlsx",head:["Табельный №","ФИО","Пункт сбора"],rows:[["990001","Тестов Иван Иванович","Уфа"]],map:{t:0,f:1,p:2,n:-1}};' : '')+(step.technicalVersion ? 'versionViewId=20;versionViewMode='+JSON.stringify(step.technicalVersion)+';' : '')+(step.lifecycleStatus ? 'employeeLifecycleStatus=function(){return '+JSON.stringify(step.lifecycleStatus)+';};' : '');
    const code=prepare+'current='+JSON.stringify(step.module)+';agSection='+JSON.stringify(step.section||'graph')+';agView='+JSON.stringify(step.view||'month')+';render();';
    try {vm.runInContext(code,ctx,{filename:'render-'+step.module+'-'+(step.section||''),timeout:15000})}
    catch (error) {throw new Error(step.label+': uncaught JavaScript error: '+error.stack)}
    const page=getElement('page').innerHTML;
    assert.ok(page.length > 0,step.label+': blank UI');
    assert.ok(!page.includes('Ошибка интерфейса'),step.label+': '+page.match(/Ошибка интерфейса[\s\S]{0,220}/)?.[0]);
    if(step.technicalVersion)assert.ok(page.includes('id="versionPreview"'),step.label+': selected technical version preview not shown');
    assert.equal(errors.length,0,step.label+': console.error: '+errors.join('\n'));
    if (step.checkCsv) {
      const parsed=vm.runInContext('parseCsvText('+JSON.stringify(step.checkCsv.text)+')',ctx);
      assert.equal(JSON.stringify(parsed),JSON.stringify(step.checkCsv.expected),step.label+': CSV parsed incorrectly');
    }
    if (step.verifyDate) {
      for (const [raw,expected] of [
        ['01.11.2026','2026-11-01'],
        ['2026-11-18','2026-11-18'],
        ['',''],
        ['31.02.2026',null],
        ['29.02.2024','2024-02-29'],
        ['29.02.2025',null],
        ['1.11.2026',null],
        ['99.12.2026',null],
      ]) {
        const actual=vm.runInContext('employeeDateISO('+JSON.stringify(raw)+')',ctx);
        assert.equal(actual,expected,'employee date conversion failed: '+raw);
      }
      vm.runInContext("employeeDateSet('efire','2026-11-18')",ctx);
      assert.equal(getElement('efiretext').value,'18.11.2026','manual text must sync from ISO');
      vm.runInContext("employeeDateClear('efire')",ctx);
      assert.equal(getElement('efiretext').value,'','clear must empty the visible date');
      assert.equal(getElement('efire').value,'','clear must empty the persisted ISO value');
      vm.runInContext("document.getElementById('efiretext').value='15.11.2026';employeeDateTyped('efire')",ctx);
      assert.equal(getElement('efire').value,'2026-11-15','manual input must update the ISO value');
    }
    if (typeof step.verifyPage === 'function') step.verifyPage(page);
    process.stdout.write('OK '+step.label+'\n');
  }
}

const screens = ['dashboard','employees','rvd','check','events','journal','help','settings']
  .map(module=>({module,label:module}));
const sections = ['graph','vacations','constructor','summary','calendar','types','flights','versions']
  .map(section=>({module:'autograf',section,label:'autograf / '+section}));
execute([{module:'dashboard',label:'empty dashboard'},{module:'employees',label:'empty employees'},{module:'autograf',label:'empty autograf'}],fixture(false));

const deletedSelectedEmployee = fixture(true);
deletedSelectedEmployee.data.graph_revisions.push({
  id:10,employee_id:99,number:1,status:'ARCHIVED',
  effective_date:'2026-11-01',reason:'Удалённый сотрудник',
  days_snapshot:[],events_snapshot:[],notes_snapshot:[],colors_snapshot:[]
});
execute([{
  module:'autograf',section:'graph',label:'deleted selected employee falls back to remaining employee',
  staleTab:'990099',archived:true,
  verifyPage(page) {
    assert.ok(page.includes('Тестов Иван Иванович'),'remaining employee should become selected');
    assert.ok(!page.includes('Просмотр архивной редакции'),'stale archive preview must be cleared');
  }
}],deletedSelectedEmployee);


execute([{
  module:'help',label:'illustrated employee handbook is accessible inside the Instruction tab',
  illustratedHelp:true,
  verifyPage(page){
    assert.ok(page.includes('id="employeeGuideFrame"'),'embedded illustration iframe must be visible');
    assert.ok(page.includes('src="/help/employees#excelload"'),'deep-link for import must resolve');
    assert.ok(page.includes('Ко всем главам'),'there must be a way back to general instructions');
  }
}],fixture(true));

execute([{
  module:'employees',label:'employee editable dates, calendar and help',verifyDate:true,
  verifyPage(page) {
    for(const id of ['ehiretext','efiretext','ehirepicker','efirepicker']) {
      assert.ok(page.includes('id="'+id+'"'),'employee date field missing: '+id);
    }
    assert.equal((page.match(/Очистить дату<\/button>/g)||[]).length,2,
      'both dates need an explicit local clear control');
    assert.ok(page.includes('ДД.ММ.ГГГГ'),'manual date format must be visible');
  }
}],fixture(true));

const unsafeName=fixture(true);
unsafeName.data.employees[0].full_name="О'Брайен \"Иван\" <Проверка>";
execute([{module:'employees',label:'employee edit button is safe for quotes and HTML',
  verifyPage(page) {
    assert.ok(page.includes('onclick="editEmpById(1)"'),'edit should use stable numeric ID');
    assert.ok(!page.includes("onclick='editEmp("),'employee JSON must not be interpolated into attributes');
    assert.ok(page.includes('О&#39;Брайен'),'apostrophe should be HTML-escaped');
  }}],unsafeName);
execute([{
  module:'employees',label:'invalid import button is disabled with an explanation',
  invalidEmployeeImport:true,
  verifyPage(page) {
    assert.ok(page.includes('Ошибок: 1'),'error count should be visible');
    assert.ok(page.includes('disabled style="opacity:.45;cursor:not-allowed"'),'button should be disabled');
    assert.ok(page.includes('Импорт недоступен: в файле 1 ошибка'),'reason for blocking import should be visible');
    assert.ok(page.includes('Записей из выбранного Excel/CSV: 1'),'import preview must explain what rows represent');
    assert.ok(page.includes('Пока ничего не сохранено'),'preview must explain data are not yet imported');
    assert.ok(page.includes('<th>№ записи</th>'),'preview table must number Excel records');
    assert.ok(page.includes('Не указан табельный номер'),'missing tab must be precise');
    assert.ok(page.includes('«Проверить файл»'),'guidance must reference actual button');
    assert.ok(!page.includes('Прочитать файл заново'),'guidance must never invent a button');
  }
}],fixture(true));
execute([{
  module:'employees',label:'quoted CSV with newlines and separators',
  checkCsv:{
    text:'Табельный №;ФИО;Подразделение\r\n42;"О""Нил; Иван\nПетрович";"Колонна 2"\r\n',
    expected:[['Табельный №','ФИО','Подразделение'],['42','О"Нил; Иван\nПетрович','Колонна 2']]
  }
}],fixture(true));


const incompatiblePoint=fixture(true);
incompatiblePoint.data.employees[0].schedule_type='Тюмень-А';
incompatiblePoint.data.schedule_types.push({
  id:3,collection_point:'Тюмень',name:'Тюмень-А',phase:'A',active:true
});
execute([{module:'employees',incompatiblePoint:true,
  label:'point import cannot silently retain incompatible assigned schedule',
  verifyPage(page) {
    assert.ok(page.includes('Вид графика «Тюмень-А» не подходит к пункту «Уфа»'),
      'incompatible schedule should be rejected before point import');
    assert.ok(page.includes('Ошибок: 1'),'point import must flag one error');
  }
}],incompatiblePoint);
const probationDisplay = fixture(true);
probationDisplay.data.employees[0].probation = true;
probationDisplay.data.employees[0].probation_end = '2027-04-14';
execute([{
  module:'autograf',section:'graph',label:'probation status is shown only once',
  lifecycleStatus:{kind:'neutral',code:'probation',text:'Испытательный срок до 14.04.2027.'},
  verifyPage(page) {
    assert.ok(page.includes('<b>Испытательный срок</b>'),'probation heading missing');
    assert.equal((page.match(/до 14\.04\.2027\./g)||[]).length,1,
      'the probation end date must be rendered once, not as a duplicate line');
    assert.ok(!page.includes('Испытательный срок: до 14.04.2027.'),
      'legacy duplicate prefix must not be rendered');
  }
}],probationDisplay);

execute([...screens,...sections,
  {module:'autograf',section:'graph',view:'year',label:'autograf / yearly graph'}
],fixture(true));

// A real user's R11 graph may already contain an official revision, notes,
// colors and a technical rollback snapshot. Exercise these distinct paths.
const versioned = fixture(true);
versioned.data.graph_revisions.push({
  id:10,employee_id:1,number:1,status:'ACTIVE',
  effective_date:'2026-11-01',schedule_type:'',reason:'Исходный график',
  days_snapshot:[...versioned.data.days],
  events_snapshot:[],notes_snapshot:[],colors_snapshot:[]
});
versioned.data.graph_versions.push({
  id:20,employee_id:1,year:2026,label:'Тест до изменения',days:[...versioned.data.days]
});
versioned.data.notes.push({
  id:30,employee_id:1,date_from:'2026-11-20',date_to:'2026-11-20',
  text:'Тестовое примечание',reminder:''
});
versioned.data.manual_colors.push({
  id:40,employee_id:1,date_from:'2026-11-20',date_to:'2026-11-20',
  color:'yellow'
});
execute([...sections,
  {module:'autograf',section:'graph',view:'year',label:'versioned / yearly graph'},
  {module:'autograf',section:'versions',technicalVersion:'view',label:'technical version / open previously saved version'},
  {module:'autograf',section:'versions',technicalVersion:'compare',label:'technical version / compare previously saved version'}
],versioned);

// Scale check: the intended workstation may hold two columns (~450 drivers).
const large = fixture(false);
for(let i=1;i<=450;i++){
  large.data.employees.push({
    id:i,tab_no:String(990000+i),full_name:'Водитель '+i,
    position:'Водитель',unit:i<=225?'Колонна 1':'Колонна 2',
    collection_point:'Тюмень',schedule_type:'',is_quarter:false,
    hire_date:'',fire_date:'',probation:false,probation_end:'',
    main_schedule_ready:false,notes:''
  });
}
execute([
  {module:'dashboard',label:'450 drivers / dashboard'},
  {module:'employees',label:'450 drivers / employees'},
  {module:'autograf',section:'graph',label:'450 drivers / active graph'}
],large);

// Regression path for viewing an archived official revision after a second
// official version becomes active; this exercises historic snapshot rendering.
const archived = fixture(true);
archived.data.graph_revisions.push(
  {id:10,employee_id:1,number:1,status:'ARCHIVED',
   effective_date:'2026-11-01',reason:'Исходный график',
   days_snapshot:[...archived.data.days],events_snapshot:[],
   notes_snapshot:[],colors_snapshot:[]},
  {id:11,employee_id:1,number:2,status:'ACTIVE',
   effective_date:'2026-11-21',reason:'Новая редакция',
   days_snapshot:[],events_snapshot:[],notes_snapshot:[],colors_snapshot:[]}
);
execute([
  {module:'autograf',section:'graph',label:'second official revision / current'},
  {module:'autograf',section:'versions',label:'second official revision / versions'},
  {module:'autograf',section:'graph',archived:true,label:'archived official revision / monthly snapshot'},
  {module:'autograf',section:'graph',archived:true,view:'year',label:'archived official revision / year snapshot'}
],archived);
console.log('R11 UI render smoke: PASS');
