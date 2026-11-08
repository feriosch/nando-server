package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/feriosch/nando-server/internal/calories/model"
	"github.com/feriosch/nando-server/internal/calories/utils"
	coreservice "github.com/feriosch/nando-server/internal/core/service"
)

// mealJSON is a Meal as the API reads and writes it. Optional strings are
// null when not set.
type mealJSON struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Kcal     int      `json:"kcal"`
	Store    *string  `json:"store"`
	Emoji    *string  `json:"emoji"`
	ImageURL *string  `json:"imageUrl"`
	Tags     []string `json:"tags"`
	Pinned   bool     `json:"pinned"`
	Archived bool     `json:"archived"`
}

// orNull is s, or nil if s is "".
func orNull(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// orEmpty is *s, or "" if s is nil.
func orEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func toMealJSON(m model.Meal) mealJSON {
	tags := m.Tags
	if tags == nil {
		tags = []string{}
	}
	return mealJSON{
		ID:       m.ID,
		Name:     m.Name,
		Kcal:     m.Kcal,
		Store:    orNull(m.Store),
		Emoji:    orNull(m.Emoji),
		ImageURL: orNull(m.ImageURL),
		Tags:     tags,
		Pinned:   m.Pinned,
		Archived: m.ArchivedAt != nil,
	}
}

// listMeals lists the Meals in use by name, or with ?archived=true the
// archived ones.
func (s *Service) listMeals(w http.ResponseWriter, r *http.Request) {
	meals, err := s.meals.List(r.Context(), r.URL.Query().Get("archived") == "true")
	if err != nil {
		coreservice.ServerError(w, err)
		return
	}
	writeMeals(w, meals)
}

// writeMeals answers 200 with meals as the API lists them.
func writeMeals(w http.ResponseWriter, meals []model.Meal) {
	list := make([]mealJSON, len(meals))
	for i, m := range meals {
		list[i] = toMealJSON(m)
	}
	coreservice.WriteJSON(w, http.StatusOK, list)
}

// mealBody is a Meal as the API is sent it: what the Owner can set of one.
type mealBody struct {
	Name     string   `json:"name"`
	Kcal     *int     `json:"kcal"`
	Store    *string  `json:"store"`
	Emoji    *string  `json:"emoji"`
	ImageURL *string  `json:"imageUrl"`
	Tags     []string `json:"tags"`
	Pinned   bool     `json:"pinned"`
}

// readMeal reads and checks the Meal in r's body. On a bad body it answers
// 400 and returns false.
func readMeal(w http.ResponseWriter, r *http.Request) (model.Meal, bool) {
	var j mealBody
	if err := json.NewDecoder(r.Body).Decode(&j); err != nil {
		http.Error(w, "the body isn't a Meal", http.StatusBadRequest)
		return model.Meal{}, false
	}
	meal, err := fromMealBody(j)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return model.Meal{}, false
	}
	return meal, true
}

// fromMealBody turns the Meal the API was sent into a Meal, or explains what's
// wrong with it.
func fromMealBody(j mealBody) (model.Meal, error) {
	name, err := utils.CheckName("name", j.Name)
	if err != nil {
		return model.Meal{}, err
	}
	kcal, err := utils.CheckKcal("kcal", j.Kcal)
	if err != nil {
		return model.Meal{}, err
	}
	store, err := utils.CheckOptional("store", j.Store, utils.MaxNameLength)
	if err != nil {
		return model.Meal{}, err
	}
	emoji, err := utils.CheckOptional("emoji", j.Emoji, utils.MaxEmojiLength)
	if err != nil {
		return model.Meal{}, err
	}
	imageURL := orEmpty(j.ImageURL)
	if u, err := url.Parse(imageURL); imageURL != "" && (err != nil || u.Scheme != "https" || u.Host == "") {
		return model.Meal{}, errors.New("imageUrl must be an https: URL, or null")
	}
	tags, err := utils.CheckTags(j.Tags)
	if err != nil {
		return model.Meal{}, err
	}
	return model.Meal{
		Name:     name,
		Kcal:     kcal,
		Store:    store,
		Emoji:    emoji,
		ImageURL: imageURL,
		Tags:     tags,
		Pinned:   j.Pinned,
	}, nil
}

// postMeal saves a new Meal.
func (s *Service) postMeal(w http.ResponseWriter, r *http.Request) {
	meal, ok := readMeal(w, r)
	if !ok {
		return
	}
	meal.CreatedAt = s.now()
	id, err := s.meals.Insert(r.Context(), meal)
	if err != nil {
		coreservice.ServerError(w, err)
		return
	}
	writeCreated(w, id)
}

// putMeal edits a Meal. Entries already logged from it keep their values.
func (s *Service) putMeal(w http.ResponseWriter, r *http.Request) {
	meal, ok := readMeal(w, r)
	if !ok {
		return
	}
	meal.ID = r.PathValue("id")
	err := s.meals.Update(r.Context(), meal)
	if errors.Is(err, model.ErrNotFound) {
		http.Error(w, "there's no such Meal", http.StatusNotFound)
		return
	} else if err != nil {
		coreservice.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteMeal archives a Meal: it leaves the lists and can't be logged, but its
// Entries stay and it can be restored.
func (s *Service) deleteMeal(w http.ResponseWriter, r *http.Request) {
	now := s.now()
	s.setArchivedAt(w, r, &now)
}

// restoreMeal brings an archived Meal back.
func (s *Service) restoreMeal(w http.ResponseWriter, r *http.Request) {
	s.setArchivedAt(w, r, nil)
}

func (s *Service) setArchivedAt(w http.ResponseWriter, r *http.Request, archivedAt *time.Time) {
	err := s.meals.SetArchivedAt(r.Context(), r.PathValue("id"), archivedAt)
	if errors.Is(err, model.ErrNotFound) {
		http.Error(w, "there's no such Meal", http.StatusNotFound)
		return
	} else if err != nil {
		coreservice.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// quickListDays is how many Days, today included, the quick list counts
// Entries over.
const quickListDays = 30

// quickMeals lists the Meals in use for one-tap logging: pinned ones first,
// then the most-logged in the last 30 Days, then by name.
func (s *Service) quickMeals(w http.ResponseWriter, r *http.Request) {
	today, err := s.core.Today(r.Context())
	if err != nil {
		coreservice.ServerError(w, err)
		return
	}
	meals, err := s.meals.List(r.Context(), false)
	if err != nil {
		coreservice.ServerError(w, err)
		return
	}
	counts, err := s.entries.CountByMeal(r.Context(), today.AddDays(1-quickListDays), today)
	if err != nil {
		coreservice.ServerError(w, err)
		return
	}
	// meals are by name, which a stable sort keeps among equals.
	slices.SortStableFunc(meals, func(a, b model.Meal) int {
		if a.Pinned != b.Pinned {
			if a.Pinned {
				return -1
			}
			return 1
		}
		return counts[b.ID] - counts[a.ID]
	})
	writeMeals(w, meals)
}
