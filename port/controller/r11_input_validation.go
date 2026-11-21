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
        "schedule_type", "hire_date", "fire_date", "probation_end", "notes",
    } {
        if err := r11StrictStringField(body, key); err != nil { return err }
    }
    for _, key := range []string{"is_quarter", "probation", "main_schedule_ready"} {
        if err := r11StrictBooleanField(body, key); err != nil { return err }
    }
    for _, key := range []string{"hire_date", "fire_date", "probation_end"} {
        if value := r11String(body, key); value != "" && !r11DateOK(value) {
            return fmt.Errorf("поле %s должно быть корректной датой YYYY-MM-DD", key)
        }
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
