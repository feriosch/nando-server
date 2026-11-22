package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/travel/model"
)

// PriceVerdictsService fetches Google's Price Verdict on a Trip's dates from
// SerpApi's Google Flights API, itself, with the key kept on the server
// (ADR 0013), within the monthly limit in the settings.
type PriceVerdictsService struct {
	verdicts model.PriceVerdictsRepository
	calls    model.SerpAPICallsRepository
	trips    model.TripsRepository
	settings *SettingsService
	core     coremodel.CoreService
	// serpAPIURL is SerpApi, whose search is at /search.json, and serpAPIKey
	// its key: empty turns Price Verdicts off.
	serpAPIURL, serpAPIKey string
	client                 *http.Client
	// fetching lets one new verdict at a time check the limit, call SerpApi
	// and be kept, so two asked at once neither pass the limit together nor
	// both call for the same dates.
	fetching sync.Mutex
}

// NewPriceVerdicts returns the PriceVerdictsService.
func NewPriceVerdicts(verdicts model.PriceVerdictsRepository, calls model.SerpAPICallsRepository, trips model.TripsRepository,
	settings *SettingsService, core coremodel.CoreService, serpAPIURL, serpAPIKey string) *PriceVerdictsService {
	return &PriceVerdictsService{
		verdicts: verdicts, calls: calls, trips: trips, settings: settings, core: core,
		serpAPIURL: strings.TrimSuffix(serpAPIURL, "/"), serpAPIKey: serpAPIKey,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Tools are what the PriceVerdictsService offers Connectors.
func (s *PriceVerdictsService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "travel_get_price_verdict",
		Description: "Google's Price Verdict on tripId's Party flying out on departDate and back on returnDate, from " +
			"Google Flights: level (low, typical or high), lowestPrice, typicalRange, history ([Unix seconds, price]) " +
			"and the cheapest offer, all in JPY for the whole Party. Asking again the same Day answers the one already " +
			"fetched. Each new one uses one of the month's SerpApi searches, so ask only for the best new Fares.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"tripId":{"type":"string"},` +
			`"party":{"type":"string","enum":["owner","with_companion"]},` +
			`"departDate":{"type":"string","description":"YYYY-MM-DD."},"returnDate":{"type":"string","description":"YYYY-MM-DD."}},` +
			`"required":["tripId","party","departDate","returnDate"]}`),
		Handle: s.getVerdict,
	}, {
		Name:        "travel_list_price_verdicts",
		Description: "The latest Price Verdicts fetched for tripId's Party, newest first, at most 20, as {verdicts: [...]}.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"tripId":{"type":"string"},` +
			`"party":{"type":"string","enum":["owner","with_companion"]}},"required":["tripId","party"]}`),
		ReadOnly: true,
		Handle:   s.listVerdicts,
	}}
}

// listedVerdicts is how many verdicts travel_list_price_verdicts answers.
const listedVerdicts = 20

func (s *PriceVerdictsService) listVerdicts(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		TripID string `json:"tripId"`
		Party  string `json:"party"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, errors.New("give tripId and party")
	}
	verdicts, err := s.verdicts.List(ctx, args.TripID, model.PartyKey(args.Party), listedVerdicts)
	if err != nil {
		return nil, err
	}
	list := make([]model.PriceVerdictJSON, len(verdicts))
	for i, v := range verdicts {
		list[i] = toPriceVerdictJSON(v)
	}
	return map[string][]model.PriceVerdictJSON{"verdicts": list}, nil
}

// getVerdict answers today's verdict on a Trip's Party's dates, fetching it
// if there is none yet.
func (s *PriceVerdictsService) getVerdict(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		TripID     string `json:"tripId"`
		Party      string `json:"party"`
		DepartDate string `json:"departDate"`
		ReturnDate string `json:"returnDate"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, errors.New("give tripId, party, departDate and returnDate")
	}
	trip, err := s.trips.Find(ctx, args.TripID)
	if errors.Is(err, coreutils.ErrNotFound) {
		return nil, errors.New("there is no Trip " + args.TripID)
	}
	if err != nil {
		return nil, err
	}
	if trip.Status != model.Watching {
		return nil, errors.New("the Trip is " + string(trip.Status) + ": only a Watching Trip's Price Verdicts are fetched")
	}
	party := model.PartyKey(args.Party)
	i := slices.IndexFunc(trip.Parties, func(p model.Party) bool { return p.Key == party })
	if i < 0 {
		return nil, fmt.Errorf("the Trip has no Party %q", args.Party)
	}
	depart, back := coremodel.Day(args.DepartDate), coremodel.Day(args.ReturnDate)
	if err := checkRange("departDate to returnDate", depart, back); err != nil {
		return nil, err
	}
	today, err := s.core.Today(ctx)
	if err != nil {
		return nil, err
	}
	if depart < today {
		return nil, fmt.Errorf("departDate %s is before today, %s", depart, today)
	}
	query := searchQuery(trip, trip.Parties[i], depart, back)
	search := query.Encode()
	if verdict, err := s.verdicts.FindOn(ctx, trip.ID, party, depart, back, today, search); err == nil {
		return toPriceVerdictJSON(verdict), nil
	} else if !errors.Is(err, coreutils.ErrNotFound) {
		return nil, err
	}
	if s.serpAPIKey == "" {
		return nil, errors.New("Price Verdicts are off: Nando has no SerpApi key")
	}
	s.fetching.Lock()
	defer s.fetching.Unlock()
	// Fetched while this one waited?
	if verdict, err := s.verdicts.FindOn(ctx, trip.ID, party, depart, back, today, search); err == nil {
		return toPriceVerdictJSON(verdict), nil
	} else if !errors.Is(err, coreutils.ErrNotFound) {
		return nil, err
	}
	if err := s.withinLimit(ctx); err != nil {
		return nil, err
	}
	verdict, err := s.fetch(ctx, trip, trip.Parties[i], depart, back, query)
	if err != nil {
		return nil, err
	}
	verdict.Day, verdict.Search = today, search
	if err := s.verdicts.Insert(ctx, verdict); err != nil {
		return nil, err
	}
	return toPriceVerdictJSON(verdict), nil
}

// withinLimit refuses when this calendar month, in the Owner Time Zone, has
// used the settings' SerpApi searches.
func (s *PriceVerdictsService) withinLimit(ctx context.Context) error {
	settings, err := s.settings.savedOrDefault(ctx)
	if err != nil {
		return err
	}
	loc, err := s.core.OwnerTimeZone(ctx)
	if err != nil {
		return err
	}
	now := s.core.Now().In(loc)
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	used, err := s.calls.CountSince(ctx, month)
	if err != nil {
		return err
	}
	if used >= settings.SerpAPIMonthlyLimit {
		return fmt.Errorf("this month's %d SerpApi searches are used (%d): Price Verdicts come back on %s",
			settings.SerpAPIMonthlyLimit, used, month.AddDate(0, 1, 0).Format(time.DateOnly))
	}
	return nil
}

// travelClasses are SerpApi's travel_class for each cabin.
var travelClasses = map[model.Cabin]string{model.Economy: "1", model.PremiumEconomy: "2", model.Business: "3", model.First: "4"}

// googleFlights is the part of SerpApi's Google Flights answer Nando reads.
type googleFlights struct {
	Error        string        `json:"error"`
	BestFlights  []flightOffer `json:"best_flights"`
	OtherFlights []flightOffer `json:"other_flights"`
	Insights     *struct {
		LowestPrice  int        `json:"lowest_price"`
		PriceLevel   string     `json:"price_level"`
		TypicalRange []int      `json:"typical_price_range"`
		History      [][2]int64 `json:"price_history"`
	} `json:"price_insights"`
}

// flightOffer is one offer in a Google Flights answer.
type flightOffer struct {
	Price   int `json:"price"`
	Flights []struct {
		Airline string `json:"airline"`
	} `json:"flights"`
}

// maxHistory is how many points of Google's price history a verdict keeps.
const maxHistory = 60

// searchQuery is what SerpApi is asked for a Trip's Party's dates, without
// the key.
func searchQuery(trip model.Trip, party model.Party, depart, back coremodel.Day) url.Values {
	return url.Values{
		"engine":        {"google_flights"},
		"type":          {"1"},
		"departure_id":  {strings.Join(trip.Origins, ",")},
		"arrival_id":    {strings.Join(trip.Destinations, ",")},
		"outbound_date": {string(depart)},
		"return_date":   {string(back)},
		"adults":        {strconv.Itoa(party.Adults)},
		"travel_class":  {travelClasses[trip.Cabin]},
		"currency":      {"JPY"},
		"hl":            {"en"},
	}
}

// fetch asks SerpApi for the verdict with query, adding the key, counting the
// call whatever comes of it. Its errors never carry the request, whose URL
// holds the key.
func (s *PriceVerdictsService) fetch(ctx context.Context, trip model.Trip, party model.Party, depart, back coremodel.Day, query url.Values) (model.PriceVerdict, error) {
	query = maps.Clone(query)
	query.Set("api_key", s.serpAPIKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.serpAPIURL+"/search.json?"+query.Encode(), nil)
	if err != nil {
		return model.PriceVerdict{}, errors.New("couldn't ask SerpApi")
	}
	if err := s.calls.Insert(ctx, s.core.Now().UTC()); err != nil {
		return model.PriceVerdict{}, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return model.PriceVerdict{}, errors.New("SerpApi didn't answer; try again later")
	}
	defer resp.Body.Close()
	var answer googleFlights
	decodeErr := json.NewDecoder(resp.Body).Decode(&answer)
	switch {
	case answer.Error != "":
		return model.PriceVerdict{}, errors.New("SerpApi refused: " + answer.Error)
	case resp.StatusCode != http.StatusOK:
		return model.PriceVerdict{}, fmt.Errorf("SerpApi answered %d", resp.StatusCode)
	case decodeErr != nil:
		return model.PriceVerdict{}, errors.New("SerpApi's answer couldn't be read")
	case answer.Insights == nil || answer.Insights.PriceLevel == "":
		return model.PriceVerdict{}, errors.New("Google has no Price Verdict for these dates")
	}
	history := answer.Insights.History
	if len(history) > maxHistory {
		history = history[len(history)-maxHistory:]
	}
	verdict := model.PriceVerdict{
		TripID: trip.ID, Party: party.Key, DepartDate: depart, ReturnDate: back, FetchedAt: s.core.Now().UTC(),
		Level: answer.Insights.PriceLevel, LowestPrice: answer.Insights.LowestPrice,
		TypicalRange: answer.Insights.TypicalRange, History: history, CheapestAirlines: []string{},
	}
	if verdict.TypicalRange == nil {
		verdict.TypicalRange = []int{}
	}
	if verdict.History == nil {
		verdict.History = [][2]int64{}
	}
	for _, offer := range slices.Concat(answer.BestFlights, answer.OtherFlights) {
		if offer.Price > 0 && (verdict.CheapestPrice == 0 || offer.Price < verdict.CheapestPrice) {
			verdict.CheapestPrice = offer.Price
			verdict.CheapestAirlines = []string{}
			for _, f := range offer.Flights {
				if !slices.Contains(verdict.CheapestAirlines, f.Airline) {
					verdict.CheapestAirlines = append(verdict.CheapestAirlines, f.Airline)
				}
			}
		}
	}
	return verdict, nil
}

func toPriceVerdictJSON(v model.PriceVerdict) model.PriceVerdictJSON {
	return model.PriceVerdictJSON{
		TripID: v.TripID, Party: string(v.Party), DepartDate: string(v.DepartDate), ReturnDate: string(v.ReturnDate),
		Level: v.Level, LowestPrice: v.LowestPrice, TypicalRange: v.TypicalRange, History: v.History,
		CheapestPrice: v.CheapestPrice, CheapestAirlines: v.CheapestAirlines,
		FetchedAt: v.FetchedAt.UTC().Format(time.RFC3339),
	}
}
