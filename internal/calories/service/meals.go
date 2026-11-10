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
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

// MealsService handles requests about Meals. It reads Entries too, for the
// quick list's counts.
type MealsService struct {
	meals   model.MealsRepository
	entries model.EntriesRepository
	core    coremodel.CoreService
	now     func() time.Time
}

// NewMeals returns the MealsService, with now as its clock.
func NewMeals(meals model.MealsRepository, entries model.EntriesRepository, core coremodel.CoreService, now func() time.Time) *MealsService {
	return &MealsService{meals: meals, entries: entries, core: core, now: now}
}

// Routes adds the Meal routes to mux, under /api/calories/meals.
func (s *MealsService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/calories/meals", s.listMeals)
	mux.HandleFunc("POST /api/calories/meals", s.postMeal)
	mux.HandleFunc("GET /api/calories/meals/quick", s.quickMeals)
	mux.HandleFunc("PUT /api/calories/meals/{id}", s.putMeal)
	mux.HandleFunc("DELETE /api/calories/meals/{id}", s.deleteMeal)
	mux.HandleFunc("POST /api/calories/meals/{id}/restore", s.restoreMeal)
}

func toMealJSON(m model.Meal) model.MealJSON {
	tags := m.Tags
	if tags == nil {
		tags = []string{}
	}
	return model.MealJSON{
		ID:       m.ID,
		Name:     m.Name,
		Kcal:     m.Kcal,
		Store:    utils.OrNull(m.Store),
		Emoji:    utils.OrNull(m.Emoji),
		ImageURL: utils.OrNull(m.ImageURL),
		Tags:     tags,
		Pinned:   m.Pinned,
		Archived: m.ArchivedAt != nil,
	}
}

// listMeals lists the Meals in use by name, or with ?archived=true the
// archived ones.
func (s *MealsService) listMeals(w http.ResponseWriter, r *http.Request) {
	meals, err := s.meals.List(r.Context(), r.URL.Query().Get("archived") == "true")
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	writeMeals(w, meals)
}

// writeMeals answers 200 with meals as the API lists them.
func writeMeals(w http.ResponseWriter, meals []model.Meal) {
	list := make([]model.MealJSON, len(meals))
	for i, m := range meals {
		list[i] = toMealJSON(m)
	}
	coreutils.WriteJSON(w, http.StatusOK, list)
}

// readMeal reads and checks the Meal in r's body. On a bad body it answers
// 400 and returns false.
func readMeal(w http.ResponseWriter, r *http.Request) (model.Meal, bool) {
	var j model.MealBody
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
func fromMealBody(j model.MealBody) (model.Meal, error) {
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
	imageURL := utils.OrEmpty(j.ImageURL)
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
func (s *MealsService) postMeal(w http.ResponseWriter, r *http.Request) {
	meal, ok := readMeal(w, r)
	if !ok {
		return
	}
	meal.CreatedAt = s.now()
	id, err := s.meals.Insert(r.Context(), meal)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteCreated(w, id)
}

// putMeal edits a Meal. Entries already logged from it keep their values.
func (s *MealsService) putMeal(w http.ResponseWriter, r *http.Request) {
	meal, ok := readMeal(w, r)
	if !ok {
		return
	}
	meal.ID = r.PathValue("id")
	err := s.meals.Update(r.Context(), meal)
	if errors.Is(err, coreutils.ErrNotFound) {
		http.Error(w, "there's no such Meal", http.StatusNotFound)
		return
	} else if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteMeal archives a Meal: it leaves the lists and can't be logged, but its
// Entries stay and it can be restored.
func (s *MealsService) deleteMeal(w http.ResponseWriter, r *http.Request) {
	now := s.now()
	s.setArchivedAt(w, r, &now)
}

// restoreMeal brings an archived Meal back.
func (s *MealsService) restoreMeal(w http.ResponseWriter, r *http.Request) {
	s.setArchivedAt(w, r, nil)
}

func (s *MealsService) setArchivedAt(w http.ResponseWriter, r *http.Request, archivedAt *time.Time) {
	err := s.meals.SetArchivedAt(r.Context(), r.PathValue("id"), archivedAt)
	if errors.Is(err, coreutils.ErrNotFound) {
		http.Error(w, "there's no such Meal", http.StatusNotFound)
		return
	} else if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// quickListDays is how many Days, today included, the quick list counts
// Entries over.
const quickListDays = 30

// quickMeals lists the Meals in use for one-tap logging: pinned ones first,
// then the most-logged in the last 30 Days, then by name.
func (s *MealsService) quickMeals(w http.ResponseWriter, r *http.Request) {
	today, err := s.core.Today(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	meals, err := s.meals.List(r.Context(), false)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	counts, err := s.entries.CountByMeal(r.Context(), today.AddDays(1-quickListDays), today)
	if err != nil {
		coreutils.ServerError(w, err)
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
