package service

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/feriosch/nando-server/internal/calories/model"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
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
	entries, err := s.repo.ListEntries(r.Context(), day)
	if err != nil {
		serverError(w, err)
		return
	}
	list := make([]entryJSON, len(entries))
	for i, e := range entries {
		list[i] = toEntryJSON(e)
	}
	writeJSON(w, http.StatusOK, list)
}

// postEntry logs an Entry on the Day in the path: from a Meal, copying its
// name and kcal, or a one-off with its own.
func (s *Service) postEntry(w http.ResponseWriter, r *http.Request) {
	day, ok := pathDay(w, r)
	if !ok || !s.checkEditable(w, r, day) {
		return
	}
	var body struct {
		MealID         *string `json:"mealId"`
		Name           string  `json:"name"`
		KcalPerServing *int    `json:"kcalPerServing"`
		Servings       int     `json:"servings"`
		// SaveAsMeal also saves a one-off Entry as a Meal, which it then
		// belongs to.
		SaveAsMeal bool `json:"saveAsMeal"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "the body isn't an Entry", http.StatusBadRequest)
		return
	}
	if err := checkServings(body.Servings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	entry := model.Entry{Day: day, Servings: body.Servings, LoggedAt: s.now()}
	if body.MealID == nil {
		var err error
		if entry.Name, err = checkName("name", body.Name); err == nil {
			entry.KcalPerServing, err = checkKcal("kcalPerServing", body.KcalPerServing)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if body.MealID != nil {
		meal, err := s.repo.FindMeal(r.Context(), *body.MealID)
		if errors.Is(err, model.ErrNotFound) {
			http.Error(w, "there's no such Meal", http.StatusBadRequest)
			return
		} else if err != nil {
			serverError(w, err)
			return
		}
		if meal.ArchivedAt != nil {
			http.Error(w, "that Meal is archived; restore it to log it", http.StatusBadRequest)
			return
		}
		entry.MealID, entry.Name, entry.KcalPerServing = meal.ID, meal.Name, meal.Kcal
	} else if body.SaveAsMeal {
		mealID, err := s.repo.InsertMeal(r.Context(), model.Meal{
			Name:      entry.Name,
			Kcal:      entry.KcalPerServing,
			CreatedAt: s.now(),
		})
		if err != nil {
			serverError(w, err)
			return
		}
		entry.MealID = mealID
	}
	id, err := s.repo.InsertEntry(r.Context(), entry)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		ID string `json:"id"`
	}{id})
}

// pathEntry returns the Entry whose ID is in r's path if its Day is in the
// Editable Window. Otherwise it answers r and returns false.
func (s *Service) pathEntry(w http.ResponseWriter, r *http.Request) (model.Entry, bool) {
	entry, err := s.repo.FindEntry(r.Context(), r.PathValue("id"))
	if errors.Is(err, model.ErrNotFound) {
		http.Error(w, "there's no such Entry", http.StatusNotFound)
		return model.Entry{}, false
	} else if err != nil {
		serverError(w, err)
		return model.Entry{}, false
	}
	return entry, s.checkEditable(w, r, entry.Day)
}

// putEntry edits an Entry's name, kcal per serving and Servings.
func (s *Service) putEntry(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name           string `json:"name"`
		KcalPerServing *int   `json:"kcalPerServing"`
		Servings       int    `json:"servings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "the body isn't an Entry", http.StatusBadRequest)
		return
	}
	name, err := checkName("name", body.Name)
	var kcal int
	if err == nil {
		kcal, err = checkKcal("kcalPerServing", body.KcalPerServing)
	}
	if err == nil {
		err = checkServings(body.Servings)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	entry, ok := s.pathEntry(w, r)
	if !ok {
		return
	}
	entry.Name, entry.KcalPerServing, entry.Servings = name, kcal, body.Servings
	if err := s.repo.UpdateEntry(r.Context(), entry); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteEntry deletes an Entry. Deleting one that doesn't exist also succeeds.
func (s *Service) deleteEntry(w http.ResponseWriter, r *http.Request) {
	entry, err := s.repo.FindEntry(r.Context(), r.PathValue("id"))
	if errors.Is(err, model.ErrNotFound) {
		w.WriteHeader(http.StatusNoContent)
		return
	} else if err != nil {
		serverError(w, err)
		return
	}
	if !s.checkEditable(w, r, entry.Day) {
		return
	}
	if err := s.repo.DeleteEntry(r.Context(), entry.ID); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
