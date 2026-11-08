package utils

import "regexp"

// halfHour matches a clock time "HH:MM" on the hour or half hour.
var halfHour = regexp.MustCompile(`^([01][0-9]|2[0-3]):(00|30)$`)

// IsHalfHour reports whether s is a clock time "HH:MM" on the hour or half
// hour, "00:00" to "23:30": the times a Notification can be sent at, since
// Core's tick runs every half hour.
func IsHalfHour(s string) bool {
	return halfHour.MatchString(s)
}
