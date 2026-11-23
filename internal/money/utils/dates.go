package utils

import "time"

// Month layouts: a month is "2026-01", a Day "2026-01-31".
const (
	MonthLayout = "2006-01"
	DayLayout   = "2006-01-02"
)

// IsMonth is whether s is a month, such as "2026-01".
func IsMonth(s string) bool {
	_, err := time.Parse(MonthLayout, s)
	return err == nil
}

// IsDay is whether s is a Day, such as "2026-01-31".
func IsDay(s string) bool {
	_, err := time.Parse(DayLayout, s)
	return err == nil
}

// MonthOf is the month of the Day day, such as "2026-01" for "2026-01-31".
func MonthOf(day string) string {
	return day[:len(MonthLayout)]
}

// LastDay is the last Day of month, such as "2026-02-28" for "2026-02".
func LastDay(month string) string {
	first, _ := time.Parse(MonthLayout, month)
	return first.AddDate(0, 1, -1).Format(DayLayout)
}
