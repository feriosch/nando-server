package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/travel/model"
)

// FaresService handles Fare Checks and the Fares they find. Only the Travel
// Assistant records and reads them, through its Tools.
type FaresService struct {
	checks model.FareChecksRepository
	fares  model.FaresRepository
	alerts model.FareAlertsRepository
	trips  model.TripsRepository
	core   coremodel.CoreService
}

// NewFares returns the FaresService. It reads Trips, to check each Fare
// against its Trip.
func NewFares(checks model.FareChecksRepository, fares model.FaresRepository, alerts model.FareAlertsRepository,
	trips model.TripsRepository, core coremodel.CoreService) *FaresService {
	return &FaresService{checks: checks, fares: fares, alerts: alerts, trips: trips, core: core}
}

// Tools are what the FaresService offers Connectors: running a Fare Check,
// and reading what the Fares say.
func (s *FaresService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "travel_start_fare_check",
		Description: "Starts a Fare Check of kind daily, weekly or asked (one the Owner asked for in the chat). " +
			"Answers its id, for travel_record_fares and travel_finish_fare_check, and lastFinished: when the last " +
			"finished check of each kind finished, or null.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string","enum":["daily","weekly","asked"]}},"required":["kind"]}`),
		Handle:      s.startCheck,
	}, {
		Name: "travel_record_fares",
		Description: "Records Fares found in the Fare Check checkId, each for a Watching Trip and one of its " +
			"parties, round trip, priced in JPY in total for the Party with total. Each Fare is kept or refused on " +
			"its own: answers {recorded: [{index, id}], refused: [{index, reason}]}, by place in the list sent.",
		InputSchema: json.RawMessage(recordFaresSchema),
		Handle:      s.recordFares,
	}, {
		Name: "travel_finish_fare_check",
		Description: "Finishes the Fare Check id, with the sources it searched, a short summary and any errors. " +
			"A check that isn't finished never counts as the last one. Answers the check.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"},` +
			`"sources":{"type":"array","items":{"type":"string"}},"summary":{"type":"string"},` +
			`"errors":{"type":"array","items":{"type":"string"}}},"required":["id"]}`),
		Handle: s.finishCheck,
	}, {
		Name: "travel_record_fare_alert",
		Description: "Records that the Owner was just told about fareId, why (reasons: lowest_seen, low_verdict, " +
			"big_drop, below_target, ending_soon) and whether it was urgent, sent at once. Call it after telling them, " +
			"so a later run doesn't repeat it. Answers the alert.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"fareId":{"type":"string"},` +
			`"reasons":{"type":"array","items":{"type":"string","enum":["lowest_seen","low_verdict","big_drop","below_target","ending_soon"]}},` +
			`"urgent":{"type":"boolean"}},"required":["fareId","reasons"]}`),
		Handle: s.recordAlert,
	}, {
		Name:        "travel_list_fare_alerts",
		Description: "The Fare Alerts sent about tripId's Party since since (RFC 3339; 30 days ago when left out), newest first, as {alerts: [...]}.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"tripId":{"type":"string"},` +
			`"party":{"type":"string","enum":["owner","with_companion"]},"since":{"type":"string"}},"required":["tripId","party"]}`),
		ReadOnly: true,
		Handle:   s.listAlerts,
	}, {
		Name:        "travel_fare_stats",
		Description: "What the Fares of tripId's Party say: lowestEver; the bestThisWeek and bestLastWeek found (weeks Monday to Sunday); byWeek, the cheapest per departure week; top, the 3 cheapest found in the last 7 days; and targetFare, the Party's Target Fare per person, or null.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"tripId":{"type":"string"},"party":{"type":"string","enum":["owner","with_companion"]}},"required":["tripId","party"]}`),
		ReadOnly:    true,
		Handle:      s.fareStats,
	}}
}

// recordFaresSchema is the JSON Schema of travel_record_fares' arguments.
const recordFaresSchema = `{"type":"object","properties":{"checkId":{"type":"string"},"fares":{"type":"array","items":{"type":"object","properties":{` +
	`"tripId":{"type":"string"},` +
	`"party":{"type":"string","enum":["owner","with_companion"]},` +
	`"source":{"type":"string","enum":["kiwi","expedia","skiplagged","slicktrip","price_alert","other"]},` +
	`"departDate":{"type":"string","description":"YYYY-MM-DD."},` +
	`"returnDate":{"type":"string","description":"YYYY-MM-DD."},` +
	`"outbound":` + legSchema + `,"inbound":` + legSchema + `,` +
	`"separateTickets":{"type":"boolean"},` +
	`"cabin":{"type":"string","enum":["economy","premium_economy","business","first"]},` +
	`"bagsIncluded":{"type":"boolean","description":"Whether total includes the Trip's checked bags."},` +
	`"total":{"type":"integer","description":"JPY, for the whole Party."},` +
	`"bookingUrl":{"type":"string"},` +
	`"leaveNeeded":{"type":"object","properties":{"days":{"type":"integer"},"withRemoteDays":{"type":"integer"}},` +
	`"description":"Only for with_companion: the Companion's days off, and with the remote days allowed."}}}}},` +
	`"required":["checkId","fares"]}`

// legSchema is the JSON Schema of one way of a Fare.
const legSchema = `{"type":"object","properties":{"flights":{"type":"array","items":{"type":"object","properties":{` +
	`"carrier":{"type":"string"},"number":{"type":"string"},"from":{"type":"string"},"to":{"type":"string"},` +
	`"departs":{"type":"string","description":"Local time, YYYY-MM-DDTHH:MM."},"arrives":{"type":"string","description":"Local time, YYYY-MM-DDTHH:MM."}}}},` +
	`"stops":{"type":"integer"},"durationMinutes":{"type":"integer"}}}`

// startCheck starts a Fare Check.
func (s *FaresService) startCheck(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil || !slices.Contains(model.FareCheckKinds, model.FareCheckKind(args.Kind)) {
		return nil, errors.New("kind must be daily, weekly or asked")
	}
	last := map[string]*string{}
	for _, kind := range model.FareCheckKinds {
		at, err := s.checks.LastFinished(ctx, kind)
		if err != nil {
			return nil, err
		}
		last[string(kind)] = nil
		if !at.IsZero() {
			text := at.UTC().Format(time.RFC3339)
			last[string(kind)] = &text
		}
	}
	id, err := s.checks.Insert(ctx, model.FareCheck{
		Kind: model.FareCheckKind(args.Kind), StartedAt: s.core.Now().UTC(), Sources: []string{}, Errors: []string{},
	})
	if err != nil {
		return nil, err
	}
	return model.FareCheckStartJSON{ID: id, LastFinished: last}, nil
}

// finishCheck finishes a Fare Check.
func (s *FaresService) finishCheck(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		ID      string   `json:"id"`
		Sources []string `json:"sources"`
		Summary string   `json:"summary"`
		Errors  []string `json:"errors"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, errors.New("give id, and sources, summary and errors if any")
	}
	if args.Sources == nil {
		args.Sources = []string{}
	}
	if args.Errors == nil {
		args.Errors = []string{}
	}
	err := s.checks.Finish(ctx, args.ID, s.core.Now().UTC(), args.Sources, args.Summary, args.Errors)
	if errors.Is(err, coreutils.ErrNotFound) {
		return nil, errors.New("there is no unfinished Fare Check " + args.ID)
	}
	if err != nil {
		return nil, err
	}
	check, err := s.checks.Find(ctx, args.ID)
	return toFareCheckJSON(check), err
}

func toFareCheckJSON(c model.FareCheck) model.FareCheckJSON {
	j := model.FareCheckJSON{
		ID: c.ID, Kind: string(c.Kind), StartedAt: c.StartedAt.UTC().Format(time.RFC3339),
		Sources: c.Sources, Summary: c.Summary, Errors: c.Errors,
	}
	if c.FinishedAt != nil {
		at := c.FinishedAt.UTC().Format(time.RFC3339)
		j.FinishedAt = &at
	}
	return j
}

// recordFares records each Fare that fits its Trip, and says why it refused
// the others.
func (s *FaresService) recordFares(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		CheckID string           `json:"checkId"`
		Fares   []model.FareJSON `json:"fares"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, errors.New("travel_record_fares' arguments don't fit its schema: " + err.Error())
	}
	check, err := s.checks.Find(ctx, args.CheckID)
	if errors.Is(err, coreutils.ErrNotFound) {
		return nil, errors.New("there is no Fare Check " + args.CheckID + ": start one with travel_start_fare_check")
	}
	if err != nil {
		return nil, err
	}
	if check.FinishedAt != nil {
		return nil, errors.New("the Fare Check " + args.CheckID + " is finished: start another with travel_start_fare_check")
	}
	answer := model.RecordedFaresJSON{Recorded: []model.RecordedFareJSON{}, Refused: []model.RefusedFareJSON{}}
	for i, j := range args.Fares {
		trip, err := s.trips.Find(ctx, j.TripID)
		if err != nil && !errors.Is(err, coreutils.ErrNotFound) {
			return nil, err
		}
		fare, reason := fareFrom(j, trip, err == nil)
		if reason != "" {
			answer.Refused = append(answer.Refused, model.RefusedFareJSON{Index: i, Reason: reason})
			continue
		}
		fare.CheckID, fare.FoundAt = check.ID, s.core.Now().UTC()
		id, err := s.fares.Insert(ctx, fare)
		if err != nil {
			return nil, err
		}
		answer.Recorded = append(answer.Recorded, model.RecordedFareJSON{Index: i, ID: id})
	}
	return answer, nil
}

// fareFrom is the Fare j describes for trip, or why it can't be recorded.
// found is whether j's Trip exists.
func fareFrom(j model.FareJSON, trip model.Trip, found bool) (model.Fare, string) {
	if !found {
		return model.Fare{}, "there is no Trip " + j.TripID
	}
	if trip.Status != model.Watching {
		return model.Fare{}, "the Trip is " + string(trip.Status) + ": only a Watching Trip's Fares are recorded"
	}
	party := model.PartyKey(j.Party)
	i := slices.IndexFunc(trip.Parties, func(p model.Party) bool { return p.Key == party })
	if i < 0 {
		return model.Fare{}, fmt.Sprintf("the Trip has no Party %q", j.Party)
	}
	if !slices.Contains(model.FareSources, model.FareSource(j.Source)) {
		return model.Fare{}, "source must be kiwi, expedia, skiplagged, slicktrip, price_alert or other"
	}
	if err := checkRange("departDate to returnDate", coremodel.Day(j.DepartDate), coremodel.Day(j.ReturnDate)); err != nil {
		return model.Fare{}, err.Error()
	}
	depart, back := coremodel.Day(j.DepartDate), coremodel.Day(j.ReturnDate)
	if !slices.Contains(cabins, model.Cabin(j.Cabin)) {
		return model.Fare{}, "cabin must be economy, premium_economy, business or first"
	}
	if j.Total <= 0 {
		return model.Fare{}, "total must be the Fare's price in JPY for the whole Party, more than 0"
	}
	if j.BookingURL != "" && !strings.HasPrefix(j.BookingURL, "https://") {
		return model.Fare{}, "bookingUrl must be an https:// link"
	}
	if j.LeaveNeeded != nil && party != model.WithCompanion {
		return model.Fare{}, "leaveNeeded is only for a Fare for the Party with the Companion"
	}
	if j.LeaveNeeded == nil && party == model.WithCompanion {
		return model.Fare{}, "a Fare for the Party with the Companion needs leaveNeeded, worked out from their calendars"
	}
	if l := j.LeaveNeeded; l != nil && (l.Days < 0 || l.WithRemoteDays < 0 || l.WithRemoteDays > l.Days) {
		return model.Fare{}, "leaveNeeded must be {days, withRemoteDays} with 0 ≤ withRemoteDays ≤ days"
	}
	adults := trip.Parties[i].Adults
	fare := model.Fare{
		TripID: trip.ID, Party: party, Source: model.FareSource(j.Source),
		DepartDate: depart, ReturnDate: back, Nights: nightsBetween(depart, back),
		Outbound: legFrom(j.Outbound), Inbound: legFrom(j.Inbound),
		SeparateTickets: j.SeparateTickets, Cabin: model.Cabin(j.Cabin), BagsIncluded: j.BagsIncluded,
		Currency: "JPY", Total: j.Total, PerPerson: (j.Total + adults/2) / adults,
		BookingURL: j.BookingURL,
	}
	if j.LeaveNeeded != nil {
		fare.LeaveNeeded = &model.LeaveNeeded{Days: j.LeaveNeeded.Days, WithRemoteDays: j.LeaveNeeded.WithRemoteDays}
	}
	return fare, fitsTrip(fare, trip)
}

// fitsTrip says why fare doesn't fit trip as it is now, or "" when it does:
// one of its Parties, a departure in one of its windows, nights in its range,
// clear of its avoid ranges, flying from and to its airports, its stops and
// cabin or better, and Separate Tickets only when welcome. Recording refuses
// a Fare that doesn't fit, and the stats leave out one that no longer does,
// after the Trip changed.
func fitsTrip(fare model.Fare, trip model.Trip) string {
	if !slices.ContainsFunc(trip.Parties, func(p model.Party) bool { return p.Key == fare.Party }) {
		return fmt.Sprintf("the Trip has no Party %q", fare.Party)
	}
	if fare.Nights < trip.Nights.Min || fare.Nights > trip.Nights.Max {
		return fmt.Sprintf("%d nights is outside the Trip's %d to %d", fare.Nights, trip.Nights.Min, trip.Nights.Max)
	}
	if !slices.ContainsFunc(trip.Windows, func(w model.Window) bool { return w.From <= fare.DepartDate && fare.DepartDate <= w.To }) {
		return "it departs on " + string(fare.DepartDate) + ", outside every window of the Trip"
	}
	for _, a := range trip.Avoid {
		if a.From <= fare.ReturnDate && fare.DepartDate <= a.To {
			return fmt.Sprintf("it touches %s to %s, which the Trip avoids", a.From, a.To)
		}
	}
	if reason := flies(fare.Outbound, trip.Origins, trip.Destinations, "out"); reason != "" {
		return reason
	}
	if reason := flies(fare.Inbound, trip.Destinations, trip.Origins, "back"); reason != "" {
		return reason
	}
	if fare.SeparateTickets && !trip.SeparateTickets {
		return "the Trip doesn't welcome Fares on Separate Tickets"
	}
	if fare.Outbound.Stops > trip.MaxStops || fare.Inbound.Stops > trip.MaxStops {
		return fmt.Sprintf("it makes more stops each way than the Trip's %d", trip.MaxStops)
	}
	if slices.Index(cabins, fare.Cabin) < slices.Index(cabins, trip.Cabin) {
		return "it is in " + string(fare.Cabin) + ", below the Trip's " + string(trip.Cabin)
	}
	return ""
}

// flies says why leg doesn't fly from one of from to one of to, or "" when it
// does, or has no flights or airport codes to tell by (some sources give
// none).
func flies(leg model.Leg, from, to []string, way string) string {
	if len(leg.Flights) == 0 {
		return ""
	}
	first, last := leg.Flights[0].From, leg.Flights[len(leg.Flights)-1].To
	if first == "" || last == "" {
		return ""
	}
	if !slices.Contains(from, first) || !slices.Contains(to, last) {
		return fmt.Sprintf("it flies %s from %s to %s, not between the Trip's %s and %s",
			way, first, last, strings.Join(from, "/"), strings.Join(to, "/"))
	}
	return ""
}

// cabins are the cabins, cheapest first.
var cabins = []model.Cabin{model.Economy, model.PremiumEconomy, model.Business, model.First}

// nightsBetween is how many nights there are from depart to back.
func nightsBetween(depart, back coremodel.Day) int {
	from, _ := time.Parse(time.DateOnly, string(depart))
	to, _ := time.Parse(time.DateOnly, string(back))
	return int(to.Sub(from).Hours() / 24)
}

func legFrom(j model.LegJSON) model.Leg {
	flights := make([]model.Flight, len(j.Flights))
	for i, f := range j.Flights {
		flights[i] = model.Flight(f)
	}
	return model.Leg{Flights: flights, Stops: j.Stops, DurationMinutes: j.DurationMinutes}
}

// fareStats answers what a Trip's Party's Fares say.
func (s *FaresService) fareStats(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		TripID string `json:"tripId"`
		Party  string `json:"party"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, errors.New("give tripId and party")
	}
	trip, err := s.trips.Find(ctx, args.TripID)
	if errors.Is(err, coreutils.ErrNotFound) {
		return nil, errors.New("there is no Trip " + args.TripID)
	}
	if err != nil {
		return nil, err
	}
	party := model.PartyKey(args.Party)
	if !slices.ContainsFunc(trip.Parties, func(p model.Party) bool { return p.Key == party }) {
		return nil, fmt.Errorf("the Trip has no Party %q", args.Party)
	}
	stats, err := s.stats(ctx, trip, party)
	if i := slices.IndexFunc(trip.TargetFares, func(t model.TargetFare) bool { return t.Party == party }); i >= 0 {
		stats.TargetFare = &trip.TargetFares[i].PerPerson
	}
	return stats, err
}

// recordAlert records a Fare Alert the Travel Assistant sent.
func (s *FaresService) recordAlert(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		FareID  string   `json:"fareId"`
		Reasons []string `json:"reasons"`
		Urgent  bool     `json:"urgent"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, errors.New("give fareId, reasons and urgent")
	}
	if len(args.Reasons) == 0 {
		return nil, errors.New("reasons must name why: lowest_seen, low_verdict, big_drop, below_target or ending_soon")
	}
	for i, r := range args.Reasons {
		if !slices.Contains(model.FareAlertReasons, r) {
			return nil, fmt.Errorf("reasons: %q must be lowest_seen, low_verdict, big_drop, below_target or ending_soon", r)
		}
		if slices.Contains(args.Reasons[:i], r) {
			return nil, fmt.Errorf("reasons has %s twice", r)
		}
	}
	fare, err := s.fares.Find(ctx, args.FareID)
	if errors.Is(err, coreutils.ErrNotFound) {
		return nil, errors.New("there is no Fare " + args.FareID)
	}
	if err != nil {
		return nil, err
	}
	// A Fare the Trip no longer fits, such as one on dates since avoided, is
	// not worth telling the Owner about.
	trip, err := s.trips.Find(ctx, fare.TripID)
	if err != nil {
		return nil, err
	}
	if reason := fitsTrip(fare, trip); reason != "" {
		return nil, errors.New("the Fare no longer fits its Trip: " + reason)
	}
	alert := model.FareAlert{
		FareID: fare.ID, TripID: fare.TripID, Party: fare.Party,
		Reasons: args.Reasons, Urgent: args.Urgent, SentAt: s.core.Now().UTC(),
	}
	if alert.ID, err = s.alerts.Insert(ctx, alert); err != nil {
		return nil, err
	}
	return toFareAlertJSON(alert), nil
}

// alertsSince is how far back travel_list_fare_alerts looks by default.
const alertsSince = 30 * 24 * time.Hour

// listAlerts answers a Trip's Party's Fare Alerts since a time.
func (s *FaresService) listAlerts(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		TripID string `json:"tripId"`
		Party  string `json:"party"`
		Since  string `json:"since"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, errors.New("give tripId, party and since")
	}
	since := s.core.Now().Add(-alertsSince)
	if args.Since != "" {
		t, err := time.Parse(time.RFC3339, args.Since)
		if err != nil {
			return nil, errors.New("since must be a time written as RFC 3339, such as 2026-11-22T00:00:00Z")
		}
		since = t
	}
	// A Trip or Party that isn't there is refused, not answered with no
	// alerts, which would read as none sent and get them sent again.
	trip, err := s.trips.Find(ctx, args.TripID)
	if errors.Is(err, coreutils.ErrNotFound) {
		return nil, errors.New("there is no Trip " + args.TripID)
	}
	if err != nil {
		return nil, err
	}
	party := model.PartyKey(args.Party)
	if !slices.ContainsFunc(trip.Parties, func(p model.Party) bool { return p.Key == party }) {
		return nil, fmt.Errorf("the Trip has no Party %q", args.Party)
	}
	alerts, err := s.alerts.List(ctx, trip.ID, party, since)
	if err != nil {
		return nil, err
	}
	list := make([]model.FareAlertJSON, len(alerts))
	for i, a := range alerts {
		list[i] = toFareAlertJSON(a)
	}
	return map[string][]model.FareAlertJSON{"alerts": list}, nil
}

func toFareAlertJSON(a model.FareAlert) model.FareAlertJSON {
	return model.FareAlertJSON{
		ID: a.ID, FareID: a.FareID, TripID: a.TripID, Party: string(a.Party),
		Reasons: a.Reasons, Urgent: a.Urgent, SentAt: a.SentAt.UTC().Format(time.RFC3339),
	}
}

// topFares is how many current Fares travel_fare_stats answers, and current
// is how recently they were found.
const (
	topFares = 3
	current  = 7 * 24 * time.Hour
)

// stats is what a Trip's Party's Fares say. Weeks run Monday to Sunday in the
// Owner Time Zone.
func (s *FaresService) stats(ctx context.Context, trip model.Trip, party model.PartyKey) (model.FareStatsJSON, error) {
	fares, loc, now, err := s.faresNow(ctx, trip, party)
	if err != nil {
		return model.FareStatsJSON{ByWeek: []model.WeekFareJSON{}, Top: []model.FareJSON{}}, err
	}
	return statsOf(fares, loc, now), nil
}

// faresNow is every Fare of a Trip's Party that fits the Trip as it is now,
// cheapest per person first, with the Owner Time Zone and the time now, which
// is all statsOf needs. A Fare that no longer fits stays stored, in case the
// Trip changes back.
func (s *FaresService) faresNow(ctx context.Context, trip model.Trip, party model.PartyKey) ([]model.Fare, *time.Location, time.Time, error) {
	fares, err := s.fares.List(ctx, trip.ID, party, time.Time{})
	if err != nil {
		return nil, nil, time.Time{}, err
	}
	fares = slices.DeleteFunc(fares, func(f model.Fare) bool { return fitsTrip(f, trip) != "" })
	loc, err := s.core.OwnerTimeZone(ctx)
	if err != nil {
		return nil, nil, time.Time{}, err
	}
	return fares, loc, s.core.Now(), nil
}

// statsOf is what fares, cheapest per person first, say at now.
func statsOf(fares []model.Fare, loc *time.Location, now time.Time) model.FareStatsJSON {
	stats := model.FareStatsJSON{ByWeek: []model.WeekFareJSON{}, Top: []model.FareJSON{}}
	thisWeek := coremodel.DayOf(now, loc).Week()
	lastWeek := thisWeek.AddDays(-7)
	byWeek := map[coremodel.Day]model.FareJSON{}
	// Each check records an itinerary it finds again, so top counts each once.
	inTop := map[string]bool{}
	// fares are cheapest first, so the first Fare to fit each is its best.
	for _, f := range fares {
		j := toFareJSON(f)
		if stats.LowestEver == nil {
			stats.LowestEver = &j
		}
		switch coremodel.DayOf(f.FoundAt, loc).Week() {
		case thisWeek:
			if stats.BestThisWeek == nil {
				stats.BestThisWeek = &j
			}
		case lastWeek:
			if stats.BestLastWeek == nil {
				stats.BestLastWeek = &j
			}
		}
		if _, ok := byWeek[f.DepartDate.Week()]; !ok {
			byWeek[f.DepartDate.Week()] = j
		}
		if key := itinerary(f); len(stats.Top) < topFares && now.Sub(f.FoundAt) <= current && !inTop[key] {
			inTop[key] = true
			stats.Top = append(stats.Top, j)
		}
	}
	for week, f := range byWeek {
		stats.ByWeek = append(stats.ByWeek, model.WeekFareJSON{Week: string(week), Fare: f})
	}
	slices.SortFunc(stats.ByWeek, func(a, b model.WeekFareJSON) int { return strings.Compare(a.Week, b.Week) })
	return stats
}

// itinerary is what makes two Fares the same itinerary: their dates, their
// flights each way, and whether they are on Separate Tickets.
func itinerary(f model.Fare) string {
	key := []string{string(f.DepartDate), string(f.ReturnDate), fmt.Sprint(f.SeparateTickets)}
	for _, leg := range []model.Leg{f.Outbound, f.Inbound} {
		key = append(key, "|")
		for _, flight := range leg.Flights {
			key = append(key, flight.Carrier+flight.Number+flight.Departs)
		}
	}
	return strings.Join(key, " ")
}

func toFareJSON(f model.Fare) model.FareJSON {
	j := model.FareJSON{
		ID: f.ID, TripID: f.TripID, Party: string(f.Party), CheckID: f.CheckID,
		FoundAt: f.FoundAt.UTC().Format(time.RFC3339), Source: string(f.Source),
		DepartDate: string(f.DepartDate), ReturnDate: string(f.ReturnDate), Nights: f.Nights,
		Outbound: toLegJSON(f.Outbound), Inbound: toLegJSON(f.Inbound),
		SeparateTickets: f.SeparateTickets, Cabin: string(f.Cabin), BagsIncluded: f.BagsIncluded,
		Currency: f.Currency, Total: f.Total, PerPerson: f.PerPerson, BookingURL: f.BookingURL,
	}
	if f.LeaveNeeded != nil {
		j.LeaveNeeded = &model.LeaveNeededJSON{Days: f.LeaveNeeded.Days, WithRemoteDays: f.LeaveNeeded.WithRemoteDays}
	}
	return j
}

func toLegJSON(l model.Leg) model.LegJSON {
	flights := make([]model.FlightJSON, len(l.Flights))
	for i, f := range l.Flights {
		flights[i] = model.FlightJSON(f)
	}
	return model.LegJSON{Flights: flights, Stops: l.Stops, DurationMinutes: l.DurationMinutes}
}
