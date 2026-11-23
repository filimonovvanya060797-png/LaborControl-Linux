package main

import (
    "os"
    "strings"
    "testing"
)

func TestR11RussianFilePickersKeepOriginalInputContracts(t *testing.T) {
    cases := []struct {
        name string
        html string
        ids []string
    }{
        {"R11", string(r11OriginalHTML), []string{"empxlsx", "pointxlsx", "flxlsx", "effile", "eventfile", "vacxlsx", "calfile6"}},
        {"Tab2", string(frontendHTML), []string{"empxlsx", "pointxlsx", "flxlsx", "effile", "eventfile", "vacxlsx", "calfile6", "factfile", "t2fact", "t2szrvd"}},
        {"state", string(r11StateHTML), []string{"stateFile"}},
    }
    for _,tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            if !strings.Contains(tc.html, "id=\"r11-ru-file-picker-script\"") {
                t.Fatal("Russian file picker controller missing")
            }
            if !strings.Contains(tc.html, "new MutationObserver(") {
                t.Fatal("file picker must support dynamically recreated screens")
            }
            for _,id := range tc.ids {
                if !strings.Contains(tc.html, "id=\""+id+"\" type=\"file\"") {
                    t.Errorf("original file input ID %q missing", id)
                }
            }
            for _,required := range []string{"Выбрать файл", "Файл не выбран", "Выбран файл: ", "aria-live", "textContent", "type = 'button'"} {
                if !strings.Contains(tc.html, required) {
                    t.Errorf("file picker missing %q", required)
                }
            }
        })
    }
    if !strings.Contains(r11WindowsImportPanel, "id=\"r11-windows-file\" type=\"file\" accept=\".json,application/json\"") {
        t.Fatal("Windows migration import input contract changed")
    }
    if !strings.Contains(string(r11StateHTML), "$(\"stateFile\").dispatchEvent(new Event(\"change\"));") {
        t.Fatal("JSON import must refresh the visible filename after clearing its input")
    }
}

func TestR11GTKNativeDialogsAreRussian(t *testing.T) {
    s,err := os.ReadFile("../desktop/main.c")
    if err != nil { t.Fatal(err) }
    for _, want := range []string{
        "\"script-dialog\", G_CALLBACK(on_script_dialog)",
        "WEBKIT_SCRIPT_DIALOG_CONFIRM",
        "WEBKIT_SCRIPT_DIALOG_ALERT",
        "WEBKIT_SCRIPT_DIALOG_PROMPT",
        "webkit_script_dialog_confirm_set_confirmed",
        "webkit_script_dialog_prompt_set_text",
        "\"_Отмена\"",
        "\"_Подтвердить\"",
        "\"_Закрыть\"",
    } {
        if !strings.Contains(string(s),want) {
            t.Errorf("GTK Russian dialog handler missing %q",want)
        }
    }
}
