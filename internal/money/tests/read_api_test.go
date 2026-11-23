package tests

import (
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// get GETs path from the web app, which must answer status, and decodes a
// 200's JSON into a T.
func get[T any](d *browser, path string, status int) T {
	d.nando.t.Helper()
	rec := d.must(http.MethodGet, path, nil, status)
	var v T
	if status == http.StatusOK {
		if err := jsonUnmarshal(rec.Body.Bytes(), &v); err != nil {
			d.nando.t.Fatalf("GET %s: %v in %q", path, err, rec.Body)
		}
	}
	return v
}

// cardJSON is the Money Home Card.
type cardJSON struct {
	Month    string `json:"month"`
	Spending struct {
		Total     int64  `json:"total"`
		Average3  *int64 `json:"average3"`
		Essential int64  `json:"essential"`
		Optional  int64  `json:"optional"`
		Unmarked  int64  `json:"unmarked"`
	} `json:"spending"`
	NetWorth struct {
		Total    int64  `json:"total"`
		Previous *int64 `json:"previous"`
	} `json:"netWorth"`
	Unconfirmed int `json:"unconfirmed"`
}

// rentMonth saves bank's Statement for month with a rent of yen and its
// closing balance.
func rentMonth(t *testing.T, session *mcp.ClientSession, bank accountJSON, month string, yen, balance float64) {
	t.Helper()
	saveStatement(t, session, map[string]any{
		"account": bank.ID, "from": month + "-01", "to": lastDays[month], "source": "csv", "closingBalance": balance,
		"transactions": []any{map[string]any{"kind": "expense", "day": month + "-05", "amount": yen, "rawText": "RENT", "category": "rent"}},
	})
}

// lastDays are the last Days of the months the read API tests use.
var lastDays = map[string]string{"2026-08": "2026-08-31", "2026-09": "2026-09-30", "2026-10": "2026-10-31", "2026-11": "2026-11-30"}

func TestTheHomeCardShowsTheLatestCompleteMonth(t *testing.T) {
	_, phone := signedInBrowser(t) // today is 23 November 2026
	session := phone.tools()

	if got := get[*cardJSON](phone, "/api/money/card", http.StatusOK); got != nil {
		t.Fatalf("with nothing in: %+v; want null", got)
	}

	bank := saveAccount(t, session, map[string]any{"name": "Bank", "kind": "bank", "currency": "JPY", "trackedFrom": "2026-08"})
	rentMonth(t, session, bank, "2026-08", 60000, 100000)
	rentMonth(t, session, bank, "2026-09", 70000, 110000)
	rentMonth(t, session, bank, "2026-10", 80000, 120000)
	// November is under way: the card still shows October.
	saveStatement(t, session, map[string]any{
		"account": bank.ID, "from": "2026-11-01", "to": "2026-11-20", "source": "csv",
		"transactions": []any{map[string]any{"kind": "expense", "day": "2026-11-03", "amount": 999, "rawText": "CAFE", "category": "eating_out", "unconfirmed": true}},
	})

	got := get[*cardJSON](phone, "/api/money/card", http.StatusOK)

	if got == nil || got.Month != "2026-10" || got.Spending.Total != 80000 || got.Spending.Average3 == nil ||
		*got.Spending.Average3 != 65000 || got.Spending.Unmarked != 80000 {
		t.Fatalf("card = %+v; want October, 80,000 against an average of 65,000", got)
	}
	if got.NetWorth.Total != 120000 || got.NetWorth.Previous == nil || *got.NetWorth.Previous != 110000 {
		t.Errorf("net worth = %+v; want 120,000 after 110,000", got.NetWorth)
	}
	if got.Unconfirmed != 1 {
		t.Errorf("unconfirmed = %d; want November's one, still waiting", got.Unconfirmed)
	}
}

// readSettingsJSON is Money's settings, as the web app reads them.
type readSettingsJSON struct {
	AskAbove     map[string]int `json:"askAbove"`
	ReminderTime string         `json:"reminderTime"`
	Reminders    []struct {
		Name     string   `json:"name"`
		Day      int      `json:"day"`
		Accounts []string `json:"accounts"`
	} `json:"reminders"`
}

func TestTheWebAppReadsMoneysSettingsWithAccountNames(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	bank := saveAccount(t, session, map[string]any{"name": "Bank", "kind": "bank", "currency": "JPY", "trackedFrom": "2026-01"})
	callTool[map[string]any](t, session, "money_save_settings", map[string]any{
		"reminders": []any{map[string]any{"name": "Bank app", "accounts": []string{bank.ID}, "day": 1}},
	})

	got := get[readSettingsJSON](phone, "/api/money/settings", http.StatusOK)

	if got.ReminderTime != "09:00" || got.AskAbove["JPY"] != 3000 || len(got.Reminders) != 1 ||
		got.Reminders[0].Name != "Bank app" || got.Reminders[0].Accounts[0] != "Bank" {
		t.Errorf("settings = %+v; want the reminder with its Account's name", got)
	}
}

func TestMoneysRoutesNeedASession(t *testing.T) {
	n, _ := signedInBrowser(t)
	stranger := &browser{nando: n}

	stranger.must(http.MethodGet, "/api/money/card", nil, http.StatusUnauthorized)
	stranger.must(http.MethodGet, "/api/money/settings", nil, http.StatusUnauthorized)
}
