package tests

import "testing"

// subscriptionJSON is a Subscription, as the Tools read it.
type subscriptionJSON struct {
	ID          string  `json:"id"`
	Merchant    string  `json:"merchant"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	Every       string  `json:"every"`
	LastCharged string  `json:"lastCharged"`
	Category    string  `json:"category"`
	Subcategory string  `json:"subcategory"`
	Essential   bool    `json:"essential"`
	Active      bool    `json:"active"`
	Late        bool    `json:"late"`
}

func TestASubscriptionPastItsChargeByAWeekIsLate(t *testing.T) {
	_, phone := signedInBrowser(t) // today is 23 November 2026
	session := phone.tools()
	streaming := callTool[merchantJSON](t, session, "money_save_merchant", map[string]any{"name": "Streaming", "category": "subscriptions"})
	save := func(lastCharged, every string) subscriptionJSON {
		return callTool[subscriptionJSON](t, session, "money_save_subscription", map[string]any{
			"merchant": streaming.ID, "amount": 990, "currency": "JPY", "every": every, "lastCharged": lastCharged,
			"category": "subscriptions", "essential": false,
		})
	}

	late := save("2026-10-10", "monthly")   // due 10 November
	onTime := save("2026-10-20", "monthly") // due 20 November: not a week yet
	yearly := save("2026-01-05", "yearly")  // due January
	if !late.Late || onTime.Late || yearly.Late || !late.Active || late.Amount != 990 {
		t.Errorf("late %+v, on time %+v, yearly %+v; want only the first late", late, onTime, yearly)
	}
	stopped := callTool[subscriptionJSON](t, session, "money_save_subscription", map[string]any{"id": late.ID, "active": false})
	if stopped.Late || stopped.Active {
		t.Errorf("after it stopped: %+v; want it neither active nor late", stopped)
	}
	if got := callTool[[]subscriptionJSON](t, session, "money_list_subscriptions", nil); len(got) != 3 {
		t.Errorf("money_list_subscriptions = %d, want 3", len(got))
	}
}
