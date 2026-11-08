package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/feriosch/nando-server/internal/calories/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
)

// SettingsService handles requests about the Calories settings: when its
// Notifications are sent.
type SettingsService struct {
	settings model.SettingsRepository
}

// NewSettings returns the SettingsService.
func NewSettings(settings model.SettingsRepository) *SettingsService {
	return &SettingsService{settings: settings}
}

// Routes adds the settings routes to mux, under /api/calories/settings.
func (s *SettingsService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/calories/settings", s.getSettings)
	mux.HandleFunc("PUT /api/calories/settings", s.putSettings)
}

// current returns the saved settings, or the defaults if none were saved.
func (s *SettingsService) current(ctx context.Context) (model.Settings, error) {
	settings, err := s.settings.Find(ctx)
	if errors.Is(err, coreutils.ErrNotFound) {
		return model.DefaultSettings, nil
	}
	return settings, err
}

// settingsJSON is the settings as the API reads and writes them. A missing
// Notification setting is nil, so it can be rejected.
type settingsJSON struct {
	NoEntriesReminder *notificationSettingJSON `json:"noEntriesReminder"`
	EveningSummary    *notificationSettingJSON `json:"eveningSummary"`
}

type notificationSettingJSON struct {
	Enabled bool   `json:"enabled"`
	Time    string `json:"time"`
}

func (s *SettingsService) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.current(r.Context())
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, settingsJSON{
		NoEntriesReminder: &notificationSettingJSON{Enabled: settings.NoEntriesReminder.Enabled, Time: settings.NoEntriesReminder.Time},
		EveningSummary:    &notificationSettingJSON{Enabled: settings.EveningSummary.Enabled, Time: settings.EveningSummary.Time},
	})
}

// fromNotificationSettingJSON checks the Notification setting field was sent,
// or explains what's wrong with it.
func fromNotificationSettingJSON(field string, j *notificationSettingJSON) (model.NotificationSetting, error) {
	if j == nil {
		return model.NotificationSetting{}, errors.New(field + " is missing")
	}
	if !coreutils.IsHalfHour(j.Time) {
		return model.NotificationSetting{}, errors.New(field + ".time must be HH:MM on the hour or half hour")
	}
	return model.NotificationSetting{Enabled: j.Enabled, Time: j.Time}, nil
}

// putSettings replaces all of the settings.
func (s *SettingsService) putSettings(w http.ResponseWriter, r *http.Request) {
	var j settingsJSON
	if err := json.NewDecoder(r.Body).Decode(&j); err != nil {
		http.Error(w, "the body isn't settings", http.StatusBadRequest)
		return
	}
	var settings model.Settings
	var err error
	if settings.NoEntriesReminder, err = fromNotificationSettingJSON("noEntriesReminder", j.NoEntriesReminder); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if settings.EveningSummary, err = fromNotificationSettingJSON("eveningSummary", j.EveningSummary); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.settings.Save(r.Context(), settings); err != nil {
		coreutils.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
