package tests

import (
	"testing"

	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/testpush"
)

func TestANotificationIsSentByItsAssistantsBotInsteadOfWebPush(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)
	phone := n.newBrowser("iPhone")
	phone.mustEnroll()
	push := testpush.New(t)
	phone.subscribe(push)
	n.core.AddNotificationProvider(&provider{due: []model.Notification{{
		ID: "career:replies-waiting:2026-11-18", Title: "Recruiters waiting", Body: "3 replies are ready for you.", URL: "/career",
	}}})

	n.mustTick()

	sent := telegram.CallsTo("sendMessage")
	if len(sent) != 1 || sent[0].Token != careerBotToken || sent[0].Params["chat_id"] != float64(ownerTelegramID) ||
		sent[0].Params["text"] != "Recruiters waiting\n3 replies are ready for you." {
		t.Errorf("sendMessage calls = %+v; want the Career bot sending the title, a line break and the body", sent)
	}
	if got := push.Received(); len(got) != 0 {
		t.Errorf("Web Push received %+v; want nothing while the Messenger is on", got)
	}
}

func TestANotificationForAnAreaWithoutABotGoesToTheCoreAssistant(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)
	// The Japanese Assistant has no bot here, and Calories has no Assistant.
	n.core.AddNotificationProvider(&provider{due: []model.Notification{
		{ID: "japanese:word-request:42:asked:1", Title: "Japanese", Body: "はし: which one did you mean?"},
		{ID: "calories:evening-summary:2026-11-18", Title: "Calories", Body: "Nothing logged today"},
	}})

	n.mustTick()

	sent := telegram.CallsTo("sendMessage")
	if len(sent) != 2 {
		t.Fatalf("sendMessage calls = %+v; want both Notifications", sent)
	}
	for _, c := range sent {
		if c.Token != coreBotToken {
			t.Errorf("%q was sent by %s; want the Core bot", c.Params["text"], c.Token)
		}
	}
}

func TestANotificationJoinsTheTalkWithoutWakingARun(t *testing.T) {
	n, _, trigger := messengerNandoWithTrigger(t)
	n.core.AddNotificationProvider(&provider{due: []model.Notification{{
		ID: "career:replies-waiting:2026-11-18", Title: "Recruiters waiting", Body: "3 replies are ready for you.",
	}}})

	n.mustTick()
	if got := len(trigger.Received()); got != 0 {
		t.Errorf("fires after a Notification = %d, want none: only the Owner's reply wakes a run", got)
	}

	n.mustPostUpdate("career", ownerSays(5001, 42, "show me the first one"))
	got := n.startRun().claim("")
	if len(got.Messages) != 2 || got.Messages[0].From != "assistant" || got.Messages[0].Waiting ||
		got.Messages[0].Text != "Recruiters waiting\n3 replies are ready for you." ||
		got.Messages[1].Text != "show me the first one" || !got.Messages[1].Waiting {
		t.Errorf("claim = %+v; want the Notification from the Assistant, then the Owner's reply, waiting", got.Messages)
	}
}

func TestARetriedTickSendsEachNotificationInTelegramOnlyOnce(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)
	n.core.AddNotificationProvider(&provider{due: []model.Notification{{
		ID: "core:weigh-in:2026-11-16", Title: "Weigh-in", Body: "Time to log this week's weight",
	}}})

	n.mustTick()
	n.mustTick() // the scheduler retries, or the next tick finds it due again

	if sent := telegram.CallsTo("sendMessage"); len(sent) != 1 {
		t.Errorf("sendMessage calls = %+v; want the Notification once", sent)
	}
	if got := n.talk("core").Messages; len(got) != 1 {
		t.Errorf("Core Talk = %+v; want the Notification once", got)
	}
}
