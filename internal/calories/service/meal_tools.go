package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/feriosch/nando-server/internal/calories/model"
	"github.com/feriosch/nando-server/internal/calories/utils"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

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
		Name: saved.Name, Kcal: &saved.Kcal, Store: utils.OrNull(saved.Store), Emoji: utils.OrNull(saved.Emoji),
		ImageURL: utils.OrNull(saved.ImageURL), Tags: saved.Tags, Pinned: saved.Pinned,
	}
	if args.ID == "" {
		body.Kcal = nil
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
