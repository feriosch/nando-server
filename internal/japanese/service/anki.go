package service

import (
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
	var file strings.Builder
	file.WriteString(jlptDeckHeader)
	for _, word := range words {
		writeCSVRow(&file, ankiRow(word))
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="jlpt-%s.csv"`, today))
	_, _ = w.Write([]byte(file.String()))
}

// flagTags are the Word Tags that have a field of their own, in field order:
// Common, JLPT N1, Expression, Onomatopoeic, Transitive and Intransitive.
var flagTags = []string{"common", "jlpt_n1", "expression", "onomatopoeic", "transitive", "intransitive"}

// ankiRow is word's row of the deck file: its fields, in ankiFields order.
func ankiRow(word model.AnkiWord) []string {
	spelling, reading := exportedSpelling(word)
	relatedFront, relatedBack := ankiRelated(word.Related, spelling)
	row := []string{
		word.ID,
		spelling,
		reading,
		word.Spanish,
		ankiType(word.PartsOfSpeech),
		ankiNotes(word.Notes),
		word.Sentence,
		word.Translation,
	}
	for _, tag := range flagTags {
		row = append(row, ankiFlag(word.Tags, tag))
	}
	return append(row, relatedFront, relatedBack, ankiUsage(word.Tags))
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
// written in kana is exported in kana, with no Pronunciation. A kana Word
// has no reading, so it is exported as spelled.
func exportedSpelling(word model.AnkiWord) (spelling, reading string) {
	if slices.Contains(word.Tags, "usually_kana") && word.Hiragana != "" {
		return word.Hiragana, ""
	}
	return word.Word, word.Hiragana
}

// partOfSpeechLabels are how the deck writes each part of speech.
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

// formSuffixes follow a part of speech's label for each Form.
var formSuffixes = map[model.Form]string{model.PlainForm: "", model.ToForm: "と", model.TekiForm: "的"}

// ankiType is the Type field: the Word's parts of speech in their stored
// order, joined by 、, each followed by と or 的 for those forms. Unknown parts
// of speech and parts of speech with no form are left out.
func ankiType(partsOfSpeech []model.PartOfSpeech) string {
	var labels []string
	for _, partOfSpeech := range partsOfSpeech {
		label, known := partOfSpeechLabels[partOfSpeech.Key]
		suffix, hasForm := formSuffixes[partOfSpeech.Form]
		if known && hasForm {
			labels = append(labels, label+suffix)
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

// usageLabels are the Usage labels that aren't the Word Tag capitalized.
var usageLabels = map[string]string{"joyogai": "Not Joyo"}

// ankiUsage is the Usage field: a label for each Word Tag that has no field
// of its own, in stored order, such as "Elderly speech" for elderly_speech.
// usually_kana has none either, but changes which spelling is exported instead.
func ankiUsage(tags []string) string {
	var usage strings.Builder
	for _, tag := range tags {
		if slices.Contains(flagTags, tag) || tag == "usually_kana" {
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
	// legacyWordLinkText is how benkyo wrote Word Links into notes before it
	// stored them: "rel: なおさら;", "sinonimo: 更に" or "antonimo: 少し". Each
	// is removed in turn, in this order, as benkyo did.
	legacyWordLinkText = []*regexp.Regexp{
		regexp.MustCompile(`(?i)rel:[` + pythonSpace + `]*[^;]+(?:;|$)`),
		regexp.MustCompile(`(?i)sinonimo:[` + pythonSpace + `]*[^;]+(?:;|$)`),
		regexp.MustCompile(`(?i)antonimo:[` + pythonSpace + `]*[^;]+(?:;|$)`),
	}
	// notesEdges are separators and spaces left at either end.
	notesEdges = regexp.MustCompile(`^[;,` + pythonSpace + `]+|[;,` + pythonSpace + `]+$`)
)

// ankiNotes is the Notes field: notes without legacy Word Link text, which
// the Related fields now show, or the separators it leaves at either end.
func ankiNotes(notes string) string {
	for _, text := range legacyWordLinkText {
		notes = text.ReplaceAllString(notes, "")
	}
	return notesEdges.ReplaceAllString(notes, "")
}

// kindPrefixes mark each kind of Word Link: = synonym, ✕ antonym, ~ related.
// Any other stored kind is marked ~.
var kindPrefixes = map[string]string{"synonym": "=", "antonym": "✕", "related": "~"}

// maskChar hides, on the front, a Kanji the other Word shares with this one.
const maskChar = "■"

// ankiRelated is the Related Front and Related Back fields: a Word's Word
// Links to stored Words and its Hints, in stored order, as lists. The front
// masks the Kanji each shares with spelling, the Word as exported, and leaves
// out the notes; the back shows them whole, with this side's notes. Both are
// empty for a Word with no Word Links or Hints stored, and an empty list for
// one whose Word Links all point at Words no longer stored.
func ankiRelated(related []model.AnkiRelated, spelling string) (front, back string) {
	if len(related) == 0 {
		return "", ""
	}
	var frontList, backList strings.Builder
	for _, item := range related {
		word, level := item.Text, ""
		if item.Other != nil {
			word, level = item.Other.Spelling, item.Other.JLPTLevel()
		}
		if word == "" {
			continue
		}
		frontList.WriteString(relatedItemHTML(item, maskSharedKanji(word, spelling), level, ""))
		backList.WriteString(relatedItemHTML(item, word, level, item.Note))
	}
	return `<ul class="related-list">` + frontList.String() + `</ul>`,
		`<ul class="related-list">` + backList.String() + `</ul>`
}

// relatedItemHTML is item as one entry of a Related list, showing it as word,
// with level and note unless they are "".
func relatedItemHTML(item model.AnkiRelated, word, level, note string) string {
	prefix, ok := kindPrefixes[item.Kind]
	if !ok {
		prefix = "~"
	}
	var html strings.Builder
	html.WriteString(`<li class="rel rel-` + item.Kind + `"><span class="rel-head">`)
	html.WriteString(`<span class="rel-type">` + prefix + `</span>`)
	html.WriteString(`<span class="rel-word">` + word + `</span>`)
	if level != "" {
		html.WriteString(`<span class="rel-level">` + level + `</span>`)
	}
	if len(item.NuanceTags) > 0 {
		html.WriteString(`<span class="rel-tags">`)
		for _, tag := range item.NuanceTags {
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
