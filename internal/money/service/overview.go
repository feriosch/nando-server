package service

import (
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
	core       coremodel.CoreService
}

// NewOverview returns the OverviewService.
func NewOverview(totals *TotalsService, statements *StatementsService, accounts model.AccountsRepository, settings *SettingsService, core coremodel.CoreService) *OverviewService {
	return &OverviewService{totals: totals, statements: statements, accounts: accounts, settings: settings, core: core}
}

// Routes adds the web app's reads, under /api/money/.
func (s *OverviewService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/money/card", s.card)
	mux.HandleFunc("GET /api/money/settings", s.getSettings)
}

// cardMonths is how far back the Home Card looks for a Complete month.
const cardMonths = 36

// card answers the Home Card, or null before any month is Complete.
func (s *OverviewService) card(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	today, err := s.core.Today(ctx)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	accounts, err := s.accounts.List(ctx)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	month := ""
	for m, n := utils.AddMonths(utils.MonthOf(string(today)), -1), 0; n < cardMonths; m, n = utils.AddMonths(m, -1), n+1 {
		// A month no Account is tracked in is not part of the history, and
		// would read as Complete; skip it, past any gap between Accounts.
		if !tracked(accounts, m) {
			continue
		}
		complete, err := s.statements.Month(ctx, m)
		if err != nil {
			coreutils.ServerError(w, err)
			return
		}
		if complete.Complete {
			month = m
			break
		}
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
	card.Spending.Total, card.Spending.Average3 = totals.Spending.Total, totals.Spending.Average3
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
		AskAbove:     map[string]int{"JPY": settings.AskAbove.JPY, "MXN": settings.AskAbove.MXN},
		ReminderTime: settings.ReminderTime,
		Reminders:    make([]model.ReminderGroupPageJSON, len(settings.Reminders)),
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
