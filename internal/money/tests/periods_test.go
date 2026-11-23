package tests

import (
	"reflect"
	"strings"
	"testing"
)

func TestAnOverlappingPeriodIsRefusedNamingTheOtherStatement(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	_, card := twoAccounts(t, session)
	october := saveStatement(t, session, map[string]any{"account": card.ID, "from": "2026-09-28", "to": "2026-10-27", "source": "pdf"})

	got := toolError(t, session, "money_save_statement", map[string]any{
		"account": card.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "pdf",
	})

	if !strings.Contains(got, october.ID) {
		t.Errorf("money_save_statement = %q; want it refused naming %s", got, october.ID)
	}
	// The next cycle, right after it, is fine.
	saveStatement(t, session, map[string]any{"account": card.ID, "from": "2026-10-28", "to": "2026-11-27", "source": "pdf"})
}

func TestALateTransactionJoinsItsOwnMonthAndIsKeptOnce(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	_, card := twoAccounts(t, session)
	saveStatement(t, session, map[string]any{
		"account": card.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "pdf",
		"transactions": []any{line("expense", "2026-10-10", 100, "SHOP")},
	})

	// A purchase of 30 October posted on November's bill, and again on
	// December's.
	november := saveStatement(t, session, map[string]any{
		"account": card.ID, "from": "2026-11-01", "to": "2026-11-30", "source": "pdf",
		"transactions": []any{line("expense", "2026-10-30", 250, "HOTEL"), line("expense", "2026-11-02", 80, "CAFE")},
	})
	december := saveStatement(t, session, map[string]any{
		"account": card.ID, "from": "2026-12-01", "to": "2026-12-31", "source": "pdf",
		"transactions": []any{line("expense", "2026-10-30", 250, "HOTEL"), line("expense", "2026-12-05", 60, "BAR")},
	})

	if november.Late != 1 || november.Skipped != 0 || december.Skipped != 1 || len(december.Transactions) != 1 {
		t.Errorf("November late %d skipped %d, December skipped %d with %d kept; want the late line kept once",
			november.Late, november.Skipped, december.Skipped, len(december.Transactions))
	}
	october := callTool[[]transactionJSON](t, session, "money_list_transactions", map[string]any{"fromMonth": "2026-10", "toMonth": "2026-10"})
	var texts []string
	for _, tr := range october {
		texts = append(texts, tr.RawText)
	}
	if !reflect.DeepEqual(texts, []string{"SHOP", "HOTEL"}) {
		t.Errorf("October's Transactions = %q; want the late one in October, once", texts)
	}
	// November sent again keeps its late line: it replaces itself.
	again := saveStatement(t, session, map[string]any{
		"account": card.ID, "from": "2026-11-01", "to": "2026-11-30", "source": "pdf",
		"transactions": []any{line("expense", "2026-10-30", 250, "HOTEL"), line("expense", "2026-11-02", 80, "CAFE")},
	})
	if again.Late != 1 || again.Skipped != 0 {
		t.Errorf("November sent again: late %d, skipped %d; want its late line kept", again.Late, again.Skipped)
	}
}

func TestADeletedStatementTakesItsTransactions(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	bank, _ := twoAccounts(t, session)
	s := saveStatement(t, session, map[string]any{
		"account": bank.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "csv",
		"transactions": []any{line("expense", "2026-10-10", 1000, "ATM")},
	})

	callTool[map[string]bool](t, session, "money_delete_statement", map[string]any{"id": s.ID})

	if got := callTool[[]transactionJSON](t, session, "money_list_transactions", nil); len(got) != 0 {
		t.Errorf("money_list_transactions = %+v; want none left", got)
	}
	if got := toolError(t, session, "money_get_statement", map[string]any{"id": s.ID}); !strings.Contains(got, "no such Statement") {
		t.Errorf("money_get_statement after deleting = %q", got)
	}
}

// monthJSON is money_get_month's answer.
type monthJSON struct {
	Month    string `json:"month"`
	Complete bool   `json:"complete"`
	Missing  []struct {
		ID string `json:"id"`
	} `json:"missing"`
	Late []struct {
		ID string `json:"id"`
	} `json:"late"`
}

func TestAMonthIsCompleteOnceEveryAccountThatHoldsItReachesItsEnd(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	bank, card := twoAccounts(t, session)
	rent := saveAccount(t, session, map[string]any{"name": "Rent card", "kind": "card", "currency": "JPY", "trackedFrom": "2026-01", "holdsMonth": false})
	savings := saveAccount(t, session, map[string]any{"name": "Savings", "kind": "savings", "currency": "MXN", "trackedFrom": "2026-01"})
	saveAccount(t, session, map[string]any{"name": "Old", "kind": "bank", "currency": "JPY", "trackedFrom": "2026-01", "closedFrom": "2026-09"})
	saveAccount(t, session, map[string]any{"name": "New", "kind": "investment", "currency": "JPY", "trackedFrom": "2026-11"})

	month := func() monthJSON {
		return callTool[monthJSON](t, session, "money_get_month", map[string]any{"month": "2026-10"})
	}
	ids := func(refs []struct {
		ID string `json:"id"`
	}) []string {
		var got []string
		for _, r := range refs {
			got = append(got, r.ID)
		}
		return got
	}
	if got := month(); got.Complete || !reflect.DeepEqual(ids(got.Missing), []string{bank.ID, card.ID, savings.ID}) ||
		!reflect.DeepEqual(ids(got.Late), []string{rent.ID}) {
		t.Fatalf("with nothing in: %+v; want bank, card and savings missing, the rent card late", got)
	}

	saveStatement(t, session, map[string]any{"account": bank.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "csv"})
	// A card closing on the 27th reaches 31 October only with November's cycle.
	saveStatement(t, session, map[string]any{"account": card.ID, "from": "2026-09-28", "to": "2026-10-27", "source": "pdf"})
	if got := month(); got.Complete || !reflect.DeepEqual(ids(got.Missing), []string{card.ID, savings.ID}) {
		t.Errorf("with October's cycle of the card: %+v; want the card and savings still missing", got)
	}
	saveStatement(t, session, map[string]any{"account": card.ID, "from": "2026-10-28", "to": "2026-11-27", "source": "pdf"})
	// A balance on a later Day is enough for savings.
	saveStatement(t, session, map[string]any{"account": savings.ID, "from": "2026-11-03", "to": "2026-11-03", "source": "screenshots", "closingBalance": 5000})

	if got := month(); !got.Complete || len(got.Missing) != 0 || !reflect.DeepEqual(ids(got.Late), []string{rent.ID}) {
		t.Errorf("with every Account that holds October in: %+v; want it Complete, the rent card still late", got)
	}
}

func TestATransactionsNoteAndOtherAccountChange(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	bank, card := twoAccounts(t, session)
	s := saveStatement(t, session, map[string]any{
		"account": bank.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "csv",
		"transactions": []any{
			line("expense", "2026-10-10", 2480, "ONLINE STORE"),
			map[string]any{"kind": "transfer", "day": "2026-10-27", "amount": 80000, "rawText": "CARD", "direction": "out"},
		},
	})
	expense, transfer := s.Transactions[0], s.Transactions[1]

	got := callTool[transactionJSON](t, session, "money_update_transaction", map[string]any{"id": expense.ID, "note": " trash bin "})
	if got.Note != "trash bin" || got.Amount != 2480 || got.StatementID != s.ID {
		t.Errorf("after a note: %+v", got)
	}
	got = callTool[transactionJSON](t, session, "money_update_transaction", map[string]any{"id": transfer.ID, "otherAccount": card.ID})
	if got.OtherAccount != card.ID || got.Direction != "out" {
		t.Errorf("after the other Account: %+v", got)
	}
	if msg := toolError(t, session, "money_update_transaction", map[string]any{"id": expense.ID, "otherAccount": card.ID}); !strings.Contains(msg, "transfer") {
		t.Errorf("an expense's other Account: %q; want it refused", msg)
	}
	if msg := toolError(t, session, "money_update_transaction", map[string]any{"id": "nope", "note": "x"}); !strings.Contains(msg, "no such Transaction") {
		t.Errorf("an unknown Transaction: %q", msg)
	}
	stored := callTool[statementJSON](t, session, "money_get_statement", map[string]any{"id": s.ID})
	if stored.Transactions[0].Note != "trash bin" || stored.Transactions[1].OtherAccount != card.ID {
		t.Errorf("money_get_statement = %+v; want both changes kept", stored.Transactions)
	}
}
