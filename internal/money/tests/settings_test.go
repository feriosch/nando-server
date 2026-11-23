package tests

import (
	"reflect"
	"strings"
	"testing"
)

// settingsJSON is Money's settings, as the Tools read them.
type settingsJSON struct {
	// AskAbove is, per currency, the amount above which an unclear
	// Transaction is asked about (ADR 0014).
	AskAbove map[string]int `json:"askAbove"`
}

// defaultSettings are Money's settings until the Owner first changes them:
// ask about unclear Transactions above ¥3,000 or MX$300.
var defaultSettings = settingsJSON{AskAbove: map[string]int{"JPY": 3000, "MXN": 300}}

func TestMoneySettingsStartAtTheirDefaults(t *testing.T) {
	_, phone := signedInBrowser(t)

	got := callTool[settingsJSON](t, phone.tools(), "money_get_settings", nil)

	if !reflect.DeepEqual(got, defaultSettings) {
		t.Errorf("money_get_settings = %+v, want the defaults %+v", got, defaultSettings)
	}
}

func TestTheAssistantChangesOnlyTheMoneySettingsItSends(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()

	got := callTool[settingsJSON](t, session, "money_save_settings", map[string]any{"askAbove": map[string]any{"JPY": 5000}})

	want := settingsJSON{AskAbove: map[string]int{"JPY": 5000, "MXN": 300}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("after changing the yen threshold: %+v, want %+v", got, want)
	}
	if got := callTool[settingsJSON](t, session, "money_get_settings", nil); !reflect.DeepEqual(got, want) {
		t.Errorf("money_get_settings = %+v, want what was saved %+v", got, want)
	}
}

func TestBadMoneySettingsAreRefused(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()

	for name, c := range map[string]struct {
		args map[string]any
		want string
	}{
		"a zero amount":     {map[string]any{"askAbove": map[string]any{"JPY": 0}}, "askAbove.JPY"},
		"a negative amount": {map[string]any{"askAbove": map[string]any{"MXN": -1}}, "askAbove.MXN"},
		"another currency":  {map[string]any{"askAbove": map[string]any{"USD": 20}}, "JPY or MXN"},
		"nothing":           {map[string]any{}, "askAbove"},
	} {
		if got := toolError(t, session, "money_save_settings", c.args); !strings.Contains(got, c.want) {
			t.Errorf("%s: %q; want it to name %q", name, got, c.want)
		}
	}
	if got := callTool[settingsJSON](t, session, "money_get_settings", nil); !reflect.DeepEqual(got, defaultSettings) {
		t.Errorf("after refused saves: %+v, want the defaults", got)
	}
}
