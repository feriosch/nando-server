package tests

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/feriosch/nando-server/internal/core"
	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/testtelegram"

	"github.com/feriosch/nando-server/internal/testtrigger"
)

func TestTheOwnersMessageWakesTheScheduledTaskOncePerAssistant(t *testing.T) {
	n, _, trigger := messengerNandoWithTrigger(t)

	n.mustPostUpdate("career", ownerSays(5001, 42, "recruiter X wrote"))
	if fires := trigger.Received(); len(fires) != 1 {
		t.Fatalf("fires after the first message = %d, want 1", len(fires))
	}
	if f := trigger.Received()[0]; f.Header.Get("Authorization") != "Bearer "+testtrigger.Token || f.Body != "{}" {
		t.Errorf("fire = %+v; want the trigger's token and an empty body", f)
	}

	// A run is on its way for the Career Assistant: no second one.
	n.mustPostUpdate("career", ownerSays(5002, 43, "and another thing"))
	if fires := trigger.Received(); len(fires) != 1 {
		t.Errorf("fires after a second Career message = %d, want still 1", len(fires))
	}

	// Another Assistant gets its own run.
	n.mustPostUpdate("core", ownerSays(6001, 7, "add a birthday"))
	if fires := trigger.Received(); len(fires) != 2 {
		t.Errorf("fires after a Core message = %d, want 2", len(fires))
	}
}

// claimJSON is what messenger_claim_talk answers.
type claimJSON struct {
	Assistant string `json:"assistant"`
	Skill     string `json:"skill"`
	Summary   string `json:"summary"`
	Messages  []struct {
		From    string `json:"from"`
		Text    string `json:"text"`
		Waiting bool   `json:"waiting"`
		Failed  bool   `json:"failed"`
		Photos  []struct {
			ID string `json:"id"`
		} `json:"photos"`
	} `json:"messages"`
	OtherAssistants []string `json:"otherAssistants"`
}

// run is a cloud run of the Assistants' scheduled task: a Connector the
// Owner approved, calling the Messenger's Tools.
type run struct {
	n       *nando
	session *mcp.ClientSession
}

func (n *nando) startRun() *run {
	n.t.Helper()
	return &run{n: n, session: n.mustConnectMCP(connectorTokens(n.t, n).AccessToken)}
}

// call calls the Tool name with args, fails the test if it fails, and decodes
// its answer into a T.
func callAs[T any](r *run, name string, args map[string]any) T {
	r.n.t.Helper()
	result, err := r.session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		r.n.t.Fatalf("%s: %v", name, err)
	}
	if result.IsError {
		r.n.t.Fatalf("%s failed: %+v", name, result.Content)
	}
	var v T
	text, _ := result.Content[0].(*mcp.TextContent)
	if text == nil || json.Unmarshal([]byte(text.Text), &v) != nil {
		r.n.t.Fatalf("%s answered %+v, want JSON", name, result.Content)
	}
	return v
}

// claim calls messenger_claim_talk, for assistant if it isn't "".
func (r *run) claim(assistant string) claimJSON {
	r.n.t.Helper()
	args := map[string]any{}
	if assistant != "" {
		args["assistant"] = assistant
	}
	return callAs[claimJSON](r, "messenger_claim_talk", args)
}

func TestARunClaimsTheWaitingTalk(t *testing.T) {
	n, _, _ := messengerNandoWithTrigger(t)
	r := n.startRun()

	if got := r.claim(""); got.Assistant != "" {
		t.Errorf("claim with nothing waiting = %+v; want an empty answer", got)
	}

	n.mustPostUpdate("career", ownerSays(5001, 42, "recruiter X wrote"))
	n.mustPostUpdate("career", ownerSays(5002, 43, "help me reply"))

	got := r.claim("")
	if got.Assistant != "career" || got.Skill != "skills/assistants/career/SKILL.md" || got.Summary != "" {
		t.Errorf("claim = %+v; want the Career Assistant, its skill, no summary", got)
	}
	if len(got.Messages) != 2 || got.Messages[0].Text != "recruiter X wrote" || got.Messages[1].Text != "help me reply" ||
		got.Messages[0].From != "owner" || !got.Messages[0].Waiting || !got.Messages[1].Waiting {
		t.Errorf("claimed messages = %+v; want both of the Owner's, waiting, in order", got.Messages)
	}
	// The Japanese Assistant has no bot, so the Owner can't be sent there.
	if !reflect.DeepEqual(got.OtherAssistants, []string{"core"}) {
		t.Errorf("otherAssistants = %v, want [core]", got.OtherAssistants)
	}
}

func TestARunClaimsTheTalkWaitingLongestAndOnlyOneRunHoldsIt(t *testing.T) {
	n, _, _ := messengerNandoWithTrigger(t)
	n.mustPostUpdate("core", ownerSays(6001, 7, "add a birthday"))
	n.clock.Advance(time.Minute)
	n.mustPostUpdate("career", ownerSays(5001, 42, "recruiter X wrote"))
	n.clock.Advance(time.Minute)
	// A newer message doesn't make the Core Talk's wait shorter.
	n.mustPostUpdate("core", ownerSays(6002, 8, "on December 27"))

	first, second := n.startRun(), n.startRun()
	if got := first.claim(""); got.Assistant != "core" || len(got.Messages) != 2 {
		t.Errorf("first claim = %+v; want the Core Talk, waiting since before the Career one", got)
	}
	if got := second.claim(""); got.Assistant != "career" {
		t.Errorf("second run's claim = %+v; want the Career Talk, as the Core one is claimed", got)
	}
	if got := n.startRun().claim(""); got.Assistant != "" {
		t.Errorf("third run's claim = %+v; want nothing, as both are claimed", got)
	}
	if got := n.startRun().claim("core"); got.Assistant != "" {
		t.Errorf("claim of the claimed Core Talk = %+v; want nothing", got)
	}
}

// reply calls messenger_reply as assistant.
func (r *run) reply(assistant, text string) {
	r.n.t.Helper()
	callAs[map[string]any](r, "messenger_reply", map[string]any{"assistant": assistant, "text": text})
}

// cleared is every message ID whose reactions a bot with token removed.
func cleared(telegram *testtelegram.Telegram, token string) []float64 {
	var ids []float64
	for _, c := range telegram.CallsTo("setMessageReaction") {
		if list, _ := c.Params["reaction"].([]any); c.Token == token && len(list) == 0 {
			ids = append(ids, c.Params["message_id"].(float64))
		}
	}
	return ids
}

func TestAReplyAnswersTheClaimedMessages(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)
	n.mustPostUpdate("career", ownerSays(5001, 42, "recruiter X wrote"))
	n.mustPostUpdate("career", ownerSays(5002, 43, "help me reply"))
	r := n.startRun()
	r.claim("")
	// The Owner adds something while the run works.
	n.mustPostUpdate("career", ownerSays(5003, 44, "oh, and be polite"))

	r.reply("career", "Here's a draft: …")

	sent := telegram.CallsTo("sendMessage")
	if len(sent) != 1 || sent[0].Token != careerBotToken || sent[0].Params["chat_id"] != float64(ownerTelegramID) ||
		sent[0].Params["text"] != "Here's a draft: …" {
		t.Fatalf("sendMessage calls = %+v; want the reply, by the Career bot, to the Owner", sent)
	}
	if got := cleared(telegram, careerBotToken); !reflect.DeepEqual(got, []float64{42, 43}) {
		t.Errorf("👀 removed from messages %v; want 42 and 43, not 44, which no reply answered yet", got)
	}
	msgs := n.talk("career").Messages
	states := []model.MessageState{}
	for _, m := range msgs {
		states = append(states, m.State)
	}
	if len(msgs) != 4 || msgs[0].State != model.MessageAnswered || msgs[1].State != model.MessageAnswered ||
		msgs[2].State != model.MessageWaiting || msgs[3].From != model.FromAssistant || msgs[3].Text != "Here's a draft: …" {
		t.Errorf("Career Talk = %+v (states %v); want 42 and 43 answered, 44 waiting, then the reply", msgs, states)
	}

	again := r.claim("career")
	if len(again.Messages) != 4 || !again.Messages[2].Waiting || again.Messages[2].Text != "oh, and be polite" {
		t.Errorf("claim again = %+v; want the Talk, with the message that came meanwhile waiting", again)
	}
}

func TestAReplyTooLongForOneTelegramMessageIsSplit(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)
	n.mustPostUpdate("career", ownerSays(5001, 42, "write me a long one"))
	r := n.startRun()
	r.claim("")

	// Telegram takes at most 4096 characters a message: split at the last
	// line break that fits, or else at 4096 characters.
	para := strings.Repeat("あ", 3000)
	long := strings.Repeat("い", 5000)
	r.reply("career", para+"\n"+para+"\n"+long)

	var got []string
	for _, c := range telegram.CallsTo("sendMessage") {
		got = append(got, c.Params["text"].(string))
	}
	want := []string{para, para, strings.Repeat("い", 4096), strings.Repeat("い", 904)}
	if !reflect.DeepEqual(got, want) {
		lengths := []int{}
		for _, g := range got {
			lengths = append(lengths, utf8.RuneCountInString(g))
		}
		t.Errorf("sent %d messages of %v characters; want 3000, 3000, 4096 and 904", len(got), lengths)
	}
}

// typingBy counts the times the bot with token showed the typing status.
func typingBy(telegram *testtelegram.Telegram, token string) int {
	count := 0
	for _, c := range telegram.CallsTo("sendChatAction") {
		if c.Token == token && c.Params["action"] == "typing" {
			count++
		}
	}
	return count
}

func TestTheRunsToolsKeepShowingTypingWhileMessagesWait(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)
	n.mustPostUpdate("career", ownerSays(5001, 42, "recruiter X wrote"))
	before := typingBy(telegram, careerBotToken)
	r := n.startRun()

	r.claim("")
	if got := typingBy(telegram, careerBotToken); got != before+1 {
		t.Errorf("typing after the claim = %d, want %d", got, before+1)
	}

	n.mustPostUpdate("career", ownerSays(5002, 43, "and another thing"))
	before = typingBy(telegram, careerBotToken)
	r.reply("career", "On it.")
	if got := typingBy(telegram, careerBotToken); got != before+1 {
		t.Errorf("typing after a reply with a message still waiting = %d, want %d", got, before+1)
	}

	r.claim("career")
	r.reply("career", "Done.")
	before = typingBy(telegram, careerBotToken)
	r.claim("career")
	if got := typingBy(telegram, careerBotToken); got != before {
		t.Errorf("typing after nothing waits = %d, want still %d", got, before)
	}
}

// newTalk calls messenger_new_talk as assistant, with summary if it isn't "".
func (r *run) newTalk(assistant, summary string) {
	r.n.t.Helper()
	args := map[string]any{"assistant": assistant}
	if summary != "" {
		args["summary"] = summary
	}
	callAs[map[string]any](r, "messenger_new_talk", args)
}

func TestANewTalkStartsFromASummaryWhenClaudeStartsIt(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)
	n.mustPostUpdate("career", ownerSays(5001, 42, "recruiter X wrote"))
	r := n.startRun()
	r.claim("")
	r.reply("career", "Here's a draft.")
	n.mustPostUpdate("career", ownerSays(5002, 43, "thanks! now about company Y"))
	r.claim("career")

	r.newTalk("career", "Drafted a reply to recruiter X. Next: company Y.")

	talk := n.talk("career")
	if talk.Summary != "Drafted a reply to recruiter X. Next: company Y." {
		t.Errorf("new Talk's summary = %q; want the run's", talk.Summary)
	}
	if len(talk.Messages) != 1 || talk.Messages[0].Text != "thanks! now about company Y" || talk.Messages[0].State != model.MessageWaiting {
		t.Errorf("new Talk's messages = %+v; want only the waiting one, still waiting", talk.Messages)
	}
	sent := telegram.CallsTo("sendMessage")
	if last := sent[len(sent)-1]; last.Params["text"] != "Started a new chat (kept a short summary)." || last.Token != careerBotToken {
		t.Errorf("last message sent = %+v; want the one-line notice, from the Career bot", last)
	}

	// The run goes on in the new Talk, and its reply answers the moved message.
	r.reply("career", "Company Y: …")
	if got := n.talk("career").Messages; len(got) != 2 || got[0].State != model.MessageAnswered {
		t.Errorf("new Talk after the reply = %+v; want the moved message answered, then the reply", got)
	}
}

func TestANewTalkTheOwnerAsksForStartsBlank(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)
	n.mustPostUpdate("core", ownerSays(6001, 7, "add a birthday"))
	r := n.startRun()
	r.claim("")
	r.reply("core", "Added.")
	n.mustPostUpdate("core", ownerSays(6002, 8, "start a new chat"))
	r.claim("core")

	r.newTalk("core", "")

	if talk := n.talk("core"); talk.Summary != "" || len(talk.Messages) != 1 || talk.Messages[0].Text != "start a new chat" {
		t.Errorf("new Talk = %+v; want no summary, and only the waiting ask, for the run to answer", talk)
	}
	sent := telegram.CallsTo("sendMessage")
	if last := sent[len(sent)-1]; last.Params["text"] != "Started a new chat." {
		t.Errorf("last message sent = %+v; want the plain notice", last)
	}
}

func TestATalkWhoseRunVanishedIsClaimedAgainAfter15Minutes(t *testing.T) {
	n, _, trigger := messengerNandoWithTrigger(t)
	n.mustPostUpdate("career", ownerSays(5001, 42, "recruiter X wrote"))
	n.startRun().claim("") // and the run dies without replying

	n.clock.Advance(5 * time.Minute)
	n.mustTick()
	if got := len(trigger.Received()); got != 1 {
		t.Errorf("fires while the claim holds = %d, want 1", got)
	}
	if got := n.startRun().claim(""); got.Assistant != "" {
		t.Errorf("claim while another holds the Talk = %+v; want nothing", got)
	}

	n.clock.Advance(11 * time.Minute)
	n.mustTick()
	if got := len(trigger.Received()); got != 2 {
		t.Errorf("fires once the claim lapsed = %d, want 2", got)
	}
	if got := n.startRun().claim(""); got.Assistant != "career" || !got.Messages[0].Waiting {
		t.Errorf("claim once the first lapsed = %+v; want the Career Talk, still waiting", got)
	}
}

func TestMessagesFailAfterThreeClaimsWithoutAReply(t *testing.T) {
	n, telegram, trigger := messengerNandoWithTrigger(t)
	n.mustPostUpdate("career", ownerSays(5001, 42, "recruiter X wrote"))
	for range 3 {
		n.startRun().claim("") // and each run dies without replying
		n.clock.Advance(16 * time.Minute)
		n.mustTick()
	}

	if got := n.talk("career").Messages; len(got) != 1 || got[0].State != model.MessageFailed {
		t.Errorf("Career Talk = %+v; want the message failed", got)
	}
	sent := telegram.CallsTo("sendMessage")
	if len(sent) != 1 || sent[0].Token != careerBotToken ||
		sent[0].Params["text"] != "Sorry, something went wrong and I couldn't answer that. Please send it again." {
		t.Errorf("sendMessage calls = %+v; want the Career bot saying something went wrong", sent)
	}
	if got := cleared(telegram, careerBotToken); !reflect.DeepEqual(got, []float64{42}) {
		t.Errorf("👀 removed from %v; want message 42", got)
	}
	fires := len(trigger.Received())
	n.clock.Advance(16 * time.Minute)
	n.mustTick()
	if got := len(trigger.Received()); got != fires {
		t.Errorf("fires after the message failed = %d, want still %d", got, fires)
	}

	// The Owner sends it again: the next run sees which one failed.
	n.mustPostUpdate("career", ownerSays(5002, 43, "recruiter X wrote"))
	got := n.startRun().claim("")
	if len(got.Messages) != 2 || !got.Messages[0].Failed || got.Messages[0].Waiting || !got.Messages[1].Waiting || got.Messages[1].Failed {
		t.Errorf("claim after a resend = %+v; want the first marked failed, the resend waiting", got.Messages)
	}
}

func TestTheOwnerHearsOnceWhenNoRunStarts(t *testing.T) {
	n, telegram, trigger := messengerNandoWithTrigger(t)
	n.mustPostUpdate("career", ownerSays(5001, 42, "recruiter X wrote"))
	n.clock.Advance(5 * time.Minute)
	n.mustTick()
	if sent := telegram.CallsTo("sendMessage"); len(sent) != 0 {
		t.Errorf("sendMessage after 5 minutes = %+v; want nothing yet", sent)
	}

	// No run ever claims it, as when the Owner's usage ran out.
	n.clock.Advance(6 * time.Minute)
	n.mustTick()
	n.clock.Advance(5 * time.Minute)
	n.mustTick()

	sent := telegram.CallsTo("sendMessage")
	if len(sent) != 1 || sent[0].Token != careerBotToken ||
		sent[0].Params["text"] != "Claude didn't start (maybe the usage limit). I'll keep trying." {
		t.Errorf("sendMessage calls = %+v; want the Career bot saying so, once", sent)
	}
	if got := cleared(telegram, careerBotToken); len(got) != 0 {
		t.Errorf("👀 removed from %v; want it kept, as the message still waits", got)
	}
	if got := len(trigger.Received()); got != 2 {
		t.Errorf("fires = %d; want the first and the tick's, after 10 minutes in flight", got)
	}
	if got := n.talk("career").Messages; got[0].State != model.MessageWaiting {
		t.Errorf("message = %+v; want it still waiting", got[0])
	}
}

func TestWithoutATriggerMessagesWaitAndNothingFails(t *testing.T) {
	telegram := testtelegram.New(t)
	n := newNandoWith(t, func(cfg *core.Config) {
		cfg.Messenger = core.MessengerConfig{
			TelegramURL:   telegram.URL,
			OwnerID:       ownerTelegramID,
			WebhookSecret: webhookSecret,
			BotTokens:     map[string]string{"core": coreBotToken},
		}
	})

	n.mustPostUpdate("core", ownerSays(6001, 7, "add a birthday"))
	n.clock.Advance(20 * time.Minute)
	n.mustTick()

	if got := n.talk("core").Messages; len(got) != 1 || got[0].State != model.MessageWaiting {
		t.Errorf("Core Talk = %+v; want the message waiting", got)
	}
	// Nothing would start a run or try again, so the Owner isn't told so.
	if sent := telegram.CallsTo("sendMessage"); len(sent) != 0 {
		t.Errorf("sendMessage calls = %+v; want none", sent)
	}
	// A run started by hand still works through it.
	r := n.startRun()
	if got := r.claim(""); got.Assistant != "core" {
		t.Errorf("claim = %+v; want the Core Talk", got)
	}
}
