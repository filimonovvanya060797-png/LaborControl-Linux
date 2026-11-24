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
  if (!document.querySelector('button[onclick="completeSchedulePair(919)"]'))
    throw Error('Existing first schedule should have one-click Create partner');
  if (document.getElementById('stadvanced')?.open)
    throw Error('Advanced schedule editor should start collapsed');
  const quickName = document.getElementById('qname');
  const quickPhase = document.getElementById('qphase');
  if (!quickName || !quickPhase || !document.getElementById('qpoint'))
    throw Error('Simple pair creation form is missing');
  quickName.value = 'Тюм-Тал-Тюм 9';
  quickPhase.value = 'A';
  updateQuickPairPreview();
  const previewText = document.getElementById('qpreview')?.textContent||'';
  if (!previewText.includes('Тюм-Тал-Тюм 9.') || !previewText.includes('свободная'))
    throw Error('Simple creation must preview dotted partner and opposite work phase');
  // The same simple flow must also accept a partner with a completely different number.
  document.getElementById('qpoint').value='Сургут';
  quickName.value='Сург-Тал-Сург 7';
  const mode=document.getElementById('qpairmode');
  if (!mode) throw Error('Partner choice is missing from simplified create form');
  mode.value='custom';schedulePairModeChanged();
  const partnerInput=document.getElementById('qpartner');
  if (!partnerInput || document.getElementById('qpartnerbox')?.style.display==='none')
    throw Error('Custom partner field was not revealed');
  partnerInput.value='Сург-Тал-Сург 16';updateQuickPairPreview();
  const customPreview=document.getElementById('qpreview')?.textContent||'';
  if (!customPreview.includes('Сург-Тал-Сург 7') || !customPreview.includes('Сург-Тал-Сург 16') ||
      !customPreview.includes('свободная'))
    throw Error('Custom 7/16 partner and opposite phase must be previewed');
  state.data.schedule_types.push({id:920,collection_point:'Сургут',name:'Сург-Тал-Сург 7',pair_name:'Сург-Тал-Сург 16',phase:'A',active:true,note:''});
  render();
  if (!document.querySelector('button[onclick="completeSchedulePair(920)"]') ||
      !document.querySelector('button[onclick="openSchedulePairPicker(920)"]'))
    throw Error('Incomplete custom numbered pair should offer both easy actions');
  openSchedulePairPicker(920);
  if (document.getElementById('qchosenpartner')?.value!=='Сург-Тал-Сург 16')
    throw Error('Choose-partner panel should retain stored custom partner name');
  closeSchedulePairPicker();
  state.data.schedule_types.push({id:921,collection_point:'Сургут',name:'Сург-Тал-Сург 16',pair_name:'Сург-Тал-Сург 7',phase:'B',active:true,note:''});
  render();
  if (schedulePairState(state.data.schedule_types.find(x=>x.id===920))!=='ready' ||
      schedulePairState(state.data.schedule_types.find(x=>x.id===921))!=='ready')
    throw Error('Ready status must use actual 7/16 names, not a dotted-name assumption');
  state.data.schedule_types=state.data.schedule_types.filter(x=>x.id!==920&&x.id!==921);
  render();

  editScheduleTypeId(919);
  if (!document.getElementById('stadvanced')?.open)
    throw Error('Edit existing record must expand advanced editor');
  if (document.getElementById('stsubmit')?.textContent !== 'Сохранить изменения')
    throw Error('Editing a schedule type must show save-edit state');
  clearScheduleType();
  if (document.getElementById('stadvanced')?.open)
    throw Error('Cancel manual edit should collapse advanced editor');
  if (document.getElementById('stsubmit')?.textContent !== 'Добавить вид графика' ||
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
  // Employee's legal weekly norm is independent from a work-shift length.
  if (Math.abs(employeeNormHour({week_hours:36},{norm_hours:8})-7.2)>0.0001 ||
      Math.abs(employeeNormHour({week_hours:36},{norm_hours:7})-6.2)>0.0001 ||
      employeeNormHour({week_hours:40},{norm_hours:7})!==7 ||
      employeeNormHour({week_hours:36},{norm_hours:0})!==0)
    throw Error('Individual 36-hour norm or shortened working day failed');
  const lateHire = {...syntheticSurgut,hire_date:'2026-01-10',fire_date:'',probation:false,week_hours:36,shift_hours:10};
  const pLate = buildByFlights(lateHire,'2026-01-01','2026-01-28',10);
  if (Object.keys(pLate.days).some(d=>d<'2026-01-10') ||
      pLate.days['2026-01-10']?.plan!=='10' ||
      pLate.days['2026-01-10']?.hours!==10 ||
      pLate.days['2026-01-07'] ||
      Object.keys(pLate.days).length!==19 ||
      pLate.shift_hours!==10 || pLate.week_hours!==36)
    throw Error('Constructor recorded dates before hire or ignored 10-hour shift');
  const pExit = buildByFlights({...lateHire,fire_date:'2026-01-18'},'2026-01-01','2026-01-28',10);
  if (Object.keys(pExit.days).some(d=>d>'2026-01-18') ||
      Object.keys(pExit.days).length!==9)
    throw Error('Constructor recorded dates after termination');
  const maleTen = buildByFlights({...syntheticSurgut,week_hours:40,shift_hours:10,hire_date:''},'2026-01-07','2026-01-28',10);
  if (maleTen.days['2026-01-08']?.hours!==10 || maleTen.days['2026-01-07']?.plan!=='ДП')
    throw Error('10-hour work shift did not preserve flight day');
  outcome.push({label:'Autograf: 36/40 weekly norms, 10/11 shifts, hire and termination bounds',ok:true,error:''});

  if (p7.days['2026-01-07']?.plan!=='ДП' || p7.days['2026-01-29']?.plan!=='ДП' ||
      p7.days['2026-01-16']?.plan==='ДП' || p7.days['2026-02-13']?.plan==='ДП')
    throw Error('Cycle 7 constructor mixed another cycle’s dates');
  if (p16.days['2026-01-16']?.plan!=='ДП' || p16.days['2026-02-13']?.plan!=='ДП' ||
      p16.days['2026-01-07']?.plan==='ДП' || p16.days['2026-01-29']?.plan==='ДП')
    throw Error('Cycle 16 constructor mixed another cycle’s dates');
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
