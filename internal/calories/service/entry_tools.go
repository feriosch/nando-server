package service

import (
	"context"
	"encoding/json"

	"github.com/feriosch/nando-server/internal/calories/model"
	"github.com/feriosch/nando-server/internal/calories/utils"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

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
	if entry, err = s.edit(ctx, args.ID, edited); err != nil {
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
