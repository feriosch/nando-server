package service

import (
	"context"
	"encoding/json"
	"math"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/money/model"
	"github.com/feriosch/nando-server/internal/money/utils"
)

// TotalsService works out a month's totals in yen: spending, income, net
// worth, Return and the saving rate (ADR 0014). The Monthly Review, the Home
// Card and the Money screen all read them.
type TotalsService struct {
	statements    model.StatementsRepository
	accounts      *AccountsService
	categories    *CategoriesService
	rates         *RatesService
	statementsSvc *StatementsService
}

// NewTotals returns the TotalsService.
func NewTotals(statements model.StatementsRepository, accounts *AccountsService, categories *CategoriesService, rates *RatesService, statementsSvc *StatementsService) *TotalsService {
	return &TotalsService{statements: statements, accounts: accounts, categories: categories, rates: rates, statementsSvc: statementsSvc}
}

// balanceWithin is how far from a month's last Day a Statement's balance
// may be to count as the month-end balance: a card closing on the 27th, a
// savings screenshot taken on the 3rd.
const balanceWithin = 10

// maxTotalsMonths is the most months money_get_totals answers at once.
const maxTotalsMonths = 36

// CategoryTotalJSON is a Category's total in yen, with its Subcategories'
// and, for spending, the month before's and the average of the 3 before.
type CategoryTotalJSON struct {
	Category      string                 `json:"category"`
	Name          string                 `json:"name"`
	Total         int64                  `json:"total"`
	Previous      *int64                 `json:"previous,omitempty"`
	Average3      *int64                 `json:"average3,omitempty"`
	Subcategories []SubcategoryTotalJSON `json:"bySubcategory,omitempty"`
}

// SubcategoryTotalJSON is a Subcategory's total in yen; an empty
// subcategory is what has none.
type SubcategoryTotalJSON struct {
	Subcategory string `json:"subcategory"`
	Name        string `json:"name"`
	Total       int64  `json:"total"`
}

// SpendingJSON is a month's spending in yen.
type SpendingJSON struct {
	Total      int64               `json:"total"`
	Previous   *int64              `json:"previous"`
	Average3   *int64              `json:"average3"`
	Essential  int64               `json:"essential"`
	Optional   int64               `json:"optional"`
	Unmarked   int64               `json:"unmarked"`
	ByCategory []CategoryTotalJSON `json:"byCategory"`
}

// IncomeJSON is a month's income in yen.
type IncomeJSON struct {
	Total      int64               `json:"total"`
	ByCategory []CategoryTotalJSON `json:"byCategory"`
}

// BalanceJSON is an Account's month-end balance: in its own currency, and
// in yen at the month-end Rate. A card's is what it owes, below 0.
type BalanceJSON struct {
	Account  string  `json:"account"`
	Name     string  `json:"name"`
	Currency string  `json:"currency"`
	Balance  float64 `json:"balance"`
	Yen      int64   `json:"yen"`
}

// NetWorthJSON is the Accounts' month-end balances in yen, and their total
// against the month before's; Missing names the Accounts with no balance
// near the month's end.
type NetWorthJSON struct {
	Total    int64            `json:"total"`
	Previous *int64           `json:"previous"`
	Accounts []BalanceJSON    `json:"accounts"`
	Missing  []AccountRefJSON `json:"missing"`
}

// ReturnJSON is how much a savings or investment Account grew in a month,
// less what was Transferred in, in yen.
type ReturnJSON struct {
	Account string `json:"account"`
	Name    string `json:"name"`
	Yen     int64  `json:"yen"`
}

// UnconfirmedJSON counts a month's Unconfirmed Transactions, and their
// amounts in yen.
type UnconfirmedJSON struct {
	Count int   `json:"count"`
	Yen   int64 `json:"yen"`
}

// MonthTotalsJSON is what money_get_month_totals answers.
type MonthTotalsJSON struct {
	Month    string           `json:"month"`
	Complete bool             `json:"complete"`
	Missing  []AccountRefJSON `json:"missing"`
	// Rate is the month's Rate, when a peso amount was turned into yen.
	Rate        *RateJSON       `json:"rate"`
	Spending    SpendingJSON    `json:"spending"`
	Income      IncomeJSON      `json:"income"`
	NetWorth    NetWorthJSON    `json:"netWorth"`
	Returns     []ReturnJSON    `json:"returns"`
	SavingRate  *float64        `json:"savingRate"`
	Unconfirmed UnconfirmedJSON `json:"unconfirmed"`
}

// MonthSumJSON is one month's spending and income in yen, as
// money_get_totals answers it.
type MonthSumJSON struct {
	Month      string           `json:"month"`
	Spending   int64            `json:"spending"`
	Income     int64            `json:"income"`
	ByCategory map[string]int64 `json:"byCategory"`
}

// converter turns amounts into yen at one month's Rate, fetching it only
// when a peso amount needs it.
type converter struct {
	month string
	rates *RatesService
	rate  *RateJSON
}

// yen is minor, in currency's minor unit, in yen: at the month's average,
// or at its end for a balance.
func (c *converter) yen(ctx context.Context, currency string, minor int64, monthEnd bool) (int64, error) {
	if currency == "JPY" {
		return minor, nil
	}
	if c.rate == nil {
		rate, err := c.rates.Rate(ctx, c.month)
		if err != nil {
			return 0, err
		}
		c.rate = &rate
	}
	rate := c.rate.Average
	if monthEnd {
		rate = c.rate.MonthEnd
	}
	return int64(math.Round(utils.FromMinor(currency, minor) * rate)), nil
}

// figures are one month's spending and income in yen.
type figures struct {
	spending, income              int64
	essential, optional, unmarked int64
	// byCategory is spending by Category key, and incomeByCategory income.
	byCategory, incomeByCategory map[string]int64
	bySubcategory                map[string]map[string]int64
	unconfirmed                  UnconfirmedJSON
	// netIn is, by Account, the money Transferred into it, less out of it, in
	// its own currency: from its own Transfers if it has any, else from
	// those of other Accounts that name it.
	netIn map[string]int64
}

// figures works out month's figures, converting with conv.
func (s *TotalsService) figures(ctx context.Context, month string, currencies map[string]string, essentials map[string]bool, conv *converter) (figures, error) {
	found, err := s.statements.Transactions(ctx, model.TransactionFilter{FromMonth: month, ToMonth: month})
	if err != nil {
		return figures{}, err
	}
	f := figures{byCategory: map[string]int64{}, incomeByCategory: map[string]int64{}, bySubcategory: map[string]map[string]int64{}, netIn: map[string]int64{}}
	own, counterpart := map[string]int64{}, map[string]int64{}
	hasOwn := map[string]bool{}
	for _, t := range found {
		currency := currencies[t.AccountID]
		if t.Kind == model.Transfer {
			sign := int64(1)
			if t.Direction == "out" {
				sign = -1
			}
			own[t.AccountID] += sign * t.Amount
			hasOwn[t.AccountID] = true
			if t.OtherAccountID != "" && currencies[t.OtherAccountID] == currency {
				counterpart[t.OtherAccountID] -= sign * t.Amount
			}
			if t.Unconfirmed {
				f.unconfirmed.Count++
			}
			continue
		}
		yen, err := conv.yen(ctx, currency, t.Amount, false)
		if err != nil {
			return figures{}, err
		}
		if t.Unconfirmed {
			f.unconfirmed.Count++
			f.unconfirmed.Yen += abs(yen)
		}
		if t.Kind == model.Income {
			f.income += yen
			f.incomeByCategory[t.Category] += yen
			continue
		}
		f.spending += yen
		f.byCategory[t.Category] += yen
		if f.bySubcategory[t.Category] == nil {
			f.bySubcategory[t.Category] = map[string]int64{}
		}
		f.bySubcategory[t.Category][t.SubcategoryID] += yen
		mark, marked := essentials[t.SubcategoryID]
		if t.Essential != nil {
			mark, marked = *t.Essential, true
		}
		switch {
		case !marked:
			f.unmarked += yen
		case mark:
			f.essential += yen
		default:
			f.optional += yen
		}
	}
	for id := range currencies {
		if hasOwn[id] {
			f.netIn[id] = own[id]
		} else {
			f.netIn[id] = counterpart[id]
		}
	}
	return f, nil
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// tracked is whether any Account is tracked in month: whether it is part of
// the Owner's history.
func tracked(accounts []model.Account, month string) bool {
	for _, a := range accounts {
		if a.TracksIn(month) {
			return true
		}
	}
	return false
}

// balance is account's month-end balance in month, in its minor unit, and
// whether it has one: the balance of its Statement ending nearest the
// month's last Day, within balanceWithin Days. A card's is what it owes,
// below 0, from its stated total when no balance is given.
func (s *TotalsService) balance(ctx context.Context, account model.Account, month string) (int64, bool, error) {
	last := utils.LastDay(month)
	near, err := s.statements.ForAccount(ctx, account.ID, utils.AddDays(last, -balanceWithin), utils.AddDays(last, balanceWithin))
	if err != nil {
		return 0, false, err
	}
	best, found, bestDistance := int64(0), false, balanceWithin+1
	for _, st := range near {
		amount := st.ClosingBalance
		if amount == nil && account.Kind == model.KindCard {
			amount = st.Total
		}
		if amount == nil {
			continue
		}
		distance := dayDistance(st.To, last)
		if distance > balanceWithin || distance >= bestDistance {
			continue
		}
		best, found, bestDistance = *amount, true, distance
		if account.Kind == model.KindCard {
			best = -abs(best)
		}
	}
	return best, found, nil
}

// dayDistance is how many Days apart two Days are.
func dayDistance(a, b string) int {
	for n := 0; n <= balanceWithin; n++ {
		if utils.AddDays(a, n) == b || utils.AddDays(a, -n) == b {
			return n
		}
	}
	return balanceWithin + 1
}

// netWorth is month's net worth, the Accounts' balances, and whether any had
// one.
func (s *TotalsService) netWorth(ctx context.Context, accounts []model.Account, month string, conv *converter) (NetWorthJSON, map[string]int64, bool, error) {
	nw := NetWorthJSON{Accounts: []BalanceJSON{}, Missing: []AccountRefJSON{}}
	balances := map[string]int64{}
	for _, a := range accounts {
		if !a.TracksIn(month) {
			continue
		}
		minor, ok, err := s.balance(ctx, a, month)
		if err != nil {
			return nw, nil, false, err
		}
		if !ok {
			nw.Missing = append(nw.Missing, AccountRefJSON{ID: a.ID, Name: a.Name})
			continue
		}
		yen, err := conv.yen(ctx, a.Currency, minor, true)
		if err != nil {
			return nw, nil, false, err
		}
		balances[a.ID] = minor
		nw.Total += yen
		nw.Accounts = append(nw.Accounts, BalanceJSON{Account: a.ID, Name: a.Name, Currency: a.Currency, Balance: utils.FromMinor(a.Currency, minor), Yen: yen})
	}
	return nw, balances, len(balances) > 0, nil
}

// MonthTotals works out month's totals in yen.
func (s *TotalsService) MonthTotals(ctx context.Context, month string) (MonthTotalsJSON, error) {
	accounts, err := s.accounts.accounts.List(ctx)
	if err != nil {
		return MonthTotalsJSON{}, err
	}
	currencies := map[string]string{}
	for _, a := range accounts {
		currencies[a.ID] = a.Currency
	}
	essentials, err := s.categories.essentials(ctx)
	if err != nil {
		return MonthTotalsJSON{}, err
	}
	names, err := s.categories.subcategoryNames(ctx)
	if err != nil {
		return MonthTotalsJSON{}, err
	}
	conv := &converter{month: month, rates: s.rates}
	this, err := s.figures(ctx, month, currencies, essentials, conv)
	if err != nil {
		return MonthTotalsJSON{}, err
	}
	// The months before, for comparing: those that are part of the history.
	var before []figures
	for n := 1; n <= 3; n++ {
		m := utils.AddMonths(month, -n)
		if !tracked(accounts, m) {
			break
		}
		f, err := s.figures(ctx, m, currencies, essentials, &converter{month: m, rates: s.rates})
		if err != nil {
			return MonthTotalsJSON{}, err
		}
		before = append(before, f)
	}
	answer := MonthTotalsJSON{Month: month, Returns: []ReturnJSON{}}
	complete, err := s.statementsSvc.Month(ctx, month)
	if err != nil {
		return MonthTotalsJSON{}, err
	}
	answer.Complete, answer.Missing = complete.Complete, complete.Missing
	answer.Spending = SpendingJSON{
		Total: this.spending, Essential: this.essential, Optional: this.optional, Unmarked: this.unmarked,
		ByCategory: []CategoryTotalJSON{},
	}
	answer.Spending.Previous, answer.Spending.Average3 = compare(before, func(f figures) int64 { return f.spending })
	answer.Income = IncomeJSON{Total: this.income, ByCategory: []CategoryTotalJSON{}}
	for _, c := range append(model.Categories, model.Category{Key: "", Name: "Uncategorized"}) {
		total, ok := this.byCategory[c.Key]
		if !ok {
			continue
		}
		ct := CategoryTotalJSON{Category: c.Key, Name: c.Name, Total: total}
		if c.Kind == model.Income {
			answer.Income.ByCategory = append(answer.Income.ByCategory, ct)
			continue
		}
		ct.Previous, ct.Average3 = compare(before, func(f figures) int64 { return f.byCategory[c.Key] })
		for sub, subTotal := range this.bySubcategory[c.Key] {
			ct.Subcategories = append(ct.Subcategories, SubcategoryTotalJSON{Subcategory: sub, Name: names[sub], Total: subTotal})
		}
		sortSubcategoryTotals(ct.Subcategories)
		answer.Spending.ByCategory = append(answer.Spending.ByCategory, ct)
	}
	nw, balances, _, err := s.netWorth(ctx, accounts, month, conv)
	if err != nil {
		return MonthTotalsJSON{}, err
	}
	previousMonth := utils.AddMonths(month, -1)
	prevConv := &converter{month: previousMonth, rates: s.rates}
	prev, prevBalances, any, err := s.netWorth(ctx, accounts, previousMonth, prevConv)
	if err != nil {
		return MonthTotalsJSON{}, err
	}
	if any {
		nw.Previous = &prev.Total
	}
	answer.NetWorth = nw
	for _, a := range accounts {
		if a.Kind != model.KindSavings && a.Kind != model.KindInvestment {
			continue
		}
		now, ok := balances[a.ID]
		then, okBefore := prevBalances[a.ID]
		if !ok || !okBefore {
			continue
		}
		yen, err := conv.yen(ctx, a.Currency, now-then-this.netIn[a.ID], true)
		if err != nil {
			return MonthTotalsJSON{}, err
		}
		answer.Returns = append(answer.Returns, ReturnJSON{Account: a.ID, Name: a.Name, Yen: yen})
	}
	if this.income > 0 {
		rate := math.Round(float64(this.income-this.spending)/float64(this.income)*10000) / 10000
		answer.SavingRate = &rate
	}
	answer.Unconfirmed = this.unconfirmed
	answer.Rate = conv.rate
	return answer, nil
}

// compare is the value of the month before, and the average of the months
// before (up to 3), from before, newest first: nil when there are none.
func compare(before []figures, value func(figures) int64) (previous, average3 *int64) {
	if len(before) == 0 {
		return nil, nil
	}
	p := value(before[0])
	sum := int64(0)
	for _, f := range before {
		sum += value(f)
	}
	avg := int64(math.Round(float64(sum) / float64(len(before))))
	return &p, &avg
}

// Totals works out each month's spending and income from fromMonth to
// toMonth.
func (s *TotalsService) Totals(ctx context.Context, fromMonth, toMonth string) ([]MonthSumJSON, error) {
	accounts, err := s.accounts.accounts.List(ctx)
	if err != nil {
		return nil, err
	}
	currencies := map[string]string{}
	for _, a := range accounts {
		currencies[a.ID] = a.Currency
	}
	essentials, err := s.categories.essentials(ctx)
	if err != nil {
		return nil, err
	}
	list := []MonthSumJSON{}
	for m := fromMonth; m <= toMonth; m = utils.AddMonths(m, 1) {
		f, err := s.figures(ctx, m, currencies, essentials, &converter{month: m, rates: s.rates})
		if err != nil {
			return nil, err
		}
		list = append(list, MonthSumJSON{Month: m, Spending: f.spending, Income: f.income, ByCategory: f.byCategory})
	}
	return list, nil
}

// Tools are what the TotalsService offers Connectors.
func (s *TotalsService) Tools() []coremodel.Tool {
	return []coremodel.Tool{{
		Name: "money_get_month_totals",
		Description: "month's (YYYY-MM) totals in yen, each amount turned into yen at the month's Rate: spending " +
			"(total, by Category and Subcategory, Essential against Optional against unmarked, each against the month " +
			"before and the average of up to 3 before), income by Category (Transfers are neither), net worth (each " +
			"Account's month-end balance; a card's is what it owes), Return of savings and investments (their growth " +
			"less what was Transferred in), the saving rate ((income − spending) ÷ income, without Return), the " +
			"Unconfirmed Transactions, and whether the month is Complete.",
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
			return s.MonthTotals(ctx, args.Month)
		},
	}, {
		Name: "money_get_totals",
		Description: "Each month's spending and income in yen, and spending by Category key, from fromMonth to " +
			"toMonth (YYYY-MM, at most 36 months), for questions over time such as how much went on eating out since summer.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"fromMonth":{"type":"string"},"toMonth":{"type":"string"}},"required":["fromMonth","toMonth"]}`),
		ReadOnly:    true,
		Handle: func(ctx context.Context, arguments json.RawMessage) (any, error) {
			var args struct {
				FromMonth string `json:"fromMonth"`
				ToMonth   string `json:"toMonth"`
			}
			if err := json.Unmarshal(arguments, &args); err != nil {
				return nil, err
			}
			if !utils.IsMonth(args.FromMonth) || !utils.IsMonth(args.ToMonth) || args.FromMonth > args.ToMonth {
				return nil, utils.Refusal{Message: "fromMonth and toMonth must be months, YYYY-MM, in order"}
			}
			if utils.AddMonths(args.FromMonth, maxTotalsMonths-1) < args.ToMonth {
				return nil, utils.Refusal{Message: "at most 36 months at once"}
			}
			return s.Totals(ctx, args.FromMonth, args.ToMonth)
		},
	}}
}
