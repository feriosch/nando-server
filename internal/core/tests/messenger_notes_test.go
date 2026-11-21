package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// replyWithNote calls messenger_reply with a Note, and returns what the Tool
// answered.
func (r *run) replyWithNote(assistant, text, note string) *mcp.CallToolResult {
	r.n.t.Helper()
	result, err := r.session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "messenger_reply",
		Arguments: map[string]any{"assistant": assistant, "text": text, "note": note},
	})
	if err != nil {
		r.n.t.Fatalf("messenger_reply: %v", err)
	}
	return result
}

func TestANoteIsKeptForTheNextRunAndNeverSentToTheOwner(t *testing.T) {
	n, telegram := messengerNando(t)
	n.mustPostUpdate("core", ownerSays(5001, 42, "answer Tanaka that Tuesday works"))
	first := n.startRun()
	first.claim("")

	note := "draft r-8812 to tanaka@example.com, re: Interview times"
	if result := first.replyWithNote("core", "Here's the draft:\nTuesday works for me.\nSend it?", note); result.IsError {
		t.Fatalf("messenger_reply with a note failed: %+v", result.Content)
	}
	for _, c := range telegram.Calls() {
		for _, v := range c.Params {
			if s, ok := v.(string); ok && strings.Contains(s, "r-8812") {
				t.Errorf("%s sent %q to Telegram; the Note must never reach the Owner", c.Method, s)
			}
		}
	}

	n.mustPostUpdate("core", ownerSays(5002, 44, "yes"))
	got := n.startRun().claim("")
	if len(got.Messages) != 3 || got.Messages[1].From != "assistant" || got.Messages[1].Note != note {
		t.Fatalf("claim = %+v; want the earlier reply carrying its Note", got.Messages)
	}
	if got.Messages[0].Note != "" || got.Messages[2].Note != "" {
		t.Errorf("claim = %+v; want a Note only on the reply that left it", got.Messages)
	}
}

func TestANoteOverAThousandCharactersIsRefusedAndNothingIsSent(t *testing.T) {
	n, telegram := messengerNando(t)
	n.mustPostUpdate("core", ownerSays(5001, 42, "hi"))
	r := n.startRun()
	r.claim("")
	before := len(telegram.CallsTo("sendMessage"))

	// 1,001 characters, each several bytes: the limit counts characters.
	result := r.replyWithNote("core", "Hello!", strings.Repeat("下", 1001))

	text, _ := result.Content[0].(*mcp.TextContent)
	if !result.IsError || text == nil || !strings.Contains(text.Text, "at most 1000") {
		t.Errorf("messenger_reply with a 1,001-character note = %+v; want a Tool error naming the limit", result.Content)
	}
	if after := len(telegram.CallsTo("sendMessage")); after != before {
		t.Errorf("sendMessage calls went from %d to %d; want nothing sent", before, after)
	}
	if exactly := r.replyWithNote("core", "Hello!", strings.Repeat("下", 1000)); exactly.IsError {
		t.Errorf("messenger_reply with a 1,000-character note failed: %+v", exactly.Content)
	}
}

func TestASplitReplyKeepsItsNoteOnTheFirstPartOnly(t *testing.T) {
	n, telegram := messengerNando(t)
	n.mustPostUpdate("core", ownerSays(5001, 42, "summarize the thread"))
	r := n.startRun()
	r.claim("")

	long := strings.Repeat("a line of the summary\n", 300) // well over one Telegram message
	if result := r.replyWithNote("core", long, "thread 18c2f"); result.IsError {
		t.Fatalf("messenger_reply failed: %+v", result.Content)
	}

	if sent := telegram.CallsTo("sendMessage"); len(sent) < 2 {
		t.Fatalf("sendMessage calls = %d; want the reply split", len(sent))
	}
	var notes []string
	for _, m := range n.talk("core").Messages {
		if m.From == "assistant" {
			notes = append(notes, m.Note)
		}
	}
	if len(notes) < 2 || notes[0] != "thread 18c2f" || strings.Join(notes[1:], "") != "" {
		t.Errorf("the reply's parts carry Notes %q; want it on the first part only", notes)
	}
}
