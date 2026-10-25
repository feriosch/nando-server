package core_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/feriosch/nando-server/internal/core/model"
)

// signedInPhone returns Nando with an enrolled, signed-in iPhone, its clock
// set to now.
func signedInPhone(t *testing.T, now time.Time) (*nando, *device) {
	t.Helper()
	n := newNando(t)
	phone := n.newDevice("iPhone")
	phone.mustEnroll()
	n.clock.Set(now)
	return n, phone
}

// saveTimeZone sets the Owner Time Zone, keeping the other settings.
func (d *device) saveTimeZone(name string) {
	d.nando.t.Helper()
	s := d.getSettings()
	s.TimeZone = name
	if rec := d.do(http.MethodPut, "/api/settings", mustJSON(d.nando.t, s)); rec.Code != http.StatusNoContent {
		d.nando.t.Fatalf("PUT /api/settings with time zone %s: status = %d, body %q", name, rec.Code, rec.Body)
	}
}

func TestTodayIsTheDayInTheOwnerTimeZone(t *testing.T) {
	ctx := context.Background()
	// Late on Sunday in UTC, already Monday in Tokyo.
	n, phone := signedInPhone(t, time.Date(2026, 10, 25, 16, 0, 0, 0, time.UTC))
	core := n.core.Service()

	if got, err := core.Today(ctx); err != nil || got != "2026-10-25" {
		t.Errorf("Today before the time zone is set = %q, %v; want 2026-10-25 (UTC)", got, err)
	}

	phone.saveTimeZone("Asia/Tokyo")

	if got, err := core.Today(ctx); err != nil || got != "2026-10-26" {
		t.Errorf("Today in Asia/Tokyo = %q, %v; want 2026-10-26", got, err)
	}
	if got, err := core.OwnerTimeZone(ctx); err != nil || got.String() != "Asia/Tokyo" {
		t.Errorf("OwnerTimeZone = %v, %v; want Asia/Tokyo", got, err)
	}
}

func TestLatestWeighInIsTheNewestWeeks(t *testing.T) {
	ctx := context.Background()
	n, phone := signedInPhone(t, wednesday)
	core := n.core.Service()

	if _, err := core.LatestWeighIn(ctx); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("LatestWeighIn before any = %v, want ErrNotFound", err)
	}

	phone.mustPutWeighIn("2026-10-19", 72.4)
	phone.mustPutWeighIn("2026-10-12", 72.8)

	got, err := core.LatestWeighIn(ctx)
	if want := (model.WeighIn{Week: "2026-10-19", Kg: 72.4}); err != nil || got != want {
		t.Errorf("LatestWeighIn = %+v, %v; want %+v", got, err, want)
	}
}
