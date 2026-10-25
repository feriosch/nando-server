package model

import "time"

// Day is one calendar date in the Owner Time Zone, as "YYYY-MM-DD". It is a
// plain date, not an instant, so changing the Owner Time Zone never moves data
// stored by Day. Days sort as strings.
type Day string

// DayOf returns the Day that t falls on in loc.
func DayOf(t time.Time, loc *time.Location) Day {
	return Day(t.In(loc).Format(time.DateOnly))
}

// ParseDay returns the Day written "YYYY-MM-DD" in s.
func ParseDay(s string) (Day, error) {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return "", err
	}
	return Day(t.Format(time.DateOnly)), nil
}

// date is d's midnight in UTC, for calendar arithmetic. A Day that isn't a
// valid date is the zero time.
func (d Day) date() time.Time {
	t, _ := time.Parse(time.DateOnly, string(d))
	return t
}

// AddDays returns the Day n days after d, or before it if n is negative.
func (d Day) AddDays(n int) Day {
	return Day(d.date().AddDate(0, 0, n).Format(time.DateOnly))
}

// Week returns the Monday that starts d's week.
func (d Day) Week() Day {
	sinceMonday := (int(d.date().Weekday()) + 6) % 7
	return d.AddDays(-sinceMonday)
}
