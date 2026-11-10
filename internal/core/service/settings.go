package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	// The Owner Time Zone mustn't depend on the image having a zoneinfo database.
	_ "time/tzdata"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/core/utils"
)

// SettingsService handles requests about the Owner's settings, and answers
// what other services and Modules need of them: the Owner Time Zone and Today.
type SettingsService struct {
	settings model.SettingsRepository
	now      func() time.Time
}

// NewSettings returns the SettingsService, with now as its clock.
func NewSettings(settings model.SettingsRepository, now func() time.Time) *SettingsService {
	return &SettingsService{settings: settings, now: now}
}

// Routes adds the settings routes to mux, under /api/settings.
func (s *SettingsService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.putSettings)
}

// Settings returns the saved settings, or the defaults if none were saved.
func (s *SettingsService) Settings(ctx context.Context) (model.Settings, error) {
	settings, err := s.settings.Find(ctx)
	if errors.Is(err, utils.ErrNotFound) {
		return model.DefaultSettings, nil
	}
	return settings, err
}

func toSettingsJSON(settings model.Settings) model.SettingsJSON {
	var height *int
	if settings.HeightCm != 0 {
		height = &settings.HeightCm
	}
	return model.SettingsJSON{
		TimeZone: settings.TimeZone,
		HeightCm: height,
		WeighInReminder: model.ReminderJSON{
			Enabled: settings.WeighInReminder.Enabled,
			Weekday: strings.ToLower(settings.WeighInReminder.Weekday.String()),
			Time:    settings.WeighInReminder.Time,
		},
	}
}

func (s *SettingsService) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.Settings(r.Context())
	if err != nil {
		utils.ServerError(w, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, toSettingsJSON(settings))
}

// weekdays are the weekdays by their API names, such as "monday".
var weekdays = func() map[string]time.Weekday {
	m := map[string]time.Weekday{}
	for d := time.Sunday; d <= time.Saturday; d++ {
		m[strings.ToLower(d.String())] = d
	}
	return m
}()

// fromSettingsJSON turns the settings the API was sent into Settings, or
// explains what's wrong with them.
func fromSettingsJSON(j model.SettingsJSON) (model.Settings, error) {
	// "" and "Local" load too, as UTC and the server's own zone.
	if _, err := time.LoadLocation(j.TimeZone); err != nil || j.TimeZone == "" || j.TimeZone == "Local" {
		return model.Settings{}, errors.New("timeZone must be an IANA time zone such as Asia/Tokyo")
	}
	if j.HeightCm != nil && (*j.HeightCm < 1 || *j.HeightCm > 300) {
		return model.Settings{}, errors.New("heightCm must be a whole number of cm from 1 to 300, or null")
	}
	weekday, ok := weekdays[j.WeighInReminder.Weekday]
	if !ok {
		return model.Settings{}, errors.New("weighInReminder.weekday must be a weekday such as monday")
	}
	if !utils.IsHalfHour(j.WeighInReminder.Time) {
		return model.Settings{}, errors.New("weighInReminder.time must be HH:MM on the hour or half hour")
	}
	settings := model.Settings{
		TimeZone: j.TimeZone,
		WeighInReminder: model.WeighInReminder{
			Enabled: j.WeighInReminder.Enabled,
			Weekday: weekday,
			Time:    j.WeighInReminder.Time,
		},
	}
	if j.HeightCm != nil {
		settings.HeightCm = *j.HeightCm
	}
	return settings, nil
}

// putSettings replaces all of the settings.
func (s *SettingsService) putSettings(w http.ResponseWriter, r *http.Request) {
	var j model.SettingsJSON
	if err := json.NewDecoder(r.Body).Decode(&j); err != nil {
		http.Error(w, "the body isn't settings", http.StatusBadRequest)
		return
	}
	settings, err := fromSettingsJSON(j)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.settings.Save(r.Context(), settings); err != nil {
		utils.ServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// OwnerTimeZone is the time zone all of Nando's dates and clock times are in.
func (s *SettingsService) OwnerTimeZone(ctx context.Context) (*time.Location, error) {
	settings, err := s.Settings(ctx)
	if err != nil {
		return nil, err
	}
	return time.LoadLocation(settings.TimeZone)
}

// Today is the current Day in the Owner Time Zone.
func (s *SettingsService) Today(ctx context.Context) (model.Day, error) {
	loc, err := s.OwnerTimeZone(ctx)
	if err != nil {
		return "", err
	}
	return model.DayOf(s.now(), loc), nil
}
