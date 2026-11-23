package main

import (
    "encoding/json"
    "fmt"
    "math"
    "strconv"
    "strings"
)

// Stage data may arrive in the legacy compact spreadsheet format ("d") or as
// structured days. Do not silently turn invalid input into zero planned hours.
func r11StageHours(v any) (float64, error) {
    var hours float64
    switch x := v.(type) {
    case float64:
        hours = x
    case float32:
        hours = float64(x)
    case int:
        hours = float64(x)
    case json.Number:
        parsed, err := x.Float64()
        if err != nil { return 0, fmt.Errorf("некорректное значение часов: %v", v) }
        hours = parsed
    case string:
        normalized := strings.ReplaceAll(strings.TrimSpace(x), ",", ".")
        if normalized == "" { return 0, nil } // Historical blank cells denote a day with 0 hours.
        parsed, err := strconv.ParseFloat(normalized, 64)
        if err != nil { return 0, fmt.Errorf("некорректное значение часов: %q", x) }
        hours = parsed
    default:
        return 0, fmt.Errorf("некорректный тип часов: %T", v)
    }
    if math.IsNaN(hours) || math.IsInf(hours, 0) || hours < 0 || hours > 24 {
        return 0, fmt.Errorf("плановые часы должны быть от 0 до 24")
    }
    return hours, nil
}

func r11ValidateStageDays(days []dayPatch) error {
    if len(days) == 0 { return fmt.Errorf("пустой пакет дней") }
    if len(days) > 100000 { return fmt.Errorf("слишком большой пакет") }
    seen := make(map[string]bool, len(days))
    for i, day := range days {
        if strings.TrimSpace(day.TabNo) == "" {
            return fmt.Errorf("строка %d: нет табельного номера", i+1)
        }
        if !r11DateOK(strings.TrimSpace(day.Date)) {
            return fmt.Errorf("строка %d: некорректная дата (нужна YYYY-MM-DD)", i+1)
        }
        if math.IsNaN(day.PlanHours) || math.IsInf(day.PlanHours, 0) || day.PlanHours < 0 || day.PlanHours > 24 {
            return fmt.Errorf("строка %d: плановые часы должны быть от 0 до 24", i+1)
        }
        key := strings.ToLower(strings.TrimSpace(day.TabNo)) + "|" + strings.TrimSpace(day.Date)
        if seen[key] {
            return fmt.Errorf("строка %d: повторяющийся день %s для сотрудника %s", i+1, day.Date, day.TabNo)
        }
        seen[key] = true
    }
    return nil
}
