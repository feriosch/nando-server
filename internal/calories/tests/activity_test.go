package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

// healthSyncJSON is a Day's Health Sync as the Day view shows it.
type healthSyncJSON struct {
	Steps      int    `json:"steps"`
	ActiveKcal int    `json:"activeKcal"`
	SyncedAt   string `json:"syncedAt"`
}

// walkJSON is a Treadmill Walk as the API answers it.
type walkJSON struct {
	ID            string  `json:"id"`
	Day           string  `json:"day"`
	Km            float64 `json:"km"`
	Steps         *int    `json:"steps"`
	Kcal          int     `json:"kcal"`
	KcalEstimated bool    `json:"kcalEstimated"`
	LoggedAt      string  `json:"loggedAt"`
}

// activityJSON is what a Day's Steps and Burned Calories come from.
type activityJSON struct {
	HealthSync *healthSyncJSON `json:"healthSync"`
	Walks      []walkJSON      `json:"walks"`
}

// activeDayJSON is the Day view's Steps and Burned Calories, and what they
// come from.
type activeDayJSON struct {
	Day            string       `json:"day"`
	Steps          *int         `json:"steps"`
	BurnedCalories *int         `json:"burnedCalories"`
	Activity       activityJSON `json:"activity"`
}

// syncDayJSON is one Day a Health Sync sends.
type syncDayJSON struct {
	Day        string `json:"day"`
	Steps      int    `json:"steps"`
	ActiveKcal int    `json:"activeKcal"`
}

// mustCreateSyncKey creates a Sync Key in Core's Settings, approving it with
// the Owner's Passkey, and returns its key, as the Owner copies it into the
// Shortcut.
func (d *browser) mustCreateSyncKey() string {
	d.nando.t.Helper()
	options := d.must(http.MethodPost, "/api/sync-keys/begin", map[string]string{"name": "iPhone Health"}, http.StatusOK)
	rec := d.must(http.MethodPost, "/api/sync-keys/finish", json.RawMessage(d.passkey.Get(d.nando.t, webOrigin, options.Body.Bytes())), http.StatusCreated)
	var created struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.Key == "" {
		d.nando.t.Fatalf("create Sync Key: %v in %q", err, rec.Body)
	}
	return created.Key
}

// healthSync sends days as the Health Sync Shortcut does, with key and no
// Session.
func (n *nando) healthSync(key string, days ...syncDayJSON) *httptest.ResponseRecorder {
	n.t.Helper()
	body, err := json.Marshal(map[string]any{"days": days})
	if err != nil {
		n.t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/calories/health-sync", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	n.handler.ServeHTTP(rec, req)
	return rec
}

// mustHealthSync sends days, which must be taken.
func (n *nando) mustHealthSync(key string, days ...syncDayJSON) {
	n.t.Helper()
	if rec := n.healthSync(key, days...); rec.Code != http.StatusNoContent {
		n.t.Fatalf("POST /api/calories/health-sync: status = %d, body %q; want %d", rec.Code, rec.Body, http.StatusNoContent)
	}
}

func TestAHealthSyncShowsOnItsDay(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	key := phone.mustCreateSyncKey()

	n.mustHealthSync(key, syncDayJSON{Day: "2026-10-21", Steps: 9812, ActiveKcal: 312})

	got := get[activeDayJSON](phone, "/api/calories/days/2026-10-21")
	if !equalInts(got.Steps, iptr(9812)) || !equalInts(got.BurnedCalories, iptr(312)) {
		t.Errorf("steps/burned = %v/%v, want 9812/312", show(got.Steps), show(got.BurnedCalories))
	}
	want := healthSyncJSON{Steps: 9812, ActiveKcal: 312, SyncedAt: "2026-10-21T12:00:00Z"}
	if got.Activity.HealthSync == nil || *got.Activity.HealthSync != want || got.Activity.Walks == nil || len(got.Activity.Walks) != 0 {
		t.Errorf("activity = %+v, want the Health Sync %+v and no walks", got.Activity, want)
	}
}

func TestALaterHealthSyncReplacesTheDaysLastOne(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	key := phone.mustCreateSyncKey()
	n.mustHealthSync(key, syncDayJSON{Day: "2026-10-21", Steps: 4000, ActiveKcal: 120}, syncDayJSON{Day: "2026-10-20", Steps: 11402, ActiveKcal: 380})

	n.clock.Set(wednesday.Add(6 * time.Hour))
	n.mustHealthSync(key, syncDayJSON{Day: "2026-10-21", Steps: 9812, ActiveKcal: 312})

	today := get[activeDayJSON](phone, "/api/calories/days/2026-10-21")
	if want := (healthSyncJSON{Steps: 9812, ActiveKcal: 312, SyncedAt: "2026-10-21T18:00:00Z"}); today.Activity.HealthSync == nil || *today.Activity.HealthSync != want {
		t.Errorf("today's Health Sync = %+v, want %+v", today.Activity.HealthSync, want)
	}
	if yesterday := get[activeDayJSON](phone, "/api/calories/days/2026-10-20"); !equalInts(yesterday.Steps, iptr(11402)) {
		t.Errorf("yesterday's steps = %v, want 11402, untouched", show(yesterday.Steps))
	}
}

func TestOnlyTodayAndYesterdayCanBeSynced(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	key := phone.mustCreateSyncKey()

	for _, day := range []string{"2026-10-22", "2026-10-19", "21-10-2026"} {
		// A good Day alongside: if one Day is wrong, none is taken.
		rec := n.healthSync(key, syncDayJSON{Day: "2026-10-21", Steps: 100, ActiveKcal: 5}, syncDayJSON{Day: day, Steps: 100, ActiveKcal: 5})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("sync %s: status = %d, want %d", day, rec.Code, http.StatusBadRequest)
		}
	}
	if got := get[activeDayJSON](phone, "/api/calories/days/2026-10-21"); got.Activity.HealthSync != nil {
		t.Errorf("today's Health Sync = %+v, want none", got.Activity.HealthSync)
	}
}

func TestAHealthSyncNeedsItsNumbers(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	key := phone.mustCreateSyncKey()

	for name, body := range map[string]string{
		"no days":           `{"days": []}`,
		"no steps":          `{"days": [{"day": "2026-10-21", "activeKcal": 5}]}`,
		"no activeKcal":     `{"days": [{"day": "2026-10-21", "steps": 5}]}`,
		"negative steps":    `{"days": [{"day": "2026-10-21", "steps": -1, "activeKcal": 5}]}`,
		"fractional steps":  `{"days": [{"day": "2026-10-21", "steps": 1.5, "activeKcal": 5}]}`,
		"too many kcal":     `{"days": [{"day": "2026-10-21", "steps": 5, "activeKcal": 10001}]}`,
		"not a days object": `[1, 2]`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/calories/health-sync", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+key)
		rec := httptest.NewRecorder()
		n.handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("sync with %s: status = %d, want %d", name, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestAHealthSyncNeedsASyncKey(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	phone.mustCreateSyncKey()

	if rec := n.healthSync("not-a-sync-key", syncDayJSON{Day: "2026-10-21", Steps: 9812, ActiveKcal: 312}); rec.Code != http.StatusUnauthorized {
		t.Errorf("sync with an unknown key: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestADayWithNoActivityHasNoStepsNotZero(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	got := get[activeDayJSON](phone, "/api/calories/days/2026-10-21")

	if got.Steps != nil || got.BurnedCalories != nil || got.Activity.HealthSync != nil || got.Activity.Walks == nil || len(got.Activity.Walks) != 0 {
		t.Errorf("Day with no activity = %+v steps/burned %v/%v; want null, null, no Health Sync and no walks",
			got.Activity, show(got.Steps), show(got.BurnedCalories))
	}
}

// walkChangedJSON is what a Tool that changes a Treadmill Walk answers: the
// walk, and its Day's Steps and Burned Calories now.
type walkChangedJSON struct {
	Walk walkJSON `json:"walk"`
	Day  struct {
		Day            string `json:"day"`
		Steps          *int   `json:"steps"`
		BurnedCalories *int   `json:"burnedCalories"`
	} `json:"day"`
}

func TestAWalksStepsComeFromItsKmAndTheOwnersHeight(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	phone.saveHeight(175)
	n.mustHealthSync(phone.mustCreateSyncKey(), syncDayJSON{Day: "2026-10-21", Steps: 9812, ActiveKcal: 312})

	got := callTool[walkChangedJSON](t, phone.tools(), "calories_log_walk", map[string]any{"km": 3.2, "kcal": 210})

	// 3.2 km at a stride of 0.415 × 175 cm = 72.625 cm: 4,406 steps.
	want := walkJSON{ID: got.Walk.ID, Day: "2026-10-21", Km: 3.2, Steps: iptr(4406), Kcal: 210, LoggedAt: "2026-10-21T12:00:00Z"}
	if got.Walk.ID == "" || !equalWalks(got.Walk, want) {
		t.Errorf("logged walk = %s, want %s", showWalk(got.Walk), showWalk(want))
	}
	if !equalInts(got.Day.Steps, iptr(9812+4406)) || !equalInts(got.Day.BurnedCalories, iptr(312+210)) {
		t.Errorf("the Day's steps/burned = %v/%v, want 14218/522", show(got.Day.Steps), show(got.Day.BurnedCalories))
	}
	day := get[activeDayJSON](phone, "/api/calories/days/2026-10-21")
	if len(day.Activity.Walks) != 1 || !equalWalks(day.Activity.Walks[0], want) || !equalInts(day.Steps, iptr(14218)) {
		t.Errorf("Day view = steps %v, walks %+v; want 14218 and the walk", show(day.Steps), day.Activity.Walks)
	}
}

func equalWalks(a, b walkJSON) bool {
	return a.ID == b.ID && a.Day == b.Day && a.Km == b.Km && equalInts(a.Steps, b.Steps) && a.Kcal == b.Kcal &&
		a.KcalEstimated == b.KcalEstimated && a.LoggedAt == b.LoggedAt
}

func showWalk(w walkJSON) string {
	return fmt.Sprintf("{%s %s %.2f km, %v steps, %d kcal estimated=%v at %s}", w.ID, w.Day, w.Km, show(w.Steps), w.Kcal, w.KcalEstimated, w.LoggedAt)
}

func TestAWalkWithoutKcalIsEstimatedFromTheLatestWeighIn(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	phone.saveHeight(175)
	phone.mustLogWeighIn("2026-10-21", 72.4)

	got := callTool[walkChangedJSON](t, phone.tools(), "calories_log_walk", map[string]any{"km": 3.2})

	// 0.5 kcal per kg per km: 0.5 × 72.4 × 3.2 = 115.84.
	if got.Walk.Kcal != 116 || !got.Walk.KcalEstimated {
		t.Errorf("walk kcal = %d estimated=%v, want 116 estimated", got.Walk.Kcal, got.Walk.KcalEstimated)
	}
}

func TestAWalkWithoutKcalOrAWeighInAsksForTheScreensKcal(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	phone.saveHeight(175)
	session := phone.tools()

	if got := toolError(t, session, "calories_log_walk", map[string]any{"km": 3.2}); !strings.Contains(got, "kcal") {
		t.Errorf("calories_log_walk without kcal or a Weigh-in says %q, want it to ask for the kcal", got)
	}
	if day := get[activeDayJSON](phone, "/api/calories/days/2026-10-21"); len(day.Activity.Walks) != 0 {
		t.Errorf("walks = %+v, want none", day.Activity.Walks)
	}
}

func TestAWalkWithoutTheOwnersHeightHasNoSteps(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	got := callTool[walkChangedJSON](t, phone.tools(), "calories_log_walk", map[string]any{"km": 3.2, "kcal": 210})

	if got.Walk.Steps != nil || got.Day.Steps != nil || !equalInts(got.Day.BurnedCalories, iptr(210)) {
		t.Errorf("walk steps %v, Day steps/burned %v/%v; want null, null and 210", show(got.Walk.Steps), show(got.Day.Steps), show(got.Day.BurnedCalories))
	}
}

func TestAWalkNeedsSensibleKmAndKcal(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	session := phone.tools()

	for name, args := range map[string]map[string]any{
		"no km":            {"kcal": 210},
		"0 km":             {"km": 0, "kcal": 210},
		"more than 100 km": {"km": 100.5, "kcal": 210},
		"negative kcal":    {"km": 3.2, "kcal": -1},
		"a future Day":     {"day": "2026-10-22", "km": 3.2, "kcal": 210},
		"an old Day":       {"day": "2026-10-13", "km": 3.2, "kcal": 210},
	} {
		if got := toolError(t, session, "calories_log_walk", args); got == "" {
			t.Errorf("calories_log_walk with %s: refused without saying why", name)
		}
	}
	if day := get[activeDayJSON](phone, "/api/calories/days/2026-10-21"); len(day.Activity.Walks) != 0 {
		t.Errorf("walks = %+v, want none", day.Activity.Walks)
	}
}

func TestTheAssistantCorrectsAWalk(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	phone.saveHeight(175)
	phone.mustLogWeighIn("2026-10-21", 72.4)
	session := phone.tools()
	logged := callTool[walkChangedJSON](t, session, "calories_log_walk", map[string]any{"km": 3.2})

	// A new km: new Steps, and the estimate follows it.
	got := callTool[walkChangedJSON](t, session, "calories_update_walk", map[string]any{"id": logged.Walk.ID, "km": 4})
	if !equalInts(got.Walk.Steps, iptr(5508)) || got.Walk.Kcal != 145 || !got.Walk.KcalEstimated || got.Walk.Km != 4 {
		t.Errorf("walk after km 4 = %s, want 4 km, 5508 steps, 145 kcal estimated", showWalk(got.Walk))
	}
	// The screen's kcal, found later: no longer an estimate.
	got = callTool[walkChangedJSON](t, session, "calories_update_walk", map[string]any{"id": logged.Walk.ID, "kcal": 230})
	if got.Walk.Kcal != 230 || got.Walk.KcalEstimated || !equalInts(got.Day.BurnedCalories, iptr(230)) {
		t.Errorf("walk after kcal 230 = %s, Day burned %v; want 230, not estimated", showWalk(got.Walk), show(got.Day.BurnedCalories))
	}
}

func TestTheAssistantDeletesAWalk(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	session := phone.tools()
	logged := callTool[walkChangedJSON](t, session, "calories_log_walk", map[string]any{"km": 3.2, "kcal": 210})

	got := callTool[walkChangedJSON](t, session, "calories_delete_walk", map[string]any{"id": logged.Walk.ID})

	if got.Day.Day != "2026-10-21" || got.Day.Steps != nil || got.Day.BurnedCalories != nil {
		t.Errorf("the Day after deleting = %+v, want 2026-10-21 with no Steps or Burned Calories", got.Day)
	}
	if day := get[activeDayJSON](phone, "/api/calories/days/2026-10-21"); len(day.Activity.Walks) != 0 {
		t.Errorf("walks = %+v, want none", day.Activity.Walks)
	}
	if msg := toolError(t, session, "calories_delete_walk", map[string]any{"id": logged.Walk.ID}); !strings.Contains(msg, "no such Treadmill Walk") {
		t.Errorf("deleting it again says %q, want that there's no such walk", msg)
	}
}

func TestAWalkOutsideTheEditableWindowCantChange(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	logged := callTool[walkChangedJSON](t, phone.tools(), "calories_log_walk", map[string]any{"km": 3.2, "kcal": 210})
	n.clock.Set(wednesday.AddDate(0, 0, 8))
	session := phone.tools()

	for name, args := range map[string]map[string]any{
		"calories_update_walk": {"id": logged.Walk.ID, "kcal": 250},
		"calories_delete_walk": {"id": logged.Walk.ID},
	} {
		if got := toolError(t, session, name, args); !strings.Contains(got, "only today and the 7 Days before it") {
			t.Errorf("%s says %q, want why it can't", name, got)
		}
	}
	if day := get[activeDayJSON](phone, "/api/calories/days/2026-10-21"); len(day.Activity.Walks) != 1 || day.Activity.Walks[0].Kcal != 210 {
		t.Errorf("walks = %+v, want the walk unchanged", day.Activity.Walks)
	}
}

// activeTotalsJSON is a Day's Steps and Burned Calories, as the Home Card
// and the history answer them beside its Consumed Calories.
type activeTotalsJSON struct {
	Day              string `json:"day"`
	ConsumedCalories *int   `json:"consumedCalories"`
	Steps            *int   `json:"steps"`
	BurnedCalories   *int   `json:"burnedCalories"`
}

func TestTheHomeCardShowsTodaysStepsAndBurnedCalories(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	n.mustHealthSync(phone.mustCreateSyncKey(), syncDayJSON{Day: "2026-10-21", Steps: 9812, ActiveKcal: 312})
	callTool[walkChangedJSON](t, phone.tools(), "calories_log_walk", map[string]any{"km": 3.2, "kcal": 210})

	got := get[activeTotalsJSON](phone, "/api/calories/home-card")

	if got.Day != "2026-10-21" || !equalInts(got.Steps, iptr(9812)) || !equalInts(got.BurnedCalories, iptr(522)) {
		t.Errorf("Home Card = %s steps/burned %v/%v, want 2026-10-21 9812/522 (the walk has no Steps without a height)",
			got.Day, show(got.Steps), show(got.BurnedCalories))
	}
}

func TestTheHistoryShowsEachDaysStepsAndBurnedCalories(t *testing.T) {
	n, phone := signedInBrowser(t, wednesday)
	phone.saveHeight(175)
	key := phone.mustCreateSyncKey()
	n.mustHealthSync(key, syncDayJSON{Day: "2026-10-20", Steps: 11402, ActiveKcal: 380})
	callTool[walkChangedJSON](t, phone.tools(), "calories_log_walk", map[string]any{"day": "2026-10-19", "km": 3.2, "kcal": 210})
	phone.mustLogEntry("2026-10-21", map[string]any{"name": "Sushi", "kcalPerServing": 45, "servings": 12})

	got := get[[]activeTotalsJSON](phone, "/api/calories/history?from=2026-10-19&to=2026-10-21")

	want := []activeTotalsJSON{
		{Day: "2026-10-21", ConsumedCalories: iptr(540)},
		{Day: "2026-10-20", Steps: iptr(11402), BurnedCalories: iptr(380)},
		{Day: "2026-10-19", Steps: iptr(4406), BurnedCalories: iptr(210)},
	}
	if len(got) != len(want) {
		t.Fatalf("history = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].Day != want[i].Day || !equalInts(got[i].ConsumedCalories, want[i].ConsumedCalories) ||
			!equalInts(got[i].Steps, want[i].Steps) || !equalInts(got[i].BurnedCalories, want[i].BurnedCalories) {
			t.Errorf("history[%d] = %s %v kcal, %v steps, %v burned; want %s %v, %v, %v", i,
				got[i].Day, show(got[i].ConsumedCalories), show(got[i].Steps), show(got[i].BurnedCalories),
				want[i].Day, show(want[i].ConsumedCalories), show(want[i].Steps), show(want[i].BurnedCalories))
		}
	}
}

func TestTheEveningSummaryAddsTodaysStepsAndBurnedCalories(t *testing.T) {
	n, phone := signedInBrowser(t, tokyoTime(9, 0))
	phone.saveTimeZone("Asia/Tokyo")
	phone.setDailyLimit(2000)
	phone.saveCaloriesSettings(off("14:00"), on("21:00"))
	phone.mustLogEntry("2026-10-22", map[string]any{"name": "Onigiri", "kcalPerServing": 230, "servings": 2})
	n.mustHealthSync(phone.mustCreateSyncKey(), syncDayJSON{Day: "2026-10-22", Steps: 9812, ActiveKcal: 340})

	n.tickAt(tokyoTime(21, 0))

	want := []notification{{Title: "Calories", Body: "Today: 460 / 2000 kcal\n9812 steps · 340 kcal burned (est.)"}}
	if got := n.received(); !slices.Equal(got, want) {
		t.Errorf("received %+v, want %+v", got, want)
	}
}

func TestTheOwnerIsToldOnceWhenStepsStopSyncing(t *testing.T) {
	n, phone := signedInBrowser(t, tokyoTime(9, 0))
	phone.saveTimeZone("Asia/Tokyo")
	key := phone.mustCreateSyncKey()

	// Never synced yet: nothing to say.
	n.tickAt(tokyoTime(9, 0).Add(72 * time.Hour))
	if got := n.received(); len(got) != 0 {
		t.Fatalf("received %+v before any Health Sync, want nothing", got)
	}

	n.clock.Set(tokyoTime(23, 30)) // Thursday 2026-10-22
	n.mustHealthSync(key, syncDayJSON{Day: "2026-10-22", Steps: 9812, ActiveKcal: 340})
	n.tickAt(tokyoTime(23, 30).Add(47*time.Hour + 30*time.Minute))
	if got := n.received(); len(got) != 0 {
		t.Fatalf("received %+v within 48 hours, want nothing", got)
	}
	n.tickAt(tokyoTime(23, 30).Add(48 * time.Hour))
	n.tickAt(tokyoTime(23, 30).Add(60 * time.Hour))

	want := []notification{{Title: "Calories", Body: "Steps haven't synced since Thu, Oct 22"}}
	if got := n.received(); !slices.Equal(got, want) {
		t.Errorf("received %+v, want %+v, once", got, want)
	}
}
