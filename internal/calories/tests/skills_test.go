package tests

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
)

// assistantSkill is the Calories Assistant's skill (ADR 0008).
const assistantSkill = "../../../skills/assistants/calories/SKILL.md"

// skillToolName is how a skill names a Calories Tool.
var skillToolName = regexp.MustCompile(`calories_[a-z_]+`)

// TestTheCaloriesAssistantUsesOnlyServedToolsAndAsksBeforeDestructiveOnes
// keeps the Calories Assistant's skill in step with the Tools: every Calories
// Tool it names is served, and its "ask the Owner first" sentence names every
// Calories Tool Nando marks destructive, including any added later, as its
// run may call them only after the Owner's yes.
func TestTheCaloriesAssistantUsesOnlyServedToolsAndAsksBeforeDestructiveOnes(t *testing.T) {
	_, phone := signedInBrowser(t, wednesday)
	listed, err := phone.connect().ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	served := map[string]bool{}
	var destructive []string
	for _, tool := range listed.Tools {
		served[tool.Name] = true
		if tool.Annotations != nil && tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint {
			destructive = append(destructive, tool.Name)
		}
	}

	b, err := os.ReadFile(assistantSkill)
	if err != nil {
		t.Fatalf("read the Calories Assistant's skill: %v", err)
	}
	text := string(b)
	if !strings.HasPrefix(text, "---\nname: assistant-calories\ndescription:") {
		t.Error("the skill must start with frontmatter naming it assistant-calories and describing it")
	}
	names := skillToolName.FindAllString(text, -1)
	if len(names) == 0 {
		t.Error("the skill names no Calories Tool; it must log and look up through them")
	}
	for _, name := range names {
		if !served[name] {
			t.Errorf("the Calories Assistant's skill names %s, which Nando doesn't serve", name)
		}
	}
	var ask string
	for line := range strings.SplitSeq(text, "\n") {
		if strings.Contains(line, "ask the Owner first") {
			ask = line
		}
	}
	if ask == "" {
		t.Fatal("the Calories Assistant's skill has no \"ask the Owner first\" sentence for the destructive Tools")
	}
	for _, name := range destructive {
		if !strings.Contains(ask, "`"+name+"`") {
			t.Errorf("the Calories Assistant's \"ask the Owner first\" sentence doesn't name %s", name)
		}
	}
}
