package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/feriosch/nando-server/internal/calories/model"
	"github.com/feriosch/nando-server/internal/calories/utils"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

// EntriesService handles requests and Tools about Entries. It reads Meals too,
// to log an Entry from one, and saves one for "save as Meal".
type EntriesService struct {
	entries model.EntriesRepository
	meals   model.MealsRepository
	// dailyLimits are read to tell a Connector how the Day it changed now
	// stands.
	dailyLimits model.DailyLimitsRepository
	core        coremodel.CoreService
	now         func() time.Time
}

// NewEntries returns the EntriesService, with now as its clock.
func NewEntries(entries model.EntriesRepository, meals model.MealsRepository, dailyLimits model.DailyLimitsRepository,
	core coremodel.CoreService, now func() time.Time) *EntriesService {
	return &EntriesService{entries: entries, meals: meals, dailyLimits: dailyLimits, core: core, now: now}
}

// Routes adds the Entry routes to mux, under /api/calories/.
func (s *EntriesService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/calories/days/{day}/entries", s.listEntries)
	mux.HandleFunc("POST /api/calories/days/{day}/entries", s.postEntry)
	mux.HandleFunc("PUT /api/calories/entries/{id}", s.putEntry)
	mux.HandleFunc("DELETE /api/calories/entries/{id}", s.deleteEntry)
}

// fromEntryValues checks all three of v's values and returns an Entry with
// them, or explains what's wrong.
func fromEntryValues(v model.EntryValues) (model.Entry, error) {
	name, err := utils.CheckName("name", v.Name)
	if err != nil {
		return model.Entry{}, err
	}
	kcal, err := utils.CheckKcal("kcalPerServing", v.KcalPerServing)
	if err != nil {
		return model.Entry{}, err
	}
	if err := utils.CheckServings(v.Servings); err != nil {
		return model.Entry{}, err
	}
	return model.Entry{Name: name, KcalPerServing: kcal, Servings: v.Servings}, nil
}

// checkEditable refuses with 409 unless day is in the Editable Window, today
// being the current Day in the Owner Time Zone.
func (s *EntriesService) checkEditable(ctx context.Context, day coremodel.Day) error {
	today, err := s.core.Today(ctx)
	if err != nil {
		return err
	}
	if !utils.InEditableWindow(day, today) {
		return utils.Refusal{Status: http.StatusConflict, Message: "only today and the 7 Days before it can be changed"}
	}
	return nil
}

// CheckEditable reports whether day is in the Editable Window. If it isn't, it
// answers 409 (or 500 if today can't be known) and returns false.
func (s *EntriesService) CheckEditable(w http.ResponseWriter, r *http.Request, day coremodel.Day) bool {
	if err := s.checkEditable(r.Context(), day); err != nil {
		utils.WriteError(w, err)
		return false
	}
	return true
}

// listEntries lists a Day's Entries in the order they were logged.
func (s *EntriesService) listEntries(w http.ResponseWriter, r *http.Request) {
	day, ok := utils.PathDay(w, r)
	if !ok {
		return
	}
	entries, err := s.entries.ListOnDay(r.Context(), day)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, utils.ToEntryJSONs(entries))
}

// logRequest is what logging an Entry is sent: a Meal and its Servings, or a
// one-off's own values.
type logRequest struct {
	MealID *string `json:"mealId"`
	model.EntryValues
	// SaveAsMeal also saves a one-off Entry as a Meal, which it then
	// belongs to.
	SaveAsMeal bool `json:"saveAsMeal"`
}

// postEntry logs an Entry on the Day in the path: from a Meal, copying its
// name and kcal, or a one-off with its own.
func (s *EntriesService) postEntry(w http.ResponseWriter, r *http.Request) {
	day, ok := utils.PathDay(w, r)
	if !ok {
		return
	}
	var body logRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "the body isn't an Entry", http.StatusBadRequest)
		return
	}
	entry, err := s.log(r.Context(), day, body)
	if err != nil {
		utils.WriteError(w, err)
		return
	}
	coreutils.WriteCreated(w, entry.ID)
}

// log logs the Entry req asks for on day, and returns it.
func (s *EntriesService) log(ctx context.Context, day coremodel.Day, req logRequest) (model.Entry, error) {
	if err := s.checkEditable(ctx, day); err != nil {
		return model.Entry{}, err
	}
	var entry model.Entry
	if req.MealID != nil {
		if req.SaveAsMeal {
			return model.Entry{}, utils.Invalid("saveAsMeal is only for a one-off Entry, not one logged from a Meal")
		}
		var err error
		if entry, err = s.entryFromMeal(ctx, *req.MealID, req.Servings); err != nil {
			return model.Entry{}, err
		}
	} else {
		var err error
		if entry, err = fromEntryValues(req.EntryValues); err != nil {
			return model.Entry{}, utils.Invalid(err.Error())
		}
		if req.SaveAsMeal {
			entry.MealID, err = s.meals.Insert(ctx, model.Meal{
				Name:      entry.Name,
				Kcal:      entry.KcalPerServing,
				CreatedAt: s.now(),
			})
			if err != nil {
				return model.Entry{}, err
			}
		}
	}
	entry.Day, entry.LoggedAt = day, s.now()
	id, err := s.entries.Insert(ctx, entry)
	if err != nil {
		return model.Entry{}, err
	}
	entry.ID = id
	return entry, nil
}

// entryFromMeal returns an Entry of servings of the Meal with mealID, with
// the Meal's name and kcal, or refuses if the Meal can't be logged.
func (s *EntriesService) entryFromMeal(ctx context.Context, mealID string, servings int) (model.Entry, error) {
	if err := utils.CheckServings(servings); err != nil {
		return model.Entry{}, utils.Invalid(err.Error())
	}
	meal, err := s.meals.Find(ctx, mealID)
	if errors.Is(err, coreutils.ErrNotFound) {
		return model.Entry{}, utils.Invalid("there's no such Meal")
	} else if err != nil {
		return model.Entry{}, err
	}
	if meal.ArchivedAt != nil {
		return model.Entry{}, utils.Invalid("that Meal is archived; restore it to log it")
	}
	return model.Entry{MealID: meal.ID, Name: meal.Name, KcalPerServing: meal.Kcal, Servings: servings}, nil
}

// editableEntry returns the Entry with id if its Day is in the Editable
// Window. It refuses with 404 if there's no such Entry, and 409 outside the
// window.
func (s *EntriesService) editableEntry(ctx context.Context, id string) (model.Entry, error) {
	entry, err := s.entries.Find(ctx, id)
	if errors.Is(err, coreutils.ErrNotFound) {
		return model.Entry{}, utils.Refusal{Status: http.StatusNotFound, Message: "there's no such Entry"}
	} else if err != nil {
		return model.Entry{}, err
	}
	return entry, s.checkEditable(ctx, entry.Day)
}

// putEntry edits an Entry's name, kcal per serving and Servings.
func (s *EntriesService) putEntry(w http.ResponseWriter, r *http.Request) {
	var body model.EntryValues
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "the body isn't an Entry", http.StatusBadRequest)
		return
	}
	edited, err := fromEntryValues(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	entry, err := s.editableEntry(r.Context(), r.PathValue("id"))
	if err == nil {
		_, err = s.edit(r.Context(), entry, edited)
	}
	if err != nil {
		utils.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// edit gives entry, found with editableEntry, edited's name, kcal per
// serving and Servings, and returns it.
func (s *EntriesService) edit(ctx context.Context, entry, edited model.Entry) (model.Entry, error) {
	entry.Name, entry.KcalPerServing, entry.Servings = edited.Name, edited.KcalPerServing, edited.Servings
	if err := s.entries.Update(ctx, entry); err != nil {
		return model.Entry{}, err
	}
	return entry, nil
}

// deleteEntry deletes an Entry. Deleting one that doesn't exist also succeeds.
func (s *EntriesService) deleteEntry(w http.ResponseWriter, r *http.Request) {
	entry, err := s.editableEntry(r.Context(), r.PathValue("id"))
	var refusal utils.Refusal
	if errors.As(err, &refusal) && refusal.Status == http.StatusNotFound {
		w.WriteHeader(http.StatusNoContent)
		return
	} else if err != nil {
		utils.WriteError(w, err)
		return
	}
	if err := s.entries.Delete(r.Context(), entry.ID); err != nil {
		coreutils.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Tools are what the EntriesService offers Connectors.
func (s *EntriesService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "calories_log_entry",
		Description: "Logs that the Owner ate something on a Day: either servings of a saved Meal (mealId), " +
			"which copies its name and kcal, or a one-off with its own name and kcalPerServing. " +
			"Answers the Entry, and the Day's Consumed Calories against its Daily Limit.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"day":{"type":"string","description":"The Day eaten on, YYYY-MM-DD in the Owner Time Zone; leave out for today."},` +
			`"mealId":{"type":"string","description":"The Meal eaten; leave out for a one-off."},` +
			`"name":{"type":"string","description":"A one-off's name."},` +
			`"kcalPerServing":{"type":"integer","description":"A one-off's kcal for one serving."},` +
			`"servings":{"type":"integer","description":"How many servings were eaten, 1 to 100."},` +
			`"saveAsMeal":{"type":"boolean","description":"Also save a one-off as a new Meal, to log it from later."}},` +
			`"required":["servings"]}`),
		Handle: s.logEntryTool,
	}, {
		Name: "calories_update_entry",
		Description: "Corrects an Entry: only the values sent change. Its Day must be in the Editable Window, " +
			"today and the 7 Days before it. Answers the Entry, and the Day's Consumed Calories against its Daily Limit.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"id":{"type":"string"},` +
			`"name":{"type":"string"},` +
			`"kcalPerServing":{"type":"integer"},` +
			`"servings":{"type":"integer","description":"1 to 100."}},` +
			`"required":["id"]}`),
		Handle: s.updateEntryTool,
	}, {
		Name: "calories_delete_entry",
		Description: "Deletes an Entry, for something the Owner didn't eat or that was logged twice. It can't be undone. " +
			"Its Day must be in the Editable Window, today and the 7 Days before it. " +
			"Answers the Day's Consumed Calories against its Daily Limit without it.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`),
		Destructive: true,
		Handle:      s.deleteEntryTool,
	}}
}

func (s *EntriesService) deleteEntryTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	entry, err := s.editableEntry(ctx, args.ID)
	if err != nil {
		return nil, err
	}
	if err := s.entries.Delete(ctx, entry.ID); err != nil {
		return nil, err
	}
	calories, _, err := dayCalories(ctx, s.entries, s.dailyLimits, entry.Day)
	if err != nil {
		return nil, err
	}
	return struct {
		Day model.CaloriesJSON `json:"day"`
	}{toCaloriesJSON(calories)}, nil
}

func (s *EntriesService) updateEntryTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		ID             string  `json:"id"`
		Name           *string `json:"name"`
		KcalPerServing *int    `json:"kcalPerServing"`
		Servings       *int    `json:"servings"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	entry, err := s.editableEntry(ctx, args.ID)
	if err != nil {
		return nil, err
	}
	values := model.EntryValues{Name: entry.Name, KcalPerServing: &entry.KcalPerServing, Servings: entry.Servings}
	if args.Name != nil {
		values.Name = *args.Name
	}
	if args.KcalPerServing != nil {
		values.KcalPerServing = args.KcalPerServing
	}
	if args.Servings != nil {
		values.Servings = *args.Servings
	}
	edited, err := fromEntryValues(values)
	if err != nil {
		return nil, utils.Invalid(err.Error())
	}
	if entry, err = s.edit(ctx, entry, edited); err != nil {
		return nil, err
	}
	return s.changed(ctx, entry)
}

func (s *EntriesService) logEntryTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		Day string `json:"day"`
		logRequest
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	day, err := toolDay(ctx, s.core, args.Day)
	if err != nil {
		return nil, err
	}
	entry, err := s.log(ctx, day, args.logRequest)
	if err != nil {
		return nil, err
	}
	return s.changed(ctx, entry)
}

// toolDay is the Day a Tool was sent, or today if it was sent none.
func toolDay(ctx context.Context, core coremodel.CoreService, day string) (coremodel.Day, error) {
	if day == "" {
		return core.Today(ctx)
	}
	parsed, err := coremodel.ParseDay(day)
	if err != nil {
		return "", utils.Invalid("the Day must be YYYY-MM-DD")
	}
	return parsed, nil
}

// changedJSON is what a Tool that changed an Entry answers.
type changedJSON struct {
	Entry model.EntryJSON    `json:"entry"`
	Day   model.CaloriesJSON `json:"day"`
}

// changed answers entry, and how its Day now stands against its Daily Limit.
func (s *EntriesService) changed(ctx context.Context, entry model.Entry) (changedJSON, error) {
	calories, _, err := dayCalories(ctx, s.entries, s.dailyLimits, entry.Day)
	if err != nil {
		return changedJSON{}, err
	}
	return changedJSON{Entry: utils.ToEntryJSONs([]model.Entry{entry})[0], Day: toCaloriesJSON(calories)}, nil
}
