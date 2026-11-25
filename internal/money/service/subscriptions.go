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
	// statements tells how far each Account's Statements reach, which is as
	// far as a missing charge can be seen.
	statements model.StatementsRepository
	core       coremodel.CoreService
}

// NewSubscriptions returns the SubscriptionsService.
func NewSubscriptions(subscriptions model.SubscriptionsRepository, merchants *MerchantsService, categories *CategoriesService, statements model.StatementsRepository, core coremodel.CoreService) *SubscriptionsService {
	return &SubscriptionsService{subscriptions: subscriptions, merchants: merchants, categories: categories, statements: statements, core: core}
}

// seenUntil is the last Day a missing charge of s could show by: how far the
// Statements reach, without a gap, from the one with the last charge of its
// Merchant, of the Account it was on, and never past today. With no charge to
// go by, it is today. An inactive Subscription is never late, so it needs no
// Statements read.
func (s *SubscriptionsService) seenUntil(ctx context.Context, sub model.Subscription, today string) (string, error) {
	if !sub.Active {
		return today, nil
	}
	charges, err := s.statements.Transactions(ctx, model.TransactionFilter{MerchantID: sub.MerchantID, Kind: model.Expense})
	if err != nil || len(charges) == 0 {
		return today, err
	}
	last := charges[len(charges)-1]
	statements, err := s.statements.ForAccount(ctx, last.AccountID, "", "")
	if err != nil {
		return today, err
	}
	// statements are by period: from the last charge's, follow them while
	// each starts by the day after the one before ends, as a missing
	// Statement could hold the charge.
	reach := ""
	for _, st := range statements {
		switch {
		case reach == "" && st.ID == last.StatementID:
			reach = st.To
		case reach != "" && st.From <= utils.AddDays(reach, 1):
			reach = max(reach, st.To)
		}
	}
	if reach == "" || reach > today {
		return today, nil
	}
	return reach, nil
}

// subscriptionJSON is sub as the Tools answer it, late or not by what its
// Statements show on today.
func (s *SubscriptionsService) subscriptionJSON(ctx context.Context, sub model.Subscription, today string) (model.SubscriptionJSON, error) {
	until, err := s.seenUntil(ctx, sub, today)
	if err != nil {
		return model.SubscriptionJSON{}, err
	}
	return toSubscriptionJSON(sub, until), nil
}

// lateAfter is how long past its expected charge an active Subscription is
// late.
const lateAfter = 7 * 24 * time.Hour

// everies are how often a Subscription can charge.
var everies = []string{"monthly", "yearly"}

// isLate is whether s is active and its expected charge is more than a week
// before until, the last Day a missing charge could show by.
func isLate(s model.Subscription, until string) bool {
	last, err := time.Parse(utils.DayLayout, s.LastCharged)
	if err != nil || !s.Active {
		return false
	}
	next := last.AddDate(0, 1, 0)
	if s.Every == "yearly" {
		next = last.AddDate(1, 0, 0)
	}
	seen, _ := time.Parse(utils.DayLayout, until)
	return seen.Sub(next) > lateAfter
}

// toSubscriptionJSON is s as the Tools answer it, late or not by until, the
// last Day a missing charge could show by.
func toSubscriptionJSON(s model.Subscription, until string) model.SubscriptionJSON {
	return model.SubscriptionJSON{
		ID: s.ID, Merchant: s.MerchantID, Amount: utils.FromMinor(s.Currency, s.Amount), Currency: s.Currency,
		Every: s.Every, LastCharged: s.LastCharged, Category: s.Category, Subcategory: s.SubcategoryID,
		Essential: s.Essential, Active: s.Active, Late: isLate(s, until),
	}
}

// List is every Subscription, active ones first, then by last charge, each
// saying whether it is late.
func (s *SubscriptionsService) List(ctx context.Context) ([]model.SubscriptionJSON, error) {
	today, err := s.core.Today(ctx)
	if err != nil {
		return nil, err
	}
	list, err := s.subscriptions.List(ctx)
	if err != nil {
		return nil, err
	}
	answer := make([]model.SubscriptionJSON, len(list))
	for i, sub := range list {
		if answer[i], err = s.subscriptionJSON(ctx, sub, string(today)); err != nil {
			return nil, err
		}
	}
	return answer, nil
}

// Tools are what the SubscriptionsService offers Connectors.
func (s *SubscriptionsService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "money_save_subscription",
		Description: "Saves a Subscription: without id, a new one (merchant, amount, currency, every, lastCharged and " +
			"essential needed); with id, only the fields given change, such as a new lastCharged or active false once " +
			"the Owner cancelled it. Answers it, late when active and more than a week past its expected charge, by the " +
			"Statements of the Account that last charged it (never past today).",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"},"merchant":{"type":"string"},` +
			`"amount":{"type":"number"},"currency":{"type":"string","enum":["JPY","MXN"]},` +
			`"every":{"type":"string","enum":["monthly","yearly"]},"lastCharged":{"type":"string","description":"YYYY-MM-DD"},` +
			`"category":{"type":"string"},"subcategory":{"type":"string"},"essential":{"type":"boolean"},"active":{"type":"boolean"}}}`),
		Handle: s.saveSubscriptionTool,
	}, {
		Name: "money_list_subscriptions",
		Description: "Every Subscription, active ones first, then by last charge, each saying whether it is late: more " +
			"than a week past its expected charge, by the Statements of the Account that last charged it (never past today).",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		ReadOnly:    true,
		Handle: func(ctx context.Context, _ json.RawMessage) (any, error) {
			return s.List(ctx)
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
		// A new Category clears the Subcategory, unless one is given too.
		sub.Category, sub.SubcategoryID = *args.Category, ""
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
	// Read what the answer needs before writing, so that a failed read
	// doesn't leave a saved Subscription that a retry would save again.
	today, err := s.core.Today(ctx)
	if err != nil {
		return nil, err
	}
	until, err := s.seenUntil(ctx, sub, string(today))
	if err != nil {
		return nil, err
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
	return toSubscriptionJSON(sub, until), nil
}
