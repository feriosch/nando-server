package tests

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// assistantsDir holds the skills an Assistant's run follows (ADR 0008): the
// Talk loop every run follows, and one skill per Assistant.
const assistantsDir = "../../../skills/assistants"

// messengerToolName is how a skill names a Messenger Tool.
var messengerToolName = regexp.MustCompile(`messenger_[a-z_]+`)

// readSkill is the SKILL.md in folder under assistantsDir, and fails the test
// unless it starts with frontmatter naming it as name and describing it.
func readSkill(t *testing.T, folder, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(assistantsDir, folder, "SKILL.md"))
	if err != nil {
		t.Fatalf("read the %s skill: %v", folder, err)
	}
	text := string(b)
	if !strings.HasPrefix(text, "---\nname: "+name+"\ndescription:") {
		t.Errorf("the %s skill must start with frontmatter naming it %s and describing it", folder, name)
	}
	return text
}

// servedTools is the name of every Tool Nando serves a Connector.
func servedTools(t *testing.T, n *nando) map[string]bool {
	t.Helper()
	listed, err := n.startRun().session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	served := map[string]bool{}
	for _, tool := range listed.Tools {
		served[tool.Name] = true
	}
	return served
}

func TestTheTalkSkillUsesTheMessengersTools(t *testing.T) {
	n, _ := messengerNando(t)
	served := servedTools(t, n)

	text := readSkill(t, "talk", "assistant-talk")
	for _, name := range []string{"messenger_claim_talk", "messenger_reply", "messenger_new_talk"} {
		if !strings.Contains(text, "`"+name+"`") {
			t.Errorf("the Talk skill never names %s", name)
		}
	}
	for _, name := range messengerToolName.FindAllString(text, -1) {
		if !served[name] {
			t.Errorf("the Talk skill names %s, which Nando doesn't serve", name)
		}
	}
}

// assistants are the Assistants with a skill, and how the skills call each.
var assistants = map[string]string{
	"core": "Core Assistant", "career": "Career Assistant", "japanese": "Japanese Assistant", "calories": "Calories Assistant",
}

func TestEachAssistantsSkillPointsToTheOthers(t *testing.T) {
	for folder := range assistants {
		t.Run(folder, func(t *testing.T) {
			text := readSkill(t, folder, "assistant-"+folder)
			for other, called := range assistants {
				if other != folder && !strings.Contains(text, called) {
					t.Errorf("the %s skill never names the %s, so it can't send the Owner there", folder, called)
				}
			}
		})
	}
}

// gmailChanges are the Gmail connector's Tools that change the Owner's mail.
// Only the Core Assistant uses them (ADR 0009).
var gmailChanges = regexp.MustCompile("`(create_draft|update_draft|send_message|reply|forward|(un)?label_(message|thread)|update_message_labels|create_label)`")

func TestOnlyTheCoreAssistantChangesMail(t *testing.T) {
	for folder := range assistants {
		if folder == "core" {
			continue
		}
		text := readSkill(t, folder, "assistant-"+folder)
		if found := gmailChanges.FindAllString(text, -1); len(found) > 0 {
			t.Errorf("the %s skill names %v, but only the Core Assistant changes mail", folder, found)
		}
	}
}

func TestTheCoreAssistantSendsMailOnlyAsADraftTheOwnerSaw(t *testing.T) {
	text := readSkill(t, "core", "assistant-core")
	for _, want := range []string{"`create_draft`", "`send_message`", "`draftId`", "yes", "Never use `reply` or `forward`"} {
		if !strings.Contains(text, want) {
			t.Errorf("the Core skill never says %s: it must draft, show, and send that draft on the Owner's yes", want)
		}
	}
	for _, banned := range []string{"trash_", "mark_message_spam", "mark_thread_spam", "delete_label"} {
		if strings.Contains(text, "`"+banned) {
			t.Errorf("the Core skill names %s, which it must never use", banned)
		}
	}
}

func TestTheCoreAssistantKnowsTheMailDigestAndTheTalkSkillKnowsNandosAsks(t *testing.T) {
	if core := readSkill(t, "core", "assistant-core"); !strings.Contains(core, "📬 Mail Digest") {
		t.Error("the Core skill never names the Mail Digest's ask, \"📬 Mail Digest\", so a run wouldn't know to answer it")
	}
	if talk := readSkill(t, "talk", "assistant-talk"); !strings.Contains(talk, "`nando`") {
		t.Error("the Talk skill never says what a message `from` `nando` is")
	}
}
