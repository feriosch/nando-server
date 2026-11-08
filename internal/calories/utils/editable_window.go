package utils

import coremodel "github.com/feriosch/nando-server/internal/core/model"

// EditableDays is how many Days before today the Editable Window reaches.
const EditableDays = 7

// InEditableWindow reports whether day is in the Editable Window: today or one
// of the 7 Days before it. Only those Days' Entries can be changed.
func InEditableWindow(day, today coremodel.Day) bool {
	return day <= today && day >= today.AddDays(-EditableDays)
}
