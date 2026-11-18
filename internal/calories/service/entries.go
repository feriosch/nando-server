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
	if !ok || !s.CheckEditable(w, r, day) {
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
	if _, err := s.edit(r.Context(), r.PathValue("id"), edited); err != nil {
		utils.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// edit gives the Entry with id edited's name, kcal per serving and Servings,
// and returns it.
func (s *EntriesService) edit(ctx context.Context, id string, edited model.Entry) (model.Entry, error) {
	entry, err := s.editableEntry(ctx, id)
	if err != nil {
		return model.Entry{}, err
	}
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
