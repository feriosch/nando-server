package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
)

// at is the time hh:mm on y-m-d, in UTC, the Owner Time Zone until set.
func at(y int, m time.Month, d, hh, mm int) time.Time {
	return time.Date(y, m, d, hh, mm, 0, 0, time.UTC)
}

// reminderAccounts are the Owner's Accounts as the reminders test them: a
// card and an account in one app, a bank in another, and a rent card that
// closes on the 27th and doesn't hold the month.
type reminderAccounts struct {
	nuCard, nuAccount, bank, rent accountJSON
}

func setUpReminders(t *testing.T, session *mcp.ClientSession) reminderAccounts {
	t.Helper()
	a := reminderAccounts{
		nuCard:    saveAccount(t, session, map[string]any{"name": "App card", "kind": "card", "currency": "MXN", "trackedFrom": "2026-10"}),
		nuAccount: saveAccount(t, session, map[string]any{"name": "App account", "kind": "bank", "currency": "MXN", "trackedFrom": "2026-10"}),
		bank:      saveAccount(t, session, map[string]any{"name": "Bank", "kind": "bank", "currency": "JPY", "trackedFrom": "2026-10"}),
		rent:      saveAccount(t, session, map[string]any{"name": "Rent card", "kind": "card", "currency": "JPY", "trackedFrom": "2026-10", "holdsMonth": false}),
	}
	callTool[map[string]any](t, session, "money_save_settings", map[string]any{
		"reminderTime": "09:00",
		"reminders": []any{
			map[string]any{"name": "App", "accounts": []string{a.nuCard.ID, a.nuAccount.ID}, "day": 1},
			map[string]any{"name": "Bank app", "accounts": []string{a.bank.ID}, "day": 1},
			map[string]any{"name": "Rent app", "accounts": []string{a.rent.ID}, "day": 28},
		},
	})
	return a
}

// month saves account's Statement for the whole of month.
func month(t *testing.T, session *mcp.ClientSession, account accountJSON, from, to string) {
	t.Helper()
	saveStatement(t, session, map[string]any{"account": account.ID, "from": from, "to": to, "source": "pdf"})
}

// reminders are the Money bot's Statement Reminders sent so far.
func (n *nando) reminders() []string {
	var got []string
	for _, text := range n.moneyMessages() {
		if strings.Contains(text, "Statement") && !strings.Contains(text, "Review") {
			got = append(got, text)
		}
	}
	return got
}

func TestAStatementReminderAsksOnlyForWhatIsMissingOnItsDay(t *testing.T) {
	n, phone := signedInBrowser(t)
	session := phone.tools()
	a := setUpReminders(t, session)
	month(t, session, a.nuCard, "2026-11-01", "2026-11-30")
	month(t, session, a.bank, "2026-11-01", "2026-11-30")
	month(t, session, a.rent, "2026-10-28", "2026-11-27")

	n.tickAt(at(2026, 11, 30, 9, 0)) // not their day
	n.tickAt(at(2026, 12, 1, 8, 30)) // not their time
	if got := n.reminders(); len(got) != 0 {
		t.Fatalf("before the day and time: %q; want none", got)
	}
	n.tickAt(at(2026, 12, 1, 9, 0))
	got := n.reminders()
	if len(got) != 1 || !strings.Contains(got[0], "App") || !strings.Contains(got[0], "App account") ||
		strings.Contains(got[0], "App card") || !strings.Contains(got[0], "November") {
		t.Fatalf("on the 1st: %q; want one, for the App's account alone, for November (the bank's is in)", got)
	}

	n.tickAt(at(2026, 12, 1, 9, 30))
	n.tickAt(at(2026, 12, 3, 9, 0))
	if got := n.reminders(); len(got) != 1 {
		t.Errorf("before 3 days passed: %d reminders; want still 1", len(got))
	}
	n.tickAt(at(2026, 12, 4, 9, 0))
	if got := n.reminders(); len(got) != 2 || !strings.Contains(got[1], "App account") {
		t.Errorf("3 days later: %q; want it again", got)
	}
	n.tickAt(at(2026, 12, 7, 9, 0))
	if got := n.reminders(); len(got) != 2 {
		t.Errorf("6 days later: %d reminders; want no third", len(got))
	}
}

func TestTheRentCardsReminderAsksForTheCycleThatJustClosed(t *testing.T) {
	n, phone := signedInBrowser(t)
	session := phone.tools()
	a := setUpReminders(t, session)
	month(t, session, a.rent, "2026-10-28", "2026-11-27")

	n.tickAt(at(2026, 11, 28, 9, 0))
	if got := n.reminders(); len(got) != 0 {
		t.Errorf("with the cycle closing on the 27th in: %q; want none", got)
	}
	n.tickAt(at(2026, 12, 28, 9, 0))
	if got := n.reminders(); len(got) != 1 || !strings.Contains(got[0], "Rent card") || !strings.Contains(got[0], "closed on 27 December") {
		t.Errorf("a month later, without the next cycle: %q; want the cycle closed on the 27th asked for", got)
	}
}

func TestBadReminderSettingsAreRefused(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.tools()
	open := saveAccount(t, session, map[string]any{"name": "Bank", "kind": "bank", "currency": "JPY", "trackedFrom": "2026-01"})
	closed := saveAccount(t, session, map[string]any{"name": "Old", "kind": "bank", "currency": "JPY", "trackedFrom": "2026-01", "closedFrom": "2026-06"})
	group := func(name string, day int, accounts ...string) map[string]any {
		return map[string]any{"name": name, "day": day, "accounts": accounts}
	}

	for name, c := range map[string]struct {
		args map[string]any
		want string
	}{
		"day 29":                   {map[string]any{"reminders": []any{group("A", 29, open.ID)}}, "day"},
		"an unknown Account":       {map[string]any{"reminders": []any{group("A", 1, "nope")}}, "no such Account"},
		"a closed Account":         {map[string]any{"reminders": []any{group("A", 1, closed.ID)}}, "closed"},
		"an Account twice":         {map[string]any{"reminders": []any{group("A", 1, open.ID), group("B", 2, open.ID)}}, "two"},
		"no name":                  {map[string]any{"reminders": []any{group(" ", 1, open.ID)}}, "name"},
		"a time off the half hour": {map[string]any{"reminderTime": "09:10"}, "reminderTime"},
	} {
		if got := toolError(t, session, "money_save_settings", c.args); !strings.Contains(got, c.want) {
			t.Errorf("%s: %q; want it to name %q", name, got, c.want)
		}
	}
}

// reviewAsks are the Money Talk's messages from Nando asking for a Monthly
// Review.
func (n *nando) reviewAsks() []coremodel.TalkMessage {
	n.t.Helper()
	talk, err := n.core.Talk(context.Background(), "money")
	if err != nil {
		n.t.Fatalf("Talk(money): %v", err)
	}
	var got []coremodel.TalkMessage
	for _, m := range talk.Messages {
		if m.From == coremodel.FromNando && strings.Contains(m.Text, "Monthly Review") {
			got = append(got, m)
		}
	}
	return got
}

func TestTheMonthlyReviewIsAskedForOnceAMonthIsComplete(t *testing.T) {
	n, phone := signedInBrowser(t)
	session := phone.tools()
	a := setUpReminders(t, session)
	n.tickAt(at(2026, 11, 23, 12, 0)) // the Module's first tick
	session = phone.tools()           // a Connector's token lasts an hour
	month(t, session, a.nuCard, "2026-10-01", "2026-10-31")
	month(t, session, a.bank, "2026-10-01", "2026-10-31")

	n.tickAt(at(2026, 11, 24, 9, 0))
	if got := n.reviewAsks(); len(got) != 0 {
		t.Fatalf("with October missing an Account: %d asks; want none", len(got))
	}

	// The rent card, which doesn't hold the month, is still missing.
	session = phone.tools()
	month(t, session, a.nuAccount, "2026-10-01", "2026-10-31")
	n.tickAt(at(2026, 11, 24, 12, 0))
	if got := n.reviewAsks(); len(got) != 0 {
		t.Errorf("before the reminder time: %d asks; want none yet", len(got))
	}
	n.tickAt(at(2026, 11, 25, 9, 0))
	got := n.reviewAsks()
	if len(got) != 1 || !strings.Contains(got[0].Text, "October 2026") || got[0].State != coremodel.MessageWaiting {
		t.Fatalf("once October is Complete: %+v; want one ask for October's Review, waiting", got)
	}

	// Sent again, October becomes Complete again: no second ask.
	session = phone.tools()
	month(t, session, a.bank, "2026-10-01", "2026-10-31")
	n.tickAt(at(2026, 11, 26, 9, 0))
	if got := n.reviewAsks(); len(got) != 1 {
		t.Errorf("after October was sent again: %d asks; want still 1", len(got))
	}
}

func TestAMonthBeforeTheModulesFirstUseIsNeverAskedFor(t *testing.T) {
	n, phone := signedInBrowser(t)
	session := phone.tools()
	bank := saveAccount(t, session, map[string]any{"name": "Bank", "kind": "bank", "currency": "JPY", "trackedFrom": "2026-08"})
	callTool[map[string]any](t, session, "money_save_settings", map[string]any{"reminderTime": "09:00"})
	n.tickAt(at(2026, 11, 23, 12, 0)) // the Module's first tick: reviews start with October
	session = phone.tools()

	month(t, session, bank, "2026-08-01", "2026-08-31")
	month(t, session, bank, "2026-09-01", "2026-09-30")
	month(t, session, bank, "2026-10-01", "2026-10-31")
	n.tickAt(at(2026, 11, 24, 9, 0))

	got := n.reviewAsks()
	if len(got) != 1 || !strings.Contains(got[0].Text, "October 2026") {
		var texts []string
		for _, m := range got {
			texts = append(texts, m.Text)
		}
		t.Errorf("asks = %q; want October's alone, not the history's", texts)
	}
}
