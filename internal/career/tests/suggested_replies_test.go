package tests

import (
	"testing"
	"time"
)

// pitched is when Tanaka's pitch in syncTanaka was sent.
var pitched = at(-3, 10, 15)

func TestASuggestedReplyIsReplacedThenRemovedByTheOwnersLaterMessage(t *testing.T) {
	_, phone := signedInBrowser(t)
	run, filed := fileTanakasConversation(t, phone)
	id := filed.Conversation.ID

	first := callTool[conversationJSON](t, run, "career_set_suggested_reply", map[string]any{
		"conversationId": id, "text": "Thank you for reaching out! Could you share the job description?",
		"language": "en", "kind": "answer", "answers": pitched.Format(time.RFC3339),
	})
	if first.SuggestedReply == nil || first.SuggestedReply.Kind != "answer" || !first.SuggestedReply.Answers.Equal(pitched) {
		t.Fatalf("career_set_suggested_reply answered %+v; want the answer to Tanaka's pitch", first.SuggestedReply)
	}
	callTool[conversationJSON](t, run, "career_set_suggested_reply", map[string]any{
		"conversationId": id, "text": "Thanks! I'm not looking right now, but let's keep in touch.",
		"language": "en", "kind": "answer", "answers": pitched.Format(time.RFC3339),
		"notes": []any{map[string]any{"wrote": "Thank you for reach out", "natural": "Thank you for reaching out", "why": "after for, the -ing form"}},
	})
	got := callTool[conversationJSON](t, run, "career_get_conversation", map[string]any{"id": id})
	if got.SuggestedReply == nil || got.SuggestedReply.Text != "Thanks! I'm not looking right now, but let's keep in touch." ||
		got.SuggestedReply.Language != "en" || len(got.SuggestedReply.Notes) != 1 || got.SuggestedReply.WrittenAt.IsZero() {
		t.Fatalf("after a second one, the Suggested Reply is %+v; want only the second, with its note", got.SuggestedReply)
	}

	// The Owner's own message from before the pitch leaves it.
	callTool[recordedJSON](t, phone.macAt(at(0, 15, 0)), "career_record_sync", map[string]any{"threads": []any{
		tanakaThread(message("m0", "owner", at(-4, 9, 0), "Hello, nice to connect.")),
	}})
	if got := callTool[conversationJSON](t, phone.connect(), "career_get_conversation", map[string]any{"id": id}); got.SuggestedReply == nil {
		t.Fatalf("the Owner's message from before the pitch removed the Suggested Reply; want it kept")
	}
	// The Owner answered on LinkedIn themselves.
	callTool[recordedJSON](t, phone.macAt(at(1, 9, 0)), "career_record_sync", map[string]any{"threads": []any{
		tanakaThread(message("m2", "owner", at(0, 20, 0), "Thank you! Not looking right now.")),
	}})
	if got := callTool[conversationJSON](t, phone.connect(), "career_get_conversation", map[string]any{"id": id}); got.SuggestedReply != nil {
		t.Errorf("after the Owner's later message the Suggested Reply is %+v; want it removed", got.SuggestedReply)
	}
}

func TestASuggestedReplyIsChecked(t *testing.T) {
	_, phone := signedInBrowser(t)
	run, filed := fileTanakasConversation(t, phone)
	reply := func(change func(map[string]any)) map[string]any {
		args := map[string]any{
			"conversationId": filed.Conversation.ID, "text": "Thank you!", "language": "en", "kind": "answer",
			"answers": pitched.Format(time.RFC3339),
		}
		change(args)
		return args
	}
	note := map[string]any{"wrote": "a", "natural": "b", "why": "c"}

	for _, args := range []map[string]any{
		reply(func(a map[string]any) { a["text"] = " " }),
		reply(func(a map[string]any) { a["language"] = "es" }),
		reply(func(a map[string]any) { a["kind"] = "decline" }),
		reply(func(a map[string]any) { a["answers"] = at(-2, 0, 0).Format(time.RFC3339) }),
		reply(func(a map[string]any) { a["notes"] = []any{note, note, note, note} }),
		reply(func(a map[string]any) { a["notes"] = []any{map[string]any{"wrote": "a"}} }),
		reply(func(a map[string]any) { a["conversationId"] = "000000000000000000000000" }),
	} {
		toolFails(t, run, "career_set_suggested_reply", args)
	}
	callTool[conversationJSON](t, run, "career_set_suggested_reply", reply(func(a map[string]any) {
		a["language"], a["kind"], a["notes"] = "ja", "late", []any{note, note, note}
	}))
}

func TestASuggestedReplyDoesTheReplyItem(t *testing.T) {
	n, phone := signedInBrowser(t)
	fileTanaka(t, phone)
	asked := at(0, 14, 0)
	callTool[recordedJSON](t, phone.macAt(at(0, 15, 0)), "career_record_sync", map[string]any{"threads": []any{
		tanakaThread(message("m2", "other", asked, "Are you free for a call this week?")),
	}})
	run := phone.connect()
	item := callTool[workJSON](t, run, "career_list_work", nil).Items[0]

	callTool[conversationJSON](t, run, "career_set_suggested_reply", map[string]any{
		"conversationId": item.Conversation.ID, "text": "Thank you, but I'm not looking right now.",
		"language": "en", "kind": "answer", "answers": asked.Format(time.RFC3339),
	})

	n.clock.Set(at(1, 8, 0))
	if left := callTool[workJSON](t, phone.connect(), "career_list_work", nil); len(left.Items) != 0 {
		t.Errorf("after the Suggested Reply, a run claimed %+v; want nothing", left.Items)
	}
}
