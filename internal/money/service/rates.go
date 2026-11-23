package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"time"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/money/model"
	"github.com/feriosch/nando-server/internal/money/utils"
)

// RatesService fetches each month's Rate from Frankfurter, which publishes
// the European Central Bank's reference rates and needs no key, and keeps it
// once the month has ended (ADR 0014). It calls an outside service, never a
// model.
type RatesService struct {
	rates model.RatesRepository
	core  coremodel.CoreService
	// url is Frankfurter's API, whose time series is at /v1/<from>..<to>.
	url  string
	http *http.Client
}

// NewRates returns the RatesService over Frankfurter at url.
func NewRates(rates model.RatesRepository, core coremodel.CoreService, url string) *RatesService {
	return &RatesService{rates: rates, core: core, url: url, http: &http.Client{Timeout: 20 * time.Second}}
}

// RateJSON is a month's Rate, pesos in yen, as the Tools answer it.
type RateJSON struct {
	Month    string  `json:"month"`
	Average  float64 `json:"average"`
	MonthEnd float64 `json:"monthEnd"`
	// Provisional is true for a month that hasn't ended: the rates so far,
	// fetched again until it ends.
	Provisional bool `json:"provisional,omitempty"`
}

// Rate is month's Rate: the stored one, or else fetched, and stored once the
// month has ended.
func (s *RatesService) Rate(ctx context.Context, month string) (RateJSON, error) {
	rate, err := s.rates.Find(ctx, month)
	if err == nil {
		return RateJSON{Month: rate.Month, Average: rate.Average, MonthEnd: rate.MonthEnd}, nil
	}
	if !errors.Is(err, coreutils.ErrNotFound) {
		return RateJSON{}, err
	}
	today, err := s.core.Today(ctx)
	if err != nil {
		return RateJSON{}, err
	}
	ended := string(today) > utils.LastDay(month)
	rate, err = s.fetch(ctx, month)
	if err != nil {
		return RateJSON{}, fmt.Errorf("couldn't fetch %s's Rate from Frankfurter just now, try once more: %w", month, err)
	}
	if ended {
		if err := s.rates.Insert(ctx, rate); err != nil {
			return RateJSON{}, err
		}
	}
	return RateJSON{Month: month, Average: rate.Average, MonthEnd: rate.MonthEnd, Provisional: !ended}, nil
}

// fetch asks Frankfurter for month's daily rates of a peso in yen, and works
// out its Rate: their average, and the last published Day's.
func (s *RatesService) fetch(ctx context.Context, month string) (model.Rate, error) {
	url := fmt.Sprintf("%s/v1/%s-01..%s?base=MXN&symbols=JPY", s.url, month, utils.LastDay(month))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return model.Rate{}, err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return model.Rate{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return model.Rate{}, fmt.Errorf("Frankfurter answered %s", resp.Status)
	}
	var body struct {
		Rates map[string]map[string]float64 `json:"rates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return model.Rate{}, err
	}
	days := make([]string, 0, len(body.Rates))
	for day, r := range body.Rates {
		if r["JPY"] > 0 && day[:len(utils.MonthLayout)] == month {
			days = append(days, day)
		}
	}
	if len(days) == 0 {
		return model.Rate{}, errors.New("Frankfurter published no rate for that month")
	}
	sort.Strings(days)
	sum := 0.0
	for _, day := range days {
		sum += body.Rates[day]["JPY"]
	}
	return model.Rate{
		Month:    month,
		Average:  math.Round(sum/float64(len(days))*10000) / 10000,
		MonthEnd: body.Rates[days[len(days)-1]]["JPY"],
	}, nil
}

// Tools are what the RatesService offers Connectors.
func (s *RatesService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "money_get_rate",
		Description: "month's Rate (YYYY-MM): a peso in yen, its average for Transactions and its last day's for " +
			"balances, from the European Central Bank. Fetched once a month has ended and never changed; provisional " +
			"while it hasn't.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"month":{"type":"string"}},"required":["month"]}`),
		ReadOnly:    true,
		Handle: func(ctx context.Context, arguments json.RawMessage) (any, error) {
			var args struct {
				Month string `json:"month"`
			}
			if err := json.Unmarshal(arguments, &args); err != nil {
				return nil, err
			}
			if !utils.IsMonth(args.Month) {
				return nil, utils.Refusal{Message: "month must be YYYY-MM"}
			}
			return s.Rate(ctx, args.Month)
		},
	}}
}
