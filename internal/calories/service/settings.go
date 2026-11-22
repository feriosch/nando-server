package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/feriosch/nando-server/internal/calories/model"
	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

// SettingsService handles requests about when the Module's Notifications
// are sent.
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
	if err == nil && settings.WeighInReminder.Time == "" {
		// Saved before the Weigh-in reminder moved here: it is off, at its default time.
		settings.WeighInReminder = model.DefaultSettings.WeighInReminder
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
		WeighInReminder: &model.WeighInReminderJSON{
			Enabled: &settings.WeighInReminder.Enabled,
			Weekday: strings.ToLower(settings.WeighInReminder.Weekday.String()),
			Time:    settings.WeighInReminder.Time,
		},
	}
}

func toNotificationSettingJSON(setting model.NotificationSetting) *model.NotificationSettingJSON {
	return &model.NotificationSettingJSON{Enabled: &setting.Enabled, Time: setting.Time}
}

// takeWeighInReminder saves reminder, the Weigh-in reminder Core kept until
// ADR 0012, unless the settings already have one of their own.
func (s *SettingsService) takeWeighInReminder(ctx context.Context, reminder coremodel.WeighInReminder) error {
	settings, err := s.settings.Find(ctx)
	switch {
	case errors.Is(err, coreutils.ErrNotFound):
		settings = model.DefaultSettings
	case err != nil:
		return err
	case settings.WeighInReminder.Time != "":
		return nil
	}
	settings.WeighInReminder = model.WeighInReminder{Enabled: reminder.Enabled, Weekday: reminder.Weekday, Time: reminder.Time}
	return s.settings.Save(ctx, settings)
}

// Tools are what the SettingsService offers Connectors: reading and changing
// when the Calories Notifications are sent.
func (s *SettingsService) Tools() []coremodel.Tool {
	setting := `{"type":"object","properties":{` +
		`"enabled":{"type":"boolean","description":"Whether it is sent."},` +
		`"time":{"type":"string","description":"When, HH:MM on the hour or half hour, in the Owner Time Zone."}}}`
	weekly := `{"type":"object","properties":{` +
		`"enabled":{"type":"boolean","description":"Whether it is sent."},` +
		`"weekday":{"type":"string","enum":["monday","tuesday","wednesday","thursday","friday","saturday","sunday"],` +
		`"description":"Which day of the week."},` +
		`"time":{"type":"string","description":"When, HH:MM on the hour or half hour, in the Owner Time Zone."}}}`
	return []coremodel.Tool{{
		Name: "calories_get_settings",
		Description: "When the Calories Notifications are sent: the no-Entries reminder (nothing logged yet today) and " +
			"the evening summary (how the Day went), each with enabled and time, and the weekly Weigh-in reminder, " +
			"with enabled, weekday and time.",
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
		InputSchema: json.RawMessage(`{"type":"object","properties":{"noEntriesReminder":` + setting +
			`,"eveningSummary":` + setting + `,"weighInReminder":` + weekly + `}}`),
		Handle: s.saveSettingsTool,
	}}
}

// saveSettingsTool changes the Notification settings and fields given.
func (s *SettingsService) saveSettingsTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args model.SettingsJSON
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, errors.New("noEntriesReminder and eveningSummary must each be {enabled, time}, and weighInReminder {enabled, weekday, time}")
	}
	if args.NoEntriesReminder == nil && args.EveningSummary == nil && args.WeighInReminder == nil {
		return nil, errors.New("give noEntriesReminder, eveningSummary, weighInReminder or several")
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
	if settings.WeighInReminder, err = changedWeekly(settings.WeighInReminder, args.WeighInReminder); err != nil {
		return nil, err
	}
	if err := s.settings.Save(ctx, settings); err != nil {
		return nil, err
	}
	return toSettingsJSON(settings), nil
}

// weekdays are the weekdays by their API names, such as "monday".
var weekdays = func() map[string]time.Weekday {
	m := map[string]time.Weekday{}
	for d := time.Sunday; d <= time.Saturday; d++ {
		m[strings.ToLower(d.String())] = d
	}
	return m
}()

// changedWeekly is the Weigh-in reminder with the fields j gives, or
// explains what's wrong with them. A nil j changes nothing.
func changedWeekly(reminder model.WeighInReminder, j *model.WeighInReminderJSON) (model.WeighInReminder, error) {
	if j == nil {
		return reminder, nil
	}
	if j.Enabled == nil && j.Weekday == "" && j.Time == "" {
		return reminder, errors.New("weighInReminder must give enabled, weekday, time or several")
	}
	if j.Enabled != nil {
		reminder.Enabled = *j.Enabled
	}
	if j.Weekday != "" {
		weekday, ok := weekdays[j.Weekday]
		if !ok {
			return reminder, errors.New("weighInReminder.weekday must be a weekday such as monday")
		}
		reminder.Weekday = weekday
	}
	if j.Time != "" {
		if !coreutils.IsHalfHour(j.Time) {
			return reminder, errors.New("weighInReminder.time must be HH:MM on the hour or half hour")
		}
		reminder.Time = j.Time
	}
	return reminder, nil
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
