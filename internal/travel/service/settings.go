package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/travel/model"
)

// SettingsService handles Travel's settings. Only the Travel Assistant reads
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
	c := settings.Companion
	weekdays := make([]string, len(c.WorkWeekdays))
	for i, d := range c.WorkWeekdays {
		weekdays[i] = strings.ToLower(d.String())
	}
	return model.SettingsJSON{
		Home: &settings.Home,
		Companion: &model.CompanionJSON{
			Name:             &c.Name,
			Calendars:        &c.Calendars,
			WorkWeekdays:     &weekdays,
			HolidayCalendar:  &c.HolidayCalendar,
			UsualLeaveDays:   &c.UsualLeaveDays,
			RemoteDaysAbroad: &c.RemoteDaysAbroad,
		},
		SerpAPIMonthlyLimit: &settings.SerpAPIMonthlyLimit,
	}
}

// Tools are what the SettingsService offers Connectors: reading and changing
// Travel's settings.
func (s *SettingsService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "travel_get_settings",
		Description: "Travel's settings: home, the airport or city codes Trips leave from by default; companion, who " +
			"travels with the Owner (name, their Google Calendar IDs, workWeekdays, the holidayCalendar of public " +
			"holidays they have off, usualLeaveDays for one trip, and remoteDaysAbroad, at most 2); and " +
			"serpApiMonthlyLimit, the most Price Verdicts a month.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		ReadOnly:    true,
		Handle: func(ctx context.Context, _ json.RawMessage) (any, error) {
			settings, err := s.savedOrDefault(ctx)
			return toSettingsJSON(settings), err
		},
	}, {
		Name: "travel_save_settings",
		Description: "Changes Travel's settings. Only the fields given change, within companion too: such as " +
			"companion {remoteDaysAbroad: 1} alone. An empty companion name means there is no Companion. Answers the settings.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{` +
			`"home":{"type":"array","items":{"type":"string"},"description":"Airport or city codes, such as the 3 letters of an IATA code."},` +
			`"companion":{"type":"object","properties":{` +
			`"name":{"type":"string"},` +
			`"calendars":{"type":"array","items":{"type":"string"},"description":"Their Google Calendar IDs."},` +
			`"workWeekdays":{"type":"array","items":{"type":"string","enum":["monday","tuesday","wednesday","thursday","friday","saturday","sunday"]}},` +
			`"holidayCalendar":{"type":"string","description":"The Google Calendar ID of the public holidays they have off."},` +
			`"usualLeaveDays":{"type":"integer","description":"Days off one trip may usually take, 0 to 30."},` +
			`"remoteDaysAbroad":{"type":"integer","description":"Workdays they may work from the destination, 0 to 2."}}},` +
			`"serpApiMonthlyLimit":{"type":"integer","description":"The most Price Verdicts a month, 0 to 10000."}}}`),
		Handle: s.saveSettingsTool,
	}}
}

// saveSettingsTool changes the settings fields given.
func (s *SettingsService) saveSettingsTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args model.SettingsJSON
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, errors.New("send home as a list of codes, companion as an object and serpApiMonthlyLimit as a number")
	}
	if args.Home == nil && args.Companion == nil && args.SerpAPIMonthlyLimit == nil {
		return nil, errors.New("give home, companion, serpApiMonthlyLimit or several")
	}
	settings, err := s.savedOrDefault(ctx)
	if err != nil {
		return nil, err
	}
	if args.Home != nil {
		if err := checkCodes("home", *args.Home); err != nil {
			return nil, err
		}
		settings.Home = *args.Home
	}
	if args.Companion != nil {
		if settings.Companion, err = changedCompanion(settings.Companion, *args.Companion); err != nil {
			return nil, err
		}
	}
	if l := args.SerpAPIMonthlyLimit; l != nil {
		if *l < 0 || *l > maxSerpAPIMonthlyLimit {
			return nil, fmt.Errorf("serpApiMonthlyLimit must be 0 to %d", maxSerpAPIMonthlyLimit)
		}
		settings.SerpAPIMonthlyLimit = *l
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

// maxSerpAPIMonthlyLimit is the highest monthly limit of Price Verdicts the
// settings take, well above any SerpApi plan the Owner would pay for.
const maxSerpAPIMonthlyLimit = 10000

// maxUsualLeaveDays is the most days off one trip may usually take.
const maxUsualLeaveDays = 30

// changedCompanion is c with the fields j gives, or explains what's wrong
// with them.
func changedCompanion(c model.Companion, j model.CompanionJSON) (model.Companion, error) {
	if j == (model.CompanionJSON{}) {
		return c, errors.New("companion must give name, calendars, workWeekdays, holidayCalendar, usualLeaveDays, remoteDaysAbroad or several")
	}
	if j.Name != nil {
		c.Name = strings.TrimSpace(*j.Name)
	}
	if j.Calendars != nil {
		for _, id := range *j.Calendars {
			if strings.TrimSpace(id) == "" {
				return c, errors.New("companion.calendars must be Google Calendar IDs, none empty")
			}
		}
		c.Calendars = *j.Calendars
	}
	if j.WorkWeekdays != nil {
		days := make([]time.Weekday, 0, len(*j.WorkWeekdays))
		for _, name := range *j.WorkWeekdays {
			day, ok := weekdays[name]
			if !ok {
				return c, fmt.Errorf("companion.workWeekdays: %q isn't a weekday such as monday", name)
			}
			if slices.Contains(days, day) {
				return c, fmt.Errorf("companion.workWeekdays has %s twice", name)
			}
			days = append(days, day)
		}
		c.WorkWeekdays = days
	}
	if j.HolidayCalendar != nil {
		c.HolidayCalendar = strings.TrimSpace(*j.HolidayCalendar)
	}
	if j.UsualLeaveDays != nil {
		if *j.UsualLeaveDays < 0 || *j.UsualLeaveDays > maxUsualLeaveDays {
			return c, fmt.Errorf("companion.usualLeaveDays must be 0 to %d", maxUsualLeaveDays)
		}
		c.UsualLeaveDays = *j.UsualLeaveDays
	}
	if j.RemoteDaysAbroad != nil {
		if *j.RemoteDaysAbroad < 0 || *j.RemoteDaysAbroad > model.MaxRemoteDaysAbroad {
			return c, fmt.Errorf("companion.remoteDaysAbroad must be 0 to %d", model.MaxRemoteDaysAbroad)
		}
		c.RemoteDaysAbroad = *j.RemoteDaysAbroad
	}
	return c, nil
}

// checkCodes explains what's wrong with field's airport or city codes, if
// anything: each is 3 capital letters, and none comes twice.
func checkCodes(field string, codes []string) error {
	for i, code := range codes {
		if len(code) != 3 || strings.ToUpper(code) != code || strings.ContainsFunc(code, func(r rune) bool { return r < 'A' || r > 'Z' }) {
			return fmt.Errorf("%s: %q isn't a code of 3 capital letters, such as an IATA airport or city code", field, code)
		}
		if slices.Contains(codes[:i], code) {
			return fmt.Errorf("%s has %s twice", field, code)
		}
	}
	return nil
}
