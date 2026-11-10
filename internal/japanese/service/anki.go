package service

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	coremodel "github.com/feriosch/nando-server/internal/core/model"
	coreutils "github.com/feriosch/nando-server/internal/core/utils"
	"github.com/feriosch/nando-server/internal/japanese/model"
)

// AnkiService exports Words as Anki deck files, filling the Owner's note type
// (see internal/japanese/anki/README.md). It asks Core for today's Day, to
// date the file.
type AnkiService struct {
	words model.WordsRepository
	core  coremodel.CoreService
}

// NewAnki returns the AnkiService.
func NewAnki(words model.WordsRepository, core coremodel.CoreService) *AnkiService {
	return &AnkiService{words: words, core: core}
}

// Routes adds the Anki routes to mux, under /api/japanese/anki.
func (s *AnkiService) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/japanese/anki/jlpt.csv", s.getJLPTDeck)
}

// ankiFields are the note type's fields, in the order a deck file fills them.
// Id comes first, so Anki matches a row to the note it already has for the
// Word and updates it.
var ankiFields = []string{
	"Id", "Word", "Pronunciation", "Definition", "Type", "Notes", "Japanese Sentence", "Translation Sentence",
	"Common", "JLPT N1", "Expression", "Onomatopoeic", "Transitive", "Intransitive", "Related Front",
	"Related Back", "Usage",
}

// jlptDeckHeader tells Anki how to read the JLPT deck file and, on AnkiMobile,
// presets the import screen with the Owner's note type and deck.
var jlptDeckHeader = "#separator:Comma\n" +
	"#html:true\n" +
	"#columns:" + strings.Join(ankiFields, ",") + "\n" +
	"#notetype:Benkyo Note Type V2\n" +
	"#deck:JLPT Benkyo Deck V2\n"

func (s *AnkiService) getJLPTDeck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	words, err := s.words.GetManyForJLPTDeck(ctx)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	today, err := s.core.Today(ctx)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	linked, err := s.linkedWords(ctx, words)
	if err != nil {
		coreutils.ServerError(w, err)
		return
	}
	var file strings.Builder
	file.WriteString(jlptDeckHeader)
	for _, word := range words {
		writeCSVRow(&file, ankiNote(word, linked))
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="jlpt-%s.csv"`, today))
	_, _ = w.Write([]byte(file.String()))
}

// linkedWords returns, by ID, every stored Word that one of words has a Word
// Link to.
func (s *AnkiService) linkedWords(ctx context.Context, words []model.Word) (map[string]model.Word, error) {
	var ids []string
	for _, word := range words {
		for _, wordLink := range word.WordLinks {
			ids = append(ids, wordLink.WordID)
		}
	}
	byID := map[string]model.Word{}
	if len(ids) == 0 {
		return byID, nil
	}
	others, err := s.words.GetManyByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, other := range others {
		byID[other.ID] = other
	}
	return byID, nil
}

// ankiNote is word's fields, in ankiFields order. linked holds the Words it
// has Word Links to, by ID.
func ankiNote(word model.Word, linked map[string]model.Word) []string {
	var sentence model.Sentence
	if len(word.Sentences) > 0 {
		sentence = word.Sentences[0]
	}
	spelling, reading := exportedSpelling(word)
	relatedFront, relatedBack := ankiRelated(word, spelling, linked)
	return []string{
		word.ID,
		spelling,
		reading,
		word.Spanish,
		ankiType(word.Type),
		ankiNotes(word.Notes),
		sentence.Sentence,
		sentence.Translation,
		ankiFlag(word.Tags, "common"),
		ankiFlag(word.Tags, "jlpt_n1"),
		ankiFlag(word.Tags, "expression"),
		ankiFlag(word.Tags, "onomatopoeic"),
		ankiFlag(word.Tags, "transitive"),
		ankiFlag(word.Tags, "intransitive"),
		relatedFront,
		relatedBack,
		ankiUsage(word.Tags),
	}
}

// writeCSVRow writes fields to file as one row, quoted as benkyo's pandas
// export quoted them: only a field holding a comma, a quote or a line break is
// quoted, with its quotes doubled.
func writeCSVRow(file *strings.Builder, fields []string) {
	for i, field := range fields {
		if i > 0 {
			file.WriteByte(',')
		}
		if strings.ContainsAny(field, ",\"\r\n") {
			field = `"` + strings.ReplaceAll(field, `"`, `""`) + `"`
		}
		file.WriteString(field)
	}
	file.WriteByte('\n')
}

// exportedSpelling is the Word and Pronunciation fields: a Word usually
// written in kana is exported in kana, with no Pronunciation.
func exportedSpelling(word model.Word) (spelling, reading string) {
	if slices.Contains(word.Tags, "usually_kana") {
		return word.Hiragana, ""
	}
	return word.Word, word.Hiragana
}

// partOfSpeechLabels are how the note writes each part of speech.
var partOfSpeechLabels = map[string]string{
	"noun":     "名",
	"suru":     "する",
	"no_adj":   "の形",
	"na_adj":   "な形",
	"i_adj":    "い形",
	"adv":      "副",
	"verb":     "動",
	"adj_noun": "名形",
	"adv_noun": "副名",
	"counter":  "回",
}

// ankiType is the Type field: the Word's parts of speech in their stored
// order, joined by 、, each followed by と or 的 for those forms. Unknown parts
// of speech and forms are left out.
func ankiType(partsOfSpeech []model.PartOfSpeech) string {
	var labels []string
	for _, partOfSpeech := range partsOfSpeech {
		label, ok := partOfSpeechLabels[partOfSpeech.Key]
		if !ok {
			continue
		}
		switch partOfSpeech.Form {
		case 1:
			labels = append(labels, label)
		case 4:
			labels = append(labels, label+"と")
		case 5:
			labels = append(labels, label+"的")
		}
	}
	return strings.Join(labels, "、")
}

// ankiFlag is the field for a Word Tag that has its own: "True" if the Word
// has it, as benkyo wrote it, or empty.
func ankiFlag(tags []string, tag string) string {
	if slices.Contains(tags, tag) {
		return "True"
	}
	return ""
}

// ownFieldTags are the Word Tags that have their own field, or, for
// usually_kana, change which spelling is exported. Every other Word Tag goes in
// the Usage field.
var ownFieldTags = []string{"common", "jlpt_n1", "expression", "onomatopoeic", "transitive", "intransitive", "usually_kana"}

// usageLabels are the Usage labels that aren't the Word Tag capitalized.
var usageLabels = map[string]string{"joyogai": "Not Joyo"}

// ankiUsage is the Usage field: a label for each Word Tag that has no field
// of its own, in stored order, such as "Elderly speech" for elderly_speech.
func ankiUsage(tags []string) string {
	var usage strings.Builder
	for _, tag := range tags {
		if slices.Contains(ownFieldTags, tag) {
			continue
		}
		label, ok := usageLabels[tag]
		if !ok {
			label = capitalize(strings.ReplaceAll(tag, "_", " "))
		}
		usage.WriteString(`<span class="tag usage">` + label + `</span>`)
	}
	return usage.String()
}

// capitalize is s with its first letter in upper case and the rest in lower
// case, as Python's str.capitalize.
func capitalize(s string) string {
	first, size := utf8.DecodeRuneInString(s)
	if size == 0 {
		return s
	}
	return string(unicode.ToTitle(first)) + strings.ToLower(s[size:])
}

// pythonSpace is what Python's \s matches in a string: the Unicode
// whitespace, which Go's \s, ASCII only, misses.
const pythonSpace = `\t\n\v\f\r \x{1c}-\x{1f}\x{85}\x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}`

var (
	// legacyRelationText is how benkyo wrote Word Links into notes before it
	// had them: "rel: なおさら;", "sinonimo: 更に" or "antonimo: 少し". Each is
	// removed in turn, in this order, as benkyo did.
	legacyRelationText = []*regexp.Regexp{
		regexp.MustCompile(`(?i)rel:[` + pythonSpace + `]*[^;]+(?:;|$)`),
		regexp.MustCompile(`(?i)sinonimo:[` + pythonSpace + `]*[^;]+(?:;|$)`),
		regexp.MustCompile(`(?i)antonimo:[` + pythonSpace + `]*[^;]+(?:;|$)`),
	}
	// notesEdges are separators and spaces left at either end.
	notesEdges = regexp.MustCompile(`^[;,` + pythonSpace + `]+|[;,` + pythonSpace + `]+$`)
)

// ankiNotes is the Notes field: notes without legacy relation text, which
// the Related fields now show, or the separators it leaves at either end.
func ankiNotes(notes string) string {
	for _, text := range legacyRelationText {
		notes = text.ReplaceAllString(notes, "")
	}
	return notesEdges.ReplaceAllString(notes, "")
}

// relatedItem is one of a Word's Word Links or Hints as the Related fields
// show it.
type relatedItem struct {
	position int
	// kind is "synonym", "antonym" or "related".
	kind string
	// word is the other Word's spelling, or the Hint's text.
	word string
	// level is the other Word's JLPT level, such as "N2", or "".
	level      string
	nuanceTags []string
	note       string
}

// kindPrefixes mark each kind of Word Link: = synonym, ✕ antonym, ~ related.
var kindPrefixes = map[string]string{"synonym": "=", "antonym": "✕", "related": "~"}

// groupLevels are the JLPT levels of the JLPT Groups.
var groupLevels = map[string]string{"jlpt_n1": "N1", "jlpt_n2": "N2", "jlpt_n3": "N3"}

// maskChar hides, on the front, a Kanji the other Word shares with this one.
const maskChar = "■"

// ankiRelated is the Related Front and Related Back fields: word's Word Links
// to stored Words and its Hints, in stored order, as lists. The front masks
// the Kanji each other Word shares with spelling, the Word as exported, and
// leaves out the notes; the back shows the other Words whole, with this side's
// notes. Both are empty for a Word with neither, and an empty list for one
// whose Word Links all point at Words no longer stored.
func ankiRelated(word model.Word, spelling string, linked map[string]model.Word) (front, back string) {
	if len(word.WordLinks) == 0 && len(word.Hints) == 0 {
		return "", ""
	}
	var items []relatedItem
	for _, wordLink := range word.WordLinks {
		other, ok := linked[wordLink.WordID]
		if !ok {
			continue
		}
		level := groupLevels[other.Group]
		if level == "" && slices.Contains(other.Tags, "jlpt_n1") {
			// An N1 Word found while doing something else carries only the Word Tag.
			level = "N1"
		}
		items = append(items, relatedItem{
			position:   wordLink.Position,
			kind:       wordLink.Kind,
			word:       cmp.Or(other.Word, other.Hiragana),
			level:      level,
			nuanceTags: wordLink.NuanceTags,
			note:       wordLink.Note,
		})
	}
	for _, hint := range word.Hints {
		if hint.Text == "" {
			continue
		}
		items = append(items, relatedItem{position: hint.Position, kind: hint.Kind, word: hint.Text, nuanceTags: hint.NuanceTags, note: hint.Note})
	}
	slices.SortFunc(items, func(a, b relatedItem) int { return a.position - b.position })

	var frontList, backList strings.Builder
	for _, item := range items {
		frontList.WriteString(relatedItemHTML(item, maskSharedKanji(item.word, spelling), ""))
		backList.WriteString(relatedItemHTML(item, item.word, item.note))
	}
	return `<ul class="related-list">` + frontList.String() + `</ul>`,
		`<ul class="related-list">` + backList.String() + `</ul>`
}

// relatedItemHTML is item as one entry of a Related list, showing its word as
// word and, unless it is "", note.
func relatedItemHTML(item relatedItem, word, note string) string {
	kind := cmp.Or(item.kind, "related")
	prefix, ok := kindPrefixes[kind]
	if !ok {
		prefix = "~"
	}
	var html strings.Builder
	html.WriteString(`<li class="rel rel-` + kind + `"><span class="rel-head">`)
	html.WriteString(`<span class="rel-type">` + prefix + `</span>`)
	html.WriteString(`<span class="rel-word">` + word + `</span>`)
	if item.level != "" {
		html.WriteString(`<span class="rel-level">` + item.level + `</span>`)
	}
	if len(item.nuanceTags) > 0 {
		html.WriteString(`<span class="rel-tags">`)
		for _, tag := range item.nuanceTags {
			html.WriteString(`<span class="rel-tag">` + tag + `</span>`)
		}
		html.WriteString(`</span>`)
	}
	html.WriteString(`</span>`)
	if note != "" {
		html.WriteString(`<span class="rel-note">` + note + `</span>`)
	}
	html.WriteString(`</li>`)
	return html.String()
}

// maskSharedKanji is word with each Kanji that is also in target replaced by
// maskChar.
func maskSharedKanji(word, target string) string {
	var masked strings.Builder
	for _, r := range word {
		if isKanji(r) && strings.ContainsRune(target, r) {
			masked.WriteString(maskChar)
		} else {
			masked.WriteRune(r)
		}
	}
	return masked.String()
}

// isKanji reports whether r is a CJK ideograph, as benkyo counted them: the
// unified block and extensions A and B.
func isKanji(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0x3400 && r <= 0x4DBF) || (r >= 0x20000 && r <= 0x2A6DF)
}
