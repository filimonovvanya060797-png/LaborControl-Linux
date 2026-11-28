#!/usr/bin/python3
"""Exercise the actual Settings download button in a real WebKitGTK view.
The test substitutes a temporary destination for the interactive GTK chooser.
"""
import base64
import hashlib
import json
import pathlib
import sys
import tempfile
import urllib.request
import gi

gi.require_version("Gtk", "3.0")
gi.require_version("WebKit2", "4.1")
from gi.repository import GLib, Gtk, WebKit2

ORIGIN = "http://127.0.0.1:8779"
URL = ORIGIN + "/api/backup/portable"

class Smoke:
    def __init__(self, folder):
        self.target = pathlib.Path(folder) / "backup.lcbackup"
        self.done = False
        self.ok = False
        self.attempts = 0
        self.window = Gtk.Window()
        self.view = WebKit2.WebView()
        self.window.add(self.view)
        self.window.show_all()
        self.view.connect("load-changed", self.loaded)
        self.view.connect("decide-policy", self.policy)
        self.view.get_context().connect("download-started", self.started)
        GLib.timeout_add_seconds(35, self.timeout)
        self.view.load_uri(ORIGIN + "/r11")

    def policy(self, view, decision, kind):
        if kind == WebKit2.PolicyDecisionType.NAVIGATION_ACTION:
            uri = decision.get_navigation_action().get_request().get_uri()
            if not uri.startswith(ORIGIN + "/") and uri != "about:blank":
                decision.ignore()
                return True
        return False

    def loaded(self, view, event):
        if event == WebKit2.LoadEvent.FINISHED:
            GLib.timeout_add(300, self.click)

    def click(self):
        if self.done:
            return False
        self.attempts += 1
        js = """(function(){if(!state?.data?.employees?.length)return 'wait';
        go('settings');let b=document.querySelector('button[onclick="portableBackupSave()"]');
        if(!b)return 'missing';b.click();return 'clicked';})()"""
        self.view.evaluate_javascript(js, -1, None, None, None, self.clicked, None)
        return False

    def clicked(self, view, result, data):
        try:
            value = view.evaluate_javascript_finish(result).to_string()
            if value == "wait" and self.attempts < 20:
                GLib.timeout_add(300, self.click)
            elif value != "clicked":
                self.finish(False, "button: " + value)
        except Exception as exc:
            self.finish(False, str(exc))

    def started(self, context, download):
        if download.get_request().get_uri() != URL:
            download.cancel()
            self.finish(False, "unexpected download URI")
            return
        download.connect("decide-destination", self.destination)
        download.connect("failed", self.failed)
        download.connect("finished", self.finished)

    def destination(self, download, suggested):
        try:
            response = download.get_response()
            if response.get_status_code() != 200 or not suggested.endswith(".lcbackup"):
                raise RuntimeError("unexpected HTTP response or filename")
            download.set_destination(self.target.as_uri())
        except Exception as exc:
            self.finish(False, str(exc))
            download.cancel()
        return True

    def failed(self, download, error):
        self.finish(False, "download failed: " + str(error))

    def finished(self, download):
        if self.done:
            return
        try:
            payload = self.target.read_bytes()
            backup = json.loads(payload)
            assert backup["format"] == "LaborControl-R11-portable/1"
            assert [e["name"] for e in backup["files"]] == ["labor_control.json", "labor_control_tab2.json"]
            for entry in backup["files"]:
                if entry["present"]:
                    data = base64.b64decode(entry["data"], validate=True)
                    assert hashlib.sha256(data).hexdigest() == entry["sha256"]
            assert backup["files"][0]["present"]
            req = urllib.request.Request(ORIGIN + "/api/backup/inspect", data=payload,
                                         headers={"Content-Type": "application/octet-stream"}, method="POST")
            with urllib.request.urlopen(req, timeout=5) as resp:
                assert json.load(resp)["ok"] is True
            self.finish(True, "WebKit Settings button: downloaded and verified actual backup bytes")
        except Exception as exc:
            self.finish(False, str(exc))

    def timeout(self):
        self.finish(False, "no completed backup download before timeout")
        return False

    def finish(self, ok, message):
        if self.done:
            return
        self.done = True
        self.ok = ok
        print(("PASS " if ok else "FAIL ") + message, flush=True)
        GLib.idle_add(Gtk.main_quit)

if __name__ == "__main__":
    with tempfile.TemporaryDirectory() as folder:
        smoke = Smoke(folder)
        Gtk.main()
        sys.exit(0 if smoke.ok else 1)
