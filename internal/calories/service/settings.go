package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/feriosch/nando-server/internal/calories/model"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

// SettingsService handles requests about the Calories Settings Section: when
// the Module's Notifications are sent.
type SettingsService struct {
	settings model.SettingsRepository
}

// NewSettings returns the SettingsService.
func NewSettings(settings model.SettingsRepository) *SettingsService {
	return &SettingsService{settings: settings}
}

// Routes adds the settings route to mux, /api/calories/settings. The web app
// only reads it: the Calories Assistant changes it, through
// calories_save_settings.
func (s *SettingsService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/calories/settings", s.getSettings)
}

// savedOrDefault returns the saved settings, or the defaults if none were saved.
func (s *SettingsService) savedOrDefault(ctx context.Context) (model.Settings, error) {
	settings, err := s.settings.Find(ctx)
	if errors.Is(err, coreutils.ErrNotFound) {
		return model.DefaultSettings, nil
	}
	return settings, err
}

func (s *SettingsService) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.savedOrDefault(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, toSettingsJSON(settings))
}

func toSettingsJSON(settings model.Settings) model.SettingsJSON {
	return model.SettingsJSON{
		NoEntriesReminder: toNotificationSettingJSON(settings.NoEntriesReminder),
		EveningSummary:    toNotificationSettingJSON(settings.EveningSummary),
	}
}

func toNotificationSettingJSON(setting model.NotificationSetting) *model.NotificationSettingJSON {
	return &model.NotificationSettingJSON{Enabled: &setting.Enabled, Time: setting.Time}
}

// Tools are what the SettingsService offers Connectors: reading and changing
// when the Calories Notifications are sent.
func (s *SettingsService) Tools() []coremodel.Tool {
	setting := `{"type":"object","properties":{` +
		`"enabled":{"type":"boolean","description":"Whether it is sent."},` +
		`"time":{"type":"string","description":"When, HH:MM on the hour or half hour, in the Owner Time Zone."}}}`
	return []coremodel.Tool{{
		Name: "calories_get_settings",
		Description: "When the Calories Notifications are sent: the no-Entries reminder (nothing logged yet today) and " +
			"the evening summary (how the Day went), each with enabled and time.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		ReadOnly:    true,
		Handle: func(ctx context.Context, _ json.RawMessage) (any, error) {
			settings, err := s.savedOrDefault(ctx)
			return toSettingsJSON(settings), err
		},
	}, {
		Name: "calories_save_settings",
		Description: "Changes when the Calories Notifications are sent. Only the Notifications given change, and within " +
			"one, only the fields given: such as noEntriesReminder {time: \"13:30\"} alone. Answers the settings.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"noEntriesReminder":` + setting + `,"eveningSummary":` + setting + `}}`),
		Handle:      s.saveSettingsTool,
	}}
}

// saveSettingsTool changes the Notification settings and fields given.
func (s *SettingsService) saveSettingsTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args model.SettingsJSON
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, errors.New("noEntriesReminder and eveningSummary must each be {enabled, time}")
	}
	if args.NoEntriesReminder == nil && args.EveningSummary == nil {
		return nil, errors.New("give noEntriesReminder, eveningSummary or both")
	}
	settings, err := s.savedOrDefault(ctx)
	if err != nil {
		return nil, err
	}
	if settings.NoEntriesReminder, err = changed("noEntriesReminder", settings.NoEntriesReminder, args.NoEntriesReminder); err != nil {
		return nil, err
	}
	if settings.EveningSummary, err = changed("eveningSummary", settings.EveningSummary, args.EveningSummary); err != nil {
		return nil, err
	}
	if err := s.settings.Save(ctx, settings); err != nil {
		return nil, err
	}
	return toSettingsJSON(settings), nil
}

// changed is setting with the fields j gives, or explains what's wrong with
// them. A nil j changes nothing.
func changed(field string, setting model.NotificationSetting, j *model.NotificationSettingJSON) (model.NotificationSetting, error) {
	if j == nil {
		return setting, nil
	}
	if j.Enabled == nil && j.Time == "" {
		return setting, errors.New(field + " must give enabled, time or both")
	}
	if j.Enabled != nil {
		setting.Enabled = *j.Enabled
	}
	if j.Time != "" {
		if !coreutils.IsHalfHour(j.Time) {
			return setting, errors.New(field + ".time must be HH:MM on the hour or half hour")
		}
		setting.Time = j.Time
	}
	return setting, nil
}
