package tests

import "testing"

func TestAConversationIsReadWithItsMessages(t *testing.T) {
	_, phone := signedInBrowser(t)
	syncTanaka(t, phone)
	run := phone.connect()
	id := callTool[workJSON](t, run, "career_list_work", nil).Items[0].Conversation.ID

	got := callTool[conversationJSON](t, run, "career_get_conversation", map[string]any{"id": id})

	if got.ID != id || got.ThreadID != "2-tanaka" || len(got.Messages) != 1 || got.SuggestedReply != nil {
		t.Errorf("career_get_conversation = %+v; want 2-tanaka with its message and no Suggested Reply", got)
	}
	toolFails(t, run, "career_get_conversation", map[string]any{"id": "000000000000000000000000"})
}
