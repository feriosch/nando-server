package tests

import (
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// accountJSON is an Account, as the Tools read it.
type accountJSON struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Currency    string `json:"currency"`
	Institution string `json:"institution"`
	Last4       string `json:"last4"`
	Note        string `json:"note"`
	TrackedFrom string `json:"trackedFrom"`
	ClosedFrom  string `json:"closedFrom"`
	HoldsMonth  bool   `json:"holdsMonth"`
}

// saveAccount saves an Account with args and answers it.
func saveAccount(t *testing.T, session *mcp.ClientSession, args map[string]any) accountJSON {
	t.Helper()
	return callTool[accountJSON](t, session, "money_save_account", args)
}

func TestAnAccountIsSavedChangedAndListed(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()

	bank := saveAccount(t, session, map[string]any{
		"name": " Bank A ", "kind": "bank", "currency": "JPY", "institution": "Bank A", "trackedFrom": "2026-01",
	})
	card := saveAccount(t, session, map[string]any{
		"name": "Card B", "kind": "card", "currency": "MXN", "last4": "1234", "trackedFrom": "2026-03",
	})
	if bank.ID == "" || bank.Name != "Bank A" || !bank.HoldsMonth || card.Last4 != "1234" {
		t.Fatalf("saved %+v and %+v; want ids, a trimmed name, and holdsMonth true by default", bank, card)
	}

	// Only the fields sent change.
	changed := saveAccount(t, session, map[string]any{"id": card.ID, "note": "PDF statements, closes at month end", "holdsMonth": false})
	want := card
	want.Note = "PDF statements, closes at month end"
	want.HoldsMonth = false
	if !reflect.DeepEqual(changed, want) {
		t.Errorf("after changing the note and holdsMonth: %+v, want %+v", changed, want)
	}

	got := callTool[[]accountJSON](t, session, "money_list_accounts", nil)
	if !reflect.DeepEqual(got, []accountJSON{bank, want}) {
		t.Errorf("money_list_accounts = %+v, want both, by name", got)
	}
}

func TestBadAccountsAreRefused(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	good := map[string]any{"name": "Card", "kind": "card", "currency": "JPY", "trackedFrom": "2026-01"}
	with := func(key string, value any) map[string]any {
		args := map[string]any{}
		for k, v := range good {
			args[k] = v
		}
		args[key] = value
		return args
	}

	for name, c := range map[string]struct {
		args map[string]any
		want string
	}{
		"a full card number":          {with("last4", "4111111111111111"), "last4"},
		"letters as digits":           {with("last4", "12ab"), "last4"},
		"an unknown kind":             {with("kind", "wallet"), "kind"},
		"another currency":            {with("currency", "USD"), "currency"},
		"no name":                     {with("name", " "), "name"},
		"a bad month":                 {with("trackedFrom", "January"), "trackedFrom"},
		"closed before it is tracked": {with("closedFrom", "2025-12"), "closedFrom"},
		"an unknown id":               {map[string]any{"id": "no-such-account", "note": "x"}, "no such Account"},
	} {
		if got := toolError(t, session, "money_save_account", c.args); !strings.Contains(got, c.want) {
			t.Errorf("%s: %q; want it to name %q", name, got, c.want)
		}
	}
	if got := callTool[[]accountJSON](t, session, "money_list_accounts", nil); len(got) != 0 {
		t.Errorf("money_list_accounts = %+v; want none saved", got)
	}
}
