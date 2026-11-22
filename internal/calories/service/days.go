package service

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/feriosch/nando-server/internal/calories/model"
	"github.com/feriosch/nando-server/internal/calories/utils"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

// DaysService answers how each Day went: its Entries and Consumed Calories
// against the Daily Limit it had. It asks the EntriesService which Days can
// be logged on.
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
	mux.HandleFunc("GET /api/calories/history", s.history)
	mux.HandleFunc("GET /api/calories/home-card", s.homeCard)
}

func toCaloriesJSON(d model.DayCalories) model.CaloriesJSON {
	return model.CaloriesJSON{
		Day:              d.Day,
		ConsumedCalories: d.ConsumedCalories,
		DailyLimit:       d.DailyLimit,
		Remaining:        d.Remaining(),
		OverLimit:        d.OverLimit(),
	}
}

// dayView returns day's Entries with its Consumed Calories against its Daily
// Limit.
func (s *DaysService) dayView(ctx context.Context, day coremodel.Day) (model.DayJSON, error) {
	calories, entries, err := s.dayCalories(ctx, day)
	if err != nil {
		return model.DayJSON{}, err
	}
	return model.DayJSON{
		CaloriesJSON: toCaloriesJSON(calories),
		Entries:      utils.ToEntryJSONs(entries),
	}, nil
}

// dayCalories returns day's Consumed Calories against the Daily Limit it had,
// and the Entries they come from.
func (s *DaysService) dayCalories(ctx context.Context, day coremodel.Day) (model.DayCalories, []model.Entry, error) {
	return dayCalories(ctx, s.entries, s.dailyLimits, day)
}

// dayCalories returns day's Consumed Calories, read from entries, against
// the Daily Limit it had, read from dailyLimits, and the Entries they come
// from.
func dayCalories(ctx context.Context, entries model.EntriesRepository, dailyLimits model.DailyLimitsRepository,
	day coremodel.Day) (model.DayCalories, []model.Entry, error) {
	list, err := entries.ListOnDay(ctx, day)
	if err != nil {
		return model.DayCalories{}, nil, err
	}
	limits, err := dailyLimits.List(ctx)
	if err != nil {
		return model.DayCalories{}, nil, err
	}
	return limits.Calories(day, model.ConsumedCalories(list)), list, nil
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
	coreutils.WriteJSON(w, http.StatusOK, view.CaloriesJSON)
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
	list := []model.CaloriesJSON{}
	for day := to; day >= from; day = day.AddDays(-1) {
		var kcal *int
		if k, logged := consumed[day]; logged {
			kcal = &k
		}
		list = append(list, toCaloriesJSON(limits.Calories(day, kcal)))
	}
	coreutils.WriteJSON(w, http.StatusOK, list)
}

// Tools are what the DaysService offers Connectors.
func (s *DaysService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "calories_get_day",
		Description: "A Day as the Nando app shows it: its Entries in the order they were logged, and its Consumed Calories " +
			"against the Daily Limit it had. An Unlogged Day has no Entries, and null Consumed Calories: not recorded, never 0 kcal.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"day":{"type":"string","description":"YYYY-MM-DD in the Owner Time Zone; leave out for today."}}}`),
		ReadOnly: true,
		Handle:   s.getDayTool,
	}}
}

func (s *DaysService) getDayTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		Day string `json:"day"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	day, err := toolDay(ctx, s.core, args.Day)
	if err != nil {
		return nil, err
	}
	return s.dayView(ctx, day)
}
