#!/usr/bin/python3
"""Synthetic live WebKitGTK navigation smoke, run under Xvfb on GitHub Actions.

No production database, no screen capture and no network beyond localhost.
"""
import json
import sys

import gi

gi.require_version("Gtk", "3.0")
gi.require_version("WebKit2", "4.1")
from gi.repository import GLib, Gtk, WebKit2

URL = "http://127.0.0.1:8779/r11"

SCRIPT = r"""
(function () {
  const outcome = [];
  function check(label) {
    const page = document.getElementById('page');
    const text = page ? page.textContent || '' : '';
    outcome.push({
      label,
      ok: Boolean(page && text.trim() && !text.includes('Ошибка интерфейса')),
      error: text.includes('Ошибка интерфейса') ? text.slice(0, 900) : ''
    });
  }
  if (!state || !state.data || !state.data.employees || !state.data.employees.length) {
    return JSON.stringify({ready:false, outcome:[], error:'state not loaded'});
  }
  for (const name of ['dashboard','employees','rvd','check','events','journal','help','settings']) {
    go(name);check(name);
  }
  go('autograf');
  for (const section of ['graph','vacations','constructor','summary','calendar','types','flights','versions']) {
    setAgSection(section);check('autograf / '+section);
  }
  // Unified date input regression: actual GTK WebKit DOM and native ISO values.
  if (!window.r11DateFields) throw Error('Unified date controls were not loaded');
  if (r11DateFields.parseDate('31.02.2026') !== null ||
      r11DateFields.parseDate('29.02.2028') !== '2028-02-29')
    throw Error('Strict date validation failed');
  go('employees');
  if (!document.getElementById('eweekly') || !document.getElementById('eshift') ||
      !document.getElementById('enormbasis'))
    throw Error('Employee weekly norm, shift or legal basis field missing');
  if (employeeWeekHours({})!==40 || employeeShiftHours({})!==11 ||
      employeeWeekHours({weekly_hours:36})!==36 || employeeShiftHours({shift_hours:10})!==10)
    throw Error('Old employee fallback and new employee hours are incorrect');
  if (employeeCalendarNorm({weekly_hours:36},{norm_hours:8})!==7.2 ||
      employeeCalendarNorm({weekly_hours:36},{norm_hours:7})!==6.2 ||
      employeeCalendarNorm({weekly_hours:36},{norm_hours:0})!==0)
    throw Error('36-hour working week must respect shortened working days');
  // Optional normative hours in XLSX/CSV must be parsed and rejected before import.
  const savedImport = empImport, savedImportMap = empImportMap;
  const hHours=['Табельный номер','ФИО','Норма недели','Часов смены','Основание нормы'];
  empImport={name:'test.csv',head:hHours,rows:[['SMOKE-WEEK-36','Учебная','36','10','ст. 320 ТК РФ']]};
  empImportMap=autoEmpMap(hHours);
  let workImport=importClassify()[0];
  if (workImport.status!=='new' || workImport.o.weekly_hours!==36 ||
      workImport.o.shift_hours!==10 || workImport.o.norm_basis!=='ст. 320 ТК РФ')
    throw Error('36/10 work regimes were not parsed from import');
  empImport.rows[0][2]='37';
  if (importClassify()[0].status!=='error')
    throw Error('Invalid normative week must block mass import before saving');
  empImport=savedImport;empImportMap=savedImportMap;
  outcome.push({label:'Autograf: 36/40 normative week and 10/11 shift in card and Excel import',ok:true,error:''});
  // A probation-end date is intentionally disabled until probation is enabled.
  document.getElementById('eprob').checked = true;
  refreshMainReadyState();
  r11DateFields.enhanceInside(document.getElementById('page'));
  const prob = document.getElementById('eprobend');
  const probText = document.querySelector('[data-r11-for="eprobend"]');
  if (!prob || !probText || probText.type !== 'text')
    throw Error('Probation date has no manual input');
  probText.value = '30112026';
  probText.dispatchEvent(new Event('input', {bubbles:true}));
  if (probText.value !== '30.11.2026' || prob.value !== '2026-11-30')
    throw Error('Probation keyboard date to ISO failed');
  go('autograf');
  setAgSection('flights');
  r11DateFields.enhanceInside(document.getElementById('page'));
  // Regression for an empty flight_routes catalog: previously every selector was empty.
  const flightPointInput = document.getElementById('flpoint');
  const batchPointInput = document.getElementById('flbatchpoint');
  const cyclePointInput = document.getElementById('fcpoint');
  const filterPointInput = document.querySelector('select[onchange*="flightPoint=this.value"]');
  if (!flightPointInput || !batchPointInput || !cyclePointInput || !filterPointInput)
    throw Error('Flight collection-point selectors are missing');
  for (const [name,field] of [['single flight',flightPointInput],['batch flight',batchPointInput],
                               ['cycle',cyclePointInput],['filter',filterPointInput]]) {
    if (![...field.options].some(o=>o.value==='Тюмень'))
      throw Error(name+' cannot select the employee city without routes');
  }
  if (cyclePointInput.value !== 'Тюмень')
    throw Error('Cycle must select existing employee point instead of an arbitrary fallback');
  if (state.data.flight_routes.length !== 0)
    throw Error('Test requires missing flight route catalog');
  const flight = document.getElementById('fldate');
  const flightText = document.querySelector('[data-r11-for="fldate"]');
  const fcFirstText = document.querySelector('[data-r11-for="fcfirst"]');
  const fcSecondText = document.querySelector('[data-r11-for="fcsecond"]');
  if (!flight || !flightText || !fcFirstText || !fcSecondText)
    throw Error('Flight date or both cycle anchors have no manual controls');
  const dateWidget = flight.closest('.r11-date-widget');
  if (dateWidget.querySelectorAll('.r11-date-btn').length !== 2)
    throw Error('Both calendar and clear buttons must be visible');
  flightText.value = '18112026';
  flightText.dispatchEvent(new Event('input', {bubbles:true}));
  if (flightText.value !== '18.11.2026' || flight.value !== '2026-11-18')
    throw Error('Flight keyboard date ISO failed');
  flight.value = '2026-11-20';
  if (flightText.value !== '20.11.2026')
    throw Error('Editing existing date did not sync');
  flightText.value = '31022026';
  flightText.dispatchEvent(new Event('input', {bubbles:true}));
  if (flight.value || flightText.getAttribute('aria-invalid') !== 'true')
    throw Error('Impossible date was accepted');
  const fakeSave = document.createElement('button');
  fakeSave.className = 'primary';
  let invalidSubmitAllowed = false;
  fakeSave.addEventListener('click', function () { invalidSubmitAllowed = true; });
  dateWidget.closest('.panel').appendChild(fakeSave);
  fakeSave.click();
  fakeSave.remove();
  if (invalidSubmitAllowed)
    throw Error('Invalid date must block save actions instead of silently saving empty ISO');
  dateWidget.querySelectorAll('.r11-date-btn')[1].click();
  if (flight.value || flightText.value)
    throw Error('Clear date did not clear text and native ISO');
  fcFirstText.value = '23112026';
  fcFirstText.dispatchEvent(new Event('input', {bubbles:true}));
  if (document.getElementById('fcfirst').value !== '2026-11-23')
    throw Error('Flight cycle anchor ISO synchronization failed');
  // No database writes: reversed user-provided anchors should fail; chronological ones preview a year.
  fcFirstText.value = '01092026';
  fcFirstText.dispatchEvent(new Event('input', {bubbles:true}));
  fcSecondText.value = '30012026';
  fcSecondText.dispatchEvent(new Event('input', {bubbles:true}));
  calcFlightCycle();
  if (flightCyclePreview !== null)
    throw Error('Reverse anchor dates must be rejected, even with valid point');
  fcSecondText.value = '30092026';
  fcSecondText.dispatchEvent(new Event('input', {bubbles:true}));
  calcFlightCycle();
  if (!flightCyclePreview || flightCyclePreview.point !== 'Тюмень' ||
      flightCyclePreview.first !== '2026-09-01' || flightCyclePreview.second !== '2026-09-30' ||
      flightCyclePreview.items.length < 3)
    throw Error('Valid cycle must preview dates for the selected point without saving');
  if (state.data.flight_dates.length)
    throw Error('Cycle preview must not write to database');
  if (!document.querySelector('#flightCyclePreviewBox button[onclick="saveFlightCycle(true)"]') ||
      !document.getElementById('fcname') || !document.getElementById('fcphase'))
    throw Error('Cycle preview must offer atomic Save cycle and pair with one editable type name');
  // Batch UX check: actual DOM after a preview must retain anchors and show cancellation.
  if (document.getElementById('fcfirst').value !== '2026-09-01' ||
      document.getElementById('fcsecond').value !== '2026-09-30')
    throw Error('Flight cycle preview lost original anchor dates after redraw');
  const cancelCycle = document.querySelector('#flightCyclePreviewBox button[onclick="cancelFlightCyclePreview()"]');
  if (!cancelCycle) throw Error('Flight cycle preview has no Cancel action');
  cancelCycle.click();
  if (flightCyclePreview || document.querySelector('#flightCyclePreviewBox button[onclick="saveFlightCycle()"]'))
    throw Error('Cancelled flight cycle preview still offers Save');
  if (document.getElementById('fcfirst').value !== '2026-09-01')
    throw Error('Cancelling a preview should preserve editable anchor dates');
  calcFlightCycle();
  r11DateFields.enhanceInside(document.getElementById('page'));
  const updatedSecond = document.querySelector('[data-r11-for="fcsecond"]');
  if (!updatedSecond) throw Error('Updated cycle form must keep typed input');
  updatedSecond.value = '02102026';
  updatedSecond.dispatchEvent(new Event('input',{bubbles:true}));
  if (flightCyclePreview !== null || document.querySelector('#flightCyclePreviewBox button[onclick="saveFlightCycle()"]'))
    throw Error('Edited input must invalidate old cycle preview automatically');
  if (state.data.flight_dates.length) throw Error('Preview or cancel wrote flight data');
  const flightEdit = {id:91,collection_point:'Тюмень',date:'2026-10-02',source:'Ручной ввод',note:'Учебный'};
  editFlight(flightEdit);
  if (document.getElementById('flsubmit')?.textContent !== 'Сохранить изменения')
    throw Error('Editing a flight must show save-edit state');
  clearFlightForm();
  if (document.getElementById('flsubmit')?.textContent !== 'Добавить дату' ||
      document.getElementById('flid')?.value)
    throw Error('Cancel flight edit must return to new-flight mode');
  outcome.push({label:'Autograf: preview Cancel, preserved dates, stale invalidation, edit/cancel',ok:true,error:''});
  outcome.push({label:'flight point selection with empty routes and cycle preview only',ok:true,error:''});
  // The constructor and vacations also expose explicit cancellation before persistence.
  const vacationTemplate = vacationPanel(agEmployee());
  vacationPreview = {start:'2026-10-01',end:'2026-10-14',days:14,holidays:0,reduction:80,absence_days:14};
  if (!vacationPanel(agEmployee()).includes('onclick="cancelVacationPreview()"'))
    throw Error('Vacation preview is missing Cancel');
  vacationPreview = null;
  setAgSection('constructor');
  constructorPreview = {warnings:[],phases:[],monthly:[],type:'Тест',days:{}};
  render();
  const cancelConstructor = document.querySelector('#constructorPreviewBox button[onclick="cancelConstructorPreview()"]');
  if (!cancelConstructor) throw Error('Constructor preview has no Cancel');
  cancelConstructor.click();
  if (constructorPreview || document.querySelector('#constructorPreviewBox button[onclick="applyConstructorPreview()"]'))
    throw Error('Cancel constructor must remove its save action');
  setAgSection('types');
  state.data.schedule_types.push({id:919,collection_point:'Тюмень',name:'Учебный',pair_name:'Учебный.',phase:'A',active:true,note:''});
  render();
  // One-click creation is deliberately removed from this tab: use the cycle card.
  if (document.getElementById('qname') || document.getElementById('qpoint') ||
      document.getElementById('qpairmode'))
    throw Error('Legacy duplicate create-pair form must be absent (#35)');
  if (document.getElementById('stadvanced')?.open ||
      document.getElementById('stsubmit')?.disabled!==true)
    throw Error('Existing type editor must start collapsed and creation disabled');
  if (!document.querySelector('button[onclick="editScheduleTypeId(919)"]'))
    throw Error('Existing schedule must still be editable');
  editScheduleTypeId(919);
  if (!document.getElementById('stadvanced')?.open)
    throw Error('Edit existing record must expand advanced editor');
  if (document.getElementById('stsubmit')?.textContent !== 'Сохранить изменения')
    throw Error('Editing a schedule type must show save-edit state');
  clearScheduleType();
  if (document.getElementById('stadvanced')?.open)
    throw Error('Cancel manual edit should collapse advanced editor');
  if (document.getElementById('stsubmit')?.textContent !== 'Выберите существующий вид' ||
      document.getElementById('stid')?.value)
    throw Error('Cancel schedule edit must reset editing state');
  state.data.schedule_types = state.data.schedule_types.filter(x=>x.id!==919);
  // Two separate independent Surgut cycles: DO NOT group by the city alone.
  const snapFlights = state.data.flight_dates.slice();
  const snapCycles = (state.data.flight_cycles||[]).slice();
  const snapTypes = state.data.schedule_types.slice();
  const snapPoint = flightPoint, snapYear = flightYear;
  state.data.flight_cycles = [
    {id:770,collection_point:'Сургут',first_anchor:'2026-01-07',second_anchor:'2026-01-29',year:2026},
    {id:880,collection_point:'Сургут',first_anchor:'2026-01-16',second_anchor:'2026-02-13',year:2026}
  ];
  state.data.flight_dates = [
    {id:771,collection_point:'Сургут',date:'2026-01-07',cycle_id:770,sequence:1,note:'Первая последовательность рейса. Цикл от 07.01.2026 / 29.01.2026'},
    {id:772,collection_point:'Сургут',date:'2026-01-29',cycle_id:770,sequence:2,note:'Вторая последовательность рейса. Цикл от 07.01.2026 / 29.01.2026'},
    {id:881,collection_point:'Сургут',date:'2026-01-16',cycle_id:880,sequence:1,note:'Первая последовательность рейса. Цикл от 16.01.2026 / 13.02.2026'},
    {id:882,collection_point:'Сургут',date:'2026-02-13',cycle_id:880,sequence:2,note:'Вторая последовательность рейса. Цикл от 16.01.2026 / 13.02.2026'}
  ];
  state.data.schedule_types = snapTypes.concat([
    {id:773,collection_point:'Сургут',name:'Сург-Тал-Сург 7',pair_name:'Сург-Тал-Сург 7.',cycle_id:770,phase:'A',active:true},
    {id:774,collection_point:'Сургут',name:'Сург-Тал-Сург 7.',pair_name:'Сург-Тал-Сург 7',cycle_id:770,phase:'B',active:true},
    {id:883,collection_point:'Сургут',name:'Сург-Тал-Сург 16',pair_name:'Сург-Тал-Сург 16.',cycle_id:880,phase:'A',active:true},
    {id:884,collection_point:'Сургут',name:'Сург-Тал-Сург 16.',pair_name:'Сург-Тал-Сург 16',cycle_id:880,phase:'B',active:true}
  ]);
  const empBase = agEmployee();
  const syntheticSurgut = {...empBase, collection_point:'Сургут', schedule_type:'Сург-Тал-Сург 7'};
  const p7 = buildByFlights(syntheticSurgut,'2026-01-07','2026-02-28');
  const p16 = buildByFlights({...syntheticSurgut,schedule_type:'Сург-Тал-Сург 16'},'2026-01-16','2026-02-28');
  if (p7.days['2026-01-07']?.plan!=='ДП' || p7.days['2026-01-29']?.plan!=='ДП' ||
      p7.days['2026-01-16']?.plan==='ДП' || p7.days['2026-02-13']?.plan==='ДП')
    throw Error('Cycle 7 constructor mixed another cycle’s dates');
  if (p16.days['2026-01-16']?.plan!=='ДП' || p16.days['2026-02-13']?.plan!=='ДП' ||
      p16.days['2026-01-07']?.plan==='ДП' || p16.days['2026-01-29']?.plan==='ДП')
    throw Error('Cycle 16 constructor mixed another cycle’s dates');
  // Employment boundaries must clamp the PREVIEW, preserving flight phases.
  const woman={...syntheticSurgut,hire_date:'2026-01-10',fire_date:'',probation:false,
               weekly_hours:36,shift_hours:10};
  const p10=buildByFlights(woman,'2026-01-01','2026-01-28',10);
  if (Object.keys(p10.days).some(d=>d<'2026-01-10') ||
      p10.days['2026-01-10']?.hours!==10 || p10.days['2026-01-10']?.plan!=='10' ||
      p10.days['2026-01-07'] || p10.phases[0]?.start!=='2026-01-10')
    throw Error('Constructor must not record days before employment and must preserve 10h phase');
  const p11=buildByFlights({...woman,hire_date:'2026-01-07',weekly_hours:40,shift_hours:11},
                           '2026-01-07','2026-01-28',11);
  if (p11.phases[0]?.plan_hours!==231 || p10.phases[0]?.plan_hours!==190)
    throw Error('10/11-hour shifts did not yield 190/231 planned hours');
  const pEnd=buildByFlights({...woman,fire_date:'2026-01-18'},
                           '2026-01-01','2026-01-28',10);
  if (Object.keys(pEnd.days).some(d=>d>'2026-01-18'))
    throw Error('Constructor wrote days after termination');
  if (Math.abs(employeeNormSum({weekly_hours:36},2026,'2026-11-01','2026-12-31')-301.4)>0.001 ||
      Math.abs(employeeNormSum({weekly_hours:36},2026,'2026-01-01','2026-12-31')-1774.4)>0.001 ||
      employeeNormSum({weekly_hours:40},2026,'2026-11-01','2026-12-31')!==335)
    throw Error('2026 individual calendar norms do not match 36/40-hour reference');
  state.data.schedule_types.push({id:885,collection_point:'Сургут',name:'Вид без цикла',phase:'A',active:true});
  let refusedUnbound = false;
  try { buildByFlights({...syntheticSurgut,schedule_type:'Вид без цикла'},'2026-01-07','2026-02-28'); }
  catch(e) { refusedUnbound = String(e.message).includes('несколько'); }
  if (!refusedUnbound) throw Error('Unbound type must not combine different Surgut cycles');
  state.data.schedule_types.pop();
  flightPoint='Сургут';flightYear=2026;setAgSection('flights');
  if (!flightCycleManagePanel().includes('исходные ДП 07.01.2026 / 29.01.2026') ||
      !flightCycleManagePanel().includes('исходные ДП 16.01.2026 / 13.02.2026'))
    throw Error('Both cycle cards must remain distinct in the UI');
  openCycleFlightEditor(772);
  if (document.getElementById('cycleeditdate')?.value!=='2026-01-29' ||
      !document.getElementById('cycleeditreason'))
    throw Error('Bound flight should have its own move-one-date editor');
  cancelCycleFlightEditor();
  state.data.schedule_types = snapTypes;
  state.data.flight_dates = snapFlights;
  state.data.flight_cycles = snapCycles;
  flightPoint=snapPoint;flightYear=snapYear;
  setAgSection('types');
  outcome.push({label:'Autograf: independent 7 and 16 cycles, isolated plans and one-flight editor',ok:true,error:''});
  outcome.push({label:'Autograf: vacation/constructor previews and schedule edit cancellation',ok:true,error:''});
  setAgSection('calendar');
  r11DateFields.enhanceInside(document.getElementById('page'));
  if (document.querySelectorAll('input[type="date"]:not(.emp-date-native)').length !==
      document.querySelectorAll('.r11-date-row input[type="date"]').length)
    throw Error('Date fields in another tab were not enhanced');
  outcome.push({label:'unified editable dates: probation, flights, anchors, invalid dates, clear, redraw',ok:true,error:''});
  setAgSection('graph');agSetView('year');check('autograf / yearly view');
  go('check');
  const frame=document.querySelector('#page iframe');
  const iframe=frame?.getAttribute('src')||'';
  return JSON.stringify({ready:true, outcome, iframe});
})()
"""


class Smoke:
    def __init__(self):
        self.done = False
        self.exit_code = 1
        self.attempts = 0
        self.window = Gtk.Window(title="LaborControl R11 CI WebKitGTK smoke")
        self.window.set_default_size(1200, 800)
        self.view = WebKit2.WebView()
        self.window.add(self.view)
        self.window.show_all()
        self.view.connect("load-failed", self.on_failure)
        self.view.connect("load-changed", self.on_load)
        GLib.timeout_add_seconds(45, self.on_timeout)
        self.view.load_uri(URL)

    def on_failure(self, view, event, failing_uri, error):
        print("WebKit load failed:", failing_uri, error, file=sys.stderr)
        self.finish(False)
        return False

    def on_load(self, view, event):
        if event == WebKit2.LoadEvent.FINISHED and not self.done:
            GLib.timeout_add(700, self.check)

    def check(self):
        if self.done:
            return False
        self.attempts += 1
        try:
            self.view.evaluate_javascript(SCRIPT, -1, None, None, None, self.on_js_result, None)
        except Exception as err:
            print("WebKit JS invocation failed:", err, file=sys.stderr)
            self.finish(False)
        return False

    def on_js_result(self, view, async_result, user_data):
        if self.done:
            return
        try:
            jsc_value = view.evaluate_javascript_finish(async_result)
            raw = jsc_value.to_string()
            data = json.loads(raw)
            if not data["ready"]:
                if self.attempts < 15:
                    GLib.timeout_add(300, self.check)
                    return
                raise RuntimeError("R11 state did not finish loading")
            failed = [x for x in data["outcome"] if not x["ok"]]
            for item in data["outcome"]:
                print(("FAIL" if not item["ok"] else "OK"), item["label"], item["error"], flush=True)
            if data.get("iframe") != "/tab2?embed=tab2":
                failed.append({"label": "same-origin Tab2", "error": str(data.get("iframe"))})
            if failed:
                print("WebKit UI failures:", failed, file=sys.stderr)
            self.finish(not failed)
        except Exception as err:
            print("WebKit JS check failed:", err, file=sys.stderr)
            self.finish(False)

    def on_timeout(self):
        if not self.done:
            print("WebKit test timed out", file=sys.stderr)
            self.finish(False)
        return False

    def finish(self, success):
        if self.done:
            return
        self.done = True
        self.exit_code = 0 if success else 1
        GLib.idle_add(Gtk.main_quit)


if __name__ == "__main__":
    try:
        smoke = Smoke()
        Gtk.main()
        if smoke.exit_code == 0:
            print("Real WebKitGTK screen navigation: PASS")
        raise SystemExit(smoke.exit_code)
    except Exception as exc:
        print("WebKit smoke setup failed:", exc, file=sys.stderr)
        raise SystemExit(1)
