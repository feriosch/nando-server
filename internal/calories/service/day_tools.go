package service

import (
	"context"
	"encoding/json"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

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
