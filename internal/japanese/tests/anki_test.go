package tests

import (
	"encoding/csv"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ankiFields are the fields of the Owner's Anki note type, Benkyo Note Type V2,
// in the order the deck file fills them (see internal/japanese/anki/README.md).
var ankiFields = []string{
	"Id", "Word", "Pronunciation", "Definition", "Type", "Notes", "Japanese Sentence", "Translation Sentence",
	"Common", "JLPT N1", "Expression", "Onomatopoeic", "Transitive", "Intransitive", "Related Front",
	"Related Back", "Usage",
}

// deckFile is the JLPT deck as Anki reads it: the header lines, then one note
// per row, by field name.
type deckFile struct {
	header []string
	notes  []map[string]string
	// raw is the whole file, as served.
	raw string
}

// exportJLPTDeck downloads the JLPT deck, which must answer 200 as a CSV file.
func exportJLPTDeck(d *browser) deckFile {
	t := d.nando.t
	t.Helper()
	rec := d.must(http.MethodGet, "/api/japanese/anki/jlpt.csv", nil, http.StatusOK)
	raw := rec.Body.String()
	file := deckFile{raw: raw}
	body := raw
	for strings.HasPrefix(body, "#") {
		line, rest, _ := strings.Cut(body, "\n")
		file.header = append(file.header, line)
		body = rest
	}
	reader := csv.NewReader(strings.NewReader(body))
	reader.FieldsPerRecord = len(ankiFields)
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("read the JLPT deck's rows: %v in %q", err, raw)
	}
	for _, row := range rows {
		note := map[string]string{}
		for i, field := range ankiFields {
			note[field] = row[i]
		}
		file.notes = append(file.notes, note)
	}
	return file
}

func TestTheJLPTDeckIsACSVFileThatPresetsAnkiMobilesImport(t *testing.T) {
	n, phone := signedInBrowser(t)
	phone.saveTimeZone("Asia/Tokyo")
	// 16:00 UTC on 2026-11-10 is already 01:00 on 2026-11-11 in Tokyo.
	n.clock.Set(time.Date(2026, 11, 10, 16, 0, 0, 0, time.UTC))
	n.store("words", word("一層", "いっそう", "aún más", "jlpt_n2"))

	rec := phone.must(http.MethodGet, "/api/japanese/anki/jlpt.csv", nil, http.StatusOK)

	if got, want := rec.Header().Get("Content-Type"), "text/csv; charset=utf-8"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
	if got, want := rec.Header().Get("Content-Disposition"), `attachment; filename="jlpt-2026-11-11.csv"`; got != want {
		t.Errorf("Content-Disposition = %q, want %q", got, want)
	}
	file := exportJLPTDeck(phone)
	wantHeader := []string{
		"#separator:Comma",
		"#html:true",
		"#columns:Id,Word,Pronunciation,Definition,Type,Notes,Japanese Sentence,Translation Sentence,Common,JLPT N1,Expression,Onomatopoeic,Transitive,Intransitive,Related Front,Related Back,Usage",
		"#notetype:Benkyo Note Type V2",
		"#deck:JLPT Benkyo Deck V2",
	}
	if !slices.Equal(file.header, wantHeader) {
		t.Errorf("header = %q,\nwant %q", file.header, wantHeader)
	}
	if len(file.notes) != 1 {
		t.Errorf("notes = %d, want 1", len(file.notes))
	}
}

// saveTimeZone sets the Owner Time Zone through Core's settings API.
func (d *browser) saveTimeZone(name string) {
	d.nando.t.Helper()
	d.must(http.MethodPut, "/api/settings", map[string]any{
		"timeZone":        name,
		"heightCm":        nil,
		"weighInReminder": map[string]any{"enabled": false, "weekday": "monday", "time": "08:00"},
	}, http.StatusNoContent)
}

func TestTheJLPTDeckHasTheWordsStudiedForTheJLPT(t *testing.T) {
	n, phone := signedInBrowser(t)
	n3 := word("一層", "いっそう", "aún más", "jlpt_n3")
	n2 := word("足跡", "あしあと", "huella", "jlpt_n2")
	n1 := word("甚だ", "はなはだ", "sumamente", "jlpt_n1")
	// An N1 Word found while doing something else is filed there, with the jlpt_n1 Word Tag.
	n1Literature := word("囁く", "ささやく", "susurrar", "literature")
	n1Literature["tags"] = bson.A{"jlpt_n1"}
	basic := word("もっと", "", "más", "basic")
	gaming := word("回復", "かいふく", "recuperación", "gaming")
	gaming["tags"] = bson.A{"common"}
	n.store("words", n3, n2, n1, n1Literature, basic, gaming)

	file := exportJLPTDeck(phone)

	var got []string
	for _, note := range file.notes {
		got = append(got, note["Id"])
	}
	want := []string{id(n3), id(n2), id(n1), id(n1Literature)}
	if !slices.Equal(got, want) {
		t.Errorf("Ids = %q, want %q", got, want)
	}
}

// ichisou is 一層 as benkyo stored it, under a new ObjectId, in JLPT N2.
func ichisou() bson.M {
	w := word("一層", "いっそう", "aún más, mucho más (formal)", "jlpt_n2")
	w["type"] = bson.D{{Key: "adv", Value: 1}}
	w["tags"] = bson.A{"common"}
	w["notes"] = "Más formal que もっと."
	w["sentences"] = bson.A{
		bson.M{"sentence": "前より一層努力しなければならない。", "translation": "Tengo que esforzarme aún más que antes."},
		bson.M{"sentence": "雨が一層強くなった。", "translation": "La lluvia se hizo aún más fuerte."},
	}
	return w
}

// onlyNote exports the JLPT deck, which must hold exactly one note, and returns it.
func onlyNote(d *browser) map[string]string {
	d.nando.t.Helper()
	file := exportJLPTDeck(d)
	if len(file.notes) != 1 {
		d.nando.t.Fatalf("notes = %d, want 1 in %q", len(file.notes), file.raw)
	}
	return file.notes[0]
}

func TestANoteHasTheWordItsReadingMeaningNotesAndFirstSentence(t *testing.T) {
	n, phone := signedInBrowser(t)
	n.store("words", ichisou())

	note := onlyNote(phone)

	for field, want := range map[string]string{
		"Word":                 "一層",
		"Pronunciation":        "いっそう",
		"Definition":           "aún más, mucho más (formal)",
		"Notes":                "Más formal que もっと.",
		"Japanese Sentence":    "前より一層努力しなければならない。",
		"Translation Sentence": "Tengo que esforzarme aún más que antes.",
	} {
		if note[field] != want {
			t.Errorf("%s = %q, want %q", field, note[field], want)
		}
	}
}

// byID exports the JLPT deck and returns its notes by Id.
func byID(d *browser) map[string]map[string]string {
	d.nando.t.Helper()
	notes := map[string]map[string]string{}
	for _, note := range exportJLPTDeck(d).notes {
		notes[note["Id"]] = note
	}
	return notes
}

func TestANotesTypeListsThePartsOfSpeechInTheOrderTheyAreStored(t *testing.T) {
	n, phone := signedInBrowser(t)
	cases := map[string]bson.D{
		"名、する": {{Key: "noun", Value: 1}, {Key: "suru", Value: 1}},
		"する、名": {{Key: "suru", Value: 1}, {Key: "noun", Value: 1}},
		"副と":   {{Key: "adv", Value: 4}},
		"な形的":  {{Key: "na_adj", Value: 5}},
		"動":    {{Key: "noun", Value: 0}, {Key: "unknown", Value: 1}, {Key: "verb", Value: 1}},
		"名形、回": {{Key: "adj_noun", Value: int64(1)}, {Key: "counter", Value: 1.0}},
		"":     {},
	}
	ids := map[string]string{}
	for want, partsOfSpeech := range cases {
		w := word("一層", "いっそう", "aún más", "jlpt_n2")
		w["type"] = partsOfSpeech
		n.store("words", w)
		ids[want] = id(w)
	}
	untyped := word("一層", "いっそう", "aún más", "jlpt_n2")
	delete(untyped, "type")
	n.store("words", untyped)
	ids["(none)"] = id(untyped)

	notes := byID(phone)

	for want, wordID := range ids {
		if want == "(none)" {
			want = ""
		}
		if got := notes[wordID]["Type"]; got != want {
			t.Errorf("Type = %q, want %q", got, want)
		}
	}
}

func TestANoteMarksTheWordTagsThatHaveTheirOwnField(t *testing.T) {
	n, phone := signedInBrowser(t)
	tagged := ichisou()
	tagged["tags"] = bson.A{"common", "jlpt_n1", "expression", "onomatopoeic", "transitive", "intransitive"}
	untagged := ichisou()
	untagged["tags"] = bson.A{}
	noTags := ichisou()
	delete(noTags, "tags")
	n.store("words", tagged, untagged, noTags)

	notes := byID(phone)

	for _, field := range []string{"Common", "JLPT N1", "Expression", "Onomatopoeic", "Transitive", "Intransitive"} {
		if got := notes[id(tagged)][field]; got != "True" {
			t.Errorf("%s with its Word Tag = %q, want True", field, got)
		}
		for _, w := range []bson.M{untagged, noTags} {
			if got := notes[id(w)][field]; got != "" {
				t.Errorf("%s without its Word Tag = %q, want empty", field, got)
			}
		}
	}
}

func TestANotesUsageHasEveryOtherWordTag(t *testing.T) {
	n, phone := signedInBrowser(t)
	usage := ichisou()
	usage["tags"] = bson.A{"common", "jlpt_n1", "usually_kana", "transitive", "joyogai", "elderly_speech", "rare"}
	onlyOwnFields := ichisou()
	onlyOwnFields["tags"] = bson.A{"common"}
	n.store("words", usage, onlyOwnFields)

	notes := byID(phone)

	want := `<span class="tag usage">Not Joyo</span>` +
		`<span class="tag usage">Elderly speech</span>` +
		`<span class="tag usage">Rare</span>`
	if got := notes[id(usage)]["Usage"]; got != want {
		t.Errorf("Usage = %q, want %q", got, want)
	}
	if got := notes[id(onlyOwnFields)]["Usage"]; got != "" {
		t.Errorf("Usage with only Word Tags that have their own field = %q, want empty", got)
	}
}

func TestANotesNotesLeaveOutLegacyRelationText(t *testing.T) {
	n, phone := signedInBrowser(t)
	cases := map[string]string{
		"rel: なおさら; Más formal que もっと": "Más formal que もっと",
		"Uso escrito; sinonimo: 更に":     "Uso escrito",
		"ANTONIMO: 少し":                  "",
		"rel: もっと":                      "",
		"sinonimo: a rel: b; c":         "",
		"Más formal que もっと.":           "Más formal que もっと.",
		"":                              "",
	}
	ids := map[string]string{}
	for notes := range cases {
		w := ichisou()
		w["notes"] = notes
		n.store("words", w)
		ids[notes] = id(w)
	}
	withoutNotes := ichisou()
	delete(withoutNotes, "notes")
	n.store("words", withoutNotes)

	notes := byID(phone)

	for stored, want := range cases {
		if got := notes[ids[stored]]["Notes"]; got != want {
			t.Errorf("Notes for %q = %q, want %q", stored, got, want)
		}
	}
	if got := notes[id(withoutNotes)]["Notes"]; got != "" {
		t.Errorf("Notes for a Word without notes = %q, want empty", got)
	}
}

func TestAUsuallyKanaWordIsExportedInKana(t *testing.T) {
	n, phone := signedInBrowser(t)
	hardly := word("殆ど", "ほとんど", "casi", "jlpt_n2")
	hardly["tags"] = bson.A{"common", "usually_kana"}
	n.store("words", hardly)

	note := onlyNote(phone)

	if note["Word"] != "ほとんど" || note["Pronunciation"] != "" {
		t.Errorf("Word, Pronunciation = %q, %q; want ほとんど and empty", note["Word"], note["Pronunciation"])
	}
}

func TestAWordLinkMasksTheSharedKanjiOnTheFrontAndShowsTheNoteOnTheBack(t *testing.T) {
	n, phone := signedInBrowser(t)
	more := ichisou()
	moreStill := word("一段と", "いちだんと", "aún más", "jlpt_n2")
	// Each side of the Word Link has its own note.
	more["related"] = bson.A{bson.M{"wordId": moreStill["_id"], "type": "related", "note": "en mayor grado"}}
	moreStill["related"] = bson.A{bson.M{"wordId": more["_id"], "type": "related", "note": "más formal"}}
	n.store("words", more, moreStill)

	notes := byID(phone)

	for _, c := range []struct {
		word        bson.M
		front, back string
	}{
		{
			more,
			`<ul class="related-list"><li class="rel rel-related"><span class="rel-head"><span class="rel-type">~</span><span class="rel-word">■段と</span><span class="rel-level">N2</span></span></li></ul>`,
			`<ul class="related-list"><li class="rel rel-related"><span class="rel-head"><span class="rel-type">~</span><span class="rel-word">一段と</span><span class="rel-level">N2</span></span><span class="rel-note">en mayor grado</span></li></ul>`,
		},
		{
			moreStill,
			`<ul class="related-list"><li class="rel rel-related"><span class="rel-head"><span class="rel-type">~</span><span class="rel-word">■層</span><span class="rel-level">N2</span></span></li></ul>`,
			`<ul class="related-list"><li class="rel rel-related"><span class="rel-head"><span class="rel-type">~</span><span class="rel-word">一層</span><span class="rel-level">N2</span></span><span class="rel-note">más formal</span></li></ul>`,
		},
	} {
		note := notes[id(c.word)]
		if note["Related Front"] != c.front {
			t.Errorf("%s: Related Front = %q,\nwant %q", note["Word"], note["Related Front"], c.front)
		}
		if note["Related Back"] != c.back {
			t.Errorf("%s: Related Back = %q,\nwant %q", note["Word"], note["Related Back"], c.back)
		}
	}
}

// relatedItems splits a Related field into its entries.
func relatedItems(field string) []string {
	field = strings.TrimPrefix(field, `<ul class="related-list">`)
	field = strings.TrimSuffix(field, `</ul>`)
	items := strings.SplitAfter(field, "</li>")
	return items[:len(items)-1]
}

func TestEachKindOfWordLinkHasItsOwnMark(t *testing.T) {
	n, phone := signedInBrowser(t)
	more := word("もっと", "", "más", "basic")
	little := word("少し", "すこし", "un poco", "basic")
	w := ichisou()
	w["related"] = bson.A{
		bson.M{"wordId": more["_id"], "type": "synonym", "note": ""},
		bson.M{"wordId": little["_id"], "type": "antonym", "note": ""},
		bson.M{"word": "層", "type": "related", "note": ""},
		bson.M{"word": "尚更", "type": "synonym", "note": ""},
	}
	n.store("words", w, more, little)

	back := relatedItems(onlyNote(phone)["Related Back"])

	want := []string{
		`<li class="rel rel-synonym"><span class="rel-head"><span class="rel-type">=</span><span class="rel-word">もっと</span></span></li>`,
		`<li class="rel rel-antonym"><span class="rel-head"><span class="rel-type">✕</span><span class="rel-word">少し</span></span></li>`,
		`<li class="rel rel-related"><span class="rel-head"><span class="rel-type">~</span><span class="rel-word">層</span></span></li>`,
		`<li class="rel rel-synonym"><span class="rel-head"><span class="rel-type">=</span><span class="rel-word">尚更</span></span></li>`,
	}
	if !slices.Equal(back, want) {
		t.Errorf("Related Back = %q,\nwant %q", back, want)
	}
}

func TestWordLinksAndHintsKeepTheirStoredOrder(t *testing.T) {
	n, phone := signedInBrowser(t)
	more := word("もっと", "", "más", "basic")
	further := word("更に", "さらに", "además", "jlpt_n3")
	w := ichisou()
	w["related"] = bson.A{
		bson.M{"wordId": further["_id"], "type": "related"},
		bson.M{"word": "層", "type": "related"},
		bson.M{"wordId": more["_id"], "type": "related"},
	}
	n.store("words", w, more, further)

	back := onlyNoteOf(phone, w)["Related Back"]

	if !(strings.Index(back, "更に") < strings.Index(back, "層") && strings.Index(back, "層") < strings.Index(back, "もっと")) {
		t.Errorf("Related Back = %q, want 更に, 層, もっと in that order", back)
	}
}

// onlyNoteOf exports the JLPT deck and returns w's note.
func onlyNoteOf(d *browser, w bson.M) map[string]string {
	d.nando.t.Helper()
	note, ok := byID(d)[id(w)]
	if !ok {
		d.nando.t.Fatalf("no note for %s", w["word"])
	}
	return note
}

func TestNuanceTagsShowOnBothSidesOfTheCard(t *testing.T) {
	n, phone := signedInBrowser(t)
	more := word("もっと", "", "más", "basic")
	w := ichisou()
	w["related"] = bson.A{bson.M{"wordId": more["_id"], "type": "synonym", "note": "x", "tags": bson.A{"casual", "spoken"}}}
	n.store("words", w, more)

	note := onlyNoteOf(phone, w)

	want := `<span class="rel-tags"><span class="rel-tag">casual</span><span class="rel-tag">spoken</span></span>`
	for _, field := range []string{"Related Front", "Related Back"} {
		if !strings.Contains(note[field], want) {
			t.Errorf("%s = %q, want it to hold %q", field, note[field], want)
		}
	}
}

func TestAWordLinkShowsTheOtherWordsJLPTLevel(t *testing.T) {
	n, phone := signedInBrowser(t)
	n1Group := word("甚だ", "はなはだ", "sumamente", "jlpt_n1")
	n1Tag := word("囁く", "ささやく", "susurrar", "literature")
	n1Tag["tags"] = bson.A{"jlpt_n1"}
	n3 := word("更に", "さらに", "además", "jlpt_n3")
	basic := word("もっと", "", "más", "basic")
	w := ichisou()
	w["related"] = bson.A{}
	for _, other := range []bson.M{n1Group, n1Tag, n3, basic} {
		w["related"] = append(w["related"].(bson.A), bson.M{"wordId": other["_id"], "type": "related"})
	}
	n.store("words", w, n1Group, n1Tag, n3, basic)

	back := relatedItems(onlyNoteOf(phone, w)["Related Back"])

	if len(back) != 4 {
		t.Fatalf("Related Back = %q, want 4 entries", back)
	}
	for i, want := range []string{"N1", "N1", "N3", ""} {
		hasLevel := strings.Contains(back[i], "rel-level")
		if want == "" && hasLevel || want != "" && !strings.Contains(back[i], `<span class="rel-level">`+want+`</span>`) {
			t.Errorf("entry %d = %q, want level %q", i, back[i], want)
		}
	}
}

func TestAWordLinkToAWordWithoutKanjiShowsItsKana(t *testing.T) {
	n, phone := signedInBrowser(t)
	more := word("", "もっと", "más", "basic")
	w := ichisou()
	w["related"] = bson.A{bson.M{"wordId": more["_id"], "type": "related"}}
	n.store("words", w, more)

	back := onlyNoteOf(phone, w)["Related Back"]

	if !strings.Contains(back, `<span class="rel-word">もっと</span>`) {
		t.Errorf("Related Back = %q, want もっと", back)
	}
}

func TestAHintIsMaskedButHasNoLevel(t *testing.T) {
	n, phone := signedInBrowser(t)
	weather := word("天気", "てんき", "tiempo", "jlpt_n3")
	weather["related"] = bson.A{bson.M{"word": "天候", "type": "synonym", "note": "formal"}}
	n.store("words", weather)

	note := onlyNote(phone)

	if want := `<span class="rel-word">■候</span>`; !strings.Contains(note["Related Front"], want) {
		t.Errorf("Related Front = %q, want it to hold %q", note["Related Front"], want)
	}
	if want := `<span class="rel-word">天候</span>`; !strings.Contains(note["Related Back"], want) {
		t.Errorf("Related Back = %q, want it to hold %q", note["Related Back"], want)
	}
	if strings.Contains(note["Related Back"], "rel-level") {
		t.Errorf("Related Back = %q, want no level for a Hint", note["Related Back"])
	}
}

func TestAWordLinkToAWordNoLongerStoredIsLeftOut(t *testing.T) {
	n, phone := signedInBrowser(t)
	dangling := ichisou()
	dangling["related"] = bson.A{bson.M{"wordId": bson.NewObjectID(), "type": "related"}}
	unlinked := ichisou()
	unlinked["related"] = bson.A{}
	n.store("words", dangling, unlinked)

	notes := byID(phone)

	for _, c := range []struct {
		word bson.M
		want string
	}{
		// benkyo wrote an empty list when every Word Link was left out.
		{dangling, `<ul class="related-list"></ul>`},
		{unlinked, ""},
	} {
		note := notes[id(c.word)]
		if note["Related Front"] != c.want || note["Related Back"] != c.want {
			t.Errorf("Related Front, Related Back = %q, %q; want %q", note["Related Front"], note["Related Back"], c.want)
		}
	}
}

func TestAUsuallyKanaWordMasksNothing(t *testing.T) {
	n, phone := signedInBrowser(t)
	hardly := word("殆ど", "ほとんど", "casi", "jlpt_n2")
	hardly["tags"] = bson.A{"usually_kana"}
	almost := word("殆", "ほとほと", "totalmente", "jlpt_n1")
	hardly["related"] = bson.A{bson.M{"wordId": almost["_id"], "type": "related"}}
	n.store("words", hardly, almost)

	front := onlyNoteOf(phone, hardly)["Related Front"]

	// The front shows the Word in kana, so the other Word's Kanji give nothing away.
	if want := `<span class="rel-word">殆</span>`; !strings.Contains(front, want) {
		t.Errorf("Related Front = %q, want it to hold %q", front, want)
	}
}

func TestCommasQuotesAndLineBreaksSurvive(t *testing.T) {
	n, phone := signedInBrowser(t)
	w := ichisou()
	w["spanish"] = `a, "b"`
	w["notes"] = "línea 1\nlínea 2"
	n.store("words", w)

	note := onlyNote(phone)

	if note["Definition"] != `a, "b"` || note["Notes"] != "línea 1\nlínea 2" {
		t.Errorf("Definition, Notes = %q, %q; want %q, %q", note["Definition"], note["Notes"], `a, "b"`, "línea 1\nlínea 2")
	}
}

func TestAWordWithoutSentencesTagsOrNotesStillFillsEveryField(t *testing.T) {
	n, phone := signedInBrowser(t)
	bare := word("一層", "いっそう", "aún más", "jlpt_n2")
	delete(bare, "sentences")
	delete(bare, "type")
	n.store("words", bare)

	note := onlyNote(phone)

	for _, field := range ankiFields[4:] {
		if note[field] != "" {
			t.Errorf("%s = %q, want empty", field, note[field])
		}
	}
}
