package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/travel/model"
)

// TripsService handles the Owner's Trips. Only the Travel Assistant reads and
// changes them, through its Tools.
type TripsService struct {
	trips    model.TripsRepository
	settings *SettingsService
	core     coremodel.CoreService
}

// NewTrips returns the TripsService. It asks the SettingsService for where
// Trips leave from by default.
func NewTrips(trips model.TripsRepository, settings *SettingsService, core coremodel.CoreService) *TripsService {
	return &TripsService{trips: trips, settings: settings, core: core}
}

// Tools are what the TripsService offers Connectors: reading and changing the
// Owner's Trips.
func (s *TripsService) Tools() []coremodel.Tool {
	id := `{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`
	return []coremodel.Tool{{
		Name:        "travel_list_trips",
		Description: "The Owner's Trips by name, as {trips: [...]}: the Watching ones, or those with status (watching, booked, dropped or all).",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","enum":["watching","booked","dropped","all"]}}}`),
		ReadOnly:    true,
		Handle:      s.listTrips,
	}, {
		Name:        "travel_get_trip",
		Description: "The Trip with id.",
		InputSchema: json.RawMessage(id),
		ReadOnly:    true,
		Handle: func(ctx context.Context, arguments json.RawMessage) (any, error) {
			trip, err := s.find(ctx, arguments)
			return toTripJSON(trip), err
		},
	}, {
		Name: "travel_save_trip",
		Description: "Saves a Trip. Without id, a new one, Watching: name, destinations, nights and windows are needed; " +
			"origins default to the settings' home, cabin to economy, checkedBags to 1, maxStops to 1, " +
			"separateTickets to true and parties to the Owner alone. With id, changes only the fields given. Answers the Trip.",
		InputSchema: json.RawMessage(tripSchema),
		Handle:      s.saveTrip,
	}, {
		Name: "travel_set_trip_status",
		Description: "Sets the Trip with id to status: booked once the Owner booked it, dropped once they no longer want it, " +
			"or watching again. Only Watching Trips are searched. Ask the Owner before dropping one. Answers the Trip.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"},` +
			`"status":{"type":"string","enum":["watching","booked","dropped"]}},"required":["id","status"]}`),
		Handle: s.setTripStatus,
	}, {
		Name: "travel_delete_trip",
		Description: "Deletes the Trip with id for good. Only on the Owner's yes in a later message: " +
			"a Trip they booked or no longer want is better set to booked or dropped.",
		InputSchema: json.RawMessage(id),
		Destructive: true,
		Handle: func(ctx context.Context, arguments json.RawMessage) (any, error) {
			trip, err := s.find(ctx, arguments)
			if err != nil {
				return nil, err
			}
			if err := s.trips.Delete(ctx, trip.ID); err != nil {
				return nil, err
			}
			return map[string]string{"deleted": trip.ID}, nil
		},
	}}
}

// statuses are the Trip statuses travel_list_trips takes, by their API names.
var statuses = map[string][]model.Status{
	"watching": {model.Watching},
	"booked":   {model.Booked},
	"dropped":  {model.Dropped},
	"all":      {model.Watching, model.Booked, model.Dropped},
}

// listTrips answers the Trips with the status asked, Watching by default.
func (s *TripsService) listTrips(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, errors.New("status must be watching, booked, dropped or all")
	}
	if args.Status == "" {
		args.Status = "watching"
	}
	wanted, ok := statuses[args.Status]
	if !ok {
		return nil, errors.New("status must be watching, booked, dropped or all")
	}
	trips, err := s.trips.List(ctx, wanted)
	if err != nil {
		return nil, err
	}
	list := make([]model.TripJSON, len(trips))
	for i, trip := range trips {
		list[i] = toTripJSON(trip)
	}
	return map[string][]model.TripJSON{"trips": list}, nil
}

// setTripStatus sets a Trip's status, from today.
func (s *TripsService) setTripStatus(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, errors.New("give the Trip's id and its status: watching, booked or dropped")
	}
	status := model.Status(args.Status)
	if status != model.Watching && status != model.Booked && status != model.Dropped {
		return nil, errors.New("status must be watching, booked or dropped")
	}
	trip, err := s.find(ctx, arguments)
	if err != nil {
		return nil, err
	}
	today, err := s.core.Today(ctx)
	if err != nil {
		return nil, err
	}
	if trip.Status == status {
		return toTripJSON(trip), nil
	}
	if status == model.Watching && !slices.ContainsFunc(trip.Windows, func(w model.Window) bool { return w.To >= today }) {
		return nil, errors.New("every window of the Trip is over before today: give it a window ahead with travel_save_trip first")
	}
	if err := s.trips.SetStatus(ctx, trip.ID, status, today); err != nil {
		if errors.Is(err, coreutils.ErrNotFound) {
			return nil, errors.New("there is no Trip " + trip.ID)
		}
		return nil, err
	}
	trip.Status, trip.StatusChangedOn = status, today
	return toTripJSON(trip), nil
}

// tripSchema is the JSON Schema of travel_save_trip's arguments.
const tripSchema = `{"type":"object","properties":{` +
	`"id":{"type":"string","description":"The Trip to change; leave out for a new one."},` +
	`"name":{"type":"string"},` +
	`"origins":{"type":"array","items":{"type":"string"},"description":"Airport or city codes, 3 capital letters each."},` +
	`"destinations":{"type":"array","items":{"type":"string"},"description":"Airport or city codes, 3 capital letters each."},` +
	`"nights":{"type":"object","properties":{"min":{"type":"integer"},"max":{"type":"integer"}},"description":"1 to 60."},` +
	`"windows":{"type":"array","items":{"type":"object","properties":{"from":{"type":"string"},"to":{"type":"string"},"note":{"type":"string"}}},` +
	`"description":"Ranges of departure dates, YYYY-MM-DD, from and to included."},` +
	`"avoid":{"type":"array","items":{"type":"object","properties":{"from":{"type":"string"},"to":{"type":"string"},"reason":{"type":"string"}}},` +
	`"description":"Ranges of dates, YYYY-MM-DD, the trip must not touch."},` +
	`"cabin":{"type":"string","enum":["economy","premium_economy","business","first"]},` +
	`"checkedBags":{"type":"integer","description":"Per person, 0 to 2."},` +
	`"maxStops":{"type":"integer","description":"Each way, 0 to 3."},` +
	`"separateTickets":{"type":"boolean","description":"Whether Fares on Separate Tickets are welcome."},` +
	`"parties":{"type":"array","items":{"type":"object","properties":{"key":{"type":"string","enum":["owner","with_companion"]},"adults":{"type":"integer"}}},` +
	`"description":"One or two: the Owner alone (1 adult), and with the Companion (2 or more)."},` +
	`"notes":{"type":"string"}}}`

// find returns the Trip whose id the arguments give.
func (s *TripsService) find(ctx context.Context, arguments json.RawMessage) (model.Trip, error) {
	var args struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil || args.ID == "" {
		return model.Trip{}, errors.New("give the Trip's id")
	}
	trip, err := s.trips.Find(ctx, args.ID)
	if errors.Is(err, coreutils.ErrNotFound) {
		return trip, errors.New("there is no Trip " + args.ID)
	}
	return trip, err
}

// saveTrip creates a Trip, or changes the fields given of one.
func (s *TripsService) saveTrip(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args model.SaveTripJSON
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, errors.New("travel_save_trip's arguments don't fit its schema: " + err.Error())
	}
	settings, err := s.settings.savedOrDefault(ctx)
	if err != nil {
		return nil, err
	}
	today, err := s.core.Today(ctx)
	if err != nil {
		return nil, err
	}
	if args.ID != "" {
		trip, err := s.trips.Find(ctx, args.ID)
		if errors.Is(err, coreutils.ErrNotFound) {
			return nil, errors.New("there is no Trip " + args.ID)
		}
		if err != nil {
			return nil, err
		}
		trip = changedTrip(trip, args)
		if err := checkTrip(trip, args, settings, today); err != nil {
			return nil, err
		}
		if err := s.trips.Update(ctx, trip); err != nil {
			if errors.Is(err, coreutils.ErrNotFound) {
				return nil, errors.New("there is no Trip " + args.ID)
			}
			return nil, err
		}
		return toTripJSON(trip), nil
	}
	trip := model.Trip{
		Origins:         settings.Home,
		Avoid:           []model.Avoid{},
		Cabin:           model.Economy,
		CheckedBags:     1,
		MaxStops:        1,
		SeparateTickets: true,
		Parties:         []model.Party{{Key: model.OwnerAlone, Adults: 1}},
		Status:          model.Watching,
		StatusChangedOn: today,
	}
	trip = changedTrip(trip, args)
	if err := checkTrip(trip, args, settings, today); err != nil {
		return nil, err
	}
	if trip.ID, err = s.trips.Insert(ctx, trip); err != nil {
		return nil, err
	}
	return toTripJSON(trip), nil
}

// changedTrip is trip with the fields args gives.
func changedTrip(trip model.Trip, args model.SaveTripJSON) model.Trip {
	if args.Name != nil {
		trip.Name = strings.TrimSpace(*args.Name)
	}
	if args.Origins != nil {
		trip.Origins = *args.Origins
	}
	if args.Destinations != nil {
		trip.Destinations = *args.Destinations
	}
	if args.Nights != nil {
		trip.Nights = model.Nights{Min: args.Nights.Min, Max: args.Nights.Max}
	}
	if args.Windows != nil {
		trip.Windows = make([]model.Window, len(*args.Windows))
		for i, w := range *args.Windows {
			trip.Windows[i] = model.Window{From: coremodel.Day(w.From), To: coremodel.Day(w.To), Note: w.Note}
		}
	}
	if args.Avoid != nil {
		trip.Avoid = make([]model.Avoid, len(*args.Avoid))
		for i, a := range *args.Avoid {
			trip.Avoid[i] = model.Avoid{From: coremodel.Day(a.From), To: coremodel.Day(a.To), Reason: a.Reason}
		}
	}
	if args.Cabin != nil {
		trip.Cabin = model.Cabin(*args.Cabin)
	}
	if args.CheckedBags != nil {
		trip.CheckedBags = *args.CheckedBags
	}
	if args.MaxStops != nil {
		trip.MaxStops = *args.MaxStops
	}
	if args.SeparateTickets != nil {
		trip.SeparateTickets = *args.SeparateTickets
	}
	if args.Parties != nil {
		trip.Parties = make([]model.Party, len(*args.Parties))
		for i, p := range *args.Parties {
			trip.Parties[i] = model.Party{Key: model.PartyKey(p.Key), Adults: p.Adults}
		}
	}
	if args.Notes != nil {
		trip.Notes = *args.Notes
	}
	return trip
}

func toTripJSON(trip model.Trip) model.TripJSON {
	windows := make([]model.WindowJSON, len(trip.Windows))
	for i, w := range trip.Windows {
		windows[i] = model.WindowJSON{From: string(w.From), To: string(w.To), Note: w.Note}
	}
	avoid := make([]model.AvoidJSON, len(trip.Avoid))
	for i, a := range trip.Avoid {
		avoid[i] = model.AvoidJSON{From: string(a.From), To: string(a.To), Reason: a.Reason}
	}
	parties := make([]model.PartyJSON, len(trip.Parties))
	for i, p := range trip.Parties {
		parties[i] = model.PartyJSON{Key: string(p.Key), Adults: p.Adults}
	}
	return model.TripJSON{
		ID:              trip.ID,
		Name:            trip.Name,
		Origins:         trip.Origins,
		Destinations:    trip.Destinations,
		Nights:          model.NightsJSON{Min: trip.Nights.Min, Max: trip.Nights.Max},
		Windows:         windows,
		Avoid:           avoid,
		Cabin:           string(trip.Cabin),
		CheckedBags:     trip.CheckedBags,
		MaxStops:        trip.MaxStops,
		SeparateTickets: trip.SeparateTickets,
		Parties:         parties,
		Status:          string(trip.Status),
		StatusChangedOn: string(trip.StatusChangedOn),
		Notes:           trip.Notes,
	}
}

// The limits of what a Trip may be.
const (
	maxTripName    = 100
	maxNights      = 60
	maxCheckedBags = 2
	maxStops       = 3
	maxAdults      = 9
)

// checkTrip explains what's wrong with trip, as args changed it, if anything.
// A window over before today is refused only when args sends the windows, and
// a Party with the Companion while there is none only when args sends the
// Parties, so an old Trip can still be changed otherwise.
func checkTrip(trip model.Trip, args model.SaveTripJSON, settings model.Settings, today coremodel.Day) error {
	if trip.Name == "" || len([]rune(trip.Name)) > maxTripName {
		return fmt.Errorf("name must be 1 to %d characters", maxTripName)
	}
	if len(trip.Origins) == 0 {
		if args.Origins != nil {
			return errors.New("origins must name at least one code")
		}
		return errors.New("origins must name at least one code: there is no home in the settings to leave from")
	}
	if err := checkCodes("origins", trip.Origins); err != nil {
		return err
	}
	if len(trip.Destinations) == 0 {
		return errors.New("destinations must name at least one code")
	}
	if err := checkCodes("destinations", trip.Destinations); err != nil {
		return err
	}
	if trip.Nights.Min < 1 || trip.Nights.Min > trip.Nights.Max || trip.Nights.Max > maxNights {
		return fmt.Errorf("nights must be {min, max} with 1 ≤ min ≤ max ≤ %d", maxNights)
	}
	if len(trip.Windows) == 0 {
		return errors.New("windows must give at least one range of departure dates")
	}
	for _, w := range trip.Windows {
		if err := checkRange("windows", w.From, w.To); err != nil {
			return err
		}
		if args.Windows != nil && w.To < today {
			return fmt.Errorf("windows: %s to %s is over before today, %s", w.From, w.To, today)
		}
	}
	for _, a := range trip.Avoid {
		if err := checkRange("avoid", a.From, a.To); err != nil {
			return err
		}
	}
	for _, w := range trip.Windows {
		if coveredByAvoid(w, trip.Avoid) {
			return fmt.Errorf("avoid covers the whole window %s to %s, alone or together", w.From, w.To)
		}
	}
	if !slices.Contains([]model.Cabin{model.Economy, model.PremiumEconomy, model.Business, model.First}, trip.Cabin) {
		return errors.New("cabin must be economy, premium_economy, business or first")
	}
	if trip.CheckedBags < 0 || trip.CheckedBags > maxCheckedBags {
		return fmt.Errorf("checkedBags must be 0 to %d per person", maxCheckedBags)
	}
	if trip.MaxStops < 0 || trip.MaxStops > maxStops {
		return fmt.Errorf("maxStops must be 0 to %d", maxStops)
	}
	return checkParties(trip.Parties, settings.Companion, args.Parties != nil)
}

// coveredByAvoid reports whether the avoid ranges, alone or together, cover
// every Day of w, leaving no Day to depart on.
func coveredByAvoid(w model.Window, avoid []model.Avoid) bool {
	// Walk the Days that begin a stretch not yet known covered: w.From, and
	// the Day after each avoid range, so the walk is as long as avoid.
	d := w.From
	for {
		i := slices.IndexFunc(avoid, func(a model.Avoid) bool { return a.From <= d && d <= a.To })
		if i < 0 {
			return false
		}
		if avoid[i].To >= w.To {
			return true
		}
		d = avoid[i].To.AddDays(1)
	}
}

// checkRange explains what's wrong with field's range of Days, if anything.
func checkRange(field string, from, to coremodel.Day) error {
	for _, d := range []coremodel.Day{from, to} {
		if parsed, err := coremodel.ParseDay(string(d)); err != nil || parsed != d {
			return fmt.Errorf("%s: %q isn't a date written YYYY-MM-DD", field, d)
		}
	}
	if from > to {
		return fmt.Errorf("%s: %s is after %s", field, from, to)
	}
	return nil
}

// checkParties explains what's wrong with a Trip's Parties, if anything: one
// or two, each once, the Owner alone as 1 adult, and with the Companion as 2
// or more, only once there is a Companion when sent is true.
func checkParties(parties []model.Party, companion model.Companion, sent bool) error {
	if len(parties) == 0 || len(parties) > 2 {
		return errors.New("parties must be one or two: owner, with_companion or both")
	}
	for i, p := range parties {
		if slices.ContainsFunc(parties[:i], func(q model.Party) bool { return q.Key == p.Key }) {
			return fmt.Errorf("parties has %s twice", p.Key)
		}
		switch p.Key {
		case model.OwnerAlone:
			if p.Adults != 1 {
				return errors.New("the owner Party is the Owner alone: 1 adult")
			}
		case model.WithCompanion:
			if p.Adults < 2 || p.Adults > maxAdults {
				return fmt.Errorf("the with_companion Party is 2 to %d adults", maxAdults)
			}
			if sent && companion.Name == "" {
				return errors.New("there is no Companion in the settings yet: set companion.name with travel_save_settings first")
			}
		default:
			return fmt.Errorf("parties: %q must be owner or with_companion", p.Key)
		}
	}
	return nil
}
