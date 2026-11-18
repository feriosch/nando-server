package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/feriosch/nando-server/internal/core"
	"github.com/feriosch/nando-server/internal/core/model"
	"github.com/feriosch/nando-server/internal/testtelegram"
	"github.com/feriosch/nando-server/internal/testtrigger"
)

const (
	ownerTelegramID    = 777001
	strangerTelegramID = 999002
	webhookSecret      = "webhook-secret-1"
	coreBotToken       = "111:core-bot-token"
	careerBotToken     = "222:career-bot-token"
)

// messengerNando is Nando with the Messenger pointed at a fake Telegram: the
// Core Assistant's bot and the Career Assistant's have tokens, and the
// Japanese Assistant's has none.
func messengerNando(t *testing.T) (*nando, *testtelegram.Telegram) {
	t.Helper()
	n, telegram, _ := messengerNandoWithTrigger(t)
	return n, telegram
}

// messengerNandoWithTrigger is messengerNando with the Assistants' scheduled
// task's API trigger, a fake.
func messengerNandoWithTrigger(t *testing.T) (*nando, *testtelegram.Telegram, *testtrigger.Trigger) {
	t.Helper()
	telegram := testtelegram.New(t)
	trigger := testtrigger.New(t)
	n := newNandoWith(t, func(cfg *core.Config) {
		cfg.Messenger = core.MessengerConfig{
			TelegramURL:   telegram.URL,
			OwnerID:       ownerTelegramID,
			WebhookSecret: webhookSecret,
			BotTokens:     map[string]string{"core": coreBotToken, "career": careerBotToken},
			TriggerURL:    trigger.URL,
			TriggerToken:  testtrigger.Token,
		}
	})
	n.core.AddAssistant(model.Assistant{Name: "career"})
	n.core.AddAssistant(model.Assistant{Name: "japanese"})
	return n, telegram, trigger
}

// telegramUpdate is an update as Telegram posts it to a bot's webhook.
type telegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *telegramMessage `json:"message,omitempty"`
}

type telegramMessage struct {
	MessageID int64            `json:"message_id"`
	Date      int64            `json:"date"`
	Chat      telegramChat     `json:"chat"`
	From      *telegramUser    `json:"from,omitempty"`
	Text      string           `json:"text,omitempty"`
	Photo     []map[string]any `json:"photo,omitempty"`
	Caption   string           `json:"caption,omitempty"`
	Sticker   map[string]any   `json:"sticker,omitempty"`
	Document  map[string]any   `json:"document,omitempty"`
}

type telegramChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type telegramUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
}

// ownerSays is the Owner's text message in their private chat with a bot.
func ownerSays(updateID, messageID int64, text string) telegramUpdate {
	return telegramUpdate{UpdateID: updateID, Message: &telegramMessage{
		MessageID: messageID,
		Date:      1795000000,
		Chat:      telegramChat{ID: ownerTelegramID, Type: "private"},
		From:      &telegramUser{ID: ownerTelegramID, FirstName: "Owner"},
		Text:      text,
	}}
}

// postUpdate posts update to assistant's webhook with secret in the header
// Telegram sends it in ("" for none).
func (n *nando) postUpdate(assistant, secret string, update telegramUpdate) *httptest.ResponseRecorder {
	n.t.Helper()
	body, err := json.Marshal(update)
	if err != nil {
		n.t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/messenger/telegram/"+assistant, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
	}
	rec := httptest.NewRecorder()
	n.handler.ServeHTTP(rec, req)
	return rec
}

// mustPostUpdate posts update as Telegram would, and fails unless it is
// answered 200.
func (n *nando) mustPostUpdate(assistant string, update telegramUpdate) {
	n.t.Helper()
	if rec := n.postUpdate(assistant, webhookSecret, update); rec.Code != http.StatusOK {
		n.t.Fatalf("POST /messenger/telegram/%s: status = %d, body %q", assistant, rec.Code, rec.Body)
	}
}

// talk is assistant's open Talk.
func (n *nando) talk(assistant string) model.Talk {
	n.t.Helper()
	talk, err := n.core.Talk(context.Background(), assistant)
	if err != nil {
		n.t.Fatalf("Talk(%s): %v", assistant, err)
	}
	return talk
}

func TestOwnersMessageWaitsInTheAssistantsTalk(t *testing.T) {
	n, telegram := messengerNando(t)

	n.mustPostUpdate("career", ownerSays(5001, 42, "recruiter X wrote, help me reply"))

	got := n.talk("career").Messages
	if len(got) != 1 || got[0].From != model.FromOwner || got[0].Text != "recruiter X wrote, help me reply" ||
		got[0].TelegramMessageID != 42 || got[0].State != model.MessageWaiting {
		t.Errorf("Career Talk's messages = %+v; want the Owner's one message, waiting", got)
	}
	if other := n.talk("core").Messages; len(other) != 0 {
		t.Errorf("Core Talk's messages = %+v; want none", other)
	}

	reactions := telegram.CallsTo("setMessageReaction")
	if len(reactions) != 1 || reactions[0].Token != careerBotToken ||
		reactions[0].Params["chat_id"] != float64(ownerTelegramID) || reactions[0].Params["message_id"] != float64(42) {
		t.Fatalf("setMessageReaction calls = %+v; want one by the Career bot on message 42", reactions)
	}
	if emoji := reactionEmoji(reactions[0]); emoji != "👀" {
		t.Errorf("reaction = %q, want 👀", emoji)
	}
	typing := telegram.CallsTo("sendChatAction")
	if len(typing) != 1 || typing[0].Token != careerBotToken || typing[0].Params["action"] != "typing" ||
		typing[0].Params["chat_id"] != float64(ownerTelegramID) {
		t.Errorf("sendChatAction calls = %+v; want typing once, by the Career bot", typing)
	}
}

// reactionEmoji is the one emoji a setMessageReaction call sets, or "".
func reactionEmoji(c testtelegram.Call) string {
	list, _ := c.Params["reaction"].([]any)
	if len(list) != 1 {
		return ""
	}
	r, _ := list[0].(map[string]any)
	emoji, _ := r["emoji"].(string)
	return emoji
}

func TestAnUpdateWithoutTheWebhookSecretIsRejected(t *testing.T) {
	n, telegram := messengerNando(t)

	for _, secret := range []string{"", "not-the-secret"} {
		if rec := n.postUpdate("career", secret, ownerSays(5001, 42, "hi")); rec.Code != http.StatusUnauthorized {
			t.Errorf("update with secret %q: status = %d, want 401", secret, rec.Code)
		}
	}

	if got := n.talk("career").Messages; len(got) != 0 {
		t.Errorf("Career Talk's messages = %+v; want none", got)
	}
	if calls := telegram.Calls(); len(calls) != 0 {
		t.Errorf("Telegram calls = %+v; want none", calls)
	}
}

func TestOnlyTheOwnersOwnMessagesAreKept(t *testing.T) {
	n, telegram := messengerNando(t)

	stranger := ownerSays(5001, 41, "hi bot")
	stranger.Message.Chat.ID, stranger.Message.From.ID = strangerTelegramID, strangerTelegramID
	group := ownerSays(5002, 42, "in a group")
	group.Message.Chat = telegramChat{ID: -100123, Type: "group"}
	noMessage := telegramUpdate{UpdateID: 5003} // such as an edited message
	for name, update := range map[string]telegramUpdate{
		"a stranger's message":  stranger,
		"the Owner in a group":  group,
		"/start":                ownerSays(5004, 43, "/start"),
		"/start with a payload": ownerSays(5005, 44, "/start hello"),
		"an update without one": noMessage,
	} {
		if rec := n.postUpdate("career", webhookSecret, update); rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200 so Telegram doesn't re-deliver it", name, rec.Code)
		}
	}

	if got := n.talk("career").Messages; len(got) != 0 {
		t.Errorf("Career Talk's messages = %+v; want none", got)
	}
	if calls := telegram.Calls(); len(calls) != 0 {
		t.Errorf("Telegram calls = %+v; want none", calls)
	}
}

func TestARedeliveredUpdateIsKeptOnce(t *testing.T) {
	n, telegram := messengerNando(t)

	n.mustPostUpdate("career", ownerSays(5001, 42, "ate curry"))
	n.mustPostUpdate("career", ownerSays(5001, 42, "ate curry"))
	// Each bot numbers its own updates, so another bot's 5001 is new.
	n.mustPostUpdate("core", ownerSays(5001, 7, "add a birthday"))

	if got := n.talk("career").Messages; len(got) != 1 {
		t.Errorf("Career Talk's messages = %+v; want the one message", got)
	}
	if got := n.talk("core").Messages; len(got) != 1 {
		t.Errorf("Core Talk's messages = %+v; want the one message", got)
	}
	if reactions := telegram.CallsTo("setMessageReaction"); len(reactions) != 2 {
		t.Errorf("setMessageReaction calls = %+v; want one per message", reactions)
	}
}

func TestAStickerOrFileIsAnsweredButNotKept(t *testing.T) {
	n, telegram := messengerNando(t)

	// An image sent as a file is a document, not a photo: not read yet (#135).
	file := ownerSays(5001, 42, "")
	file.Message.Caption = "the job description"
	file.Message.Document = map[string]any{"file_id": "BQAC-file", "file_name": "jd.pdf", "mime_type": "application/pdf"}
	sticker := ownerSays(5002, 43, "")
	sticker.Message.Sticker = map[string]any{"file_id": "CAAC-sticker"}
	n.mustPostUpdate("career", file)
	n.mustPostUpdate("career", sticker)
	n.mustPostUpdate("career", file) // re-delivered

	if got := n.talk("career").Messages; len(got) != 0 {
		t.Errorf("Career Talk's messages = %+v; want none", got)
	}
	sent := telegram.CallsTo("sendMessage")
	if len(sent) != 2 {
		t.Fatalf("sendMessage calls = %+v; want one per file or sticker", sent)
	}
	for _, c := range sent {
		if c.Token != careerBotToken || c.Params["chat_id"] != float64(ownerTelegramID) || c.Params["text"] != "I can only read text for now." {
			t.Errorf("sendMessage = %+v; want the Career bot telling the Owner it reads only text", c)
		}
	}
	if reactions := telegram.CallsTo("setMessageReaction"); len(reactions) != 0 {
		t.Errorf("setMessageReaction calls = %+v; want none, as nothing waits", reactions)
	}
}

func TestAnAssistantWithoutABotHasNoWebhook(t *testing.T) {
	n, telegram := messengerNando(t)

	// japanese is an Assistant without a bot token; calories is none at all.
	for _, assistant := range []string{"japanese", "calories"} {
		if rec := n.postUpdate(assistant, webhookSecret, ownerSays(5001, 42, "hi")); rec.Code != http.StatusNotFound {
			t.Errorf("POST /messenger/telegram/%s: status = %d, want 404", assistant, rec.Code)
		}
		if got := n.talk(assistant).Messages; len(got) != 0 {
			t.Errorf("%s Talk's messages = %+v; want none", assistant, got)
		}
	}
	if calls := telegram.Calls(); len(calls) != 0 {
		t.Errorf("Telegram calls = %+v; want none", calls)
	}
}

func TestAMessageIsKeptWhenTelegramFailsToShowItArrived(t *testing.T) {
	n, telegram := messengerNando(t)
	telegram.Fail("setMessageReaction")
	telegram.Fail("sendChatAction")

	n.mustPostUpdate("career", ownerSays(5001, 42, "ate curry"))

	got := n.talk("career").Messages
	if len(got) != 1 || got[0].Text != "ate curry" || got[0].State != model.MessageWaiting {
		t.Errorf("Career Talk's messages = %+v; want the message, waiting", got)
	}
}

func TestRegisterWebhooksPointsEveryBotAtItsWebhook(t *testing.T) {
	n, telegram := messengerNando(t)

	if err := n.core.RegisterWebhooks(context.Background()); err != nil {
		t.Fatalf("RegisterWebhooks: %v", err)
	}

	for token, url := range map[string]string{
		coreBotToken:   "https://api.example.com/messenger/telegram/core",
		careerBotToken: "https://api.example.com/messenger/telegram/career",
	} {
		if got := telegram.WebhookOf(token); got != (testtelegram.Webhook{URL: url, SecretToken: webhookSecret}) {
			t.Errorf("webhook of %s = %+v; want %s with the webhook secret", token, got, url)
		}
	}
	calls := telegram.CallsTo("setWebhook")
	if len(calls) != 2 {
		t.Fatalf("setWebhook calls = %+v; want one per bot, and none for the Japanese Assistant, which has none", calls)
	}
	for _, c := range calls {
		if updates, _ := c.Params["allowed_updates"].([]any); len(updates) != 1 || updates[0] != "message" {
			t.Errorf("setWebhook allowed_updates = %v; want only messages", c.Params["allowed_updates"])
		}
		if c.Params["drop_pending_updates"] == true {
			t.Errorf("setWebhook drops pending updates; want them kept")
		}
	}
}

func TestRegisterWebhooksReportsABotTelegramRefused(t *testing.T) {
	n, telegram := messengerNando(t)
	telegram.Fail("setWebhook")

	err := n.core.RegisterWebhooks(context.Background())
	if err == nil {
		t.Fatal("RegisterWebhooks = nil; want the failure")
	}
	for _, token := range []string{coreBotToken, careerBotToken} {
		if strings.Contains(err.Error(), token) {
			t.Errorf("RegisterWebhooks error %q reveals a bot token", err)
		}
	}
}

func TestAMessageNandoFailedToKeepIsKeptWhenTelegramDeliversItAgain(t *testing.T) {
	n, telegram := messengerNando(t)
	ctx := context.Background()
	// Have MongoDB refuse every Talk, as when it is briefly unavailable.
	setTalksValidator := func(validator bson.D) {
		t.Helper()
		if err := n.db.RunCommand(ctx, bson.D{{Key: "collMod", Value: "talks"}, {Key: "validator", Value: validator}}).Err(); err != nil {
			t.Fatalf("collMod talks: %v", err)
		}
	}
	setTalksValidator(bson.D{{Key: "assistant", Value: "no Talk is ever this"}})

	if rec := n.postUpdate("career", webhookSecret, ownerSays(5001, 42, "ate curry")); rec.Code != http.StatusInternalServerError {
		t.Fatalf("update Nando can't keep: status = %d, want 500 so Telegram delivers it again", rec.Code)
	}
	if reactions := telegram.CallsTo("setMessageReaction"); len(reactions) != 0 {
		t.Errorf("setMessageReaction calls = %+v; want none for a message not kept", reactions)
	}

	setTalksValidator(bson.D{})
	n.mustPostUpdate("career", ownerSays(5001, 42, "ate curry"))

	got := n.talk("career").Messages
	if len(got) != 1 || got[0].Text != "ate curry" || got[0].State != model.MessageWaiting {
		t.Errorf("Career Talk's messages = %+v; want the re-delivered message, waiting", got)
	}
}
