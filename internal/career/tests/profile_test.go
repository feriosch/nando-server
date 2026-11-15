package tests

import "testing"

// profileJSON is the Career Profile, as its Tools read and write it.
type profileJSON struct {
	Markdown string `json:"markdown"`
}

func TestTheCareerProfileIsOneDocumentTheRunReads(t *testing.T) {
	_, phone := signedInBrowser(t)
	chat := phone.connect()
	if got := callTool[profileJSON](t, chat, "career_get_profile", nil); got.Markdown != "" {
		t.Errorf("before it is saved, the Career Profile is %q; want empty", got.Markdown)
	}

	profile := "# Career Profile\n\nBackend engineer in Tokyo, 6 years of Go.\n\n## Deal-breakers\n\n- Full-time office"
	callTool[profileJSON](t, chat, "career_save_profile", map[string]any{"markdown": profile})
	callTool[profileJSON](t, chat, "career_save_profile", map[string]any{"markdown": profile + "\n- No remote at all"})

	if got := callTool[profileJSON](t, chat, "career_get_profile", nil); got.Markdown != profile+"\n- No remote at all" {
		t.Errorf("career_get_profile = %q; want the last one saved", got.Markdown)
	}
	syncTanaka(t, phone)
	if got := callTool[workJSON](t, phone.connect(), "career_list_work", nil); got.Profile != profile+"\n- No remote at all" {
		t.Errorf("the run's work has the profile %q; want the Career Profile", got.Profile)
	}
}
