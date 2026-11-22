package service

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/travel/model"
)

// OverviewService answers the web app's reads of Travel: the list of Trips,
// a Trip's page, the Home Cards and the settings. The web app only reads
// Travel (ADR 0013): the Travel Assistant writes, through the Tools.
type OverviewService struct {
	trips    model.TripsRepository
	verdicts model.PriceVerdictsRepository
	stats    *FaresService
	settings *SettingsService
}

// NewOverview returns the OverviewService. It asks the FaresService for a
// Party's Fares and what they say.
func NewOverview(trips model.TripsRepository, verdicts model.PriceVerdictsRepository,
	stats *FaresService, settings *SettingsService) *OverviewService {
	return &OverviewService{trips: trips, verdicts: verdicts, stats: stats, settings: settings}
}

// Routes adds the web app's routes to mux, all under /api/travel/.
func (s *OverviewService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/travel/trips", s.listTrips)
	mux.HandleFunc("GET /api/travel/trips/{id}", s.tripPage)
	mux.HandleFunc("GET /api/travel/cards", s.cards)
	mux.HandleFunc("GET /api/travel/settings", s.getSettings)
}

// pageVerdicts is how many Price Verdicts a Trip's page shows per Party.
const pageVerdicts = 5

// allStatuses are every Trip status.
var allStatuses = []model.Status{model.Watching, model.Booked, model.Dropped}

func (s *OverviewService) listTrips(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	trips, err := s.trips.List(ctx, allStatuses)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	// By name already: Watching ones first, keeping that order.
	slices.SortStableFunc(trips, func(a, b model.Trip) int {
		return boolOrder(a.Status != model.Watching, b.Status != model.Watching)
	})
	list := model.TripsJSON{Trips: make([]model.TripSummaryJSON, len(trips))}
	for i, trip := range trips {
		summary := model.TripSummaryJSON{
			ID: trip.ID, Name: trip.Name, Origins: trip.Origins, Destinations: trip.Destinations,
			Status: string(trip.Status), Parties: make([]model.PartySummaryJSON, len(trip.Parties)),
		}
		for j, party := range trip.Parties {
			if summary.Parties[j], err = s.partySummary(ctx, trip, party); err != nil {
				coreutils.ServerError(w, err)
				return
			}
		}
		list.Trips[i] = summary
	}
	coreutils.WriteJSON(w, http.StatusOK, list)
}

// boolOrder orders false before true.
func boolOrder(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a:
		return -1
	}
	return 1
}

// partySummary is a Party's line in the list of Trips.
func (s *OverviewService) partySummary(ctx context.Context, trip model.Trip, party model.Party) (model.PartySummaryJSON, error) {
	stats, err := s.stats.stats(ctx, trip.ID, party.Key)
	if err != nil {
		return model.PartySummaryJSON{}, err
	}
	summary := model.PartySummaryJSON{Key: string(party.Key), Adults: party.Adults, Best: bestNow(stats), TargetFare: targetFare(trip, party.Key)}
	if stats.BestThisWeek != nil && stats.BestLastWeek != nil {
		change := stats.BestThisWeek.PerPerson - stats.BestLastWeek.PerPerson
		summary.SinceLastWeek = &change
	}
	verdicts, err := s.verdicts.List(ctx, trip.ID, party.Key, 1)
	if err != nil {
		return summary, err
	}
	if len(verdicts) > 0 {
		summary.VerdictLevel = &verdicts[0].Level
	}
	return summary, nil
}

// bestNow is a Party's best Fare now: the cheapest found in the last 7
// days, or nil.
func bestNow(stats model.FareStatsJSON) *model.FareJSON {
	if len(stats.Top) > 0 {
		return &stats.Top[0]
	}
	return nil
}

// targetFare is party's Target Fare on trip, or nil.
func targetFare(trip model.Trip, party model.PartyKey) *int {
	if i := slices.IndexFunc(trip.TargetFares, func(t model.TargetFare) bool { return t.Party == party }); i >= 0 {
		return &trip.TargetFares[i].PerPerson
	}
	return nil
}

func (s *OverviewService) tripPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	trip, err := s.trips.Find(ctx, r.PathValue("id"))
	if errors.Is(err, coreutils.ErrNotFound) {
		http.Error(w, "no such Trip", http.StatusNotFound)
		return
	}
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	page := model.TripPageJSON{Trip: toTripJSON(trip), Parties: make([]model.PartyPageJSON, len(trip.Parties))}
	for i, party := range trip.Parties {
		if page.Parties[i], err = s.partyPage(ctx, trip, party); err != nil {
			coreutils.ServerError(w, err)
			return
		}
	}
	coreutils.WriteJSON(w, http.StatusOK, page)
}

// partyPage is a Party on a Trip's page.
func (s *OverviewService) partyPage(ctx context.Context, trip model.Trip, party model.Party) (model.PartyPageJSON, error) {
	// One read of the Fares for the stats and the history, so both agree.
	fares, loc, now, err := s.stats.faresNow(ctx, trip.ID, party.Key)
	if err != nil {
		return model.PartyPageJSON{}, err
	}
	stats := statsOf(fares, loc, now)
	page := model.PartyPageJSON{
		Key: string(party.Key), Adults: party.Adults, History: []model.DayFareJSON{},
		ByWeek: stats.ByWeek, Top: stats.Top, TargetFare: targetFare(trip, party.Key),
	}
	best := map[coremodel.Day]int{}
	// fares are cheapest first, so the first of each Day is its best.
	for _, f := range fares {
		day := coremodel.DayOf(f.FoundAt, loc)
		if _, ok := best[day]; !ok {
			best[day] = f.PerPerson
		}
		if page.BestSingleTicket == nil && !f.SeparateTickets && now.Sub(f.FoundAt) <= current {
			single := toFareJSON(f)
			page.BestSingleTicket = &single
		}
	}
	for day, perPerson := range best {
		page.History = append(page.History, model.DayFareJSON{Day: string(day), PerPerson: perPerson})
	}
	slices.SortFunc(page.History, func(a, b model.DayFareJSON) int { return strings.Compare(a.Day, b.Day) })
	verdicts, err := s.verdicts.List(ctx, trip.ID, party.Key, pageVerdicts)
	if err != nil {
		return page, err
	}
	page.Verdicts = make([]model.PriceVerdictJSON, len(verdicts))
	for i, v := range verdicts {
		page.Verdicts[i] = toPriceVerdictJSON(v)
	}
	return page, nil
}

func (s *OverviewService) cards(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	trips, err := s.trips.List(ctx, []model.Status{model.Watching})
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	cards := model.CardsJSON{Cards: make([]model.HomeCardJSON, len(trips))}
	for i, trip := range trips {
		card := model.HomeCardJSON{TripID: trip.ID, Name: trip.Name}
		for _, party := range trip.Parties {
			stats, err := s.stats.stats(ctx, trip.ID, party.Key)
			if err != nil {
				coreutils.ServerError(w, err)
				return
			}
			best := bestNow(stats)
			if best == nil || (card.PerPerson != nil && *card.PerPerson <= best.PerPerson) {
				continue
			}
			card.Party, card.PerPerson, card.Trend = &best.Party, &best.PerPerson, trend(stats)
		}
		cards.Cards[i] = card
	}
	coreutils.WriteJSON(w, http.StatusOK, cards)
}

// trend is how a Party's best per person moved since last week: down, flat
// or up, or nil without a best both weeks.
func trend(stats model.FareStatsJSON) *string {
	if stats.BestThisWeek == nil || stats.BestLastWeek == nil {
		return nil
	}
	t := "flat"
	switch now, then := stats.BestThisWeek.PerPerson, stats.BestLastWeek.PerPerson; {
	case now < then:
		t = "down"
	case now > then:
		t = "up"
	}
	return &t
}

func (s *OverviewService) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.settings.savedOrDefault(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, toSettingsJSON(settings))
}
