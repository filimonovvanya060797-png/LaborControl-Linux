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
assert.equal(run("autoEmpMap(['Табельный номер','ФИО','Режим работы']).work_mode"),undefined);
run("state.data.employees=[{id:1,tab_no:'T-01',full_name:'Тест',gender:'F',week_hours:35,norm_reason:'Основание',work_mode:'FIVE_DAY'}];state.data.days=[];empImport={head:['Табельный номер','ФИО','Пол','Режим работы'],rows:[['T-01','Тест','М',''],['T-02','Новый','Ж','5/2'],['T-03','Режим не используется','М','6/1']]};empImportMap=autoEmpMap(empImport.head)");
const update=JSON.parse(run('JSON.stringify(importClassify())'));
assert.equal(update[0].status,'change');
assert.equal(update[0].o.work_mode,undefined);
assert.equal(update[0].e.work_mode,'FIVE_DAY');
assert.equal(update[1].status,'new');
assert.equal(update[2].status,'new');
assert.equal(run("importExpectedWeek(importClassify()[0])"),35);
assert.equal(run("empPayload(state.data.employees[0],'').work_mode"),undefined);
assert.ok(!run('employees()').includes('id="eworkmode"'));
run("agTab='T-01';constructorMethodKey='';constructorForm=null");
assert.equal(run('constructorBuildMode(agEmployee())'),'SHIFT');
assert.match(run('autografConstructor()'),/Способ построения графика/);
run("constructorMethod='FIVE_DAY'");
assert.match(run('autografConstructor()'),/Построить по календарю/);
assert.ok(!run('autografConstructor()').includes('id="cshifthours"'));
assert.equal(run("buildFiveDay({id:10,week_hours:40},'2026-01-01','2026-12-31').monthly.reduce((s,m)=>s+m.plan,0)"),1972);
run("state.data.days=[{employee_id:1,date:'2026-11-02',plan:'7',plan_hours:7,construction_mode:'FIVE_DAY'}];agView='month';agMonth=11");
assert.equal(run('visibleConstructionMode(agEmployee())'),'FIVE_DAY');
assert.equal(run('latestConstructionMode(agEmployee())'),'FIVE_DAY');
run("state.data.days.push({employee_id:1,date:'2026-11-03',plan:'11',plan_hours:11,construction_mode:'SHIFT'})");
assert.equal(run('visibleConstructionMode(agEmployee())'),'MIXED');
assert.equal(run('latestConstructionMode(agEmployee())'),'SHIFT');
console.log('Journal #47: calendar and constructor-method smoke: PASS');

// #47 Nov 28: visual balance and construction provenance, never mutate stored cells.
for(const [raw,expected] of [[1774.400000000005-1774.4,0],[459.799999999999-459.8,0],[1972-1972,0],[0.001,0.001],[-0.001,-0.001]])assert.equal(run('roundedBalance('+raw+')'),expected);
for(const [week,normal,short] of [[40,8,7],[36,7.2,6.2],[35,7,6],[24,4.8,3.8]]){
 const e={id:1,week_hours:week};
 const check=(date,h,mode='FIVE_DAY')=>JSON.parse(run('JSON.stringify(calendarPlanMark('+JSON.stringify(date)+','+JSON.stringify({plan:h?String(h):'В',plan_hours:h,construction_mode:mode})+','+JSON.stringify(e)+'))'));
 assert.equal(check('2026-11-02',normal).bg,'');
 assert.equal(check('2026-11-03',short).bg,'#fff0c9');
 assert.equal(check('2026-11-02',normal+0.1).bg,'#fce2d5');
 assert.equal(check('2026-11-03',normal).bg,'#fce2d5');
 assert.equal(check('2026-11-04',0).bg,'');
 assert.equal(check('2026-11-04',normal).bg,'#fce2d5');
 assert.equal(check('2026-11-03',11,'SHIFT'),null);
}
run("state.data.employees=[{id:1,tab_no:'T-01',full_name:'Тест',week_hours:36}];state.data.days=[];state.data.events=[];state.data.rvd=[];state.data.manual_colors=[];agTab='T-01';agYear=2026;agMonth=11;agView='year';constructorMethodKey='';constructorForm=null");
const actualF=JSON.parse(run("JSON.stringify(buildFiveDay(agEmployee(),'2026-01-01','2026-12-31'))"));
assert.equal(run("roundedBalance(buildFiveDay(agEmployee(),'2026-01-01','2026-12-31').monthly.reduce((s,m)=>s+m.plan,0))"),1774.4);
assert.equal(Object.keys(actualF.days).length,365);
assert.match(run("fiveDayPreviewHtml(buildFiveDay(agEmployee(),'2026-01-01','2026-12-31'))"),/Итого.*1774,4 ч/);
run("state.data.days=Object.values(buildFiveDay(agEmployee(),'2026-01-01','2026-12-31').days).map(d=>({...d,employee_id:1,plan_hours:d.hours,construction_mode:'FIVE_DAY'}))");
assert.equal(run("visualFor('2026-11-03',dayByDate('2026-11-03')).bg"),'#fff0c9');
assert.match(run("graphDayTooltip('2026-11-03',dayByDate('2026-11-03'),visualFor('2026-11-03',dayByDate('2026-11-03')))"),/Норма: 6,2 ч/);
assert.match(run('autografGraph()'),/class="n ">0<\/div><div class="small">норма соблюдена/);
assert.match(run('graphLegendHtml()'),/Сокращённый день/);
assert.match(run('revisionActiveYearView(agEmployee(),null,validateGraph(agEmployee(),2026))'),/title="[^"]*Сокращённый предпраздничный день/);
run("state.data.manual_colors=[{employee_id:1,date:'2026-11-03',color:'green'}]");
assert.equal(run("visualFor('2026-11-03',dayByDate('2026-11-03')).bg"),'#dff3df');
run("constructorMethod='FIVE_DAY';constructorMethodKey='1|2026'");
assert.ok(!run('autografConstructor()').includes('Продолжить график на следующий год'));
// Two cycles with missing saved phases are safe only if both halves reference their own persisted cycle.
run("state.data.flight_cycles=[{id:70,collection_point:'Сургут'},{id:160,collection_point:'Сургут'}];state.data.schedule_types=[{id:71,name:'Сург-Тал-Сург 7',collection_point:'Сургут',cycle_id:70},{id:72,name:'Сург-Тал-Сург 7.',collection_point:'Сургут',cycle_id:70},{id:161,name:'Сург-Тал-Сург 16',collection_point:'Сургут',cycle_id:160},{id:162,name:'Сург-Тал-Сург 16.',collection_point:'Сургут',cycle_id:160}];state.data.flight_dates=[{date:'2026-01-07',collection_point:'Сургут',cycle_id:70,sequence:1},{date:'2026-01-29',collection_point:'Сургут',cycle_id:70,sequence:2},{date:'2026-01-16',collection_point:'Сургут',cycle_id:160,sequence:1},{date:'2026-02-13',collection_point:'Сургут',cycle_id:160,sequence:2}]");
for(const [name,start,next] of [['Сург-Тал-Сург 7','2026-01-10','2026-01-29'],['Сург-Тал-Сург 16','2026-01-20','2026-02-13']]){
 for(const suffix of ['','.']){
  const e={id:1,collection_point:'Сургут',schedule_type:name+suffix,week_hours:40,shift_hours:11};
  const p=JSON.parse(run('JSON.stringify(buildByFlights('+JSON.stringify(e)+','+JSON.stringify(start)+',"2026-02-20"))'));
  assert.equal(p.days[start].plan,suffix?'В':'11');
  assert.equal(p.days[next].plan,'ДП');
  assert.equal(p.days[next].state,suffix?'WORK':'FREE');
 }
}
assert.equal(run('state.data.schedule_types[0].phase'),undefined,'inferred phase must not change persisted data');
run("state.data.schedule_types[1].cycle_id=160");
assert.equal(run('resolvedSchedulePhase(state.data.schedule_types[0])'),'');
run("state.data.schedule_types[1].cycle_id=70;state.data.schedule_types[0].phase='B'");
assert.equal(run('resolvedSchedulePhase(state.data.schedule_types[0])'),'');
run("state.data.schedule_types[0].phase='';state.data.schedule_types[0].cycle_id=0;state.data.schedule_types[1].cycle_id=0");
assert.equal(run('resolvedSchedulePhase(state.data.schedule_types[0])'),'');
console.log('Journal #47: neutral zero, calendar color and linked dotted pairs: PASS');


// #47 ALT-07R: selection shown by the UI must match actual constructor state.
run("state.data.schedule_types[0].cycle_id=70;state.data.schedule_types[1].cycle_id=70;state.data.schedule_types[0].phase='';state.data.employees=[{id:1,tab_no:'T-01',full_name:'Тест',collection_point:'Сургут',schedule_type:'',week_hours:40,shift_hours:11}];state.data.days=[];agTab='T-01';constructorSchedule='';constructorMethodKey='';constructorForm=null;constructorPreview=null");
assert.match(run('autografConstructor()'), /<option value="" disabled selected>Выберите вид графика/);
assert.match(run('autografConstructor()'), /onclick="runConstructor\(\)" disabled>Построить по полётам/);
assert.equal(run('constructorEmp().schedule_type'),'');
run("constructorSchedule='Сург-Тал-Сург 16'");
assert.match(run('autografConstructor()'), /value="Сург-Тал-Сург 16" selected/);
assert.match(run('autografConstructor()'), /Первая последовательность цикла — рабочая вахта/);
assert.ok(!run('autografConstructor()').includes('onclick="runConstructor()" disabled'));
assert.equal(run('agEmployee().schedule_type'),'','preview selection must not write card');
run("agEmployee().norm_pending=true");
assert.match(run('autografConstructor()'), /value="Не назначена" disabled/);
assert.match(run('autografConstructor()'), /onclick="runConstructor\(\)" disabled/);
run("agEmployee().norm_pending=false");

for(const [name,first,second] of [['Сург-Тал-Сург 7','2026-01-07','2026-01-29'],['Сург-Тал-Сург 16','2026-01-16','2026-02-13']]){
 for(const suffix of ['','.']){
  for(const hours of [10,11]){
   const e={id:1,collection_point:'Сургут',schedule_type:name+suffix,week_hours:40,shift_hours:hours,hire_date:'2025-12-25'};
   const build=(start,end,overrides={})=>JSON.parse(run('JSON.stringify(buildByFlights('+JSON.stringify({...e,...overrides})+','+JSON.stringify(start)+','+JSON.stringify(end)+'))'));
   const p=build('2026-01-01','2026-02-20');
   assert.equal(p.actual_start,'2026-01-01');
   assert.equal(p.days['2026-01-01'].plan,suffix?String(hours):'В');
   assert.equal(p.days[first].plan,'ДП');
   assert.equal(p.days[first].hours,0);
   assert.equal(p.days[second].plan,'ДП');
   assert.equal(p.days[second].state,suffix?'WORK':'FREE');
   assert.deepEqual(Object.values(p.days).filter(d=>d.plan==='ДП').map(d=>d.date),[first,second]);
   assert.equal(p.phases[0].plan_hours,(Number(first.slice(-2))-1)*(suffix?hours:0));
   assert.equal(p.monthly.reduce((s,m)=>s+m.plan,0),Object.values(p.days).reduce((s,d)=>s+d.hours,0));
   const hired=build('2026-01-01','2026-02-20',{hire_date:'2026-01-04'});
   assert.equal(hired.actual_start,'2026-01-04');
   assert.equal(hired.days['2026-01-03'],undefined);
   const onlyBefore=build('2026-01-02','2026-01-04');
   assert.equal(Object.keys(onlyBefore.days).length,3);
   assert.ok(Object.values(onlyBefore.days).every(d=>d.plan!=='ДП'));
   const hiredOn=build('2026-01-01','2026-02-20',{hire_date:first});
   assert.equal(hiredOn.actual_start,first);
   const hiredAfter=build('2026-01-01','2026-02-20',{hire_date:'2026-01-20'});
   assert.equal(hiredAfter.actual_start,'2026-01-20');
   assert.equal(hiredAfter.days['2026-01-20'].plan,suffix?'В':String(hours));
   const fired=build('2026-01-01','2026-02-20',{fire_date:'2026-01-04'});
   assert.equal(fired.actual_end,'2026-01-04');
  }
 }
}
run("state.data.flight_dates.push({date:'2025-12-19',collection_point:'Сургут',cycle_id:160,sequence:2})");
const prior=JSON.parse(run("JSON.stringify(buildByFlights({id:1,collection_point:'Сургут',schedule_type:'Сург-Тал-Сург 16.',week_hours:40,shift_hours:11},'2026-01-01','2026-01-20'))"));
assert.equal(prior.days['2026-01-01'].plan,'11');
assert.match(prior.phases[0].events,/19.12.2025/);
assert.ok(!prior.warnings.some(x=>x.includes('противоположная фаза')));
console.log('Journal #47: explicit selection and pre-anchor phase: PASS');
