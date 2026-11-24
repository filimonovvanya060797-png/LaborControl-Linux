#!/usr/bin/env node
'use strict';
// Regression against the exact embedded JavaScript, not a reimplementation.
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const path=require('node:path');
const html=fs.readFileSync(path.resolve(__dirname,'../r11_original.html'),'utf8');
const m=html.match(/<script>([\s\S]*?)<\/script>/);
assert.ok(m,'missing embedded JS');
const js=m[1].replace(/\bload\(\);\s*$/,'');
const elements=new Map();
function el(id){if(!elements.has(id))elements.set(id,{value:'',checked:false,style:{},innerHTML:'',textContent:'',querySelectorAll(){return []},querySelector(){return null},focus(){},setAttribute(){}});return elements.get(id)}
const ctx=vm.createContext({
 console, document:{getElementById:el,querySelectorAll(){return []},querySelector(){return null}},
 window:{scrollTo(){}},localStorage:{},setTimeout(){},clearTimeout(){},
 alert(){},confirm(){return false},prompt(){return ''},
 fetch(){throw Error('No real backend in JS smoke')},
 URL,TextDecoder,TextEncoder,Blob
});
vm.runInContext(js,ctx,{filename:'r11_actual_embedded.js',timeout:15000});
function run(expr){return vm.runInContext(expr,ctx,{timeout:15000})}
run('state={data:{employees:[],days:[],calendar:[],events:[],rvd:[],journal:[],flight_dates:[],flight_routes:[],schedule_types:[],graph_revisions:[],graph_versions:[],schedule_changes:[]},discrepancies:[]};normalize();agYear=2026;calMonth=11;calSelectedDate="2026-11-03";');
function plan(v){return JSON.parse(run('JSON.stringify(planInfo('+JSON.stringify(v)+'))'))}
for(const [text,hours] of [['7.3',7.3],['7,3',7.3],['9.2',9.2],['9,2',9.2],['7.25',7.25],['10,5',10.5],['1/9.2',9.2],['2/7,25',7.25],['11',11]]){
  assert.equal(plan(text)?.hours,hours,'decimal parse: '+text);
}
for(const x of ['0','0.5','11.1','12','24','7,abc','9..2','7.1234',''])assert.equal(plan(x),null,'invalid plan accepted: '+x);
assert.equal(plan('В').hours,0);
assert.equal(plan('ДП').hours,0);
assert.equal(run('fmt(7.25)'),'7,25','display must not truncate hundredths');
assert.equal(run('fmt(7.333)'),'7,333','display must not truncate thousandths');
for(const x of ['М','м',' Мужчина ','мужчина'])assert.equal(run('employeeGenderCode('+JSON.stringify(x)+')'),'M','male '+x);
for(const x of ['Ж','ж',' ЖЕНЩИНА ','женщина'])assert.equal(run('employeeGenderCode('+JSON.stringify(x)+')'),'F','female '+x);
assert.equal(run("employeeGenderCode('')"),'');
assert.equal(run("employeeGenderCode('Михаил')"),'');
const mapping=JSON.parse(run("JSON.stringify(autoEmpMap(['Табельный номер','ФИО','Пол']))"));
assert.equal(mapping.gender,'2');
const mappingWithout=JSON.parse(run("JSON.stringify(autoEmpMap(['Табельный номер','ФИО','Полёт']))"));
assert.equal(mappingWithout.gender,'','a flight column must not be read as gender');
run("empImport={name:'test.xlsx',head:['Табельный номер','ФИО','Пол'],rows:[['1','Тестовый мужчина','М'],['2','Тестовая женщина','женщина'],['3','Неизвестный','']]};empImportMap={tab_no:'0',full_name:'1',position:'',unit:'',hire_date:'',fire_date:'',gender:'2'};");
const imp=JSON.parse(run('JSON.stringify(importClassify())'));
assert.equal(imp[0].o.gender,'M');
assert.equal(imp[1].o.gender,'F');
assert.equal(imp[2].status,'new');
assert.match(imp[2].msg,/не указан/);
assert.equal(run('calModeYearNorm(2026)'),1972,'2026 40-hour norm');
run('calModeWeek=36;');
assert.equal(run('calModeYearNorm(2026)'),1774.4,'2026 36-hour norm');
assert.equal(run("calModeHour({norm_hours:8})"),7.2);
assert.equal(run("calModeHour({norm_hours:7})"),6.2);
assert.equal(run("calModeHour({norm_hours:0})"),0);
assert.equal(run("employeeNorm({gender:'F',hire_date:'2026-11-01'},2026)"),301.4,'female norm from hiring date');
assert.equal(run('calModeMonthNorm(2026,11)'),143,'November 36 hour norm');
assert.ok(run('productionCalendar()').includes('Женщины · 36 ч/нед.'),'calendar toggle not visible');
run("state.data.employees=[{id:1,tab_no:'T-01',full_name:'Тест',gender:'F',week_hours:36,hire_date:'2026-11-01'}];state.data.days=[{id:2,employee_id:1,date:'2026-11-03',plan:'7.3',plan_hours:7.3,state:'WORK'}]");
const validation=JSON.parse(run("JSON.stringify(validateGraph(state.data.employees[0],2026))"));
assert.ok(!validation.issues.some(x=>x.rule==='hours'),'fractional plan must not be flagged invalid');
console.log('Gender / 40-36 calendar / decimal hours embedded R11 UI smoke: PASS');
