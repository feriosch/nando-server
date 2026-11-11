package tests

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/feriosch/nando-server/internal/testconnector"
)

// connect has the Owner approve a Connector with the iPhone's Passkey and
// returns its MCP session, as Claude would have it.
func (d *browser) connect() *mcp.ClientSession {
	d.nando.t.Helper()
	return testconnector.Connect(d.nando.t, d.nando.handler, d.passkey, webOrigin)
}

// callTool calls the Tool name with args and decodes its result into a T,
// failing the test if the call or the Tool fails.
func callTool[T any](t *testing.T, session *mcp.ClientSession, name string, args map[string]any) T {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("%s failed: %+v", name, result.Content)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("%s answered %#v, want JSON text", name, result.Content[0])
	}
	var v T
	if err := json.Unmarshal([]byte(text.Text), &v); err != nil {
		t.Fatalf("%s: %v in %q", name, err, text.Text)
	}
	return v
}

// wordsJSON is what japanese_get_word answers: every Word that matches.
type wordsJSON struct {
	Words []wordJSON `json:"words"`
}

func TestAConnectorGetsAWordByIDSpellingOrReading(t *testing.T) {
	n, phone := signedInBrowser(t)
	n.store("groups", group("jlpt_n2", "JLPT N2", n2Picture))
	footprint := word("足跡", "あしあと", "huella, pisada", "jlpt_n2")
	footprint["tags"] = bson.A{"common"}
	n.store("words", footprint, word("足", "あし", "pie, pierna", "jlpt_n2"))
	session := phone.connect()

	want := []wordJSON{{
		ID:        id(footprint),
		Word:      "足跡",
		Hiragana:  ptr("あしあと"),
		Spanish:   "huella, pisada",
		Type:      map[string]int{"noun": 1},
		Group:     groupJSON{Key: "jlpt_n2", Name: "JLPT N2", ImageURL: ptr(n2Picture)},
		Tags:      []string{"common"},
		Sentences: []sentenceJSON{},
		WordLinks: []wordLinkJSON{},
		Hints:     []hintJSON{},
		Kanji:     []kanjiRowJSON{},
	}}
	for name, args := range map[string]map[string]any{
		"by id":       {"id": id(footprint)},
		"by spelling": {"word": "足跡"},
		"by reading":  {"word": "あしあと"},
	} {
		if got := callTool[wordsJSON](t, session, "japanese_get_word", args); !reflect.DeepEqual(got.Words, want) {
			t.Errorf("japanese_get_word %s = %+v,\nwant %+v", name, got.Words, want)
		}
	}
	if got := callTool[wordsJSON](t, session, "japanese_get_word", map[string]any{"word": "足跡を残す"}); got.Words == nil || len(got.Words) != 0 {
		t.Errorf("japanese_get_word for a Word that isn't stored = %+v, want words: []", got)
	}
}
