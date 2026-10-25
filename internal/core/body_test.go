package core_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// bodyJSON is the Body Home Card as GET /api/body answers it.
type bodyJSON struct {
	Latest *weighInJSON `json:"latest"`
	BMI    *float64     `json:"bmi"`
	Change *float64     `json:"change"`
}

func (d *device) getBody() bodyJSON {
	d.nando.t.Helper()
	rec := d.do(http.MethodGet, "/api/body", nil)
	if rec.Code != http.StatusOK {
		d.nando.t.Fatalf("GET /api/body: status = %d, body %q", rec.Code, rec.Body)
	}
	var b bodyJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		d.nando.t.Fatalf("GET /api/body: %v in %q", err, rec.Body)
	}
	return b
}

func TestBodyCardShowsTheLatestWeighInWithBMIAndChange(t *testing.T) {
	_, phone := signedInPhone(t, wednesday)
	phone.saveHeight(175)
	phone.mustPutWeighIn("2026-10-12", 72.8)
	phone.mustPutWeighIn("2026-10-19", 72.4)

	got := phone.getBody()

	if got.Latest == nil || *got.Latest != (weighInJSON{Week: "2026-10-19", Kg: 72.4}) {
		t.Errorf("latest = %+v, want 72.4 kg in the week of 2026-10-19", got.Latest)
	}
	// 72.4 kg / (1.75 m)² = 23.64…
	if got.BMI == nil || *got.BMI != 23.6 {
		t.Errorf("bmi = %v, want 23.6", fmtPtr(got.BMI))
	}
	if got.Change == nil || *got.Change != -0.4 {
		t.Errorf("change = %v, want -0.4", fmtPtr(got.Change))
	}
}

func fmtPtr(f *float64) any {
	if f == nil {
		return "null"
	}
	return *f
}

func TestBodyCardLeavesOutWhatItCannotDerive(t *testing.T) {
	_, phone := signedInPhone(t, wednesday)

	if got := phone.getBody(); got.Latest != nil || got.BMI != nil || got.Change != nil {
		t.Errorf("Body card before any Weigh-in = %+v, want all null", got)
	}

	// No height, and no earlier Weigh-in to compare with.
	phone.mustPutWeighIn("2026-10-19", 72.4)

	got := phone.getBody()
	if got.Latest == nil || got.Latest.Kg != 72.4 {
		t.Errorf("latest = %+v, want 72.4 kg", got.Latest)
	}
	if got.BMI != nil || got.Change != nil {
		t.Errorf("bmi = %v, change = %v; want both null", fmtPtr(got.BMI), fmtPtr(got.Change))
	}
}

func TestBodyCardChangeSkipsWeeksWithoutAWeighIn(t *testing.T) {
	n, phone := signedInPhone(t, wednesday.AddDate(0, 0, -14))
	phone.mustPutWeighIn("2026-10-05", 73.0)
	// No Weigh-in in the week of 2026-10-12.
	n.clock.Set(wednesday)
	phone.mustPutWeighIn("2026-10-19", 72.4)

	got := phone.getBody()

	if got.Change == nil || *got.Change != -0.6 {
		t.Errorf("change = %v, want -0.6 (since the Weigh-in of 2026-10-05)", fmtPtr(got.Change))
	}
}
