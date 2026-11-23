package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/money/model"
	"github.com/feriosch/nando-server/internal/money/utils"
)

// SettingsService handles Money's settings. Only the Money Assistant reads
// and changes them, through its Tools.
type SettingsService struct {
	settings model.SettingsRepository
}

// NewSettings returns the SettingsService.
func NewSettings(settings model.SettingsRepository) *SettingsService {
	return &SettingsService{settings: settings}
}

// savedOrDefault returns the saved settings, or the defaults if none were saved.
func (s *SettingsService) savedOrDefault(ctx context.Context) (model.Settings, error) {
	settings, err := s.settings.Find(ctx)
	if errors.Is(err, coreutils.ErrNotFound) {
		return model.DefaultSettings(), nil
	}
	return settings, err
}

func toSettingsJSON(settings model.Settings) model.SettingsJSON {
	return model.SettingsJSON{AskAbove: map[string]int{"JPY": settings.AskAbove.JPY, "MXN": settings.AskAbove.MXN}}
}

// Tools are what the SettingsService offers Connectors: reading and changing
// Money's settings.
func (s *SettingsService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "money_get_settings",
		Description: "Money's settings: askAbove, per currency (JPY, MXN), the amount above which a Transaction you " +
			"can't tell is asked about; below it, keep your best guess, Unconfirmed.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		ReadOnly:    true,
		Handle: func(ctx context.Context, _ json.RawMessage) (any, error) {
			settings, err := s.savedOrDefault(ctx)
			return toSettingsJSON(settings), err
		},
	}, {
		Name: "money_save_settings",
		Description: "Changes Money's settings. Only the fields given change, within askAbove too: such as " +
			"askAbove {JPY: 5000} alone. Answers the settings.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"askAbove":{"type":"object","properties":{` +
			`"JPY":{"type":"integer","description":"Whole yen, above 0."},` +
			`"MXN":{"type":"integer","description":"Whole pesos, above 0."}}}}}`),
		Handle: s.saveSettingsTool,
	}}
}

// saveSettingsTool changes the settings fields given.
func (s *SettingsService) saveSettingsTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args model.SettingsJSON
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, utils.Refusal{Message: "send askAbove as an object of whole amounts by currency, such as {\"JPY\": 3000}"}
	}
	if len(args.AskAbove) == 0 {
		return nil, utils.Refusal{Message: "give askAbove, with JPY, MXN or both"}
	}
	settings, err := s.savedOrDefault(ctx)
	if err != nil {
		return nil, err
	}
	for currency, amount := range args.AskAbove {
		var field *int
		switch currency {
		case "JPY":
			field = &settings.AskAbove.JPY
		case "MXN":
			field = &settings.AskAbove.MXN
		default:
			return nil, utils.Refusal{Message: fmt.Sprintf("askAbove: %q isn't one of the Owner's currencies, JPY or MXN", currency)}
		}
		if amount <= 0 {
			return nil, utils.Refusal{Message: fmt.Sprintf("askAbove.%s must be a whole amount above 0", currency)}
		}
		*field = amount
	}
	if err := s.settings.Save(ctx, settings); err != nil {
		return nil, err
	}
	return toSettingsJSON(settings), nil
}
