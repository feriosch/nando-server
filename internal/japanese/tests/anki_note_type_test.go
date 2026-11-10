package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The note type the JLPT deck fills is documented in internal/japanese/anki:
// its templates and README must agree with the fields the deck file has.

const noteTypeDir = "../anki"

// servedFields are the fields the JLPT deck's #columns line names.
func servedFields(t *testing.T) []string {
	t.Helper()
	_, phone := signedInBrowser(t)
	for _, line := range exportJLPTDeck(phone).header {
		if columns, ok := strings.CutPrefix(line, "#columns:"); ok {
			return strings.Split(columns, ",")
		}
	}
	t.Fatal("the JLPT deck has no #columns line")
	return nil
}

var templateField = regexp.MustCompile(`\{\{[#/^]?([^}]+)\}\}`)

// templateFields are the fields a card template uses.
func templateFields(t *testing.T, template string) map[string]bool {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(noteTypeDir, template))
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]bool{}
	for _, m := range templateField.FindAllStringSubmatch(string(b), -1) {
		fields[strings.TrimSpace(m[1])] = true
	}
	return fields
}

var templates = []string{"definition_card/front.html", "definition_card/back.html", "word_card/front.html", "word_card/back.html"}

func TestTheCardTemplatesUseOnlyFieldsTheDeckHas(t *testing.T) {
	fields := servedFields(t)
	for _, template := range templates {
		for field := range templateFields(t, template) {
			if field != "FrontSide" && !slices.Contains(fields, field) {
				t.Errorf("%s uses %q, which the deck doesn't have", template, field)
			}
		}
		if templateFields(t, template)["Id"] {
			t.Errorf("%s shows the Id", template)
		}
	}
}

func TestOnlyTheDefinitionCardsFrontShowsTheMaskedWordLinks(t *testing.T) {
	for template, want := range map[string]map[string]bool{
		"definition_card/front.html": {"Related Front": true, "Related Back": false},
		"definition_card/back.html":  {"Related Back": true},
		"word_card/front.html":       {"Related Back": false},
		"word_card/back.html":        {"Related Back": true},
	} {
		fields := templateFields(t, template)
		for field, uses := range want {
			if fields[field] != uses {
				t.Errorf("%s uses %s: %v, want %v", template, field, fields[field], uses)
			}
		}
	}
}

func TestTheNoteTypesREADMEListsTheFieldsInTheDecksOrder(t *testing.T) {
	fields := servedFields(t)
	b, err := os.ReadFile(filepath.Join(noteTypeDir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	var listed []string
	for _, m := range regexp.MustCompile(`(?m)^\d+\. (.+)$`).FindAllStringSubmatch(string(b), -1) {
		listed = append(listed, m[1])
	}
	if !slices.Equal(listed, fields) {
		t.Errorf("README lists %q, want %q", listed, fields)
	}
}
