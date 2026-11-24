package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/money/model"
	"github.com/feriosch/nando-server/internal/money/utils"
)

// SettingsService handles Money's settings. Only the Money Assistant reads
// and changes them, through its Tools.
type SettingsService struct {
	settings model.SettingsRepository
	// accounts checks the Accounts reminder groups name.
	accounts model.AccountsRepository
}

// NewSettings returns the SettingsService.
func NewSettings(settings model.SettingsRepository, accounts model.AccountsRepository) *SettingsService {
	return &SettingsService{settings: settings, accounts: accounts}
}

// savedOrDefault returns the saved settings, or the defaults if none were saved.
func (s *SettingsService) savedOrDefault(ctx context.Context) (model.Settings, error) {
	settings, err := s.settings.Find(ctx)
	if errors.Is(err, coreutils.ErrNotFound) {
		return model.DefaultSettings(), nil
	}
	if err == nil && settings.ReminderTime == "" {
		// Saved before Statement Reminders: none yet, at the default time.
		settings.ReminderTime, settings.Reminders = model.DefaultSettings().ReminderTime, []model.ReminderGroup{}
	}
	return settings, err
}

func toSettingsJSON(settings model.Settings) model.SettingsJSON {
	groups := make([]model.ReminderGroupJSON, len(settings.Reminders))
	for i, g := range settings.Reminders {
		groups[i] = model.ReminderGroupJSON{Name: g.Name, Accounts: g.AccountIDs, Day: g.Day}
	}
	marks := map[string]*bool{}
	for key, mark := range settings.EssentialByCategory {
		marks[key] = &mark
	}
	return model.SettingsJSON{
		AskAbove:  map[string]int{"JPY": settings.AskAbove.JPY, "MXN": settings.AskAbove.MXN},
		Reminders: &groups, ReminderTime: &settings.ReminderTime, EssentialByCategory: marks,
	}
}

// Tools are what the SettingsService offers Connectors: reading and changing
// Money's settings.
func (s *SettingsService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "money_get_settings",
		Description: "Money's settings: askAbove, per currency (JPY, MXN), the amount above which a Transaction you " +
			"can't tell is asked about (below it, keep your best guess, Unconfirmed); reminders, the Statement " +
			"Reminders, one per app the Owner opens {name, accounts, day of the month}; and reminderTime, when " +
			"reminders and Monthly Review asks are sent; and essentialByCategory, each expense Category's default mark, " +
			"Essential (true) or Optional (false), for what has no Subcategory there.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		ReadOnly:    true,
		Handle: func(ctx context.Context, _ json.RawMessage) (any, error) {
			settings, err := s.savedOrDefault(ctx)
			return toSettingsJSON(settings), err
		},
	}, {
		Name: "money_save_settings",
		Description: "Changes Money's settings. Only the fields given change, within askAbove too: such as " +
			"askAbove {JPY: 5000} alone. reminders, when given, replaces the whole list: each Account in at most one " +
			"group, open, and each day 1 to 28. Answers the settings.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"askAbove":{"type":"object","properties":{` +
			`"JPY":{"type":"integer","description":"Whole yen, above 0."},` +
			`"MXN":{"type":"integer","description":"Whole pesos, above 0."}}},` +
			`"reminders":{"type":"array","items":{"type":"object","properties":{` +
			`"name":{"type":"string","description":"The app the Owner opens, such as a bank's."},` +
			`"accounts":{"type":"array","items":{"type":"string"},"description":"The ids of the Accounts whose Statements it holds."},` +
			`"day":{"type":"integer","description":"Day of the month, 1 to 28: the day after a card closes, or the 1st."}},` +
			`"required":["name","accounts","day"]}},` +
			`"reminderTime":{"type":"string","description":"HH:MM on the hour or half hour, in the Owner Time Zone."},` +
			`"essentialByCategory":{"type":"object","additionalProperties":{"type":["boolean","null"]},` +
			`"description":"By expense Category key: true Essential, false Optional, null to remove its mark. Only the Categories sent change."}}}`),
		Handle: s.saveSettingsTool,
	}}
}

// saveSettingsTool changes the settings fields given.
func (s *SettingsService) saveSettingsTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args model.SettingsJSON
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, utils.Refusal{Message: "send askAbove as an object of whole amounts by currency, such as {\"JPY\": 3000}"}
	}
	if len(args.AskAbove) == 0 && args.Reminders == nil && args.ReminderTime == nil && len(args.EssentialByCategory) == 0 {
		return nil, utils.Refusal{Message: "give askAbove, reminders, reminderTime, essentialByCategory or several"}
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
	if len(args.EssentialByCategory) > 0 {
		marks := map[string]bool{}
		for key, mark := range settings.EssentialByCategory {
			marks[key] = mark
		}
		for key, mark := range args.EssentialByCategory {
			category, ok := model.CategoryOf(key)
			if !ok || category.Kind != model.Expense {
				return nil, utils.Refusal{Message: fmt.Sprintf("essentialByCategory: %q isn't an expense Category's key, from money_list_categories", key)}
			}
			if mark == nil {
				delete(marks, key)
			} else {
				marks[key] = *mark
			}
		}
		settings.EssentialByCategory = marks
	}
	if args.ReminderTime != nil {
		if !coreutils.IsHalfHour(*args.ReminderTime) {
			return nil, utils.Refusal{Message: "reminderTime must be HH:MM on the hour or half hour"}
		}
		settings.ReminderTime = *args.ReminderTime
	}
	if args.Reminders != nil {
		groups, err := s.checkReminders(ctx, *args.Reminders)
		if err != nil {
			return nil, err
		}
		settings.Reminders = groups
	}
	if err := s.settings.Save(ctx, settings); err != nil {
		return nil, err
	}
	return toSettingsJSON(settings), nil
}

// checkReminders is the reminder groups given, or a Refusal saying what is
// wrong with them.
func (s *SettingsService) checkReminders(ctx context.Context, given []model.ReminderGroupJSON) ([]model.ReminderGroup, error) {
	groups := make([]model.ReminderGroup, len(given))
	seen := map[string]string{}
	names := map[string]bool{}
	for i, g := range given {
		name := strings.TrimSpace(g.Name)
		if name == "" {
			return nil, utils.Refusal{Message: fmt.Sprintf("reminders[%d] needs a name", i)}
		}
		// A reminder's Notification is named after it, so two of one name
		// would let only one through.
		if names[strings.ToLower(name)] {
			return nil, utils.Refusal{Message: fmt.Sprintf("%q is used twice: give each reminder its own name", name)}
		}
		names[strings.ToLower(name)] = true
		if g.Day < 1 || g.Day > 28 {
			return nil, utils.Refusal{Message: fmt.Sprintf("%s's day must be 1 to 28, so that every month has it", name)}
		}
		for _, id := range g.Accounts {
			account, err := s.accounts.Find(ctx, id)
			if errors.Is(err, coreutils.ErrNotFound) {
				return nil, utils.Refusal{Message: fmt.Sprintf("%s: there's no such Account %q", name, id)}
			}
			if err != nil {
				return nil, err
			}
			if account.ClosedFrom != "" {
				return nil, utils.Refusal{Message: fmt.Sprintf("%s: %s is closed", name, account.Name)}
			}
			if other, ok := seen[id]; ok {
				return nil, utils.Refusal{Message: fmt.Sprintf("%s is in two reminders, %s and %s", account.Name, other, name)}
			}
			seen[id] = name
		}
		groups[i] = model.ReminderGroup{Name: name, AccountIDs: g.Accounts, Day: g.Day}
	}
	return groups, nil
}
