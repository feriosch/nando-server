package tests

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
)

// assistantSkill is the Money Assistant's skill (ADR 0014).
const assistantSkill = "../../../skills/assistants/money/SKILL.md"

// skillToolName is how a skill names a Money Tool.
var skillToolName = regexp.MustCompile(`money_[a-z_]+`)

// readAssistantSkill is the Money Assistant's skill, which must start with
// frontmatter naming and describing it.
func readAssistantSkill(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(assistantSkill)
	if err != nil {
		t.Fatalf("read the Money Assistant's skill: %v", err)
	}
	text := string(b)
	if !strings.HasPrefix(text, "---\nname: assistant-money\ndescription:") {
		t.Error("the skill must start with frontmatter naming it assistant-money and describing it")
	}
	return text
}

// TestTheMoneyAssistantUsesOnlyServedToolsAndAsksBeforeDestructiveOnes keeps
// the skill in step with the Tools: every Money Tool it names is served, and
// its "ask the Owner first" sentence names every Money Tool Nando marks
// destructive, including any added later.
func TestTheMoneyAssistantUsesOnlyServedToolsAndAsksBeforeDestructiveOnes(t *testing.T) {
	_, phone := signedInBrowser(t)
	listed, err := phone.tools().ListTools(context.Background(), nil)
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

	text := readAssistantSkill(t)
	for _, name := range skillToolName.FindAllString(text, -1) {
		if !served[name] {
			t.Errorf("the Money Assistant's skill names %s, which Nando doesn't serve", name)
		}
	}
	var ask string
	for line := range strings.SplitSeq(text, "\n") {
		if strings.Contains(line, "ask the Owner first") {
			ask = line
		}
	}
	if ask == "" {
		t.Fatal("the skill has no \"ask the Owner first\" sentence for the destructive Tools")
	}
	moneyDestructive := 0
	for _, name := range destructive {
		if strings.HasPrefix(name, "money_") {
			moneyDestructive++
			if !strings.Contains(ask, "`"+name+"`") {
				t.Errorf("the \"ask the Owner first\" sentence doesn't name %s", name)
			}
		}
	}
	if moneyDestructive < 2 {
		t.Errorf("%d Money Tools are destructive; want deleting a Statement and merging Subcategories marked so", moneyDestructive)
	}
}

// TestTheMoneyAssistantReadsStatementsAndWritesTheReview checks the skill
// holds ADR 0014's ways: Statements, Merchants, Unconfirmed lines, the
// Review, and Gmail read only.
func TestTheMoneyAssistantReadsStatementsAndWritesTheReview(t *testing.T) {
	text := readAssistantSkill(t)
	for _, want := range []string{
		"📊 Monthly Review", "🧾 Statements", // the Notifications, as Nando sends them
		"`messenger_get_file`", "`money_save_statement`", "`money_find_merchant`", "`teachMerchant`",
		"`money_get_month_totals`", "`money_save_subscription`", "Unconfirmed", "askAbove",
		"`Banks/*`", "`Payments/*`", "Essential", "Optional", "Transfer",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the skill never says %q", want)
		}
	}
}
