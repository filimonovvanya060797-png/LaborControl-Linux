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
        setAttribute() {}, getAttribute() {return null},
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
    const code='current='+JSON.stringify(step.module)+';agSection='+JSON.stringify(step.section||'graph')+';agView='+JSON.stringify(step.view||'month')+';render();';
    try {vm.runInContext(code,ctx,{filename:'render-'+step.module+'-'+(step.section||''),timeout:15000})}
    catch (error) {throw new Error(step.label+': uncaught JavaScript error: '+error.stack)}
    const page=getElement('page').innerHTML;
    assert.ok(page.length > 0,step.label+': blank UI');
    assert.ok(!page.includes('Ошибка интерфейса'),step.label+': '+page.match(/Ошибка интерфейса[\s\S]{0,220}/)?.[0]);
    assert.equal(errors.length,0,step.label+': console.error: '+errors.join('\n'));
    process.stdout.write('OK '+step.label+'\n');
  }
}

const screens = ['dashboard','employees','rvd','check','events','journal','help','settings']
  .map(module=>({module,label:module}));
const sections = ['graph','vacations','constructor','summary','calendar','types','flights','versions']
  .map(section=>({module:'autograf',section,label:'autograf / '+section}));
execute([{module:'dashboard',label:'empty dashboard'},{module:'employees',label:'empty employees'},{module:'autograf',label:'empty autograf'}],fixture(false));
execute([...screens,...sections,
  {module:'autograf',section:'graph',view:'year',label:'autograf / yearly graph'}
],fixture(true));
console.log('R11 UI render smoke: PASS');
