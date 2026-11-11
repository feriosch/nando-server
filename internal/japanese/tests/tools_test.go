package tests

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"sync"
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

// createdJSON is what a Tool that adds something answers: its id.
type createdJSON struct {
	ID string `json:"id"`
}

// whisper is a Word as Claude adds it, with every field.
func whisper() map[string]any {
	return map[string]any{
		"word":      "囁く",
		"hiragana":  "ささやく",
		"spanish":   "susurrar (a alguien, en voz baja)",
		"type":      map[string]any{"verb": 1},
		"group":     "jlpt_n2",
		"tags":      []string{"common", "usually_kana"},
		"notes":     "JLPT N2",
		"sentences": []map[string]string{{"sentence": "彼は耳元で囁いた。", "translation": "He whispered in my ear."}},
	}
}

// with is args with key set to value, or without key if value is nil.
func with(args map[string]any, key string, value any) map[string]any {
	changed := map[string]any{}
	for k, v := range args {
		changed[k] = v
	}
	if value == nil {
		delete(changed, key)
	} else {
		changed[key] = value
	}
	return changed
}

func TestAConnectorAddsAWord(t *testing.T) {
	n, phone := signedInBrowser(t)
	n.store("groups", group("jlpt_n2", "JLPT N2", n2Picture))
	session := phone.connect()

	added := callTool[createdJSON](t, session, "japanese_add_word", whisper())

	want := wordJSON{
		ID:        added.ID,
		Word:      "囁く",
		Hiragana:  ptr("ささやく"),
		Spanish:   "susurrar (a alguien, en voz baja)",
		Type:      map[string]int{"verb": 1},
		Group:     groupJSON{Key: "jlpt_n2", Name: "JLPT N2", ImageURL: ptr(n2Picture)},
		Tags:      []string{"common", "usually_kana"},
		Notes:     ptr("JLPT N2"),
		Sentences: []sentenceJSON{{Sentence: "彼は耳元で囁いた。", Translation: "He whispered in my ear."}},
		WordLinks: []wordLinkJSON{},
		Hints:     []hintJSON{},
		Kanji:     []kanjiRowJSON{},
	}
	if got := get[wordJSON](phone, "/api/japanese/words/"+added.ID); !reflect.DeepEqual(got, want) {
		t.Errorf("the added Word's page = %+v,\nwant %+v", got, want)
	}
}

func TestAWordWithoutAReadingIsStoredInKana(t *testing.T) {
	n, phone := signedInBrowser(t)
	n.store("groups", group("miscellaneous", "Miscellaneous", "https://storage.googleapis.com/benkyo/misc.png"))
	session := phone.connect()

	added := callTool[createdJSON](t, session, "japanese_add_word", map[string]any{
		"word": "あしからず", "spanish": "no lo tomes a mal", "type": map[string]any{"adv": 1}, "group": "miscellaneous",
	})

	got := get[wordJSON](phone, "/api/japanese/words/"+added.ID)
	if got.Word != "あしからず" || got.Hiragana != nil || got.Notes != nil || len(got.Tags) != 0 || len(got.Sentences) != 0 {
		t.Errorf("the added kana Word = %+v, want あしからず with no reading, notes, tags or sentences", got)
	}
}

func TestAWordAlreadyStoredIsRefused(t *testing.T) {
	n, phone := signedInBrowser(t)
	n.store("groups", group("jlpt_n2", "JLPT N2", n2Picture), group("basic", "Basic", "https://storage.googleapis.com/benkyo/basic.png"))
	n.store("words", word("包む", "つつむ", "envolver", "basic"), word("くるむ", "", "envolver (con tela)", "basic"))
	session := phone.connect()
	callTool[createdJSON](t, session, "japanese_add_word", whisper())

	// The same spelling and reading, even in another Group.
	toolFails(t, session, "japanese_add_word", with(whisper(), "group", "basic"))
	toolFails(t, session, "japanese_add_word", map[string]any{"word": "包む", "hiragana": "つつむ", "spanish": "x", "type": map[string]any{"verb": 1}, "group": "basic"})
	toolFails(t, session, "japanese_add_word", map[string]any{"word": "くるむ", "spanish": "x", "type": map[string]any{"verb": 1}, "group": "basic"})
	// A spelling shared by another reading is a different Word.
	callTool[createdJSON](t, session, "japanese_add_word", map[string]any{"word": "包む", "hiragana": "くるむ", "spanish": "envolver (con tela)", "type": map[string]any{"verb": 1}, "group": "basic"})

	for spelling, want := range map[string]int{"囁く": 1, "包む": 2, "くるむ": 2} {
		if got := callTool[wordsJSON](t, session, "japanese_get_word", map[string]any{"word": spelling}); len(got.Words) != want {
			t.Errorf("Words spelled or read %s: %d, want %d", spelling, len(got.Words), want)
		}
	}
}

func TestOnlyOneOfManySimultaneousAddsOfAWordSucceeds(t *testing.T) {
	n, phone := signedInBrowser(t)
	n.store("groups", group("jlpt_n2", "JLPT N2", n2Picture))
	session := phone.connect()

	var wg sync.WaitGroup
	results := make([]bool, 5)
	for i := range results {
		wg.Go(func() {
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "japanese_add_word", Arguments: whisper()})
			results[i] = err == nil && !result.IsError
		})
	}
	wg.Wait()

	if added := slices.Index(results, true); added == -1 || slices.Index(results[added+1:], true) != -1 {
		t.Errorf("simultaneous adds that succeeded: %v, want exactly one", results)
	}
	if got := callTool[wordsJSON](t, session, "japanese_get_word", map[string]any{"word": "囁く"}); len(got.Words) != 1 {
		t.Errorf("Words spelled 囁く after simultaneous adds: %d, want 1", len(got.Words))
	}
}

func TestAnInvalidWordIsRefused(t *testing.T) {
	n, phone := signedInBrowser(t)
	n.store("groups", group("jlpt_n2", "JLPT N2", n2Picture))
	session := phone.connect()

	for name, args := range map[string]map[string]any{
		"no spelling":                    with(whisper(), "word", nil),
		"blank spelling":                 with(whisper(), "word", " "),
		"no Spanish":                     with(whisper(), "spanish", nil),
		"no type":                        with(whisper(), "type", nil),
		"an unknown type":                with(whisper(), "type", map[string]any{"verbo": 1}),
		"a type value out of 1–8":        with(whisper(), "type", map[string]any{"verb": 9}),
		"no Group":                       with(whisper(), "group", nil),
		"a Group not stored":             with(whisper(), "group", "jlpt_n4"),
		"a camelCase tag":                with(whisper(), "tags", []string{"usuallyKana"}),
		"a tag with spaces":              with(whisper(), "tags", []string{"usually kana"}),
		"a sentence without translation": with(whisper(), "sentences", []map[string]string{{"sentence": "彼は囁いた。"}}),
	} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "japanese_add_word", Arguments: args})
		if err != nil || !result.IsError {
			t.Errorf("japanese_add_word with %s: %v, %+v; want the Tool to fail", name, err, result)
		}
	}
	if got := callTool[wordsJSON](t, session, "japanese_get_word", map[string]any{"word": "囁く"}); len(got.Words) != 0 {
		t.Errorf("Words after only invalid adds: %+v, want none", got.Words)
	}
}

func TestAConnectorUpdatesOnlyTheFieldsItSends(t *testing.T) {
	n, phone := signedInBrowser(t)
	n.store("groups", group("jlpt_n2", "JLPT N2", n2Picture))
	murmur := word("呟く", "つぶやく", "murmurar", "jlpt_n2")
	whispering := word("囁く", "ささやく", "susurrar", "jlpt_n2")
	whispering["tags"] = bson.A{"common"}
	whispering["notes"] = "JLPT N2"
	whispering["sentences"] = bson.A{bson.M{"sentence": "彼は耳元で囁いた。", "translation": "He whispered in my ear."}}
	whispering["related"] = bson.A{
		bson.M{"wordId": murmur["_id"], "type": "related", "note": "more to oneself", "tags": bson.A{"spoken"}},
		bson.M{"word": "小声", "type": "related", "note": "", "tags": bson.A{}},
	}
	n.store("words", whispering, murmur)
	session := phone.connect()
	before := get[wordJSON](phone, "/api/japanese/words/"+id(whispering))

	callTool[map[string]any](t, session, "japanese_update_word", map[string]any{
		"id":      id(whispering),
		"spanish": "susurrar (a alguien, en voz baja)",
		"tags":    []string{"common", "usually_kana"},
	})

	want := before
	want.Spanish = "susurrar (a alguien, en voz baja)"
	want.Tags = []string{"common", "usually_kana"}
	if got := get[wordJSON](phone, "/api/japanese/words/"+id(whispering)); !reflect.DeepEqual(got, want) {
		t.Errorf("after updating spanish and tags, the Word = %+v,\nwant %+v", got, want)
	}
}

func TestAKanaWordIsMovedToItsKanjiSpelling(t *testing.T) {
	n, phone := signedInBrowser(t)
	where := word("どこ", "", "dónde", "basic")
	where["notes"] = "kana"
	n.store("words", where)
	session := phone.connect()

	callTool[map[string]any](t, session, "japanese_update_word", map[string]any{
		"id": id(where), "word": "何処", "hiragana": "どこ", "tags": []string{"common", "usually_kana"}, "notes": "",
	})

	got := get[wordJSON](phone, "/api/japanese/words/"+id(where))
	if got.Word != "何処" || got.Hiragana == nil || *got.Hiragana != "どこ" || got.Notes != nil || got.Spanish != "dónde" {
		t.Errorf("the Word = %+v, want 何処 read どこ, no notes, Spanish unchanged", got)
	}
	if found := callTool[wordsJSON](t, session, "japanese_get_word", map[string]any{"word": "どこ"}); len(found.Words) != 1 {
		t.Errorf("Words read どこ: %d, want 1", len(found.Words))
	}
}

func TestAnUpdateChecksOnlyTheFieldsItSends(t *testing.T) {
	n, phone := signedInBrowser(t)
	// A stray form benkyo stored that the add rules don't allow any more.
	stray := word("どんどん", "", "rápidamente", "basic")
	stray["type"] = bson.M{"adv": 9}
	n.store("words", stray)
	session := phone.connect()

	callTool[map[string]any](t, session, "japanese_update_word", map[string]any{"id": id(stray), "spanish": "rápidamente, sin parar"})

	if got := get[wordJSON](phone, "/api/japanese/words/"+id(stray)); got.Spanish != "rápidamente, sin parar" || got.Type["adv"] != 9 {
		t.Errorf("the Word = %+v, want the new Spanish and its type left as stored", got)
	}
}

func TestAnUpdateThatCantBeMadeIsRefused(t *testing.T) {
	n, phone := signedInBrowser(t)
	n.store("groups", group("jlpt_n2", "JLPT N2", n2Picture))
	whispering := word("囁く", "ささやく", "susurrar", "jlpt_n2")
	n.store("words", whispering, word("呟く", "つぶやく", "murmurar", "jlpt_n2"))
	session := phone.connect()
	update := func(fields map[string]any) map[string]any { return with(fields, "id", id(whispering)) }

	for name, args := range map[string]map[string]any{
		"no id":                               {"spanish": "x"},
		"an unknown id":                       {"id": "6af05d493ea403a78bf5997b", "spanish": "x"},
		"nothing to change":                   update(map[string]any{}),
		"a Group":                             update(map[string]any{"group": "basic"}),
		"Word Links":                          update(map[string]any{"related": []any{}}),
		"another Word's spelling and reading": update(map[string]any{"word": "呟く", "hiragana": "つぶやく"}),
		"a blank spelling":                    update(map[string]any{"word": " "}),
		"a camelCase tag":                     update(map[string]any{"tags": []string{"usuallyKana"}}),
		"an unknown type":                     update(map[string]any{"type": map[string]any{"verbo": 1}}),
	} {
		t.Run(name, func(t *testing.T) { toolFails(t, session, "japanese_update_word", args) })
	}
	if got := get[wordJSON](phone, "/api/japanese/words/"+id(whispering)); got.Word != "囁く" || got.Spanish != "susurrar" || got.Group.Key != "jlpt_n2" {
		t.Errorf("the Word after refused updates = %+v, want it unchanged", got)
	}
}

// linkedJSON is what japanese_add_word_link answers: which sides it wrote.
type linkedJSON struct {
	FromSideWritten bool `json:"fromSideWritten"`
	BackSideWritten bool `json:"backSideWritten"`
}

// wordLinksOf is the Word Links on a Word's page.
func wordLinksOf(phone *browser, wordID string) []wordLinkJSON {
	return get[wordJSON](phone, "/api/japanese/words/"+wordID).WordLinks
}

// rowOf is a Word as a Word Link shows it.
func rowOf(w bson.M) wordRowJSON {
	return wordRowJSON{ID: id(w), Word: w["word"].(string), Hiragana: ptr(w["hiragana"].(string)), Spanish: w["spanish"].(string), Group: w["group"].(string)}
}

func TestAConnectorLinksTwoWordsBothWaysWithSwappedNuanceTags(t *testing.T) {
	n, phone := signedInBrowser(t)
	whispering := word("囁く", "ささやく", "susurrar", "jlpt_n2")
	murmur := word("呟く", "つぶやく", "murmurar", "jlpt_n2")
	n.store("words", whispering, murmur)
	session := phone.connect()

	got := callTool[linkedJSON](t, session, "japanese_add_word_link", map[string]any{
		"from": id(whispering), "to": id(murmur), "kind": "related",
		"note": "Said to oneself, not to someone.", "nuanceTags": []string{"spoken", "formal", "rare"},
	})

	if !got.FromSideWritten || !got.BackSideWritten {
		t.Errorf("sides written = %+v, want both", got)
	}
	if want := []wordLinkJSON{{Kind: "related", Note: ptr("Said to oneself, not to someone."), NuanceTags: []string{"spoken", "formal", "rare"}, Word: rowOf(murmur)}}; !reflect.DeepEqual(wordLinksOf(phone, id(whispering)), want) {
		t.Errorf("囁く's Word Links = %+v,\nwant %+v", wordLinksOf(phone, id(whispering)), want)
	}
	// The way back has the same note and the opposite Nuance Tags.
	if want := []wordLinkJSON{{Kind: "related", Note: ptr("Said to oneself, not to someone."), NuanceTags: []string{"written", "casual", "rare"}, Word: rowOf(whispering)}}; !reflect.DeepEqual(wordLinksOf(phone, id(murmur)), want) {
		t.Errorf("呟く's Word Links = %+v,\nwant %+v", wordLinksOf(phone, id(murmur)), want)
	}
}

func TestAnExpressionAndItsPartSwapTheirNuanceTagsWithTheirOwnNotes(t *testing.T) {
	n, phone := signedInBrowser(t)
	turn := word("身を翻す", "みをひるがえす", "girar el cuerpo", "gaming")
	body := word("身", "み", "cuerpo", "basic")
	n.store("words", turn, body)
	session := phone.connect()

	callTool[linkedJSON](t, session, "japanese_add_word_link", map[string]any{
		"from": id(turn), "to": id(body), "kind": "related",
		"note": "The body; this expression is about it turning.", "nuanceTags": []string{"part"},
		"noteBack": "Expression: turn the body quickly, to dodge or leave.",
	})

	if got := wordLinksOf(phone, id(turn)); len(got) != 1 || !reflect.DeepEqual(got[0].NuanceTags, []string{"part"}) || *got[0].Note != "The body; this expression is about it turning." {
		t.Errorf("身を翻す's Word Links = %+v, want 身 tagged part", got)
	}
	if got := wordLinksOf(phone, id(body)); len(got) != 1 || !reflect.DeepEqual(got[0].NuanceTags, []string{"expression"}) || *got[0].Note != "Expression: turn the body quickly, to dodge or leave." {
		t.Errorf("身's Word Links = %+v, want 身を翻す tagged expression with its own note", got)
	}
}

func TestASideThatAlreadyExistsIsSkipped(t *testing.T) {
	n, phone := signedInBrowser(t)
	hope := word("希望", "きぼう", "esperanza", "basic")
	expect := word("期待", "きたい", "expectativa", "jlpt_n2")
	expect["related"] = bson.A{bson.M{"wordId": hope["_id"], "type": "synonym", "note": "Already here.", "tags": bson.A{}}}
	n.store("words", expect, hope)
	session := phone.connect()
	link := map[string]any{"from": id(expect), "to": id(hope), "kind": "related", "note": "New note.", "nuanceTags": []string{"written"}}

	first := callTool[linkedJSON](t, session, "japanese_add_word_link", link)
	again := callTool[linkedJSON](t, session, "japanese_add_word_link", link)

	if first.FromSideWritten || !first.BackSideWritten {
		t.Errorf("first link: sides written = %+v, want only the way back", first)
	}
	if again.FromSideWritten || again.BackSideWritten {
		t.Errorf("linking again: sides written = %+v, want none", again)
	}
	if got := wordLinksOf(phone, id(expect)); len(got) != 1 || got[0].Kind != "synonym" || *got[0].Note != "Already here." {
		t.Errorf("期待's Word Links = %+v, want only its own synonym side, unchanged", got)
	}
	if got := wordLinksOf(phone, id(hope)); len(got) != 1 || got[0].Kind != "related" || !reflect.DeepEqual(got[0].NuanceTags, []string{"spoken"}) {
		t.Errorf("希望's Word Links = %+v, want one related side back, tagged spoken", got)
	}
}

func TestAWordLinkThatCantBeMadeIsRefused(t *testing.T) {
	n, phone := signedInBrowser(t)
	whispering := word("囁く", "ささやく", "susurrar", "jlpt_n2")
	murmur := word("呟く", "つぶやく", "murmurar", "jlpt_n2")
	n.store("words", whispering, murmur)
	session := phone.connect()
	link := map[string]any{"from": id(whispering), "to": id(murmur), "kind": "synonym", "note": "", "nuanceTags": []string{}}

	for name, args := range map[string]map[string]any{
		"an unknown Word":  with(link, "to", "6af05d493ea403a78bf5997b"),
		"no from":          with(link, "from", nil),
		"a link to itself": with(link, "to", id(whispering)),
		"an unknown kind":  with(link, "kind", "similar"),
		"a camelCase tag":  with(link, "nuanceTags", []string{"moreFormal"}),
	} {
		t.Run(name, func(t *testing.T) { toolFails(t, session, "japanese_add_word_link", args) })
	}
	if got := wordLinksOf(phone, id(whispering)); len(got) != 0 {
		t.Errorf("囁く's Word Links after refused links = %+v, want none", got)
	}
}

// linkedPair is two stored Words with a Word Link both ways, as benkyo
// stored them, and the session of a Connector.
func linkedPair(t *testing.T) (*browser, *mcp.ClientSession, bson.M, bson.M) {
	t.Helper()
	n, phone := signedInBrowser(t)
	whispering := word("囁く", "ささやく", "susurrar", "jlpt_n2")
	murmur := word("呟く", "つぶやく", "murmurar", "jlpt_n2")
	hope := word("希望", "きぼう", "esperanza", "basic")
	whispering["related"] = bson.A{
		bson.M{"wordId": murmur["_id"], "type": "related", "note": "To oneself.", "tags": bson.A{"spoken"}},
		bson.M{"wordId": hope["_id"], "type": "related", "note": "Unrelated link.", "tags": bson.A{}},
		bson.M{"word": "小声", "type": "related", "note": "", "tags": bson.A{}},
	}
	murmur["related"] = bson.A{bson.M{"wordId": whispering["_id"], "type": "related", "note": "To someone.", "tags": bson.A{"written"}}}
	n.store("words", whispering, murmur, hope)
	return phone, phone.connect(), whispering, murmur
}

func TestAConnectorUpdatesOneSideOfAWordLink(t *testing.T) {
	phone, session, whispering, murmur := linkedPair(t)
	backBefore := wordLinksOf(phone, id(murmur))

	callTool[map[string]any](t, session, "japanese_update_word_link", map[string]any{
		"from": id(whispering), "to": id(murmur), "kind": "synonym", "nuanceTags": []string{"spoken", "rare"},
	})

	got := wordLinksOf(phone, id(whispering))
	if len(got) != 2 || got[0].Kind != "synonym" || *got[0].Note != "To oneself." || !reflect.DeepEqual(got[0].NuanceTags, []string{"spoken", "rare"}) {
		t.Errorf("囁く's Word Links = %+v, want 呟く now a synonym tagged spoken, rare, its note kept", got)
	}
	if got[1].Word.Word != "希望" || *got[1].Note != "Unrelated link." {
		t.Errorf("囁く's other Word Link = %+v, want it unchanged", got[1])
	}
	if got := wordLinksOf(phone, id(murmur)); !reflect.DeepEqual(got, backBefore) {
		t.Errorf("呟く's side = %+v, want it left as it was: %+v", got, backBefore)
	}
}

func TestAWordLinkUpdateThatCantBeMadeIsRefused(t *testing.T) {
	phone, session, whispering, murmur := linkedPair(t)
	update := map[string]any{"from": id(whispering), "to": id(murmur)}

	for name, args := range map[string]map[string]any{
		"no Word Link to it": {"from": id(murmur), "to": "6af05d493ea403a78bf5997b", "note": "x"},
		"nothing to change":  update,
		"an unknown kind":    with(update, "kind", "similar"),
		"a camelCase tag":    with(update, "nuanceTags", []string{"moreFormal"}),
	} {
		t.Run(name, func(t *testing.T) { toolFails(t, session, "japanese_update_word_link", args) })
	}
	if got := wordLinksOf(phone, id(whispering)); got[0].Kind != "related" || *got[0].Note != "To oneself." {
		t.Errorf("囁く's side after refused updates = %+v, want it unchanged", got[0])
	}
}

// unlinkedJSON is what japanese_remove_word_link answers: which sides it removed.
type unlinkedJSON struct {
	FromSideRemoved bool `json:"fromSideRemoved"`
	BackSideRemoved bool `json:"backSideRemoved"`
}

func TestAConnectorRemovesBothSidesOfAWordLink(t *testing.T) {
	phone, session, whispering, murmur := linkedPair(t)

	got := callTool[unlinkedJSON](t, session, "japanese_remove_word_link", map[string]any{"from": id(murmur), "to": id(whispering)})

	if !got.FromSideRemoved || !got.BackSideRemoved {
		t.Errorf("sides removed = %+v, want both", got)
	}
	if links := wordLinksOf(phone, id(murmur)); len(links) != 0 {
		t.Errorf("呟く's Word Links = %+v, want none", links)
	}
	page := get[wordJSON](phone, "/api/japanese/words/"+id(whispering))
	if len(page.WordLinks) != 1 || page.WordLinks[0].Word.Word != "希望" || len(page.Hints) != 1 {
		t.Errorf("囁く = %+v, want only its link to 希望 and its Hint left", page)
	}
	toolFails(t, session, "japanese_remove_word_link", map[string]any{"from": id(murmur), "to": id(whispering)})
}

func TestRemovingAOneSidedWordLinkRemovesTheSideThereIs(t *testing.T) {
	phone, session, whispering, _ := linkedPair(t)
	hope := wordLinksOf(phone, id(whispering))[1].Word.ID

	got := callTool[unlinkedJSON](t, session, "japanese_remove_word_link", map[string]any{"from": id(whispering), "to": hope})

	if !got.FromSideRemoved || got.BackSideRemoved {
		t.Errorf("sides removed = %+v, want only 囁く's", got)
	}
}

// hintWrittenJSON is what japanese_add_hint answers.
type hintWrittenJSON struct {
	Written bool `json:"written"`
}

// hintsOf is the Hints on a Word's page.
func hintsOf(phone *browser, wordID string) []hintJSON {
	return get[wordJSON](phone, "/api/japanese/words/"+wordID).Hints
}

func TestAConnectorAddsAHintOnce(t *testing.T) {
	n, phone := signedInBrowser(t)
	expect := word("期待", "きたい", "expectativa", "jlpt_n2")
	n.store("words", expect)
	session := phone.connect()
	hint := map[string]any{"word": id(expect), "text": "天気", "note": "Se espera buen tiempo."}

	first := callTool[hintWrittenJSON](t, session, "japanese_add_hint", hint)
	again := callTool[hintWrittenJSON](t, session, "japanese_add_hint", hint)
	callTool[hintWrittenJSON](t, session, "japanese_add_hint", map[string]any{"word": id(expect), "text": "楽しみ"})

	if !first.Written || again.Written {
		t.Errorf("written: first %v, again %v; want true, then false", first.Written, again.Written)
	}
	want := []hintJSON{{Text: "天気", Note: ptr("Se espera buen tiempo.")}, {Text: "楽しみ"}}
	if got := hintsOf(phone, id(expect)); !reflect.DeepEqual(got, want) {
		t.Errorf("Hints = %+v, want %+v", got, want)
	}
}

func TestAHintThatCantBeAddedIsRefused(t *testing.T) {
	n, phone := signedInBrowser(t)
	expect := word("期待", "きたい", "expectativa", "jlpt_n2")
	n.store("words", expect, word("希望", "きぼう", "esperanza", "basic"))
	session := phone.connect()

	for name, args := range map[string]map[string]any{
		"a stored Word's spelling": {"word": id(expect), "text": "希望"},
		"an unknown Word":          {"word": "6af05d493ea403a78bf5997b", "text": "天気"},
		"blank text":               {"word": id(expect), "text": " "},
	} {
		t.Run(name, func(t *testing.T) { toolFails(t, session, "japanese_add_hint", args) })
	}
	if got := hintsOf(phone, id(expect)); len(got) != 0 {
		t.Errorf("Hints after refused adds = %+v, want none", got)
	}
}

func TestAConnectorRemovesAHint(t *testing.T) {
	phone, session, whispering, _ := linkedPair(t)

	callTool[map[string]any](t, session, "japanese_remove_hint", map[string]any{"word": id(whispering), "text": "小声"})

	page := get[wordJSON](phone, "/api/japanese/words/"+id(whispering))
	if len(page.Hints) != 0 || len(page.WordLinks) != 2 {
		t.Errorf("囁く = %+v, want no Hints and both Word Links left", page)
	}
	toolFails(t, session, "japanese_remove_hint", map[string]any{"word": id(whispering), "text": "小声"})
}
