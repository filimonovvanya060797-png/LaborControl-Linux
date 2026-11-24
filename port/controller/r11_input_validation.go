package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "math"
)

// These checks apply to incoming write payloads, not historical imported
// Windows R11 data. They reject invalid JSON types instead of storing a
// malformed value and breaking the UI later.
func r11StrictStringField(body map[string]json.RawMessage, key string) error {
    raw := r11Raw(body, key)
    if len(raw) == 0 { return nil }
    if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
        return fmt.Errorf("поле %s должно быть строкой, а не null", key)
    }
    var value string
    if err := json.Unmarshal(raw, &value); err != nil {
        return fmt.Errorf("поле %s должно быть строкой", key)
    }
    return nil
}

func r11StrictBooleanField(body map[string]json.RawMessage, key string) error {
    raw := r11Raw(body, key)
    if len(raw) == 0 { return nil }
    if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
        return fmt.Errorf("поле %s должно быть true или false", key)
    }
    var value bool
    if err := json.Unmarshal(raw, &value); err != nil {
        return fmt.Errorf("поле %s должно быть true или false", key)
    }
    return nil
}

func r11StrictHoursField(body map[string]json.RawMessage, key string, maximum float64) error {
    raw := r11Raw(body, key)
    if len(raw) == 0 { return nil }
    if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
        return fmt.Errorf("поле %s должно содержать число", key)
    }
    var value float64
    if err := json.Unmarshal(raw, &value); err != nil {
        return fmt.Errorf("поле %s должно содержать число", key)
    }
    if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > maximum {
        return fmt.Errorf("поле %s: допустимые часы от 0 до %g", key, maximum)
    }
    return nil
}

func r11StrictEmployeePayload(body map[string]json.RawMessage) error {
    for _, key := range []string{
        "tab_no", "full_name", "position", "unit", "collection_point",
        "schedule_type", "hire_date", "fire_date", "probation_end", "notes", "norm_reason",
    } {
        if err := r11StrictStringField(body, key); err != nil { return err }
    }
    for _, key := range []string{"is_quarter", "probation", "main_schedule_ready"} {
        if err := r11StrictBooleanField(body, key); err != nil { return err }
    }
    if raw:=r11Raw(body,"gender");len(raw)>0 {
        var v string
        if err:=json.Unmarshal(raw,&v);err!=nil || (v!="" && v!="M" && v!="F") {
            return fmt.Errorf("пол сотрудника должен быть М, Ж или не указан")
        }
    }
    if len(r11String(body, "norm_reason")) > 300 { return fmt.Errorf("основание нормы должно быть короче 301 символа") }
    if raw := r11Raw(body, "week_hours"); len(raw) > 0 {
        var v float64
        if err := json.Unmarshal(raw, &v); err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 1 || v > 40 || math.Abs(v*10-math.Round(v*10)) > 0.000001 {
            return fmt.Errorf("норма рабочей недели должна быть числом от 1 до 40 ч с точностью 0,1 ч")
        }
    }
    if raw := r11Raw(body, "shift_hours"); len(raw) > 0 {
        var v float64
        if err := json.Unmarshal(raw, &v); err != nil || (v != 10 && v != 11) {
            return fmt.Errorf("продолжительность вахтовой смены — 10 или 11 часов")
        }
    }
    for _, key := range []string{"hire_date", "fire_date", "probation_end"} {
        if value := r11String(body, key); value != "" && !r11DateOK(value) {
            return fmt.Errorf("поле %s должно быть корректной датой YYYY-MM-DD", key)
        }
    }
    hire, fire := r11String(body, "hire_date"), r11String(body, "fire_date")
    probationEnd := r11String(body, "probation_end")
    if hire != "" && fire != "" && fire < hire {
        return fmt.Errorf("дата увольнения не может быть раньше даты приёма")
    }
    if hire != "" && probationEnd != "" && probationEnd < hire {
        return fmt.Errorf("испытательный срок не может закончиться раньше даты приёма")
    }
    var probation bool
    if raw := r11Raw(body, "probation"); len(raw) != 0 {
        _ = json.Unmarshal(raw, &probation) // type was validated above
    }
    if probation && probationEnd == "" {
        return fmt.Errorf("укажите дату окончания испытательного срока")
    }
    if !probation && probationEnd != "" {
        return fmt.Errorf("дата окончания испытательного срока указана, но галочка не установлена")
    }
    return nil
}

func r11StrictDayPayload(body map[string]json.RawMessage) error {
    for _, key := range []string{
        "Plan", "Fact", "Shift", "State", "Comment", "ChangeReason",
    } {
        if err := r11StrictStringField(body, key); err != nil { return err }
    }
    for _, key := range []string{"PlanHours", "FactHours"} {
        if err := r11StrictHoursField(body, key, 24); err != nil { return err }
    }
    return nil
}

func r11StrictCalendarPayload(body map[string]json.RawMessage) error {
    for _, key := range []string{"day_type", "note", "source"} {
        if err := r11StrictStringField(body, key); err != nil { return err }
    }
    return r11StrictHoursField(body, "norm_hours", 24)
}

 
// An archived revision snapshot replaces records belonging to one driver.
// Reject duplicate IDs/dates and IDs already occupied by other drivers.
// Never partially restore a corrupted snapshot to the shared database.
func r11ValidateRevisionSnapshotRows(
    collection string,
    snapshot []map[string]json.RawMessage,
    current []map[string]json.RawMessage,
    employeeID int64,
) error {
    otherIDs := map[int64]bool{}
    for _, item := range current {
        if r11Int(item, "employee_id") != employeeID {
            if id := r11Int(item, "id"); id > 0 { otherIDs[id] = true }
        }
    }
    seenIDs := map[int64]bool{}
    seenDates := map[string]bool{}
    for _, item := range snapshot {
        id := r11Int(item, "id")
        if id < 1 || seenIDs[id] || otherIDs[id] {
            return fmt.Errorf("снимок %s содержит пустой, повторный или занятый ID %d", collection, id)
        }
        seenIDs[id] = true
        if r11Int(item, "employee_id") != employeeID {
            return fmt.Errorf("снимок %s содержит запись другого сотрудника", collection)
        }
        if collection == "days" {
            date := r11String(item, "date")
            if !r11DateOK(date) || seenDates[date] {
                return fmt.Errorf("снимок графика содержит неверную или повторную дату %q", date)
            }
            seenDates[date] = true
        }
    }
    return nil
}
