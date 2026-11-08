package service

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/feriosch/nando-server/internal/calories/model"
	"github.com/feriosch/nando-server/internal/calories/utils"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreservice "github.com/feriosch/nando-server/internal/core/service"
)

// entryJSON is an Entry as the API lists it.
type entryJSON struct {
	ID             string        `json:"id"`
	Day            coremodel.Day `json:"day"`
	MealID         *string       `json:"mealId"`
	Name           string        `json:"name"`
	KcalPerServing int           `json:"kcalPerServing"`
	Servings       int           `json:"servings"`
	Kcal           int           `json:"kcal"`
	LoggedAt       string        `json:"loggedAt"`
}

func toEntryJSON(e model.Entry) entryJSON {
	return entryJSON{
		ID:             e.ID,
		Day:            e.Day,
		MealID:         orNull(e.MealID),
		Name:           e.Name,
		KcalPerServing: e.KcalPerServing,
		Servings:       e.Servings,
		Kcal:           e.Kcal(),
		LoggedAt:       e.LoggedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// entryValues are what the Owner sets of an Entry, as the API is sent them.
type entryValues struct {
	Name           string `json:"name"`
	KcalPerServing *int   `json:"kcalPerServing"`
	Servings       int    `json:"servings"`
}

// toEntry checks all three values and returns an Entry with them, or explains
// what's wrong.
func (v entryValues) toEntry() (model.Entry, error) {
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

// checkEditable reports whether day is in the Editable Window, today being the
// current Day in the Owner Time Zone. If it isn't, it answers 409 (or 500 if
// today can't be known) and returns false.
func (s *Service) checkEditable(w http.ResponseWriter, r *http.Request, day coremodel.Day) bool {
	today, err := s.core.Today(r.Context())
	if err != nil {
		coreservice.ServerError(w, err)
		return false
	}
	if !utils.InEditableWindow(day, today) {
		http.Error(w, "only today and the 7 Days before it can be changed", http.StatusConflict)
		return false
	}
	return true
}

// pathDay returns the Day in r's path, or answers 400 and returns false.
func pathDay(w http.ResponseWriter, r *http.Request) (coremodel.Day, bool) {
	day, err := coremodel.ParseDay(r.PathValue("day"))
	if err != nil {
		http.Error(w, "the Day must be YYYY-MM-DD", http.StatusBadRequest)
		return "", false
	}
	return day, true
}

// listEntries lists a Day's Entries in the order they were logged.
func (s *Service) listEntries(w http.ResponseWriter, r *http.Request) {
	day, ok := pathDay(w, r)
	if !ok {
		return
	}
	entries, err := s.entries.ListOnDay(r.Context(), day)
	if err != nil {
		coreservice.ServerError(w, err)
		return
	}
	list := make([]entryJSON, len(entries))
	for i, e := range entries {
		list[i] = toEntryJSON(e)
	}
	coreservice.WriteJSON(w, http.StatusOK, list)
}

// postEntry logs an Entry on the Day in the path: from a Meal, copying its
// name and kcal, or a one-off with its own.
func (s *Service) postEntry(w http.ResponseWriter, r *http.Request) {
	day, ok := pathDay(w, r)
	if !ok || !s.checkEditable(w, r, day) {
		return
	}
	var body struct {
		MealID *string `json:"mealId"`
		entryValues
		// SaveAsMeal also saves a one-off Entry as a Meal, which it then
		// belongs to.
		SaveAsMeal bool `json:"saveAsMeal"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "the body isn't an Entry", http.StatusBadRequest)
		return
	}
	var entry model.Entry
	if body.MealID != nil {
		if body.SaveAsMeal {
			http.Error(w, "saveAsMeal is only for a one-off Entry, not one logged from a Meal", http.StatusBadRequest)
			return
		}
		if entry, ok = s.entryFromMeal(w, r, *body.MealID, body.Servings); !ok {
			return
		}
	} else {
		var err error
		if entry, err = body.toEntry(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.SaveAsMeal {
			entry.MealID, err = s.meals.Insert(r.Context(), model.Meal{
				Name:      entry.Name,
				Kcal:      entry.KcalPerServing,
				CreatedAt: s.now(),
			})

			if err != nil {
				coreservice.ServerError(w, err)
				return
			}
		}
	}
	entry.Day, entry.LoggedAt = day, s.now()
	id, err := s.entries.Insert(r.Context(), entry)
	if err != nil {
		coreservice.ServerError(w, err)
		return
	}
	writeCreated(w, id)
}

// entryFromMeal returns an Entry of servings of the Meal with mealID, with
// the Meal's name and kcal. If the Meal can't be logged, it answers r and
// returns false.
func (s *Service) entryFromMeal(w http.ResponseWriter, r *http.Request, mealID string, servings int) (model.Entry, bool) {
	if err := utils.CheckServings(servings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return model.Entry{}, false
	}
	meal, err := s.meals.Find(r.Context(), mealID)
	if errors.Is(err, model.ErrNotFound) {
		http.Error(w, "there's no such Meal", http.StatusBadRequest)
		return model.Entry{}, false
	} else if err != nil {
		coreservice.ServerError(w, err)
		return model.Entry{}, false
	}
	if meal.ArchivedAt != nil {
		http.Error(w, "that Meal is archived; restore it to log it", http.StatusBadRequest)
		return model.Entry{}, false
	}
	return model.Entry{MealID: meal.ID, Name: meal.Name, KcalPerServing: meal.Kcal, Servings: servings}, true
}

// editableEntry returns the Entry whose ID is in r's path if its Day is in the
// Editable Window. Otherwise it answers r and returns false: with ifMissing if
// there's no such Entry, or 409 outside the window.
func (s *Service) editableEntry(w http.ResponseWriter, r *http.Request, ifMissing int) (model.Entry, bool) {
	entry, err := s.entries.Find(r.Context(), r.PathValue("id"))
	if errors.Is(err, model.ErrNotFound) {
		if ifMissing == http.StatusNoContent {
			w.WriteHeader(ifMissing)
		} else {
			http.Error(w, "there's no such Entry", ifMissing)
		}
		return model.Entry{}, false
	} else if err != nil {
		coreservice.ServerError(w, err)
		return model.Entry{}, false
	}
	return entry, s.checkEditable(w, r, entry.Day)
}

// putEntry edits an Entry's name, kcal per serving and Servings.
func (s *Service) putEntry(w http.ResponseWriter, r *http.Request) {
	var body entryValues
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "the body isn't an Entry", http.StatusBadRequest)
		return
	}
	edited, err := body.toEntry()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	entry, ok := s.editableEntry(w, r, http.StatusNotFound)
	if !ok {
		return
	}
	entry.Name, entry.KcalPerServing, entry.Servings = edited.Name, edited.KcalPerServing, edited.Servings
	if err := s.entries.Update(r.Context(), entry); err != nil {
		coreservice.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteEntry deletes an Entry. Deleting one that doesn't exist also succeeds.
func (s *Service) deleteEntry(w http.ResponseWriter, r *http.Request) {
	entry, ok := s.editableEntry(w, r, http.StatusNoContent)
	if !ok {
		return
	}
	if err := s.entries.Delete(r.Context(), entry.ID); err != nil {
		coreservice.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
