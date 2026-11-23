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
