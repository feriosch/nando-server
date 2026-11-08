package service

import (
	"context"
	"net/http"
	"strconv"

	"github.com/feriosch/nando-server/internal/calories/model"
	"github.com/feriosch/nando-server/internal/calories/utils"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

// DaysService answers how each Day went: its Entries and Consumed Calories
// against the Daily Limit it had.
type DaysService struct {
	entries     model.EntriesRepository
	dailyLimits model.DailyLimitsRepository
	core        coremodel.CoreService
}

// NewDays returns the DaysService.
func NewDays(entries model.EntriesRepository, dailyLimits model.DailyLimitsRepository, core coremodel.CoreService) *DaysService {
	return &DaysService{entries: entries, dailyLimits: dailyLimits, core: core}
}

// Routes adds the Day routes to mux, under /api/calories/.
func (s *DaysService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/calories/days/{day}", s.getDay)
	mux.HandleFunc("GET /api/calories/days/{day}/preview", s.previewEntry)
	mux.HandleFunc("GET /api/calories/history", s.history)
	mux.HandleFunc("GET /api/calories/home-card", s.homeCard)
}

// summaryJSON is a Day's Consumed Calories against its Daily Limit, as the
// Day view and the Home Card answer them. An Unlogged Day isn't 0 kcal, so it
// has neither Consumed Calories nor anything remaining.
type summaryJSON struct {
	Day              coremodel.Day `json:"day"`
	ConsumedCalories *int          `json:"consumedCalories"`
	DailyLimit       *int          `json:"dailyLimit"`
	Remaining        *int          `json:"remaining"`
}

// dayJSON is the Day view as the API answers it.
type dayJSON struct {
	summaryJSON
	Entries []entryJSON `json:"entries"`
}

// dayView returns day's Entries with its summary.
func (s *DaysService) dayView(ctx context.Context, day coremodel.Day) (dayJSON, error) {
	entries, err := s.entries.ListOnDay(ctx, day)
	if err != nil {
		return dayJSON{}, err
	}
	limits, err := s.dailyLimits.List(ctx)
	if err != nil {
		return dayJSON{}, err
	}
	view := dayJSON{
		summaryJSON: summaryJSON{Day: day, DailyLimit: limits.On(day)},
		Entries:     make([]entryJSON, len(entries)),
	}
	consumed := 0
	for i, e := range entries {
		view.Entries[i] = toEntryJSON(e)
		consumed += e.Kcal()
	}
	if len(entries) > 0 {
		view.ConsumedCalories = &consumed
		if view.DailyLimit != nil {
			remaining := *view.DailyLimit - consumed
			view.Remaining = &remaining
		}
	}
	return view, nil
}

// getDay answers the Day view: the Day's Entries, its Consumed Calories, its
// Daily Limit and what remains of it.
func (s *DaysService) getDay(w http.ResponseWriter, r *http.Request) {
	day, ok := pathDay(w, r)
	if !ok {
		return
	}
	view, err := s.dayView(r.Context(), day)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, view)
}

// homeCard answers the Calories Home Card: today's summary.
func (s *DaysService) homeCard(w http.ResponseWriter, r *http.Request) {
	today, err := s.core.Today(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	view, err := s.dayView(r.Context(), today)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, view.summaryJSON)
}

// maxEntryKcal is the most kcal one Entry can have: the most kcal per
// serving times the most Servings.
const maxEntryKcal = utils.MaxKcal * utils.MaxServings

// previewJSON is what logging an Entry would make of a Day.
type previewJSON struct {
	ConsumedCalories int  `json:"consumedCalories"`
	DailyLimit       *int `json:"dailyLimit"`
	Remaining        *int `json:"remaining"`
	OverLimit        bool `json:"overLimit"`
}

// previewEntry answers what logging an Entry of ?kcal= on the Day would make
// of it: its Consumed Calories, what would remain of its Daily Limit, and
// whether that would take it over.
func (s *DaysService) previewEntry(w http.ResponseWriter, r *http.Request) {
	day, ok := pathDay(w, r)
	if !ok {
		return
	}
	kcal, err := strconv.Atoi(r.URL.Query().Get("kcal"))
	if err != nil || kcal < 0 || kcal > maxEntryKcal {
		http.Error(w, "kcal must be a whole number from 0 to 1000000", http.StatusBadRequest)
		return
	}
	entries, err := s.entries.ListOnDay(r.Context(), day)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	limits, err := s.dailyLimits.List(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	preview := previewJSON{ConsumedCalories: kcal, DailyLimit: limits.On(day)}
	for _, e := range entries {
		preview.ConsumedCalories += e.Kcal()
	}
	if preview.DailyLimit != nil {
		remaining := *preview.DailyLimit - preview.ConsumedCalories
		preview.Remaining, preview.OverLimit = &remaining, remaining < 0
	}
	coreutils.WriteJSON(w, http.StatusOK, preview)
}

// maxHistoryDays is the most Days the history answers at once: a leap year.
const maxHistoryDays = 366

// historyDayJSON is one Day of the history as the API lists it.
type historyDayJSON struct {
	Day              coremodel.Day `json:"day"`
	ConsumedCalories *int          `json:"consumedCalories"`
	DailyLimit       *int          `json:"dailyLimit"`
	OverLimit        *bool         `json:"overLimit"`
}

// history lists every Day from ?from= to ?to=, newest first, with its
// Consumed Calories against the Daily Limit it had. An Unlogged Day has
// neither Consumed Calories nor whether it went over.
func (s *DaysService) history(w http.ResponseWriter, r *http.Request) {
	from, errFrom := coremodel.ParseDay(r.URL.Query().Get("from"))
	to, errTo := coremodel.ParseDay(r.URL.Query().Get("to"))
	if errFrom != nil || errTo != nil {
		http.Error(w, "from and to must be Days written YYYY-MM-DD", http.StatusBadRequest)
		return
	}
	if from > to || from.AddDays(maxHistoryDays-1) < to {
		http.Error(w, "from must be at most 366 Days before to, and not after it", http.StatusBadRequest)
		return
	}
	consumed, err := s.entries.ConsumedByDay(r.Context(), from, to)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	limits, err := s.dailyLimits.List(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	list := []historyDayJSON{}
	for day := to; day >= from; day = day.AddDays(-1) {
		row := historyDayJSON{Day: day, DailyLimit: limits.On(day)}
		if kcal, logged := consumed[day]; logged {
			over := row.DailyLimit != nil && kcal > *row.DailyLimit
			row.ConsumedCalories, row.OverLimit = &kcal, &over
		}
		list = append(list, row)
	}
	coreutils.WriteJSON(w, http.StatusOK, list)
}
