package tests

import (
	"net/http"
	"slices"
	"testing"
	"time"
)

// monthSummaryJSON is one Complete month, as the Money screen lists it.
type monthSummaryJSON struct {
	Month    string `json:"month"`
	Spending struct {
		Total     int64 `json:"total"`
		Essential int64 `json:"essential"`
		Optional  int64 `json:"optional"`
		Unmarked  int64 `json:"unmarked"`
	} `json:"spending"`
	Income     int64    `json:"income"`
	SavingRate *float64 `json:"savingRate"`
	NetWorth   struct {
		Total   int64 `json:"total"`
		Missing []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"missing"`
	} `json:"netWorth"`
}

func TestTheMoneyScreenListsTheCompleteMonthsNewestFirst(t *testing.T) {
	_, phone := signedInBrowser(t) // today is 23 November 2026
	session := phone.tools()

	if got := get[[]monthSummaryJSON](phone, "/api/money/months", http.StatusOK); got == nil || len(got) != 0 {
		t.Fatalf("with nothing in: %+v; want []", got)
	}

	bank := saveAccount(t, session, map[string]any{"name": "Bank", "kind": "bank", "currency": "JPY", "trackedFrom": "2026-08"})
	rentMonth(t, session, bank, "2026-08", 60000, 100000)
	rentMonth(t, session, bank, "2026-09", 70000, 110000)
	saveStatement(t, session, map[string]any{
		"account": bank.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "csv", "closingBalance": 120000,
		"transactions": []any{
			map[string]any{"kind": "income", "day": "2026-10-25", "amount": 100000, "rawText": "PAY", "category": "salary"},
			map[string]any{"kind": "expense", "day": "2026-10-05", "amount": 60000, "rawText": "RENT", "category": "rent", "essential": true},
			map[string]any{"kind": "expense", "day": "2026-10-09", "amount": 20000, "rawText": "CAFE", "category": "eating_out", "essential": false},
		},
	})
	// November is under way, so not Complete: it is in no list.
	saveStatement(t, session, map[string]any{"account": bank.ID, "from": "2026-11-01", "to": "2026-11-20", "source": "csv"})

	got := get[[]monthSummaryJSON](phone, "/api/money/months", http.StatusOK)

	if len(got) != 3 || got[0].Month != "2026-10" || got[1].Month != "2026-09" || got[2].Month != "2026-08" {
		t.Fatalf("months = %+v; want October, September and August, newest first", got)
	}
	october := got[0]
	if october.Spending.Total != 80000 || october.Spending.Essential != 60000 || october.Spending.Optional != 20000 || october.Spending.Unmarked != 0 {
		t.Errorf("October's spending = %+v; want 80,000: 60,000 Essential and 20,000 Optional", october.Spending)
	}
	if october.Income != 100000 || october.SavingRate == nil || *october.SavingRate != 0.2 {
		t.Errorf("October's income = %d, saving rate %v; want 100,000 and 0.2", october.Income, october.SavingRate)
	}
	if october.NetWorth.Total != 120000 || got[2].NetWorth.Total != 100000 || len(october.NetWorth.Missing) != 0 {
		t.Errorf("net worth = %+v, then %+v; want 120,000 in October and 100,000 in August", october.NetWorth, got[2].NetWorth)
	}
	if got[1].SavingRate != nil {
		t.Errorf("September's saving rate = %v; want none without income", *got[1].SavingRate)
	}
}

// screenMonthAndCard saves a bank in yen and a card in pesos, both tracked
// from October 2026, each with a Statement for October, so October is
// Complete: 60,000 of rent from the bank, and MX$1,000 eating out and a
// MX$250 Refund of it, Unconfirmed, on the card, at October's average of 8.
func screenMonthAndCard(t *testing.T, phone *browser) (bank, card accountJSON) {
	t.Helper()
	session := phone.tools()
	bank = saveAccount(t, session, map[string]any{"name": "Bank", "kind": "bank", "currency": "JPY", "trackedFrom": "2026-10"})
	card = saveAccount(t, session, map[string]any{"name": "Card", "kind": "card", "currency": "MXN", "trackedFrom": "2026-10"})
	saveStatement(t, session, map[string]any{
		"account": bank.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "csv", "closingBalance": 500000,
		"transactions": []any{map[string]any{"kind": "expense", "day": "2026-10-05", "amount": 60000, "rawText": "RENT", "category": "rent"}},
	})
	saveStatement(t, session, map[string]any{
		"account": card.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "pdf", "closingBalance": 750,
		"transactions": []any{
			map[string]any{"kind": "expense", "day": "2026-10-10", "amount": 1000, "rawText": "TACOS", "category": "eating_out"},
			map[string]any{"kind": "expense", "day": "2026-10-12", "amount": -250, "rawText": "TACOS REFUND", "category": "eating_out", "unconfirmed": true},
		},
	})
	return bank, card
}

func TestTheMoneyScreenReadsACompleteMonthsTotals(t *testing.T) {
	_, phone := signedInBrowser(t)
	screenMonthAndCard(t, phone)

	got := get[totalsJSON](phone, "/api/money/months/2026-10", http.StatusOK)

	if got.Month != "2026-10" || !got.Complete || got.Spending.Total != 66000 || len(got.Spending.ByCategory) != 2 ||
		got.Spending.ByCategory[0].Category != "rent" || got.Spending.ByCategory[1].Total != 6000 {
		t.Errorf("October = %+v; want 66,000: rent 60,000 and eating out MX$750 in yen, 6,000", got.Spending)
	}
	if got.NetWorth.Total != 500000-6750 || got.Unconfirmed.Count != 1 {
		t.Errorf("net worth %d, unconfirmed %+v; want 493,250, the card's MX$750 owed at 9, and the Refund", got.NetWorth.Total, got.Unconfirmed)
	}
}

func TestAMonthThatIsntCompleteIsNotFound(t *testing.T) {
	_, phone := signedInBrowser(t)
	screenMonthAndCard(t, phone)

	for _, month := range []string{"2026-11", "2026-09", "2026-13", "october"} {
		phone.must(http.MethodGet, "/api/money/months/"+month, nil, http.StatusNotFound)
	}
}

// categoryPageJSON is one Category's page for a month.
type categoryPageJSON struct {
	Month         string `json:"month"`
	Category      string `json:"category"`
	Name          string `json:"name"`
	Total         int64  `json:"total"`
	Subcategories []struct {
		Subcategory string `json:"subcategory"`
		Name        string `json:"name"`
		Total       int64  `json:"total"`
	} `json:"subcategories"`
	Transactions []struct {
		ID          string  `json:"id"`
		Day         string  `json:"day"`
		Merchant    string  `json:"merchant"`
		Amount      float64 `json:"amount"`
		Currency    string  `json:"currency"`
		Yen         int64   `json:"yen"`
		Account     string  `json:"account"`
		Subcategory string  `json:"subcategory"`
		Essential   *bool   `json:"essential"`
		Unconfirmed bool    `json:"unconfirmed"`
		Note        string  `json:"note"`
	} `json:"transactions"`
	History []struct {
		Month string `json:"month"`
		Total int64  `json:"total"`
	} `json:"history"`
}

func TestACategorysPageListsItsTransactionsWithNames(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	card := saveAccount(t, session, map[string]any{"name": "Card", "kind": "card", "currency": "MXN", "trackedFrom": "2026-10"})
	tacos := saveSubcategory(t, session, map[string]any{"category": "eating_out", "name": "Tacos", "essential": false})
	place := callTool[merchantJSON](t, session, "money_save_merchant", map[string]any{"name": "Taco Place", "rawTexts": []string{"TACOS 123"}})
	saveStatement(t, session, map[string]any{
		"account": card.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "pdf", "closingBalance": 750,
		"transactions": []any{
			map[string]any{"kind": "expense", "day": "2026-10-12", "amount": -250, "rawText": "REFUND 9", "category": "eating_out", "unconfirmed": true},
			map[string]any{"kind": "expense", "day": "2026-10-10", "amount": 1000, "rawText": "TACOS 123", "category": "eating_out",
				"subcategory": tacos.ID, "merchant": place.ID, "note": "lunch"},
			map[string]any{"kind": "expense", "day": "2026-10-11", "amount": 500, "rawText": "SHOP", "category": "shopping"},
		},
	})

	got := get[categoryPageJSON](phone, "/api/money/months/2026-10/categories/eating_out", http.StatusOK)

	if got.Month != "2026-10" || got.Category != "eating_out" || got.Name != "Eating Out" || got.Total != 6000 {
		t.Errorf("page = %s %s %q %d; want October's Eating Out, 6,000", got.Month, got.Category, got.Name, got.Total)
	}
	if len(got.Transactions) != 2 {
		t.Fatalf("transactions = %+v; want the two eating out, not the shopping", got.Transactions)
	}
	first, refund := got.Transactions[0], got.Transactions[1]
	if first.Day != "2026-10-10" || first.Merchant != "Taco Place" || first.Amount != 1000 || first.Currency != "MXN" ||
		first.Yen != 8000 || first.Account != "Card" || first.Subcategory != "Tacos" || first.Essential == nil || *first.Essential ||
		first.Unconfirmed || first.Note != "lunch" {
		t.Errorf("first = %+v; want the tacos, by Day, with their names, in pesos and yen", first)
	}
	if refund.Merchant != "REFUND 9" || refund.Amount != -250 || refund.Yen != -2000 || !refund.Unconfirmed || refund.Essential != nil {
		t.Errorf("refund = %+v; want its raw text for a name, negative, Unconfirmed and unmarked", refund)
	}
	if len(got.Subcategories) != 2 || got.Subcategories[0].Name != "Tacos" || got.Subcategories[0].Total != 8000 ||
		got.Subcategories[1].Subcategory != "" || got.Subcategories[1].Total != -2000 {
		t.Errorf("subcategories = %+v; want Tacos 8,000, then -2,000 with none", got.Subcategories)
	}
}

func TestACategoryWithoutSubcategoriesListsNone(t *testing.T) {
	_, phone := signedInBrowser(t)
	screenMonthAndCard(t, phone)

	got := get[categoryPageJSON](phone, "/api/money/months/2026-10/categories/rent", http.StatusOK)

	if got.Subcategories == nil || len(got.Subcategories) != 0 || len(got.Transactions) != 1 {
		t.Errorf("page = %+v; want [] Subcategories and the rent", got)
	}
}

func TestACategorysPageHasItsLastTwelveCompleteMonths(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	bank := saveAccount(t, session, map[string]any{"name": "Bank", "kind": "bank", "currency": "JPY", "trackedFrom": "2025-09"})
	cafe := func(month, to string, yen float64) {
		saveStatement(t, session, map[string]any{
			"account": bank.ID, "from": month + "-01", "to": to, "source": "csv",
			"transactions": []any{map[string]any{"kind": "expense", "day": month + "-03", "amount": yen, "rawText": "CAFE", "category": "eating_out"}},
		})
	}
	// September 2025 is past the twelve months; December 2025 has no Statement,
	// so it isn't Complete.
	cafe("2025-09", "2025-09-30", 100)
	cafe("2025-10", "2025-10-31", 200)
	cafe("2025-11", "2025-11-30", 300)
	for _, m := range []string{"2026-01", "2026-02", "2026-03", "2026-04", "2026-05", "2026-06", "2026-07", "2026-08", "2026-09"} {
		saveStatement(t, session, map[string]any{"account": bank.ID, "from": m + "-01", "to": lastDayOf(m), "source": "csv"})
	}
	cafe("2026-10", "2026-10-31", 400)

	got := get[categoryPageJSON](phone, "/api/money/months/2026-09/categories/eating_out", http.StatusOK)

	var months []string
	for _, h := range got.History {
		months = append(months, h.Month)
	}
	want := []string{"2025-10", "2025-11", "2026-01", "2026-02", "2026-03", "2026-04", "2026-05", "2026-06", "2026-07", "2026-08", "2026-09"}
	if !slices.Equal(months, want) || got.History[0].Total != 200 || got.History[1].Total != 300 || got.History[2].Total != 0 {
		t.Errorf("history = %+v; want the Complete months from October 2025 to September 2026, oldest first", got.History)
	}
}

func TestAnUnknownOrIncomeCategoryIsNotFound(t *testing.T) {
	_, phone := signedInBrowser(t)
	screenMonthAndCard(t, phone)

	for _, path := range []string{"2026-10/categories/toys", "2026-10/categories/salary", "2026-11/categories/rent"} {
		phone.must(http.MethodGet, "/api/money/months/"+path, nil, http.StatusNotFound)
	}
}

// lastDayOf is the last Day of month, YYYY-MM.
func lastDayOf(month string) string {
	t, _ := time.Parse("2006-01", month)
	return t.AddDate(0, 1, -1).Format("2006-01-02")
}

// subscriptionsPageJSON is the Money screen's Subscriptions.
type subscriptionsPageJSON struct {
	MonthlyTotal  int64 `json:"monthlyTotal"`
	Subscriptions []struct {
		ID           string  `json:"id"`
		Merchant     string  `json:"merchant"`
		MerchantName string  `json:"merchantName"`
		Amount       float64 `json:"amount"`
		Currency     string  `json:"currency"`
		Every        string  `json:"every"`
		Active       bool    `json:"active"`
		Late         bool    `json:"late"`
		MonthlyYen   *int64  `json:"monthlyYen"`
	} `json:"subscriptions"`
}

func TestTheSubscriptionsPageCostsEachInYenAMonth(t *testing.T) {
	_, phone := signedInBrowser(t) // today is 23 November 2026: October's average is 8
	session := phone.tools()
	merchant := func(name string) string {
		return callTool[merchantJSON](t, session, "money_save_merchant", map[string]any{"name": name, "category": "subscriptions"}).ID
	}
	save := func(merchant string, amount float64, currency, every, lastCharged string) string {
		return callTool[subscriptionJSON](t, session, "money_save_subscription", map[string]any{
			"merchant": merchant, "amount": amount, "currency": currency, "every": every, "lastCharged": lastCharged,
			"category": "subscriptions", "essential": false,
		}).ID
	}
	music := save(merchant("Music"), 980, "JPY", "monthly", "2026-11-15")
	storage := save(merchant("Storage"), 12000, "JPY", "yearly", "2026-03-01")
	video := save(merchant("Video"), 100, "MXN", "monthly", "2026-11-10")
	stopped := save(merchant("Old"), 500, "JPY", "monthly", "2026-11-01")
	callTool[subscriptionJSON](t, session, "money_save_subscription", map[string]any{"id": stopped, "active": false})

	got := get[subscriptionsPageJSON](phone, "/api/money/subscriptions", http.StatusOK)

	monthly := map[string]*int64{}
	names := map[string]string{}
	for _, sub := range got.Subscriptions {
		monthly[sub.ID], names[sub.ID] = sub.MonthlyYen, sub.MerchantName
	}
	if len(got.Subscriptions) != 4 || names[music] != "Music" || names[video] != "Video" {
		t.Fatalf("subscriptions = %+v; want all four, with their Merchants' names", got.Subscriptions)
	}
	if monthly[music] == nil || *monthly[music] != 980 || monthly[storage] == nil || *monthly[storage] != 1000 ||
		monthly[video] == nil || *monthly[video] != 800 || monthly[stopped] != nil {
		t.Errorf("monthly = 980 %v, 1,000 %v, 800 %v, none %v; want a yearly one by 12, pesos at October's 8, none when stopped",
			monthly[music], monthly[storage], monthly[video], monthly[stopped])
	}
	if got.MonthlyTotal != 2780 {
		t.Errorf("monthly total = %d; want 2,780, the three active", got.MonthlyTotal)
	}
}

// netWorthMonthJSON is one Complete month's net worth, as the Money screen
// charts it.
type netWorthMonthJSON struct {
	Month    string `json:"month"`
	Total    int64  `json:"total"`
	Accounts []struct {
		Account  string  `json:"account"`
		Name     string  `json:"name"`
		Currency string  `json:"currency"`
		Balance  float64 `json:"balance"`
		Yen      int64   `json:"yen"`
	} `json:"accounts"`
	Returns []struct {
		Account string `json:"account"`
		Name    string `json:"name"`
		Yen     int64  `json:"yen"`
	} `json:"returns"`
	Missing []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"missing"`
}

func TestNetWorthIsListedForEachCompleteMonthWithItsReturn(t *testing.T) {
	_, phone := signedInBrowser(t) // September's Rate is 6; October ends at 9
	session := phone.tools()
	bank := saveAccount(t, session, map[string]any{"name": "Bank", "kind": "bank", "currency": "JPY", "trackedFrom": "2026-09"})
	pot := saveAccount(t, session, map[string]any{"name": "Pot", "kind": "savings", "currency": "MXN", "trackedFrom": "2026-09"})
	balance := func(account accountJSON, from, to string, amount float64, transactions []any) {
		saveStatement(t, session, map[string]any{
			"account": account.ID, "from": from, "to": to, "source": "screenshots", "closingBalance": amount, "transactions": transactions,
		})
	}
	balance(bank, "2026-09-01", "2026-09-30", 100000, []any{})
	balance(pot, "2026-09-01", "2026-09-30", 1000, []any{})
	balance(bank, "2026-10-01", "2026-10-31", 120000, []any{})
	// MX$500 put in, and MX$100 grown.
	balance(pot, "2026-10-01", "2026-10-31", 1600, []any{
		map[string]any{"kind": "transfer", "direction": "in", "day": "2026-10-02", "amount": 500, "rawText": "DEPOSIT"},
	})

	got := get[[]netWorthMonthJSON](phone, "/api/money/net-worth", http.StatusOK)

	if len(got) != 2 || got[0].Month != "2026-10" || got[0].Total != 134400 || got[1].Month != "2026-09" || got[1].Total != 106000 {
		t.Fatalf("net worth = %+v; want October's 134,400, then September's 106,000", got)
	}
	if len(got[0].Accounts) != 2 || len(got[0].Missing) != 0 {
		t.Errorf("October's accounts = %+v; want both balances", got[0].Accounts)
	}
	for _, a := range got[0].Accounts {
		if a.Name == "Pot" && (a.Currency != "MXN" || a.Balance != 1600 || a.Yen != 14400) {
			t.Errorf("Pot = %+v; want MX$1,600, 14,400 at October's end", a)
		}
	}
	if len(got[0].Returns) != 1 || got[0].Returns[0].Name != "Pot" || got[0].Returns[0].Yen != 900 {
		t.Errorf("October's returns = %+v; want the Pot's MX$100, 900", got[0].Returns)
	}
	if len(got[1].Returns) != 0 {
		t.Errorf("September's returns = %+v; want none, with no month before", got[1].Returns)
	}
}

func TestTheMoneyScreensRoutesNeedASession(t *testing.T) {
	n, _ := signedInBrowser(t)
	stranger := &browser{nando: n}

	for _, path := range []string{"/api/money/months", "/api/money/months/2026-10", "/api/money/months/2026-10/categories/rent",
		"/api/money/subscriptions", "/api/money/net-worth"} {
		stranger.must(http.MethodGet, path, nil, http.StatusUnauthorized)
	}
}
