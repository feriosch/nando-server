package service

import (
	"context"
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

// MealsService handles requests and Tools about Meals. It reads Entries too, for the
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
	coreutils.WriteJSON(w, http.StatusOK, toMealJSONs(meals))
}

func toMealJSONs(meals []model.Meal) []model.MealJSON {
	list := make([]model.MealJSON, len(meals))
	for i, m := range meals {
		list[i] = toMealJSON(m)
	}
	return list
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

// quickListDays is how many Days, today included, the quick list counts
// Entries over.
const quickListDays = 30

// quick returns the Meals in use in the quick list's order.
func (s *MealsService) quick(ctx context.Context) ([]model.Meal, error) {
	today, err := s.core.Today(ctx)
	if err != nil {
		return nil, err
	}
	meals, err := s.meals.List(ctx, false)
	if err != nil {
		return nil, err
	}
	counts, err := s.entries.CountByMeal(ctx, today.AddDays(1-quickListDays), today)
	if err != nil {
		return nil, err
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
	return meals, nil
}

// Tools are what the MealsService offers Connectors.
func (s *MealsService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "calories_list_meals",
		Description: "The Owner's Meals: what they eat repeatedly, saved with its kcal per serving, store and Meal Tags, " +
			"to log in one go. Those in use come as the Nando app's quick list has them: pinned first, then the most logged " +
			"in the last 30 Days. With archived, the archived ones instead, which can't be logged until restored.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"archived":{"type":"boolean","description":"List the archived Meals instead of those in use."}}}`),
		ReadOnly: true,
		Handle:   s.listMealsTool,
	}, {
		Name: "calories_save_meal",
		Description: "Saves something the Owner eats repeatedly as a new Meal, with its kcal per serving, " +
			"and optionally the store it comes from, one emoji and free-form Meal Tags such as \"snack\". " +
			"With id, changes that Meal instead: only the fields sent change, and tags replaces all its Meal Tags. " +
			"Entries already logged from it keep their name and kcal. Answers the Meal.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"id":{"type":"string","description":"The Meal to change; leave out to save a new one."},` +
			`"name":{"type":"string","description":"1 to 100 characters."},` +
			`"kcal":{"type":"integer","description":"Per serving, 0 to 10,000."},` +
			`"store":{"type":"string"},` +
			`"emoji":{"type":"string"},` +
			`"tags":{"type":"array","items":{"type":"string"},"description":"Meal Tags, each 1 to 30 characters."},` +
			`"pinned":{"type":"boolean","description":"Pinned Meals lead the quick list."}},` +
			`"required":[]}`),
		Handle: s.saveMealTool,
	}, {
		Name: "calories_archive_meal",
		Description: "Archives a Meal the Owner no longer eats: it leaves the lists and can't be logged, " +
			"but the Entries logged from it stay. calories_restore_meal brings it back. Answers the Meal.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`),
		Destructive: true,
		Handle:      s.archiveMealTool,
	}, {
		Name:        "calories_restore_meal",
		Description: "Brings an archived Meal back into use, to be logged again. Answers the Meal.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`),
		Handle:      s.restoreMealTool,
	}}
}

func (s *MealsService) archiveMealTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	now := s.now()
	return s.setArchivedAtTool(ctx, arguments, &now)
}

func (s *MealsService) restoreMealTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	return s.setArchivedAtTool(ctx, arguments, nil)
}

// setArchivedAtTool archives the Meal a Tool was sent at archivedAt, or with
// nil restores it, and answers the Meal.
func (s *MealsService) setArchivedAtTool(ctx context.Context, arguments json.RawMessage, archivedAt *time.Time) (any, error) {
	var args struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	err := s.meals.SetArchivedAt(ctx, args.ID, archivedAt)
	if errors.Is(err, coreutils.ErrNotFound) {
		return nil, utils.Invalid("there's no such Meal")
	} else if err != nil {
		return nil, err
	}
	meal, err := s.meals.Find(ctx, args.ID)
	if err != nil {
		return nil, err
	}
	return toMealJSON(meal), nil
}

func (s *MealsService) saveMealTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		ID     string    `json:"id"`
		Name   *string   `json:"name"`
		Kcal   *int      `json:"kcal"`
		Store  *string   `json:"store"`
		Emoji  *string   `json:"emoji"`
		Tags   *[]string `json:"tags"`
		Pinned *bool     `json:"pinned"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	// A new Meal starts empty; a saved one, as it is.
	var saved model.Meal
	if args.ID != "" {
		var err error
		saved, err = s.meals.Find(ctx, args.ID)
		if errors.Is(err, coreutils.ErrNotFound) {
			return nil, utils.Invalid("there's no such Meal")
		} else if err != nil {
			return nil, err
		}
	}
	body := model.MealBody{
		Name: saved.Name, Store: utils.OrNull(saved.Store), Emoji: utils.OrNull(saved.Emoji),
		ImageURL: utils.OrNull(saved.ImageURL), Tags: saved.Tags, Pinned: saved.Pinned,
	}
	if args.ID != "" {
		body.Kcal = &saved.Kcal
	}
	if args.Name != nil {
		body.Name = *args.Name
	}
	if args.Kcal != nil {
		body.Kcal = args.Kcal
	}
	if args.Store != nil {
		body.Store = args.Store
	}
	if args.Emoji != nil {
		body.Emoji = args.Emoji
	}
	if args.Tags != nil {
		body.Tags = *args.Tags
	}
	if args.Pinned != nil {
		body.Pinned = *args.Pinned
	}
	meal, err := fromMealBody(body)
	if err != nil {
		return nil, utils.Invalid(err.Error())
	}
	meal.ID, meal.CreatedAt, meal.ArchivedAt = saved.ID, saved.CreatedAt, saved.ArchivedAt
	if meal.ID == "" {
		meal.CreatedAt = s.now()
		meal.ID, err = s.meals.Insert(ctx, meal)
	} else {
		err = s.meals.Update(ctx, meal)
	}
	if err != nil {
		return nil, err
	}
	return toMealJSON(meal), nil
}

func (s *MealsService) listMealsTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		Archived bool `json:"archived"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, err
	}
	var meals []model.Meal
	var err error
	if args.Archived {
		meals, err = s.meals.List(ctx, true)
	} else {
		meals, err = s.quick(ctx)
	}
	if err != nil {
		return nil, err
	}
	return struct {
		Meals []model.MealJSON `json:"meals"`
	}{toMealJSONs(meals)}, nil
}
