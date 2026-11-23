package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/money/model"
	"github.com/feriosch/nando-server/internal/money/utils"
)

// SubscriptionsService handles the Subscriptions Claude keeps. Nando only
// stores them, and says which are late, so the Review can ask whether one
// was cancelled (ADR 0014).
type SubscriptionsService struct {
	subscriptions model.SubscriptionsRepository
	merchants     *MerchantsService
	categories    *CategoriesService
	core          coremodel.CoreService
}

// NewSubscriptions returns the SubscriptionsService.
func NewSubscriptions(subscriptions model.SubscriptionsRepository, merchants *MerchantsService, categories *CategoriesService, core coremodel.CoreService) *SubscriptionsService {
	return &SubscriptionsService{subscriptions: subscriptions, merchants: merchants, categories: categories, core: core}
}

// lateAfter is how long past its expected charge an active Subscription is
// late.
const lateAfter = 7 * 24 * time.Hour

// everies are how often a Subscription can charge.
var everies = []string{"monthly", "yearly"}

// SubscriptionJSON is a Subscription as the Tools answer it.
type SubscriptionJSON struct {
	ID          string  `json:"id"`
	Merchant    string  `json:"merchant"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	Every       string  `json:"every"`
	LastCharged string  `json:"lastCharged"`
	Category    string  `json:"category"`
	Subcategory string  `json:"subcategory"`
	Essential   bool    `json:"essential"`
	Active      bool    `json:"active"`
	// Late is whether it is active and its expected charge is more than a
	// week past.
	Late bool `json:"late"`
}

// isLate is whether s is active and more than a week past its expected
// charge on today.
func isLate(s model.Subscription, today string) bool {
	last, err := time.Parse(utils.DayLayout, s.LastCharged)
	if err != nil || !s.Active {
		return false
	}
	next := last.AddDate(0, 1, 0)
	if s.Every == "yearly" {
		next = last.AddDate(1, 0, 0)
	}
	now, _ := time.Parse(utils.DayLayout, today)
	return now.Sub(next) > lateAfter
}

func toSubscriptionJSON(s model.Subscription, today string) SubscriptionJSON {
	return SubscriptionJSON{
		ID: s.ID, Merchant: s.MerchantID, Amount: utils.FromMinor(s.Currency, s.Amount), Currency: s.Currency,
		Every: s.Every, LastCharged: s.LastCharged, Category: s.Category, Subcategory: s.SubcategoryID,
		Essential: s.Essential, Active: s.Active, Late: isLate(s, today),
	}
}

// Tools are what the SubscriptionsService offers Connectors.
func (s *SubscriptionsService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "money_save_subscription",
		Description: "Saves a Subscription: without id, a new one (merchant, amount, currency, every, lastCharged and " +
			"essential needed); with id, only the fields given change, such as a new lastCharged or active false once " +
			"the Owner cancelled it. Answers it, late when active and more than a week past its expected charge.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"},"merchant":{"type":"string"},` +
			`"amount":{"type":"number"},"currency":{"type":"string","enum":["JPY","MXN"]},` +
			`"every":{"type":"string","enum":["monthly","yearly"]},"lastCharged":{"type":"string","description":"YYYY-MM-DD"},` +
			`"category":{"type":"string"},"subcategory":{"type":"string"},"essential":{"type":"boolean"},"active":{"type":"boolean"}}}`),
		Handle: s.saveSubscriptionTool,
	}, {
		Name:        "money_list_subscriptions",
		Description: "Every Subscription, active ones first, then by last charge, each saying whether it is late.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		ReadOnly:    true,
		Handle: func(ctx context.Context, _ json.RawMessage) (any, error) {
			today, err := s.core.Today(ctx)
			if err != nil {
				return nil, err
			}
			list, err := s.subscriptions.List(ctx)
			if err != nil {
				return nil, err
			}
			answer := make([]SubscriptionJSON, len(list))
			for i, sub := range list {
				answer[i] = toSubscriptionJSON(sub, string(today))
			}
			return answer, nil
		},
	}}
}

func (s *SubscriptionsService) saveSubscriptionTool(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args struct {
		ID          *string  `json:"id"`
		Merchant    *string  `json:"merchant"`
		Amount      *float64 `json:"amount"`
		Currency    *string  `json:"currency"`
		Every       *string  `json:"every"`
		LastCharged *string  `json:"lastCharged"`
		Category    *string  `json:"category"`
		Subcategory *string  `json:"subcategory"`
		Essential   *bool    `json:"essential"`
		Active      *bool    `json:"active"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return nil, utils.Refusal{Message: "send amount as a number, essential and active as true or false, and the rest as strings"}
	}
	sub := model.Subscription{Active: true}
	if args.ID != nil {
		var err error
		sub, err = s.subscriptions.Find(ctx, *args.ID)
		if errors.Is(err, coreutils.ErrNotFound) {
			return nil, utils.Refusal{Message: fmt.Sprintf("there's no such Subscription %q", *args.ID)}
		}
		if err != nil {
			return nil, err
		}
	} else if args.Merchant == nil || args.Amount == nil || args.Currency == nil || args.Every == nil || args.LastCharged == nil || args.Essential == nil {
		return nil, utils.Refusal{Message: "a new Subscription needs merchant, amount, currency, every, lastCharged and essential"}
	}
	if args.Merchant != nil {
		if _, err := s.merchants.find(ctx, *args.Merchant); err != nil {
			return nil, err
		}
		sub.MerchantID = *args.Merchant
	}
	if args.Currency != nil {
		if !slices.Contains(model.Currencies, *args.Currency) {
			return nil, utils.Refusal{Message: fmt.Sprintf("currency %q isn't JPY or MXN", *args.Currency)}
		}
		if args.Amount == nil && *args.Currency != sub.Currency {
			return nil, utils.Refusal{Message: "a new currency needs its amount too"}
		}
		sub.Currency = *args.Currency
	}
	if args.Amount != nil {
		minor, err := utils.ToMinor(sub.Currency, *args.Amount)
		if err != nil {
			return nil, err
		}
		if minor <= 0 {
			return nil, utils.Refusal{Message: "amount must be above 0"}
		}
		sub.Amount = minor
	}
	if args.Every != nil {
		if !slices.Contains(everies, *args.Every) {
			return nil, utils.Refusal{Message: "every must be monthly or yearly"}
		}
		sub.Every = *args.Every
	}
	if args.LastCharged != nil {
		if !utils.IsDay(*args.LastCharged) {
			return nil, utils.Refusal{Message: "lastCharged must be a Day, YYYY-MM-DD"}
		}
		sub.LastCharged = *args.LastCharged
	}
	if args.Category != nil {
		sub.Category = *args.Category
	}
	if args.Subcategory != nil {
		sub.SubcategoryID = *args.Subcategory
	}
	if err := s.categories.checkPlace(ctx, model.Expense, sub.Category, sub.SubcategoryID); err != nil {
		return nil, utils.Refusal{Message: err.Error()}
	}
	if args.Essential != nil {
		sub.Essential = *args.Essential
	}
	if args.Active != nil {
		sub.Active = *args.Active
	}
	if sub.ID == "" {
		id, err := s.subscriptions.Insert(ctx, sub)
		if err != nil {
			return nil, err
		}
		sub.ID = id
	} else if err := s.subscriptions.Replace(ctx, sub); err != nil {
		return nil, err
	}
	today, err := s.core.Today(ctx)
	if err != nil {
		return nil, err
	}
	return toSubscriptionJSON(sub, string(today)), nil
}
