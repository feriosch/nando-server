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

// spellingsOf is the spelling of each Word a search found, in order.
func spellingsOf(found searchJSON) []string { return spellings(found.Words) }

func TestAConnectorSearchesAsTheSearchScreenDoes(t *testing.T) {
	n, phone := signedInBrowser(t)
	foot := word("足", "あし", "pie, pierna", "basic")
	n.store("words", foot, word("足跡", "あしあと", "huella", "jlpt_n2"), word("明日", "あした", "mañana", "basic"))
	n.store("kanji", kanji("足", "pie", 51))
	session := phone.connect()

	got := callTool[searchJSON](t, session, "japanese_search", map[string]any{"q": "あし"})
	want := get[searchJSON](phone, "/api/japanese/search?q=%E3%81%82%E3%81%97")

	if !reflect.DeepEqual(got, want) || len(got.Words) != 3 {
		t.Errorf("japanese_search あし = %+v,\nwant the search screen's %+v", got, want)
	}
}

func TestAConnectorSearchesOneFieldForWordLinkCandidates(t *testing.T) {
	n, phone := signedInBrowser(t)
	n.store("words",
		word("足", "あし", "pie, pierna", "basic"),
		word("足跡", "あしあと", "huella", "jlpt_n2"),
		word("満足", "まんぞく", "satisfacción", "jlpt_n3"),
		word("歩く", "あるく", "caminar, ir a pie", "basic"),
		word("明日", "あした", "mañana", "basic"),
	)
	n.store("kanji", kanji("足", "pie", 51))
	session := phone.connect()

	for _, tc := range []struct {
		q, in string
		want  []string
	}{
		// Words whose spelling contains the Kanji 足, exact first, then shorter.
		{"足", "spelling", []string{"足", "満足", "足跡"}},
		// A reading isn't a spelling.
		{"あし", "spelling", []string{}},
		// Words whose Spanish meaning contains "pie", ignoring accents and case.
		{"PIE", "spanish", []string{"足", "歩く"}},
		{"satisfaccion", "spanish", []string{"満足"}},
	} {
		got := callTool[searchJSON](t, session, "japanese_search", map[string]any{"q": tc.q, "in": tc.in})
		if !reflect.DeepEqual(spellingsOf(got), tc.want) || len(got.Kanji) != 0 {
			t.Errorf("japanese_search %q in %s = %q and Kanji %+v, want %q and no Kanji", tc.q, tc.in, spellingsOf(got), got.Kanji, tc.want)
		}
	}
}

func TestASearchWithoutWhatToFindIsRefused(t *testing.T) {
	_, phone := signedInBrowser(t)
	session := phone.connect()

	for name, args := range map[string]map[string]any{
		"no q":          {},
		"blank q":       {"q": "  "},
		"unknown field": {"q": "足", "in": "notes"},
	} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "japanese_search", Arguments: args})
		if err != nil || !result.IsError {
			t.Errorf("japanese_search with %s: %v, %+v; want the Tool to fail", name, err, result)
		}
	}
}

// toolFails checks that calling the Tool name with args fails, as a Tool
// error the Connector's model reads.
func toolFails(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil || !result.IsError {
		t.Errorf("%s with %v: %v, %+v; want the Tool to fail", name, args, err, result)
	}
}

func TestAConnectorGetsAKanjiByItsCharacter(t *testing.T) {
	n, phone := signedInBrowser(t)
	every := kanji("毎", "cada", 486)
	sea := kanji("海", "mar", 491)
	sea["on"] = "カイ"
	sea["components"] = bson.A{"gotas de agua", "cada"}
	n.store("kanji", every, sea)
	n.store("irregular_components", irregularComponent("gotas de agua"))
	n.store("words", word("海外", "かいがい", "extranjero", "basic"))
	session := phone.connect()

	got := callTool[kanjiJSON](t, session, "japanese_get_kanji", map[string]any{"kanji": "海"})

	if want := get[kanjiJSON](phone, "/api/japanese/kanji/"+id(sea)); !reflect.DeepEqual(got, want) || len(got.Words) != 1 {
		t.Errorf("japanese_get_kanji 海 = %+v,\nwant the Kanji page's %+v", got, want)
	}
	for _, character := range []string{"山", "海外", ""} {
		toolFails(t, session, "japanese_get_kanji", map[string]any{"kanji": character})
	}
}

// tagsJSON is what japanese_list_tags answers: each tag in use and how many
// times, most used first.
type tagsJSON struct {
	WordTags   []tagCountJSON `json:"wordTags"`
	NuanceTags []tagCountJSON `json:"nuanceTags"`
}

type tagCountJSON struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

func TestAConnectorListsTheTagsInUse(t *testing.T) {
	n, phone := signedInBrowser(t)
	whisper := word("囁く", "ささやく", "susurrar", "jlpt_n2")
	murmur := word("呟く", "つぶやく", "murmurar", "jlpt_n2")
	talk := word("話す", "はなす", "hablar", "basic")
	whisper["tags"] = bson.A{"common", "usually_kana"}
	murmur["tags"] = bson.A{"common", "rare"}
	talk["tags"] = bson.A{"common"}
	whisper["related"] = bson.A{
		bson.M{"wordId": murmur["_id"], "type": "related", "note": "", "tags": bson.A{"spoken"}},
		bson.M{"wordId": talk["_id"], "type": "related", "note": "", "tags": bson.A{"spoken", "casual"}},
		// A Hint: benkyo stored tags on some, but they aren't Nuance Tags.
		bson.M{"word": "小声", "type": "related", "note": "", "tags": bson.A{"written"}},
	}
	n.store("words", whisper, murmur, talk)
	session := phone.connect()

	got := callTool[tagsJSON](t, session, "japanese_list_tags", map[string]any{})

	want := tagsJSON{
		WordTags:   []tagCountJSON{{"common", 3}, {"rare", 1}, {"usually_kana", 1}},
		NuanceTags: []tagCountJSON{{"spoken", 2}, {"casual", 1}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("japanese_list_tags = %+v,\nwant %+v", got, want)
	}
}
