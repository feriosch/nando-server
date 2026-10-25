package core_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// weighInJSON is one Weigh-in as GET /api/weigh-ins lists it.
type weighInJSON struct {
	Week string  `json:"week"`
	Kg   float64 `json:"kg"`
}

// wednesday is a Wednesday at noon UTC, in the week starting Monday 2026-10-19.
var wednesday = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

// putWeighIn logs kg as the Weigh-in for the week starting on Monday week.
func (d *device) putWeighIn(week string, kg float64) *httptest.ResponseRecorder {
	d.nando.t.Helper()
	return d.do(http.MethodPut, "/api/weigh-ins/"+week, []byte(fmt.Sprintf(`{"kg": %v}`, kg)))
}

func (d *device) mustPutWeighIn(week string, kg float64) {
	d.nando.t.Helper()
	if rec := d.putWeighIn(week, kg); rec.Code != http.StatusNoContent {
		d.nando.t.Fatalf("PUT /api/weigh-ins/%s: status = %d, body %q", week, rec.Code, rec.Body)
	}
}

func (d *device) listWeighIns() []weighInJSON {
	d.nando.t.Helper()
	rec := d.do(http.MethodGet, "/api/weigh-ins", nil)
	if rec.Code != http.StatusOK {
		d.nando.t.Fatalf("GET /api/weigh-ins: status = %d, body %q", rec.Code, rec.Body)
	}
	var weighIns []weighInJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &weighIns); err != nil {
		d.nando.t.Fatalf("GET /api/weigh-ins: %v in %q", err, rec.Body)
	}
	return weighIns
}

func TestOwnerLogsThisWeeksWeighIn(t *testing.T) {
	_, phone := signedInPhone(t, wednesday)

	phone.mustPutWeighIn("2026-10-19", 72.4)

	got := phone.listWeighIns()
	if len(got) != 1 || got[0] != (weighInJSON{Week: "2026-10-19", Kg: 72.4}) {
		t.Errorf("Weigh-ins = %+v, want only 72.4 kg in the week of 2026-10-19", got)
	}
}

func TestALaterWeighInReplacesTheWeeksEarlierOne(t *testing.T) {
	_, phone := signedInPhone(t, wednesday)
	phone.mustPutWeighIn("2026-10-19", 72.4)

	phone.mustPutWeighIn("2026-10-19", 71.9)

	got := phone.listWeighIns()
	if len(got) != 1 || got[0] != (weighInJSON{Week: "2026-10-19", Kg: 71.9}) {
		t.Errorf("Weigh-ins = %+v, want only 71.9 kg in the week of 2026-10-19", got)
	}
}

func TestOnlyThisAndLastWeekCanBeLogged(t *testing.T) {
	_, phone := signedInPhone(t, wednesday)

	for week, want := range map[string]int{
		"2026-10-26": http.StatusConflict,  // next week
		"2026-10-19": http.StatusNoContent, // this week
		"2026-10-12": http.StatusNoContent, // last week
		"2026-10-05": http.StatusConflict,  // two weeks ago
	} {
		if rec := phone.putWeighIn(week, 72.4); rec.Code != want {
			t.Errorf("PUT /api/weigh-ins/%s on %s: status = %d, want %d", week, wednesday.Format(time.DateOnly), rec.Code, want)
		}
	}

	got := phone.listWeighIns()
	if len(got) != 2 || got[0].Week != "2026-10-19" || got[1].Week != "2026-10-12" {
		t.Errorf("Weigh-ins = %+v, want this week's and last week's, newest first", got)
	}
}

func TestInvalidWeighInsAreRejected(t *testing.T) {
	_, phone := signedInPhone(t, wednesday)

	for name, req := range map[string]struct{ week, body string }{
		"week not starting on a Monday": {"2026-10-20", `{"kg": 72.4}`},
		"week that isn't a date":        {"this-week", `{"kg": 72.4}`},
		"week not written YYYY-MM-DD":   {"2026-10-019", `{"kg": 72.4}`},
		"weight of zero":                {"2026-10-19", `{"kg": 0}`},
		"negative weight":               {"2026-10-19", `{"kg": -72.4}`},
		"weight over 500 kg":            {"2026-10-19", `{"kg": 500.1}`},
		"weight to 2 decimals":          {"2026-10-19", `{"kg": 72.45}`},
		"no weight":                     {"2026-10-19", `{}`},
		"weight as a string":            {"2026-10-19", `{"kg": "72.4"}`},
	} {
		if rec := phone.do(http.MethodPut, "/api/weigh-ins/"+req.week, []byte(req.body)); rec.Code != http.StatusBadRequest {
			t.Errorf("PUT a Weigh-in with %s: status = %d, want %d", name, rec.Code, http.StatusBadRequest)
		}
	}

	if got := phone.listWeighIns(); len(got) != 0 {
		t.Errorf("Weigh-ins after only invalid ones = %+v, want none", got)
	}
}

func TestOwnerDeletesAWeighIn(t *testing.T) {
	_, phone := signedInPhone(t, wednesday)
	phone.mustPutWeighIn("2026-10-12", 72.8)
	phone.mustPutWeighIn("2026-10-19", 72.4)

	if rec := phone.do(http.MethodDelete, "/api/weigh-ins/2026-10-19", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/weigh-ins/2026-10-19: status = %d, body %q", rec.Code, rec.Body)
	}

	got := phone.listWeighIns()
	if len(got) != 1 || got[0].Week != "2026-10-12" {
		t.Errorf("Weigh-ins after deleting this week's = %+v, want only last week's", got)
	}
}

func TestWeighInsBeforeLastWeekCanNoLongerBeChanged(t *testing.T) {
	n, phone := signedInPhone(t, wednesday)
	phone.mustPutWeighIn("2026-10-19", 72.4)
	n.clock.Set(wednesday.AddDate(0, 0, 14))

	if rec := phone.putWeighIn("2026-10-19", 71.0); rec.Code != http.StatusConflict {
		t.Errorf("PUT the Weigh-in from two weeks ago: status = %d, want %d", rec.Code, http.StatusConflict)
	}
	if rec := phone.do(http.MethodDelete, "/api/weigh-ins/2026-10-19", nil); rec.Code != http.StatusConflict {
		t.Errorf("DELETE the Weigh-in from two weeks ago: status = %d, want %d", rec.Code, http.StatusConflict)
	}

	got := phone.listWeighIns()
	if len(got) != 1 || got[0] != (weighInJSON{Week: "2026-10-19", Kg: 72.4}) {
		t.Errorf("Weigh-ins = %+v, want the one from two weeks ago unchanged", got)
	}
}

func TestChangingTheTimeZoneDoesNotMoveWeighIns(t *testing.T) {
	// Sunday evening in UTC, already Monday morning of the next week in Tokyo.
	_, phone := signedInPhone(t, time.Date(2026, 10, 25, 20, 0, 0, 0, time.UTC))
	phone.mustPutWeighIn("2026-10-19", 72.4)
	if rec := phone.putWeighIn("2026-10-26", 72.0); rec.Code != http.StatusConflict {
		t.Fatalf("PUT next week's Weigh-in in UTC: status = %d, want %d", rec.Code, http.StatusConflict)
	}

	phone.saveTimeZone("Asia/Tokyo")

	got := phone.listWeighIns()
	if len(got) != 1 || got[0] != (weighInJSON{Week: "2026-10-19", Kg: 72.4}) {
		t.Errorf("Weigh-ins after moving to Asia/Tokyo = %+v, want 72.4 kg still in the week of 2026-10-19", got)
	}
	// What moves is which week is this week.
	if rec := phone.putWeighIn("2026-10-26", 72.0); rec.Code != http.StatusNoContent {
		t.Errorf("PUT this week's Weigh-in in Asia/Tokyo: status = %d, body %q", rec.Code, rec.Body)
	}
}
