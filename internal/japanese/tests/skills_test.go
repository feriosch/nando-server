package tests

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// skillsDir holds the skills Claude follows to work with the Owner's Words,
// in the repo's skills/japanese folder.
const skillsDir = "../../../skills/japanese"

// skills are the skills that must exist, each in its own folder.
var skills = []string{"add-word", "suggest-related"}

// toolName is how a skill names a Japanese Tool.
var toolName = regexp.MustCompile(`japanese_[a-z_]+`)

// oldAccess is what a skill must no longer use or mention: benkyo's direct
// database access and the Owner's PC. Every lookup and write goes through
// the Tools.
var oldAccess = []string{"mongosh", "curl ", "db.words", "claude-db.env", "MONGODB_URI", `C:\`, "/c/Users", "Boogs", ".benkyo", "word-log"}

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
			names := toolName.FindAllString(text, -1)
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
