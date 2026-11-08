package tests

import (
	"testing"
	"time"
)

// homeCardJSON is the Calories Home Card as the API answers it.
type homeCardJSON struct {
	Day              string `json:"day"`
	ConsumedCalories *int   `json:"consumedCalories"`
	DailyLimit       *int   `json:"dailyLimit"`
	Remaining        *int   `json:"remaining"`
}

func TestHomeCardShowsTodaysConsumedCaloriesAgainstTheDailyLimit(t *testing.T) {
	// 01:00 on Thursday in Tokyo, still Wednesday in UTC.
	_, phone := signedInPhone(t, time.Date(2026, 10, 21, 16, 0, 0, 0, time.UTC))
	phone.saveTimeZone("Asia/Tokyo")
	phone.setDailyLimit(2000)

	got := get[homeCardJSON](phone, "/api/calories/home-card")
	if got.Day != "2026-10-22" || got.ConsumedCalories != nil || !equalInts(got.DailyLimit, iptr(2000)) || got.Remaining != nil {
		t.Errorf("home card before logging = %s %v/%v/%v, want 2026-10-22 null/2000/null",
			got.Day, deref(got.ConsumedCalories), deref(got.DailyLimit), deref(got.Remaining))
	}

	phone.mustLogEntry("2026-10-22", map[string]any{"name": "Onigiri", "kcalPerServing": 230, "servings": 2})
	phone.mustLogEntry("2026-10-21", map[string]any{"name": "Ramen", "kcalPerServing": 900, "servings": 1})

	got = get[homeCardJSON](phone, "/api/calories/home-card")
	if got.Day != "2026-10-22" || !equalInts(got.ConsumedCalories, iptr(460)) || !equalInts(got.DailyLimit, iptr(2000)) || !equalInts(got.Remaining, iptr(1540)) {
		t.Errorf("home card = %s %v/%v/%v, want 2026-10-22 460/2000/1540",
			got.Day, deref(got.ConsumedCalories), deref(got.DailyLimit), deref(got.Remaining))
	}
}
