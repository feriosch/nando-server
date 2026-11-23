package tests

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// transactionJSON is a Transaction, as the Tools read it.
type transactionJSON struct {
	ID           string  `json:"id"`
	Kind         string  `json:"kind"`
	Day          string  `json:"day"`
	Amount       float64 `json:"amount"`
	Direction    string  `json:"direction"`
	RawText      string  `json:"rawText"`
	Note         string  `json:"note"`
	OtherAccount string  `json:"otherAccount"`
	StatementID  string  `json:"statement"`
	AccountID    string  `json:"account"`
}

// statementJSON is a Statement, as the Tools read it.
type statementJSON struct {
	ID             string            `json:"id"`
	Account        string            `json:"account"`
	From           string            `json:"from"`
	To             string            `json:"to"`
	Source         string            `json:"source"`
	FileIDs        []string          `json:"fileIds"`
	Total          *float64          `json:"total"`
	ClosingBalance *float64          `json:"closingBalance"`
	Transactions   []transactionJSON `json:"transactions"`
	Late           int               `json:"late"`
	Skipped        int               `json:"skipped"`
}

// line is one Transaction to save, as the Assistant sends it.
func line(kind, day string, amount float64, rawText string) map[string]any {
	return map[string]any{"kind": kind, "day": day, "amount": amount, "rawText": rawText}
}

// saveStatement saves a Statement with args and answers it.
func saveStatement(t *testing.T, session *mcp.ClientSession, args map[string]any) statementJSON {
	t.Helper()
	return callTool[statementJSON](t, session, "money_save_statement", args)
}

// twoAccounts saves a bank Account in yen and a card in pesos, both tracked
// from January 2026.
func twoAccounts(t *testing.T, session *mcp.ClientSession) (bank, card accountJSON) {
	t.Helper()
	bank = saveAccount(t, session, map[string]any{"name": "Bank", "kind": "bank", "currency": "JPY", "trackedFrom": "2026-01"})
	card = saveAccount(t, session, map[string]any{"name": "Card", "kind": "card", "currency": "MXN", "trackedFrom": "2026-01"})
	return bank, card
}

func TestAStatementIsSavedWithItsTransactionsAndReplacedWhenSentAgain(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	_, card := twoAccounts(t, session)

	first := saveStatement(t, session, map[string]any{
		"account": card.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "pdf", "fileIds": []string{"f1"},
		"total": 1234.50,
		"transactions": []any{
			line("expense", "2026-10-03", 199.9, "OXXO TEST"),
			line("expense", "2026-10-20", 1034.6, "SHOP TEST"),
		},
	})
	if first.ID == "" || first.Total == nil || *first.Total != 1234.5 || len(first.Transactions) != 2 ||
		first.Transactions[0].ID == "" || first.Transactions[0].Amount != 199.9 || first.Transactions[1].RawText != "SHOP TEST" {
		t.Fatalf("money_save_statement = %+v; want it saved, with its 2 Transactions", first)
	}

	again := saveStatement(t, session, map[string]any{
		"account": card.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "pdf", "fileIds": []string{"f2"},
		"transactions": []any{line("expense", "2026-10-05", 50, "CAFE TEST")},
	})
	if again.ID != first.ID {
		t.Errorf("sent again: id %q, want the same Statement %q", again.ID, first.ID)
	}
	got := callTool[statementJSON](t, session, "money_get_statement", map[string]any{"id": first.ID})
	if len(got.Transactions) != 1 || got.Transactions[0].RawText != "CAFE TEST" || got.Total != nil ||
		len(got.FileIDs) != 1 || got.FileIDs[0] != "f2" {
		t.Errorf("money_get_statement = %+v; want only what was sent again", got)
	}
	listed := callTool[[]transactionJSON](t, session, "money_list_transactions", map[string]any{"account": card.ID})
	if len(listed) != 1 || listed[0].RawText != "CAFE TEST" || listed[0].StatementID != first.ID || listed[0].AccountID != card.ID {
		t.Errorf("money_list_transactions = %+v; want only the Transaction sent again", listed)
	}
}

func TestBadStatementsAreRefused(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	bank, card := twoAccounts(t, session)
	good := func() map[string]any {
		return map[string]any{
			"account": bank.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "csv",
			"transactions": []any{line("expense", "2026-10-03", 500, "ATM")},
		}
	}
	with := func(key string, value any) map[string]any {
		args := good()
		args[key] = value
		return args
	}
	withLine := func(l map[string]any) map[string]any { return with("transactions", []any{l}) }
	transfer := line("transfer", "2026-10-27", 80000, "CARD PAYMENT")

	for name, c := range map[string]struct {
		args map[string]any
		want string
	}{
		"an unknown Account":      {with("account", "nope"), "no such Account"},
		"a period backwards":      {with("to", "2026-09-30"), "from"},
		"an unknown source":       {with("source", "fax"), "source"},
		"yen with decimals":       {withLine(line("expense", "2026-10-03", 500.5, "ATM")), "whole yen"},
		"an unknown kind":         {withLine(line("gift", "2026-10-03", 500, "ATM")), "kind"},
		"a line after the period": {withLine(line("expense", "2026-11-02", 500, "ATM")), "after"},
		"a negative income":       {withLine(line("income", "2026-10-25", -1, "PAY")), "amount"},
		"a transfer with no way":  {withLine(transfer), "direction"},
		"a transfer to itself":    {withLine(map[string]any{"kind": "transfer", "day": "2026-10-27", "amount": 1, "rawText": "X", "direction": "out", "otherAccount": bank.ID}), "otherAccount"},
		"a transfer to nowhere":   {withLine(map[string]any{"kind": "transfer", "day": "2026-10-27", "amount": 1, "rawText": "X", "direction": "out", "otherAccount": "nope"}), "no such Account"},
		"a day that isn't one":    {withLine(line("expense", "2026-10-32", 500, "ATM")), "day"},
	} {
		if got := toolError(t, session, "money_save_statement", c.args); !strings.Contains(got, c.want) {
			t.Errorf("%s: %q; want it to name %q", name, got, c.want)
		}
	}
	// Pesos take centavos.
	saveStatement(t, session, map[string]any{
		"account": card.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "pdf",
		"transactions": []any{line("expense", "2026-10-03", 99.99, "OXXO")},
	})
	if got := toolError(t, session, "money_save_statement", map[string]any{
		"account": card.ID, "from": "2026-11-01", "to": "2026-11-30", "source": "pdf",
		"transactions": []any{line("expense", "2026-11-03", 9.999, "OXXO")},
	}); !strings.Contains(got, "centavos") {
		t.Errorf("pesos past centavos: %q; want it refused", got)
	}
}
