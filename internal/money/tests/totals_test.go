package tests

import (
	"reflect"
	"strings"
	"testing"
)

// rateJSON is a month's Rate, pesos in yen.
type rateJSON struct {
	Month       string  `json:"month"`
	Average     float64 `json:"average"`
	MonthEnd    float64 `json:"monthEnd"`
	Provisional bool    `json:"provisional"`
}

// amountJSON is a total in yen, against the month before and the 3 before.
type categoryTotalJSON struct {
	Category string `json:"category"`
	Name     string `json:"name"`
	Total    int64  `json:"total"`
	Previous *int64 `json:"previous"`
	Average3 *int64 `json:"average3"`
}

// totalsJSON is money_get_month_totals' answer.
type totalsJSON struct {
	Month    string    `json:"month"`
	Complete bool      `json:"complete"`
	Rate     *rateJSON `json:"rate"`
	Spending struct {
		Total      int64               `json:"total"`
		Previous   *int64              `json:"previous"`
		Average3   *int64              `json:"average3"`
		Essential  int64               `json:"essential"`
		Optional   int64               `json:"optional"`
		Unmarked   int64               `json:"unmarked"`
		ByCategory []categoryTotalJSON `json:"byCategory"`
	} `json:"spending"`
	Income struct {
		Total      int64               `json:"total"`
		ByCategory []categoryTotalJSON `json:"byCategory"`
	} `json:"income"`
	NetWorth struct {
		Total    int64  `json:"total"`
		Previous *int64 `json:"previous"`
		Accounts []struct {
			Account string  `json:"account"`
			Balance float64 `json:"balance"`
			Yen     int64   `json:"yen"`
		} `json:"accounts"`
	} `json:"netWorth"`
	Returns []struct {
		Account string `json:"account"`
		Yen     int64  `json:"yen"`
	} `json:"returns"`
	SavingRate  *float64 `json:"savingRate"`
	Unconfirmed struct {
		Count int   `json:"count"`
		Yen   int64 `json:"yen"`
	} `json:"unconfirmed"`
}

func TestARateIsFetchedOnceAndNeverChanges(t *testing.T) {
	n, phone := signedInBrowser(t) // today is 23 November 2026
	session := phone.tools()

	n.rates.failing = true
	if msg := toolError(t, session, "money_get_rate", map[string]any{"month": "2026-10"}); !strings.Contains(msg, "Rate") {
		t.Errorf("with Frankfurter down: %q; want it to say the Rate couldn't be fetched", msg)
	}
	n.rates.failing = false
	got := callTool[rateJSON](t, session, "money_get_rate", map[string]any{"month": "2026-10"})
	if got != (rateJSON{Month: "2026-10", Average: 8, MonthEnd: 9}) {
		t.Errorf("October's Rate = %+v; want an average of 8 and 9 at its end", got)
	}
	calls := n.rates.callCount()
	callTool[rateJSON](t, session, "money_get_rate", map[string]any{"month": "2026-10"})
	if n.rates.callCount() != calls {
		t.Errorf("asked again: %d calls, want %d: a stored Rate is never fetched again", n.rates.callCount(), calls)
	}

	// November hasn't ended: its Rate so far is provisional, and not kept.
	if got := callTool[rateJSON](t, session, "money_get_rate", map[string]any{"month": "2026-11"}); !got.Provisional {
		t.Errorf("November's Rate = %+v; want it provisional", got)
	}
	calls = n.rates.callCount()
	callTool[rateJSON](t, session, "money_get_rate", map[string]any{"month": "2026-11"})
	if n.rates.callCount() == calls {
		t.Errorf("a provisional Rate was kept")
	}
}

func TestAMonthsTotalsAreInYen(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	bank := saveAccount(t, session, map[string]any{"name": "Bank", "kind": "bank", "currency": "JPY", "trackedFrom": "2026-10"})
	card := saveAccount(t, session, map[string]any{"name": "Card", "kind": "card", "currency": "MXN", "trackedFrom": "2026-10"})
	fashion := saveSubcategory(t, session, map[string]any{"category": "shopping", "name": "Fashion", "essential": false})
	rent := saveSubcategory(t, session, map[string]any{"category": "rent", "name": "Flat", "essential": true})

	saveStatement(t, session, map[string]any{
		"account": bank.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "csv", "closingBalance": 500000,
		"transactions": []any{
			map[string]any{"kind": "income", "day": "2026-10-25", "amount": 300000, "rawText": "PAY", "category": "salary"},
			map[string]any{"kind": "expense", "day": "2026-10-27", "amount": 90000, "rawText": "RENT", "category": "rent", "subcategory": rent.ID},
			map[string]any{"kind": "transfer", "day": "2026-10-28", "amount": 16000, "direction": "out", "rawText": "CARD", "otherAccount": card.ID},
			map[string]any{"kind": "expense", "day": "2026-10-29", "amount": 3000, "rawText": "SHOP", "category": "other", "unconfirmed": true},
		},
	})
	// In pesos, at October's average of 8 yen: 2,000 pesos of clothes, 500
	// back. The card owes 1,500 pesos at the month's end, at 9 yen.
	saveStatement(t, session, map[string]any{
		"account": card.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "pdf", "total": 1500,
		"transactions": []any{
			map[string]any{"kind": "expense", "day": "2026-10-05", "amount": 2000, "rawText": "STORE", "category": "shopping", "subcategory": fashion.ID},
			map[string]any{"kind": "expense", "day": "2026-10-09", "amount": -500, "rawText": "STORE REFUND", "category": "shopping", "subcategory": fashion.ID},
		},
	})

	got := callTool[totalsJSON](t, session, "money_get_month_totals", map[string]any{"month": "2026-10"})

	// 90,000 rent + 3,000 other + (2,000 − 500) × 8 = 105,000; the transfer is neither.
	if !got.Complete || got.Rate == nil || got.Rate.Average != 8 || got.Spending.Total != 105000 {
		t.Fatalf("October = %+v; want it Complete, at 8, spending 105,000", got)
	}
	if got.Spending.Essential != 90000 || got.Spending.Optional != 12000 || got.Spending.Unmarked != 3000 {
		t.Errorf("essential %d, optional %d, unmarked %d; want 90,000, 12,000 and 3,000",
			got.Spending.Essential, got.Spending.Optional, got.Spending.Unmarked)
	}
	byCategory := map[string]int64{}
	for _, c := range got.Spending.ByCategory {
		byCategory[c.Category] = c.Total
	}
	if !reflect.DeepEqual(byCategory, map[string]int64{"rent": 90000, "shopping": 12000, "other": 3000}) {
		t.Errorf("by Category = %v", byCategory)
	}
	if got.Income.Total != 300000 || got.Spending.Previous != nil || got.Spending.Average3 != nil {
		t.Errorf("income %d, previous %v, average %v; want 300,000, and nothing to compare with yet",
			got.Income.Total, got.Spending.Previous, got.Spending.Average3)
	}
	// 500,000 in the bank, less 1,500 pesos owed at 9 yen.
	if got.NetWorth.Total != 486500 || got.NetWorth.Previous != nil {
		t.Errorf("net worth %d, previous %v; want 486,500, and none before", got.NetWorth.Total, got.NetWorth.Previous)
	}
	if got.SavingRate == nil || *got.SavingRate != 0.65 {
		t.Errorf("saving rate %v; want (300,000 − 105,000) ÷ 300,000 = 0.65", got.SavingRate)
	}
	if got.Unconfirmed.Count != 1 || got.Unconfirmed.Yen != 3000 {
		t.Errorf("unconfirmed %+v; want the one 3,000 yen line", got.Unconfirmed)
	}
}

func TestReturnLeavesOutWhatWasTransferredIn(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	bank := saveAccount(t, session, map[string]any{"name": "Bank", "kind": "bank", "currency": "JPY", "trackedFrom": "2026-09"})
	savings := saveAccount(t, session, map[string]any{"name": "Savings", "kind": "savings", "currency": "JPY", "trackedFrom": "2026-09"})
	saveStatement(t, session, map[string]any{"account": savings.ID, "from": "2026-09-30", "to": "2026-09-30", "source": "screenshots", "closingBalance": 100000})
	saveStatement(t, session, map[string]any{"account": bank.ID, "from": "2026-09-01", "to": "2026-09-30", "source": "csv", "closingBalance": 0})
	// 20,000 moved in from the bank, and it grew by 1,000 more.
	saveStatement(t, session, map[string]any{
		"account": bank.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "csv", "closingBalance": 0,
		"transactions": []any{
			map[string]any{"kind": "income", "day": "2026-10-25", "amount": 50000, "rawText": "PAY", "category": "salary"},
			map[string]any{"kind": "transfer", "day": "2026-10-26", "amount": 20000, "direction": "out", "rawText": "TO SAVINGS", "otherAccount": savings.ID},
			map[string]any{"kind": "expense", "day": "2026-10-27", "amount": 30000, "rawText": "RENT", "category": "rent"},
		},
	})
	saveStatement(t, session, map[string]any{"account": savings.ID, "from": "2026-10-31", "to": "2026-10-31", "source": "screenshots", "closingBalance": 121000})

	got := callTool[totalsJSON](t, session, "money_get_month_totals", map[string]any{"month": "2026-10"})

	if len(got.Returns) != 1 || got.Returns[0].Account != savings.ID || got.Returns[0].Yen != 1000 {
		t.Errorf("returns = %+v; want the savings' 1,000, without the 20,000 moved in", got.Returns)
	}
	// (50,000 − 30,000) ÷ 50,000: the Return is no saving.
	if got.SavingRate == nil || *got.SavingRate != 0.4 {
		t.Errorf("saving rate %v; want 0.4", got.SavingRate)
	}
	if got.NetWorth.Previous == nil || *got.NetWorth.Previous != 100000 || got.NetWorth.Total != 121000 {
		t.Errorf("net worth %d after %v; want 121,000 after 100,000", got.NetWorth.Total, got.NetWorth.Previous)
	}
}

func TestSpendingComparesWithTheMonthBeforeAndTheThreeBefore(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	bank := saveAccount(t, session, map[string]any{"name": "Bank", "kind": "bank", "currency": "JPY", "trackedFrom": "2026-07"})
	for month, rent := range map[string]float64{"07": 60000, "08": 70000, "09": 80000, "10": 100000} {
		saveStatement(t, session, map[string]any{
			"account": bank.ID, "from": "2026-" + month + "-01", "to": "2026-" + month + "-28", "source": "csv",
			"transactions": []any{map[string]any{"kind": "expense", "day": "2026-" + month + "-05", "amount": rent, "rawText": "RENT", "category": "rent"}},
		})
	}

	got := callTool[totalsJSON](t, session, "money_get_month_totals", map[string]any{"month": "2026-10"})

	if got.Spending.Previous == nil || *got.Spending.Previous != 80000 || got.Spending.Average3 == nil || *got.Spending.Average3 != 70000 {
		t.Errorf("previous %v, average %v; want 80,000 and 70,000", got.Spending.Previous, got.Spending.Average3)
	}
	// August has only July before it.
	august := callTool[totalsJSON](t, session, "money_get_month_totals", map[string]any{"month": "2026-08"})
	if august.Spending.Average3 == nil || *august.Spending.Average3 != 60000 {
		t.Errorf("August's average of the months before = %v; want July's 60,000 alone", august.Spending.Average3)
	}

	months := callTool[[]struct {
		Month    string `json:"month"`
		Spending int64  `json:"spending"`
	}](t, session, "money_get_totals", map[string]any{"fromMonth": "2026-08", "toMonth": "2026-10"})
	if len(months) != 3 || months[0].Month != "2026-08" || months[0].Spending != 70000 || months[2].Spending != 100000 {
		t.Errorf("money_get_totals = %+v; want August to October", months)
	}
}
