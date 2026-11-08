package service

import (
	"context"
	"net/http"
	"slices"

	"github.com/feriosch/nando-server/internal/calories/model"
	"github.com/feriosch/nando-server/internal/calories/utils"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

// DaysService answers how each Day went: its Entries and Consumed Calories
// against the Daily Limit it had. It asks the EntriesService which Days can
// be logged on.
type DaysService struct {
	entries        model.EntriesRepository
	dailyLimits    model.DailyLimitsRepository
	entriesService *EntriesService
	core           coremodel.CoreService
}

// NewDays returns the DaysService.
func NewDays(entries model.EntriesRepository, dailyLimits model.DailyLimitsRepository, entriesService *EntriesService, core coremodel.CoreService) *DaysService {
	return &DaysService{entries: entries, dailyLimits: dailyLimits, entriesService: entriesService, core: core}
}

// Routes adds the Day routes to mux, under /api/calories/.
func (s *DaysService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/calories/days/{day}", s.getDay)
	mux.HandleFunc("GET /api/calories/days/{day}/preview", s.previewEntry)
	mux.HandleFunc("GET /api/calories/history", s.history)
	mux.HandleFunc("GET /api/calories/home-card", s.homeCard)
}

// caloriesJSON is a Day's Consumed Calories against its Daily Limit, as the
// Day view, the preview, the history and the Home Card answer them.
type caloriesJSON struct {
	Day              coremodel.Day `json:"day"`
	ConsumedCalories *int          `json:"consumedCalories"`
	DailyLimit       *int          `json:"dailyLimit"`
	Remaining        *int          `json:"remaining"`
	OverLimit        *bool         `json:"overLimit"`
}

func toCaloriesJSON(d model.DayCalories) caloriesJSON {
	return caloriesJSON{
		Day:              d.Day,
		ConsumedCalories: d.ConsumedCalories,
		DailyLimit:       d.DailyLimit,
		Remaining:        d.Remaining(),
		OverLimit:        d.OverLimit(),
	}
}

// dayJSON is the Day view as the API answers it.
type dayJSON struct {
	caloriesJSON
	Entries []utils.EntryJSON `json:"entries"`
}

// dayView returns day's Entries with its Consumed Calories against its Daily
// Limit.
func (s *DaysService) dayView(ctx context.Context, day coremodel.Day) (dayJSON, error) {
	calories, entries, err := s.dayCalories(ctx, day)
	if err != nil {
		return dayJSON{}, err
	}
	return dayJSON{
		caloriesJSON: toCaloriesJSON(calories),
		Entries:      utils.ToEntryJSONs(entries),
	}, nil
}

// dayCalories returns day's Consumed Calories against the Daily Limit it had,
// and the Entries they come from.
func (s *DaysService) dayCalories(ctx context.Context, day coremodel.Day) (model.DayCalories, []model.Entry, error) {
	entries, err := s.entries.ListOnDay(ctx, day)
	if err != nil {
		return model.DayCalories{}, nil, err
	}
	limits, err := s.dailyLimits.List(ctx)
	if err != nil {
		return model.DayCalories{}, nil, err
	}
	return limits.Calories(day, model.ConsumedCalories(entries)), entries, nil
}

// getDay answers the Day view: the Day's Entries, its Consumed Calories, its
// Daily Limit and what remains of it.
func (s *DaysService) getDay(w http.ResponseWriter, r *http.Request) {
	day, ok := utils.PathDay(w, r)
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

// homeCard answers the Calories Home Card: today's Consumed Calories against
// the Daily Limit.
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
	coreutils.WriteJSON(w, http.StatusOK, view.caloriesJSON)
}

// previewEntry answers what logging an Entry of ?kcal= on the Day would make
// of it: its Consumed Calories, what would remain of its Daily Limit, and
// whether that would take it over. With ?replacing= an Entry's ID, it answers
// for editing that Entry to ?kcal= instead.
func (s *DaysService) previewEntry(w http.ResponseWriter, r *http.Request) {
	day, ok := utils.PathDay(w, r)
	if !ok {
		return
	}
	kcal, err := utils.CheckEntryKcal("kcal", r.URL.Query().Get("kcal"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !s.entriesService.CheckEditable(w, r, day) {
		return
	}
	entries, err := s.entries.ListOnDay(r.Context(), day)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	if replacing := r.URL.Query().Get("replacing"); replacing != "" {
		i := slices.IndexFunc(entries, func(e model.Entry) bool { return e.ID == replacing })
		if i < 0 {
			http.Error(w, "replacing must be an Entry on this Day", http.StatusBadRequest)
			return
		}
		entries = slices.Delete(entries, i, i+1)
	}
	limits, err := s.dailyLimits.List(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	prospective := model.Entry{KcalPerServing: kcal, Servings: 1}
	consumed := model.ConsumedCalories(append(entries, prospective))
	coreutils.WriteJSON(w, http.StatusOK, toCaloriesJSON(limits.Calories(day, consumed)))
}

// maxHistoryDays is the most Days the history answers at once: a leap year.
const maxHistoryDays = 366

// history lists every Day from ?from= to ?to= (today at the latest), newest
// first, with its Consumed Calories against the Daily Limit it had.
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
	today, err := s.core.Today(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	if to > today {
		http.Error(w, "to must be today or before it", http.StatusBadRequest)
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
	list := []caloriesJSON{}
	for day := to; day >= from; day = day.AddDays(-1) {
		var kcal *int
		if k, logged := consumed[day]; logged {
			kcal = &k
		}
		list = append(list, toCaloriesJSON(limits.Calories(day, kcal)))
	}
	coreutils.WriteJSON(w, http.StatusOK, list)
}
