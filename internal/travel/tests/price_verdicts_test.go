package tests

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/feriosch/nando-server/internal/travel"
)

// priceVerdictJSON is Google's verdict on a Trip's Party's dates, as the
// Tools answer it.
type priceVerdictJSON struct {
	TripID           string     `json:"tripId"`
	Party            string     `json:"party"`
	DepartDate       string     `json:"departDate"`
	ReturnDate       string     `json:"returnDate"`
	Level            string     `json:"level"`
	LowestPrice      int        `json:"lowestPrice"`
	TypicalRange     []int      `json:"typicalRange"`
	History          [][2]int64 `json:"history"`
	CheapestPrice    int        `json:"cheapestPrice"`
	CheapestAirlines []string   `json:"cheapestAirlines"`
	FetchedAt        string     `json:"fetchedAt"`
}

// verdict asks for the Price Verdict on trip's Party party, leaving on depart
// and back on back.
func verdictArgs(trip tripJSON, party, depart, back string) map[string]any {
	return map[string]any{"tripId": trip.ID, "party": party, "departDate": depart, "returnDate": back}
}

func TestAPriceVerdictIsGooglesViewOfTheTripsDatesAndIsKept(t *testing.T) {
	n, phone := signedInBrowser(t)
	trip := newTrip(t, phone, map[string]any{"origins": []string{"AAA", "AAC"}, "cabin": "premium_economy"})
	session := phone.tools()

	got := callTool[priceVerdictJSON](t, session, "travel_get_price_verdict", verdictArgs(trip, "owner", "2027-05-12", "2027-05-19"))

	want := priceVerdictJSON{
		TripID: trip.ID, Party: "owner", DepartDate: "2027-05-12", ReturnDate: "2027-05-19",
		Level: "low", LowestPrice: 221000, TypicalRange: []int{240000, 300000},
		History:       [][2]int64{{1795000000, 250000}, {1795086400, 240000}, {1795172800, 221000}},
		CheapestPrice: 221000, CheapestAirlines: []string{"Air Z"},
		FetchedAt: "2026-11-22T12:00:00Z",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("travel_get_price_verdict = %+v, want %+v", got, want)
	}
	calls := n.serpAPI.calls()
	if len(calls) != 1 {
		t.Fatalf("SerpApi was called %d times, want once", len(calls))
	}
	for param, want := range map[string]string{
		"engine": "google_flights", "api_key": serpAPIKey, "type": "1",
		"departure_id": "AAA,AAC", "arrival_id": "BBB",
		"outbound_date": "2027-05-12", "return_date": "2027-05-19",
		"adults": "1", "travel_class": "2", "currency": "JPY",
	} {
		if got := calls[0].Get(param); got != want {
			t.Errorf("SerpApi's %s = %q, want %q", param, got, want)
		}
	}
	list := callTool[struct {
		Verdicts []priceVerdictJSON `json:"verdicts"`
	}](t, session, "travel_list_price_verdicts", map[string]any{"tripId": trip.ID, "party": "owner"})
	if len(list.Verdicts) != 1 || !reflect.DeepEqual(list.Verdicts[0], want) {
		t.Errorf("travel_list_price_verdicts = %+v, want the one verdict", list.Verdicts)
	}
}

func TestAskingAgainTheSameDayAnswersTheVerdictAlreadyFetched(t *testing.T) {
	n, phone := signedInBrowser(t)
	trip := newTrip(t, phone, nil)
	args := verdictArgs(trip, "owner", "2027-05-12", "2027-05-19")

	first := callTool[priceVerdictJSON](t, phone.tools(), "travel_get_price_verdict", args)
	n.clock.Set(today.Add(6 * time.Hour))
	again := callTool[priceVerdictJSON](t, phone.tools(), "travel_get_price_verdict", args)

	if !reflect.DeepEqual(again, first) || len(n.serpAPI.calls()) != 1 {
		t.Errorf("the same Day: %+v after %d calls; want the first verdict, from one call", again, len(n.serpAPI.calls()))
	}
	// Other dates are another verdict.
	callTool[priceVerdictJSON](t, phone.tools(), "travel_get_price_verdict", verdictArgs(trip, "owner", "2027-05-13", "2027-05-20"))
	// The next Day asks again.
	n.clock.Set(today.AddDate(0, 0, 1))
	callTool[priceVerdictJSON](t, phone.tools(), "travel_get_price_verdict", args)
	if got := len(n.serpAPI.calls()); got != 3 {
		t.Errorf("SerpApi calls = %d, want 3: other dates, then the next Day", got)
	}
}

func TestPriceVerdictsStopAtTheMonthlyLimitUntilTheMonthTurns(t *testing.T) {
	n, phone := signedInBrowser(t)
	trip := newTrip(t, phone, nil)
	callTool[settingsJSON](t, phone.tools(), "travel_save_settings", map[string]any{"serpApiMonthlyLimit": 2})

	callTool[priceVerdictJSON](t, phone.tools(), "travel_get_price_verdict", verdictArgs(trip, "owner", "2027-05-12", "2027-05-19"))
	callTool[priceVerdictJSON](t, phone.tools(), "travel_get_price_verdict", verdictArgs(trip, "owner", "2027-05-13", "2027-05-20"))
	refusal := toolError(t, phone.tools(), "travel_get_price_verdict", verdictArgs(trip, "owner", "2027-05-14", "2027-05-21"))
	if !strings.Contains(refusal, "2026-12-01") {
		t.Errorf("the refusal %q doesn't say the searches come back on 2026-12-01", refusal)
	}
	// A verdict already fetched today still answers.
	callTool[priceVerdictJSON](t, phone.tools(), "travel_get_price_verdict", verdictArgs(trip, "owner", "2027-05-12", "2027-05-19"))
	if got := len(n.serpAPI.calls()); got != 2 {
		t.Errorf("SerpApi calls = %d, want 2, the limit", got)
	}

	n.clock.Set(time.Date(2026, 12, 1, 0, 30, 0, 0, time.UTC))
	callTool[priceVerdictJSON](t, phone.tools(), "travel_get_price_verdict", verdictArgs(trip, "owner", "2027-05-14", "2027-05-21"))
}

func TestWithoutAKeyPriceVerdictsAreRefusedAndSearchesGoOn(t *testing.T) {
	n, phone := signedInBrowserWith(t, func(c *travel.Config) { c.SerpAPIKey = "" })
	trip := newTrip(t, phone, nil)

	refusal := toolError(t, phone.tools(), "travel_get_price_verdict", verdictArgs(trip, "owner", "2027-05-12", "2027-05-19"))

	if !strings.Contains(refusal, "off") || len(n.serpAPI.calls()) != 0 {
		t.Errorf("without a key: %q after %d calls; want Price Verdicts off and no call", refusal, len(n.serpAPI.calls()))
	}
	if got, _ := record(t, phone, "daily", fare(trip, "owner", "2027-05-12", "2027-05-19", 112000, nil)); len(got.Recorded) != 1 {
		t.Errorf("recording a Fare without a SerpApi key = %+v; want it recorded", got)
	}
}

func TestAFailedSerpAPICallIsCountedKeepsNothingAndNeverShowsTheKey(t *testing.T) {
	n, phone := signedInBrowser(t)
	trip := newTrip(t, phone, nil)
	callTool[settingsJSON](t, phone.tools(), "travel_save_settings", map[string]any{"serpApiMonthlyLimit": 3})
	args := verdictArgs(trip, "owner", "2027-05-12", "2027-05-19")

	for _, answer := range []struct {
		status int
		body   string
	}{
		{http.StatusUnauthorized, `{"error": "Invalid API key."}`},
		{http.StatusOK, `{"search_metadata": {"status": "Success"}, "best_flights": []}`}, // no price_insights
		{http.StatusInternalServerError, `oops`},
	} {
		n.serpAPI.answer(answer.status, answer.body)
		if refusal := toolError(t, phone.tools(), "travel_get_price_verdict", args); strings.Contains(refusal, serpAPIKey) {
			t.Errorf("the refusal %q shows the key", refusal)
		}
	}
	list := callTool[struct {
		Verdicts []priceVerdictJSON `json:"verdicts"`
	}](t, phone.tools(), "travel_list_price_verdicts", map[string]any{"tripId": trip.ID, "party": "owner"})
	if len(list.Verdicts) != 0 {
		t.Errorf("verdicts after only failures = %+v, want none", list.Verdicts)
	}
	// All three calls count, so the limit of 3 is used up.
	n.serpAPI.answer(http.StatusOK, googleFlightsAnswer)
	toolError(t, phone.tools(), "travel_get_price_verdict", args)
}

func TestAPriceVerdictIsOnlyForATripsOwnPartyAndRealDates(t *testing.T) {
	n, phone := signedInBrowser(t)
	trip := newTrip(t, phone, nil)

	for _, args := range []map[string]any{
		verdictArgs(trip, "with_companion", "2027-05-12", "2027-05-19"),
		verdictArgs(tripJSON{ID: "nosuchtrip"}, "owner", "2027-05-12", "2027-05-19"),
		verdictArgs(trip, "owner", "2027-05-19", "2027-05-12"),
		verdictArgs(trip, "owner", "next week", "2027-05-19"),
	} {
		toolError(t, phone.tools(), "travel_get_price_verdict", args)
	}
	if got := len(n.serpAPI.calls()); got != 0 {
		t.Errorf("SerpApi calls = %d, want none", got)
	}
}
