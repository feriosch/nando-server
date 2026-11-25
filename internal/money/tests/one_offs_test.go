package tests

import (
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// oneOffTotalsJSON is what a month's totals say about One-offs.
type oneOffTotalsJSON struct {
	Spending struct {
		Total      int64  `json:"total"`
		OneOff     int64  `json:"oneOff"`
		Previous   *int64 `json:"previous"`
		Average3   *int64 `json:"average3"`
		Unmarked   int64  `json:"unmarked"`
		ByCategory []struct {
			Category string `json:"category"`
			Total    int64  `json:"total"`
			OneOff   int64  `json:"oneOff"`
			Previous *int64 `json:"previous"`
			Average3 *int64 `json:"average3"`
		} `json:"byCategory"`
	} `json:"spending"`
	SavingRate *float64 `json:"savingRate"`
}

// shoppingMonths saves a bank in yen, tracked from August 2026, with 10,000
// of shopping in August, the same in September with a 100,000 laptop marked
// a One-off and a 30,000 trade-in credit for it, also a One-off, and 13,000
// in October. It answers the bank and the laptop's Transaction.
func shoppingMonths(t *testing.T, session *mcp.ClientSession) (accountJSON, transactionJSON) {
	t.Helper()
	bank := saveAccount(t, session, map[string]any{"name": "Bank", "kind": "bank", "currency": "JPY", "trackedFrom": "2026-08"})
	shop := func(day string, yen float64) map[string]any {
		return map[string]any{"kind": "expense", "day": day, "amount": yen, "rawText": "SHOP", "category": "shopping"}
	}
	saveStatement(t, session, map[string]any{
		"account": bank.ID, "from": "2026-08-01", "to": "2026-08-31", "source": "csv", "transactions": []any{shop("2026-08-04", 10000)},
	})
	september := saveStatement(t, session, map[string]any{
		"account": bank.ID, "from": "2026-09-01", "to": "2026-09-30", "source": "csv",
		"transactions": []any{
			shop("2026-09-04", 10000),
			map[string]any{"kind": "expense", "day": "2026-09-10", "amount": 100000, "rawText": "LAPTOP", "category": "shopping", "oneOff": true},
			map[string]any{"kind": "expense", "day": "2026-09-12", "amount": -30000, "rawText": "TRADE IN", "category": "shopping", "oneOff": true},
		},
	})
	saveStatement(t, session, map[string]any{
		"account": bank.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "csv", "transactions": []any{shop("2026-10-04", 13000)},
	})
	for _, tr := range september.Transactions {
		if tr.RawText == "LAPTOP" {
			return bank, tr
		}
	}
	t.Fatal("the laptop wasn't saved")
	return bank, transactionJSON{}
}

func TestAOneOffCountsInItsMonth(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	shoppingMonths(t, session)

	got := callTool[oneOffTotalsJSON](t, session, "money_get_month_totals", map[string]any{"month": "2026-09"})

	if got.Spending.Total != 80000 || got.Spending.OneOff != 70000 || got.Spending.Unmarked != 80000 {
		t.Errorf("September's spending = %+v; want 80,000 in all, 70,000 of it One-off, the trade-in lowering it", got.Spending)
	}
	if len(got.Spending.ByCategory) != 1 || got.Spending.ByCategory[0].OneOff != 70000 {
		t.Errorf("by Category = %+v; want Shopping's 70,000 One-off", got.Spending.ByCategory)
	}
	if p := got.Spending.Previous; p == nil || *p != 10000 {
		t.Errorf("previous = %v; want August's 10,000", p)
	}
}

func TestTheMonthsAfterAOneOffCompareWithoutIt(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	shoppingMonths(t, session)

	got := callTool[oneOffTotalsJSON](t, session, "money_get_month_totals", map[string]any{"month": "2026-10"})

	if p, a := got.Spending.Previous, got.Spending.Average3; p == nil || *p != 10000 || a == nil || *a != 10000 {
		t.Errorf("October's previous %v and average %v; want 10,000 each, without September's One-offs", p, a)
	}
	shopping := got.Spending.ByCategory[0]
	if p, a := shopping.Previous, shopping.Average3; p == nil || *p != 10000 || a == nil || *a != 10000 {
		t.Errorf("Shopping's previous %v and average %v; want 10,000 each", p, a)
	}

	months := callTool[[]struct {
		Month    string `json:"month"`
		Spending int64  `json:"spending"`
		OneOff   int64  `json:"oneOff"`
	}](t, session, "money_get_totals", map[string]any{"fromMonth": "2026-09", "toMonth": "2026-09"})
	if len(months) != 1 || months[0].Spending != 80000 || months[0].OneOff != 70000 {
		t.Errorf("money_get_totals = %+v; want September's 80,000, 70,000 of it One-off", months)
	}
}

func TestOnlyAnExpenseCanBeAOneOff(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	bank := saveAccount(t, session, map[string]any{"name": "Bank", "kind": "bank", "currency": "JPY", "trackedFrom": "2026-10"})

	for _, line := range []map[string]any{
		{"kind": "income", "day": "2026-10-25", "amount": 100000, "rawText": "PAY", "category": "salary", "oneOff": true},
		{"kind": "transfer", "day": "2026-10-26", "amount": 5000, "direction": "out", "rawText": "MOVE", "oneOff": true},
	} {
		got := toolError(t, session, "money_save_statement", map[string]any{
			"account": bank.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "csv", "transactions": []any{line},
		})
		if !strings.Contains(got, "oneOff") {
			t.Errorf("a %s marked One-off: %q; want it refused, naming oneOff", line["kind"], got)
		}
	}
}

// oneOffTransactionJSON is a Transaction, with its One-off mark.
type oneOffTransactionJSON struct {
	ID     string `json:"id"`
	OneOff bool   `json:"oneOff"`
}

func TestAOneOffIsMarkedClearedAndFound(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	_, laptop := shoppingMonths(t, session)

	if got := callTool[[]oneOffTransactionJSON](t, session, "money_list_transactions", map[string]any{"oneOff": true}); len(got) != 2 {
		t.Errorf("One-offs = %+v; want the laptop and its trade-in", got)
	}
	cleared := callTool[oneOffTransactionJSON](t, session, "money_update_transaction", map[string]any{"id": laptop.ID, "oneOff": false})
	if cleared.OneOff {
		t.Errorf("after clearing: %+v; want it no longer a One-off", cleared)
	}
	if got := callTool[[]oneOffTransactionJSON](t, session, "money_list_transactions", map[string]any{"oneOff": false, "fromMonth": "2026-09", "toMonth": "2026-09"}); len(got) != 2 {
		t.Errorf("not One-offs in September = %+v; want the 10,000 and the laptop", got)
	}
	marked := callTool[oneOffTransactionJSON](t, session, "money_update_transaction", map[string]any{"id": laptop.ID, "oneOff": true})
	if !marked.OneOff {
		t.Errorf("after marking: %+v; want it a One-off", marked)
	}
}

func TestTheWebAppReadsOneOffs(t *testing.T) {
	_, phone := signedInBrowser(t) // today is 23 November 2026
	shoppingMonths(t, phone.tools())

	months := get[[]struct {
		Month    string `json:"month"`
		Spending struct {
			OneOff int64 `json:"oneOff"`
		} `json:"spending"`
	}](phone, "/api/money/months", http.StatusOK)
	if len(months) != 3 || months[1].Month != "2026-09" || months[1].Spending.OneOff != 70000 {
		t.Errorf("months = %+v; want September's 70,000 One-off", months)
	}

	page := get[struct {
		Total        int64  `json:"total"`
		OneOff       int64  `json:"oneOff"`
		Average3     *int64 `json:"average3"`
		Transactions []struct {
			Merchant string `json:"merchant"`
			OneOff   bool   `json:"oneOff"`
		} `json:"transactions"`
		History []struct {
			Month  string `json:"month"`
			OneOff int64  `json:"oneOff"`
		} `json:"history"`
	}](phone, "/api/money/months/2026-10/categories/shopping", http.StatusOK)
	if page.OneOff != 0 || page.Average3 == nil || *page.Average3 != 10000 {
		t.Errorf("October's Shopping = %d One-off, average %v; want none, and 10,000 without September's", page.OneOff, page.Average3)
	}
	if len(page.History) != 3 || page.History[1].OneOff != 70000 {
		t.Errorf("history = %+v; want September's 70,000 One-off", page.History)
	}

	september := get[struct {
		OneOff       int64 `json:"oneOff"`
		Transactions []struct {
			Merchant string `json:"merchant"`
			OneOff   bool   `json:"oneOff"`
		} `json:"transactions"`
	}](phone, "/api/money/months/2026-09/categories/shopping", http.StatusOK)
	if september.OneOff != 70000 || len(september.Transactions) != 3 || september.Transactions[0].OneOff || !september.Transactions[1].OneOff {
		t.Errorf("September's Shopping = %+v; want its One-offs marked", september)
	}

	card := get[struct {
		Spending struct {
			OneOff int64 `json:"oneOff"`
		} `json:"spending"`
	}](phone, "/api/money/card", http.StatusOK)
	if card.Spending.OneOff != 0 {
		t.Errorf("October's card = %+v; want no One-off", card)
	}
}
