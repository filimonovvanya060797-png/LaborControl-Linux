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
assert.equal(run("importExpectedWeek({o:{gender:'F'},e:{gender:'M',week_hours:40}})"),36,'gender re-import should change the default week');
assert.equal(run("importExpectedWeek({o:{gender:'M'},e:{gender:'F',week_hours:35,norm_reason:'Трудовой договор'}})"),35,'documented custom week must survive gender import');
assert.equal(run("importExpectedWeek({o:{gender:''},e:{gender:'F',week_hours:36}})"),36,'empty Excel gender must preserve previous week');
assert.equal(run("importExpectedWeek({o:{gender:''}})"),0,'unknown sex must not assign an implicit 40h norm');
console.log('Gender / 40-36 calendar / decimal hours embedded R11 UI smoke: PASS');

// #47: literal interpolation must not appear on the calendar.
run('calModeWeek=40;');
assert.match(run('productionCalendar()'),/отображаемая норма 1972 ч \(40 ч\/нед\.\)/);
assert.ok(!run('productionCalendar()').includes('${fmt(calModeYearNorm(agYear))}'));
run('calModeWeek=36;');
assert.match(run('productionCalendar()'),/отображаемая норма 1774,4 ч \(36 ч\/нед\.\)/);
assert.equal(run("calModeRangeNorm(2026,'2026-11-02','2026-11-04')"),13.4);
run('calModeWeek=40;');
assert.equal(run("calModeRangeNorm(2026,'2026-11-02','2026-11-04')"),15);
assert.equal(run("employeeNorm({gender:'F',week_hours:36},2026)"),1774.4);
for(const [week,normal,short,nov] of [[40,8,7,159],[36,7.2,6.2,143],[35,7,6,139],[24,4.8,3.8,95]]) {
  const p=JSON.parse(run("JSON.stringify(buildFiveDay({id:10,work_mode:'FIVE_DAY',week_hours:"+week+"},'2026-11-01','2026-11-30'))"));
  assert.equal(Object.keys(p.days).length,30);
  assert.equal(p.days['2026-11-02'].hours,normal);
  assert.equal(p.days['2026-11-03'].hours,short);
  assert.equal(p.days['2026-11-04'].hours,0);
  assert.equal(p.days['2026-11-07'].plan,'В');
  assert.equal(p.monthly[0].plan,nov);
}
const bounded=JSON.parse(run("JSON.stringify(buildFiveDay({id:10,work_mode:'FIVE_DAY',week_hours:36,hire_date:'2026-11-02',fire_date:'2026-11-04'},'2026-11-01','2026-11-30'))"));
assert.deepEqual(Object.keys(bounded.days),['2026-11-02','2026-11-03','2026-11-04']);
assert.equal(bounded.monthly[0].plan,13.4);
assert.throws(()=>run("buildFiveDay({id:10,norm_pending:true},'2026-11-01','2026-11-30')"));
assert.throws(()=>run("buildFiveDay({id:10,week_hours:40},'2027-11-01','2027-11-30')"));
assert.equal(run("employeeWorkMode({})"),'SHIFT');
assert.equal(run("workModeCode('  ПЯТИДНЕВКА  ')"),'FIVE_DAY');
assert.equal(run("workModeCode('Вахтовый метод')"),'SHIFT');
assert.equal(run("workModeCode('6/1')"),'');
assert.equal(run("autoEmpMap(['Табельный номер','ФИО','Режим работы']).work_mode"),'2');
run("state.data.employees=[{id:1,tab_no:'T-01',full_name:'Тест',gender:'F',week_hours:35,norm_reason:'Основание',work_mode:'FIVE_DAY'}];state.data.days=[];empImport={head:['Табельный номер','ФИО','Пол','Режим работы'],rows:[['T-01','Тест','М',''],['T-02','Новый','Ж','5/2'],['T-03','Ошибка','М','6/1']]};empImportMap=autoEmpMap(empImport.head)");
const update=JSON.parse(run('JSON.stringify(importClassify())'));
assert.equal(update[0].status,'change');
assert.equal(update[0].o.work_mode,'');
assert.equal(update[0].e.work_mode,'FIVE_DAY');
assert.equal(update[1].o.work_mode,'FIVE_DAY');
assert.equal(update[2].status,'error');
assert.equal(run("importExpectedWeek(importClassify()[0])"),35);
run("state.data.days=[{employee_id:1,date:'2026-11-02',plan:'7',plan_hours:7}];empImport.rows=[['T-01','Тест','Ж','Вахта']]");
assert.equal(run("importClassify()[0].status"),'error');
console.log('Journal #47: calendar toggle and five-day construction/import smoke: PASS');
