package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"
	// The Owner Time Zone mustn't depend on the image having a zoneinfo database.
	_ "time/tzdata"

	"github.com/feriosch/nando-server/internal/core/model"
)

// settings returns the saved settings, or the defaults if none were saved.
func (s *Service) settings(ctx context.Context) (model.Settings, error) {
	settings, err := s.repo.FindSettings(ctx)
	if errors.Is(err, model.ErrNotFound) {
		return model.DefaultSettings, nil
	}
	return settings, err
}

// settingsJSON is the settings as the API reads and writes them. HeightCm is
// a float only so that a fractional height is rejected by name.
type settingsJSON struct {
	TimeZone        string       `json:"timeZone"`
	HeightCm        *float64     `json:"heightCm"`
	WeighInReminder reminderJSON `json:"weighInReminder"`
}

type reminderJSON struct {
	Enabled bool   `json:"enabled"`
	Weekday string `json:"weekday"`
	Time    string `json:"time"`
}

func toSettingsJSON(settings model.Settings) settingsJSON {
	var height *float64
	if settings.HeightCm != 0 {
		h := float64(settings.HeightCm)
		height = &h
	}
	return settingsJSON{
		TimeZone: settings.TimeZone,
		HeightCm: height,
		WeighInReminder: reminderJSON{
			Enabled: settings.WeighInReminder.Enabled,
			Weekday: strings.ToLower(settings.WeighInReminder.Weekday.String()),
			Time:    settings.WeighInReminder.Time,
		},
	}
}

func (s *Service) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.settings(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, toSettingsJSON(settings))
}

// weekdays are the weekdays by their API names, such as "monday".
var weekdays = func() map[string]time.Weekday {
	m := map[string]time.Weekday{}
	for d := time.Sunday; d <= time.Saturday; d++ {
		m[strings.ToLower(d.String())] = d
	}
	return m
}()

// halfHour matches a clock time "HH:MM" on the hour or half hour.
var halfHour = regexp.MustCompile(`^([01][0-9]|2[0-3]):(00|30)$`)

// fromSettingsJSON turns the settings the API was sent into Settings, or
// explains what's wrong with them.
func fromSettingsJSON(j settingsJSON) (model.Settings, error) {
	// "" and "Local" load too, as UTC and the server's own zone.
	if _, err := time.LoadLocation(j.TimeZone); err != nil || j.TimeZone == "" || j.TimeZone == "Local" {
		return model.Settings{}, errors.New("timeZone must be an IANA time zone such as Asia/Tokyo")
	}
	if h := j.HeightCm; h != nil && (*h != math.Trunc(*h) || *h < 1 || *h > 300) {
		return model.Settings{}, errors.New("heightCm must be a whole number of cm from 1 to 300, or null")
	}
	weekday, ok := weekdays[j.WeighInReminder.Weekday]
	if !ok {
		return model.Settings{}, errors.New("weighInReminder.weekday must be a weekday such as monday")
	}
	if !halfHour.MatchString(j.WeighInReminder.Time) {
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
		settings.HeightCm = int(*j.HeightCm)
	}
	return settings, nil
}

// putSettings replaces all of the settings.
func (s *Service) putSettings(w http.ResponseWriter, r *http.Request) {
	var j settingsJSON
	if err := json.NewDecoder(r.Body).Decode(&j); err != nil {
		http.Error(w, "body must be settings, as GET /api/settings answers them", http.StatusBadRequest)
		return
	}
	settings, err := fromSettingsJSON(j)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.repo.SaveSettings(r.Context(), settings); err != nil {
		serverError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// OwnerTimeZone is the time zone all of Nando's dates and clock times are in.
func (s *Service) OwnerTimeZone(ctx context.Context) (*time.Location, error) {
	settings, err := s.settings(ctx)
	if err != nil {
		return nil, err
	}
	return time.LoadLocation(settings.TimeZone)
}

// Today is the current Day in the Owner Time Zone.
func (s *Service) Today(ctx context.Context) (model.Day, error) {
	loc, err := s.OwnerTimeZone(ctx)
	if err != nil {
		return "", err
	}
	return model.DayOf(s.now(), loc), nil
}
