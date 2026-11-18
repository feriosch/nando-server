package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ownerSendsPhoto is the Owner's photo, with caption, as Telegram posts it:
// the same photo in each size Telegram made of it, smallest first, each
// file_id naming its size.
func ownerSendsPhoto(updateID, messageID int64, caption string) telegramUpdate {
	u := ownerSays(updateID, messageID, "")
	u.Message.Caption = caption
	u.Message.Photo = []map[string]any{
		{"file_id": "photo-90", "file_unique_id": "u90", "width": 90, "height": 68},
		{"file_id": "photo-800", "file_unique_id": "u800", "width": 800, "height": 600},
		{"file_id": "photo-1280", "file_unique_id": "u1280", "width": 1280, "height": 960},
		{"file_id": "photo-2560", "file_unique_id": "u2560", "width": 2560, "height": 1920},
	}
	return u
}

func TestAPhotoWithACaptionWaitsAsOneMessage(t *testing.T) {
	n, telegram, trigger := messengerNandoWithTrigger(t)

	n.mustPostUpdate("career", ownerSendsPhoto(5001, 42, "recruiter X sent this, help me reply"))

	if reactions := telegram.CallsTo("setMessageReaction"); len(reactions) != 1 || reactionEmoji(reactions[0]) != "👀" {
		t.Errorf("setMessageReaction calls = %+v; want the 👀 on the photo", reactions)
	}
	if got := len(trigger.Received()); got != 1 {
		t.Errorf("fires = %d, want 1", got)
	}
	got := n.startRun().claim("")
	if got.Assistant != "career" || len(got.Messages) != 1 || got.Messages[0].Text != "recruiter X sent this, help me reply" ||
		!got.Messages[0].Waiting || len(got.Messages[0].Photos) != 1 || got.Messages[0].Photos[0].ID == "" {
		t.Errorf("claim = %+v; want the one message, its caption as its text, with its photo", got)
	}
}

func TestAPhotoWithoutACaptionWaitsToo(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)

	n.mustPostUpdate("career", ownerSendsPhoto(5001, 42, ""))

	if sent := telegram.CallsTo("sendMessage"); len(sent) != 0 {
		t.Errorf("sendMessage calls = %+v; want none: a photo isn't only text, but it is read now", sent)
	}
	got := n.startRun().claim("")
	if len(got.Messages) != 1 || got.Messages[0].Text != "" || !got.Messages[0].Waiting || len(got.Messages[0].Photos) != 1 {
		t.Errorf("claim = %+v; want the one message, without text, with its photo, waiting", got)
	}
}

// getPhoto calls messenger_get_photo as assistant, for the photo with id.
func (r *run) getPhoto(assistant, id string) *mcp.CallToolResult {
	r.n.t.Helper()
	result, err := r.session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "messenger_get_photo",
		Arguments: map[string]any{"assistant": assistant, "id": id},
	})
	if err != nil {
		r.n.t.Fatalf("messenger_get_photo: %v", err)
	}
	return result
}

func TestARunSeesThePhotoAtUpTo1280Pixels(t *testing.T) {
	n, telegram, _ := messengerNandoWithTrigger(t)
	for _, side := range []string{"90", "800", "1280", "2560"} {
		telegram.SetFile("photo-"+side, []byte("JPEG bytes, "+side+" px"))
	}
	n.mustPostUpdate("career", ownerSendsPhoto(5001, 42, "who is this recruiter?"))
	r := n.startRun()
	id := r.claim("").Messages[0].Photos[0].ID

	result := r.getPhoto("career", id)

	if result.IsError || len(result.Content) != 1 {
		t.Fatalf("messenger_get_photo = %+v; want one image", result.Content)
	}
	image, ok := result.Content[0].(*mcp.ImageContent)
	if !ok {
		t.Fatalf("messenger_get_photo answered %#v; want an image", result.Content[0])
	}
	if image.MIMEType != "image/jpeg" || string(image.Data) != "JPEG bytes, 1280 px" {
		t.Errorf("image = %s, %q; want image/jpeg, the 1280 px size", image.MIMEType, image.Data)
	}
	for _, c := range telegram.CallsTo("getFile") {
		if c.Token != careerBotToken {
			t.Errorf("getFile by %s; want the Career bot, which got the photo", c.Token)
		}
	}
}

func TestAPhotoTelegramDoesntHaveIsAnErrorWithoutTheBotToken(t *testing.T) {
	n, _, _ := messengerNandoWithTrigger(t)
	// Telegram keeps none of this photo's files, as when it dropped an old one.
	n.mustPostUpdate("career", ownerSendsPhoto(5001, 42, "remember this one?"))
	r := n.startRun()
	gone := r.claim("").Messages[0].Photos[0].ID

	for name, id := range map[string]string{"a photo Telegram no longer has": gone, "an unknown id": "no-such-photo"} {
		result := r.getPhoto("career", id)
		if !result.IsError || len(result.Content) != 1 {
			t.Errorf("%s: messenger_get_photo = %+v; want a Tool error", name, result.Content)
			continue
		}
		text, _ := result.Content[0].(*mcp.TextContent)
		if text == nil || strings.Contains(text.Text, careerBotToken) || strings.Contains(text.Text, "career-bot-token") {
			t.Errorf("%s: error %+v; want one that doesn't reveal the bot token", name, result.Content[0])
		}
	}
}
