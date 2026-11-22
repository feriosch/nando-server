package tests

import (
	"net/http"
	"reflect"
	"strconv"
	"testing"
)

// weightCardJSON is the Weight Home Card as the API answers it.
type weightCardJSON struct {
	Latest *weighInJSON `json:"latest"`
	BMI    *float64     `json:"bmi"`
	Change *float64     `json:"change"`
}

// weighInsJSON is what the Weigh-in Tools answer: every Weigh-in, with the
// Weight Home Card's numbers.
type weighInsJSON struct {
	WeighIns []weighInJSON `json:"weighIns"`
	Latest   *weighInJSON  `json:"latest"`
	BMI      *float64      `json:"bmi"`
	Change   *float64      `json:"change"`
}

// saveHeight sets the Owner's height through Core's settings API, keeping
// the time zone.
func (d *browser) saveHeight(cm int) {
	d.nando.t.Helper()
	settings := get[map[string]any](d, "/api/settings")
	settings["heightCm"] = cm
	d.must(http.MethodPut, "/api/settings", settings, http.StatusNoContent)
}

func fptr(f float64) *float64 { return &f }

func TestTheWeightCardShowsTheLatestWeighInWithBMIAndChange(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	phone.saveHeight(175)
	phone.mustLogWeighIn("2026-10-12", 72.8)
	phone.mustLogWeighIn("2026-10-19", 72.4)

	got := get[weightCardJSON](phone, "/api/calories/weight-card")

	// 72.4 kg / (1.75 m)² = 23.64…
	want := weightCardJSON{Latest: &weighInJSON{Week: "2026-10-19", Kg: 72.4}, BMI: fptr(23.6), Change: fptr(-0.4)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Weight card = %s, want %s", showCard(got), showCard(want))
	}
}

func TestTheWeightCardLeavesOutWhatItCannotDerive(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)

	if got := get[weightCardJSON](phone, "/api/calories/weight-card"); got != (weightCardJSON{}) {
		t.Errorf("Weight card before any Weigh-in = %s, want all null", showCard(got))
	}

	// No height, and the week before has no Weigh-in.
	phone.mustLogWeighIn("2026-10-19", 72.4)

	got := get[weightCardJSON](phone, "/api/calories/weight-card")
	if want := (weightCardJSON{Latest: &weighInJSON{Week: "2026-10-19", Kg: 72.4}}); !reflect.DeepEqual(got, want) {
		t.Errorf("Weight card = %s, want %s", showCard(got), showCard(want))
	}
}

func TestTheAssistantReadsTheWeighInsWithTheCardsNumbers(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	phone.saveHeight(175)
	phone.mustLogWeighIn("2026-10-12", 72.8)

	// Logging answers the same as listing: "72.4 kg, −0.4 since last week, BMI 23.6".
	logged := callTool[weighInsJSON](t, phone.tools(), "calories_log_weigh_in", map[string]any{"kg": 72.4})
	listed := callTool[weighInsJSON](t, phone.tools(), "calories_list_weigh_ins", nil)

	want := weighInsJSON{
		WeighIns: []weighInJSON{{Week: "2026-10-19", Kg: 72.4}, {Week: "2026-10-12", Kg: 72.8}},
		Latest:   &weighInJSON{Week: "2026-10-19", Kg: 72.4},
		BMI:      fptr(23.6),
		Change:   fptr(-0.4),
	}
	for name, got := range map[string]weighInsJSON{"calories_log_weigh_in": logged, "calories_list_weigh_ins": listed} {
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %+v (%s), want %+v (%s)", name, got.WeighIns, showCard(weightCardJSON{got.Latest, got.BMI, got.Change}),
				want.WeighIns, showCard(weightCardJSON{want.Latest, want.BMI, want.Change}))
		}
	}
}

// showCard writes card with its pointers followed, null for nil.
func showCard(card weightCardJSON) string {
	latest := "null"
	if card.Latest != nil {
		latest = card.Latest.Week + " " + fmtFloat(&card.Latest.Kg)
	}
	return "{latest " + latest + ", bmi " + fmtFloat(card.BMI) + ", change " + fmtFloat(card.Change) + "}"
}

func fmtFloat(f *float64) string {
	if f == nil {
		return "null"
	}
	return strconv.FormatFloat(*f, 'f', -1, 64)
}
