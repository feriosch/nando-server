package tests

import (
	"testing"
	"time"

	"github.com/feriosch/nando-server/internal/calories/model"
)

func TestHomeCardShowsTodaysConsumedCaloriesAgainstTheDailyLimit(t *testing.T) {
	// 01:00 on Thursday in Tokyo, still Wednesday in UTC.
	_, phone := signedInBrowser(t, time.Date(2026, 10, 21, 16, 0, 0, 0, time.UTC))
	phone.saveTimeZone("Asia/Tokyo")
	phone.setDailyLimit(2000)

	got := get[model.CaloriesJSON](phone, "/api/calories/home-card")
	if want := (model.CaloriesJSON{Day: "2026-10-22", DailyLimit: iptr(2000)}); !equalCalories(got, want) {
		t.Errorf("home card before logging = %s, want %s", showCalories(got), showCalories(want))
	}

	phone.mustLogEntry("2026-10-22", map[string]any{"name": "Onigiri", "kcalPerServing": 230, "servings": 2})
	phone.mustLogEntry("2026-10-21", map[string]any{"name": "Ramen", "kcalPerServing": 900, "servings": 1})

	got = get[model.CaloriesJSON](phone, "/api/calories/home-card")
	want := model.CaloriesJSON{Day: "2026-10-22", ConsumedCalories: iptr(460), DailyLimit: iptr(2000), Remaining: iptr(1540), OverLimit: bptr(false)}
	if !equalCalories(got, want) {
		t.Errorf("home card = %s, want %s", showCalories(got), showCalories(want))
	}
}
