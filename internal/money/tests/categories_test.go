package tests

import (
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// categoryJSON is a Category with its Subcategories, as the Tools read it.
type categoryJSON struct {
	Key           string            `json:"key"`
	Name          string            `json:"name"`
	Kind          string            `json:"kind"`
	Subcategories []subcategoryJSON `json:"subcategories"`
}

// subcategoryJSON is a Subcategory, as the Tools read it.
type subcategoryJSON struct {
	ID        string `json:"id"`
	Category  string `json:"category"`
	Name      string `json:"name"`
	Essential bool   `json:"essential"`
}

// whatItIs is what a Transaction is, as the Tools read it with it.
type whatItIs struct {
	ID                string `json:"id"`
	Category          string `json:"category"`
	Subcategory       string `json:"subcategory"`
	Merchant          string `json:"merchant"`
	EssentialOverride *bool  `json:"essentialOverride"`
	Essential         *bool  `json:"essential"`
	Unconfirmed       bool   `json:"unconfirmed"`
	RawText           string `json:"rawText"`
}

// saveSubcategory saves a Subcategory with args and answers it.
func saveSubcategory(t *testing.T, session *mcp.ClientSession, args map[string]any) subcategoryJSON {
	t.Helper()
	return callTool[subcategoryJSON](t, session, "money_save_subcategory", args)
}

// oneStatement saves a bank Account's October Statement with lines, and
// answers what each Transaction is, in order.
func oneStatement(t *testing.T, session *mcp.ClientSession, lines ...map[string]any) (accountJSON, []whatItIs) {
	t.Helper()
	bank, _ := twoAccounts(t, session)
	s := saveStatement(t, session, map[string]any{
		"account": bank.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "csv", "transactions": lines,
	})
	got := callTool[[]whatItIs](t, session, "money_list_transactions", map[string]any{"account": bank.ID})
	if len(got) != len(s.Transactions) {
		t.Fatalf("listed %d of %d Transactions", len(got), len(s.Transactions))
	}
	return bank, got
}

func TestTheCategoriesAreAFixedListWithTheirSubcategories(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	fashion := saveSubcategory(t, session, map[string]any{"category": "shopping", "name": " Fashion ", "essential": false})

	got := callTool[[]categoryJSON](t, session, "money_list_categories", nil)

	var names []string
	var shopping categoryJSON
	for _, c := range got {
		names = append(names, c.Name)
		if c.Key == "shopping" {
			shopping = c
		}
	}
	want := []string{
		"Rent", "Water, Gas & Electricity", "Phone & Internet", "Groceries", "Eating Out", "Convenience Store",
		"Public Transport", "Travel", "Subscriptions", "Shopping", "Personal Care", "Beauty", "Health",
		"Entertainment", "Education", "Gifts", "Fees & Interest", "Cash & PayPay (unitemized)", "Other",
		"Salary", "Interest", "Other income",
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("money_list_categories names = %q, want %q", names, want)
	}
	if shopping.Kind != "expense" || len(shopping.Subcategories) != 1 || shopping.Subcategories[0] != fashion || fashion.Name != "Fashion" {
		t.Errorf("Shopping = %+v; want an expense Category with Fashion", shopping)
	}
}

func TestWhatATransactionIsMustFitItsKind(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	bank, _ := twoAccounts(t, session)
	fashion := saveSubcategory(t, session, map[string]any{"category": "shopping", "name": "Fashion", "essential": false})
	with := func(extra map[string]any) map[string]any {
		l := line("expense", "2026-10-03", 500, "SHOP")
		for k, v := range extra {
			l[k] = v
		}
		return map[string]any{"account": bank.ID, "from": "2026-10-01", "to": "2026-10-31", "source": "csv", "transactions": []any{l}}
	}

	for name, c := range map[string]struct {
		args map[string]any
		want string
	}{
		"an unknown Category":              {with(map[string]any{"category": "toys"}), "category"},
		"a Subcategory of another":         {with(map[string]any{"category": "groceries", "subcategory": fashion.ID}), "subcategory"},
		"an income Category on an expense": {with(map[string]any{"category": "salary"}), "category"},
		"an unknown Merchant":              {with(map[string]any{"category": "shopping", "merchant": "nope"}), "no such Merchant"},
		"a transfer with a Category": {with(map[string]any{"kind": "transfer", "direction": "out", "category": "other"}),
			"transfer"},
	} {
		if got := toolError(t, session, "money_save_statement", c.args); !strings.Contains(got, c.want) {
			t.Errorf("%s: %q; want it to name %q", name, got, c.want)
		}
	}
	if got := toolError(t, session, "money_save_subcategory", map[string]any{"category": "toys", "name": "X", "essential": true}); !strings.Contains(got, "category") {
		t.Errorf("a Subcategory of an unknown Category: %q", got)
	}
}

func TestEssentialComesFromTheTransactionThenItsSubcategory(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	dental := saveSubcategory(t, session, map[string]any{"category": "personal_care", "name": "Dental", "essential": true})

	_, got := oneStatement(t, session,
		map[string]any{"kind": "expense", "day": "2026-10-03", "amount": 5000, "rawText": "DENTIST", "category": "personal_care", "subcategory": dental.ID},
		map[string]any{"kind": "expense", "day": "2026-10-04", "amount": 9000, "rawText": "DENTIST", "category": "personal_care", "subcategory": dental.ID, "essential": false},
		map[string]any{"kind": "expense", "day": "2026-10-05", "amount": 700, "rawText": "SOMETHING", "category": "other"},
	)

	if e := got[0].Essential; e == nil || !*e {
		t.Errorf("from its Subcategory: essential = %v, want true", e)
	}
	if e, o := got[1].Essential, got[1].EssentialOverride; e == nil || *e || o == nil || *o {
		t.Errorf("overridden: essential = %v, override %v, want false", e, o)
	}
	if got[2].Essential != nil {
		t.Errorf("with no Subcategory: essential = %v, want unmarked", *got[2].Essential)
	}

	// Changing the Subcategory's mark changes those that follow it.
	saveSubcategory(t, session, map[string]any{"id": dental.ID, "essential": false})
	again := callTool[[]whatItIs](t, session, "money_list_transactions", map[string]any{"subcategory": dental.ID})
	if len(again) != 2 || again[0].Essential == nil || *again[0].Essential {
		t.Errorf("after Dental became Optional: %+v; want the first one Optional too", again)
	}
}

func TestUnconfirmedTransactionsAreListedUntilConfirmed(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	_, got := oneStatement(t, session,
		map[string]any{"kind": "expense", "day": "2026-10-03", "amount": 2480, "rawText": "ONLINE STORE", "category": "shopping", "unconfirmed": true},
		map[string]any{"kind": "expense", "day": "2026-10-04", "amount": 300, "rawText": "CAFE", "category": "eating_out"},
	)

	unconfirmed := callTool[[]whatItIs](t, session, "money_list_transactions", map[string]any{"unconfirmed": true})
	if len(unconfirmed) != 1 || unconfirmed[0].ID != got[0].ID {
		t.Fatalf("unconfirmed = %+v; want the online store only", unconfirmed)
	}
	callTool[whatItIs](t, session, "money_update_transaction", map[string]any{"id": got[0].ID, "category": "gifts", "unconfirmed": false})
	if left := callTool[[]whatItIs](t, session, "money_list_transactions", map[string]any{"unconfirmed": true}); len(left) != 0 {
		t.Errorf("after confirming: %+v; want none", left)
	}
	if gifts := callTool[[]whatItIs](t, session, "money_list_transactions", map[string]any{"category": "gifts"}); len(gifts) != 1 {
		t.Errorf("Gifts = %+v; want the confirmed one", gifts)
	}
}
