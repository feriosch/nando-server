package tests

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// skillsDir holds the skills Claude follows to work with the Owner's Words,
// in the repo's skills/japanese folder, each in its own folder.
const skillsDir = "../../../skills/japanese"

// requiredSkills are the skills that must exist among them.
var requiredSkills = []string{"add-word", "suggest-related", "word-requests"}

// skillToolName is how a skill names a Japanese Tool.
var skillToolName = regexp.MustCompile(`japanese_[a-z_]+`)

// oldAccess is what a skill must no longer use or mention: benkyo's direct
// database access and paths on the Owner's PC. Every lookup and write goes
// through the Tools.
var oldAccess = []string{"mongosh", "curl ", "db.words", "claude-db.env", "MONGODB_URI", `C:\`, "/c/Users", ".benkyo", "word-log"}

func TestTheSkillsUseOnlyTheToolsNandoServes(t *testing.T) {
	_, phone := signedInBrowser(t)
	listed, err := phone.connect().ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	served := map[string]bool{}
	for _, tool := range listed.Tools {
		served[tool.Name] = true
	}

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		t.Fatalf("list the skills: %v", err)
	}
	var skills []string
	for _, entry := range entries {
		if entry.IsDir() {
			skills = append(skills, entry.Name())
		}
	}
	for _, skill := range requiredSkills {
		if !slices.Contains(skills, skill) {
			t.Errorf("the %s skill is missing from %s", skill, skillsDir)
		}
	}

	for _, skill := range skills {
		t.Run(skill, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join(skillsDir, skill, "SKILL.md"))
			if err != nil {
				t.Fatalf("read the skill: %v", err)
			}
			text := string(b)
			if !strings.HasPrefix(text, "---\nname: "+skill+"\ndescription:") {
				t.Errorf("SKILL.md must start with frontmatter naming %s and describing it, for claude.ai to load it", skill)
			}
			names := skillToolName.FindAllString(text, -1)
			if len(names) == 0 {
				t.Errorf("the skill names no Tool; it must look up and write through them")
			}
			for _, name := range names {
				if !served[name] {
					t.Errorf("the skill names %s, which Nando doesn't serve", name)
				}
			}
			for _, old := range oldAccess {
				if strings.Contains(text, old) {
					t.Errorf("the skill still mentions %q: data goes through the Tools only", old)
				}
			}
		})
	}
}

// TestTheUnattendedSkillBansEveryDestructiveTool keeps the one guard ADR 0006
// relies on: no Owner watches a scheduled run, so the word-requests skill's
// NEVER sentence must name every Tool Nando marks destructive, including any
// added later.
func TestTheUnattendedSkillBansEveryDestructiveTool(t *testing.T) {
	_, phone := signedInBrowser(t)
	listed, err := phone.connect().ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	var destructive []string
	for _, tool := range listed.Tools {
		if tool.Annotations != nil && tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint {
			destructive = append(destructive, tool.Name)
		}
	}
	if len(destructive) == 0 {
		t.Fatal("no Tool is marked destructive; this test would check nothing")
	}

	b, err := os.ReadFile(filepath.Join(skillsDir, "word-requests", "SKILL.md"))
	if err != nil {
		t.Fatalf("read the skill: %v", err)
	}
	var ban string
	for line := range strings.SplitSeq(string(b), "\n") {
		if strings.Contains(line, "NEVER call") {
			ban = line
		}
	}
	if ban == "" {
		t.Fatal("the word-requests skill has no NEVER call sentence banning the destructive Tools")
	}
	for _, name := range destructive {
		if !strings.Contains(ban, "`"+name+"`") {
			t.Errorf("the word-requests skill's NEVER call sentence doesn't name %s", name)
		}
	}
}
