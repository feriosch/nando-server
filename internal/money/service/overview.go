package service

import (
	"context"
	"math"
	"net/http"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/money/model"
	"github.com/feriosch/nando-server/internal/money/utils"
)

// OverviewService serves the web app's reads of Money: the Home Card and the
// settings. The web app only reads Money; the Money Assistant writes
// (ADR 0014).
type OverviewService struct {
	totals     *TotalsService
	statements *StatementsService
	accounts   model.AccountsRepository
	settings   *SettingsService
	// subscriptions and merchants are for the Subscriptions page, and rates
	// to cost them in yen.
	subscriptions *SubscriptionsService
	merchants     model.MerchantsRepository
	rates         *RatesService
	core          coremodel.CoreService
}

// NewOverview returns the OverviewService.
func NewOverview(totals *TotalsService, statements *StatementsService, accounts model.AccountsRepository, settings *SettingsService,
	subscriptions *SubscriptionsService, merchants model.MerchantsRepository, rates *RatesService, core coremodel.CoreService) *OverviewService {
	return &OverviewService{totals: totals, statements: statements, accounts: accounts, settings: settings,
		subscriptions: subscriptions, merchants: merchants, rates: rates, core: core}
}

// Routes adds the web app's reads, under /api/money/.
func (s *OverviewService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/money/card", s.card)
	mux.HandleFunc("GET /api/money/settings", s.getSettings)
	mux.HandleFunc("GET /api/money/months", s.months)
	mux.HandleFunc("GET /api/money/months/{month}", s.month)
	mux.HandleFunc("GET /api/money/months/{month}/categories/{key}", s.category)
	mux.HandleFunc("GET /api/money/subscriptions", s.subscriptionsPage)
	mux.HandleFunc("GET /api/money/net-worth", s.netWorth)
}

// cardMonths is how far back the Home Card looks for a Complete month.
const cardMonths = 36

// completeMonths are the Complete months before today's, newest first, at
// most limit of them, looking back at most cardMonths months.
func (s *OverviewService) completeMonths(ctx context.Context, today coremodel.Day, limit int) ([]string, error) {
	accounts, err := s.accounts.List(ctx)
	if err != nil {
		return nil, err
	}
	months := []string{}
	for m, n := utils.AddMonths(utils.MonthOf(string(today)), -1), 0; n < cardMonths && len(months) < limit; m, n = utils.AddMonths(m, -1), n+1 {
		// A month no Account is tracked in is not part of the history, and
		// would read as Complete; skip it, past any gap between Accounts.
		if !tracked(accounts, m) {
			continue
		}
		month, err := s.statements.Month(ctx, m)
		if err != nil {
			return nil, err
		}
		if month.Complete {
			months = append(months, m)
		}
	}
	return months, nil
}

// card answers the Home Card, or null before any month is Complete.
func (s *OverviewService) card(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	today, err := s.core.Today(ctx)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	complete, err := s.completeMonths(ctx, today, 1)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	month := ""
	if len(complete) > 0 {
		month = complete[0]
	}
	if month == "" {
		coreutils.WriteJSON(w, http.StatusOK, nil)
		return
	}
	totals, err := s.totals.MonthTotals(ctx, month)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	unconfirmed := true
	waiting, err := s.statements.statements.Transactions(ctx, model.TransactionFilter{Unconfirmed: &unconfirmed})
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	card := model.CardJSON{Month: month, Unconfirmed: len(waiting)}
	card.Spending.Total, card.Spending.OneOff, card.Spending.Average3 = totals.Spending.Total, totals.Spending.OneOff, totals.Spending.Average3
	card.Spending.Essential, card.Spending.Optional, card.Spending.Unmarked = totals.Spending.Essential, totals.Spending.Optional, totals.Spending.Unmarked
	card.NetWorth.Total, card.NetWorth.Previous, card.NetWorth.Missing = totals.NetWorth.Total, totals.NetWorth.Previous, totals.NetWorth.Missing
	coreutils.WriteJSON(w, http.StatusOK, card)
}

// getSettings answers the settings for the Settings Section, read-only.
func (s *OverviewService) getSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	settings, err := s.settings.savedOrDefault(ctx)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	accounts, err := s.accounts.List(ctx)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	names := map[string]string{}
	for _, a := range accounts {
		names[a.ID] = a.Name
	}
	page := model.SettingsPageJSON{
		EssentialByCategory: map[string]bool{},
		AskAbove:            map[string]int{"JPY": settings.AskAbove.JPY, "MXN": settings.AskAbove.MXN},
		ReminderTime:        settings.ReminderTime,
		Reminders:           make([]model.ReminderGroupPageJSON, len(settings.Reminders)),
	}
	for key, mark := range settings.EssentialByCategory {
		page.EssentialByCategory[key] = mark
	}
	for i, g := range settings.Reminders {
		group := model.ReminderGroupPageJSON{Name: g.Name, Day: g.Day, Accounts: []string{}}
		for _, id := range g.AccountIDs {
			group.Accounts = append(group.Accounts, names[id])
		}
		page.Reminders[i] = group
	}
	coreutils.WriteJSON(w, http.StatusOK, page)
}

// months answers the Complete months, newest first, for the Money screen.
func (s *OverviewService) months(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	today, err := s.core.Today(ctx)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	complete, err := s.completeMonths(ctx, today, cardMonths)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	summaries, err := s.totals.Summaries(ctx, complete)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, summaries)
}

// completeness answers whether a month, before today's, is part of the
// history and Complete: the only months the Money screen shows. It reads
// today and the Accounts once, for the many months a page asks about.
func (s *OverviewService) completeness(ctx context.Context) (func(month string) (bool, error), error) {
	today, err := s.core.Today(ctx)
	if err != nil {
		return nil, err
	}
	accounts, err := s.accounts.List(ctx)
	if err != nil {
		return nil, err
	}
	thisMonth := utils.MonthOf(string(today))
	return func(month string) (bool, error) {
		if !utils.IsMonth(month) || month >= thisMonth || !tracked(accounts, month) {
			return false, nil
		}
		complete, err := s.statements.Month(ctx, month)
		if err != nil {
			return false, err
		}
		return complete.Complete, nil
	}, nil
}

// month answers a Complete month's totals, or 404.
func (s *OverviewService) month(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	month := r.PathValue("month")
	isComplete, err := s.completeness(ctx)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	complete, err := isComplete(month)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	if !complete {
		http.Error(w, "no such Complete month", http.StatusNotFound)
		return
	}
	totals, err := s.totals.MonthTotals(ctx, month)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, totals)
}

// uncategorized is the key, in a Category page's path, of the Expenses
// not yet in any Category, which a month's totals list as "Uncategorized".
const uncategorized = "uncategorized"

// category answers an expense Category's page for a Complete month, or 404.
func (s *OverviewService) category(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	month, key := r.PathValue("month"), r.PathValue("key")
	category, ok := model.CategoryOf(key)
	if key == uncategorized {
		category, ok = model.Category{Key: "", Name: "Uncategorized", Kind: model.Expense}, true
	}
	if !ok || category.Kind != model.Expense {
		http.Error(w, "no such expense Category", http.StatusNotFound)
		return
	}
	isComplete, err := s.completeness(ctx)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	complete, err := isComplete(month)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	if !complete {
		http.Error(w, "no such Complete month", http.StatusNotFound)
		return
	}
	page, err := s.totals.CategoryPage(ctx, month, category, isComplete)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	page.Category = key
	coreutils.WriteJSON(w, http.StatusOK, page)
}

// subscriptionsPage answers every Subscription, each active one costed a
// month in yen at the latest Rate, last month's average.
func (s *OverviewService) subscriptionsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.subscriptions.List(ctx)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	merchants, err := s.merchants.List(ctx)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	names := map[string]string{}
	for _, m := range merchants {
		names[m.ID] = m.Name
	}
	today, err := s.core.Today(ctx)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	conv := &converter{month: utils.AddMonths(utils.MonthOf(string(today)), -1), rates: s.rates}
	page := model.SubscriptionsPageJSON{Subscriptions: make([]model.ScreenSubscriptionJSON, len(list))}
	for i, sub := range list {
		screen := model.ScreenSubscriptionJSON{SubscriptionJSON: sub, MerchantName: names[sub.Merchant]}
		if sub.Active {
			minor, err := utils.ToMinor(sub.Currency, sub.Amount)
			if err != nil {
				coreutils.ServerError(w, err)
				return
			}
			yen, err := conv.yen(ctx, sub.Currency, minor, false)
			if err != nil {
				coreutils.ServerError(w, err)
				return
			}
			if sub.Every == "yearly" {
				yen = int64(math.Round(float64(yen) / 12))
			}
			screen.MonthlyYen = &yen
			page.MonthlyTotal += yen
		}
		page.Subscriptions[i] = screen
	}
	coreutils.WriteJSON(w, http.StatusOK, page)
}

// netWorth answers each Complete month's net worth, newest first.
func (s *OverviewService) netWorth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	today, err := s.core.Today(ctx)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	complete, err := s.completeMonths(ctx, today, cardMonths)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	months, err := s.totals.NetWorths(ctx, complete)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	coreutils.WriteJSON(w, http.StatusOK, months)
}
