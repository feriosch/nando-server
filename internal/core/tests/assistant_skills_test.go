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
