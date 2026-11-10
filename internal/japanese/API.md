# Japanese API

The Japanese Module: searching the Owner's Words and Kanji, the Word and Kanji pages, and the JLPT deck for Anki. Every route is under `/api/japanese/` and, like all of `/api/`, needs a valid Session cookie (see [Core's API](../core/API.md)). The routes are read-only: Words and Kanji are added and improved by Claude, not through this API. Bodies are JSON, except the deck file. Errors are a status code with a short plain-text message.

Ids are the 24-character hex ObjectIds the Words and Kanji have had since benkyo, which are also Anki's `Id`. Word Tags and Nuance Tags are returned as stored, in snake_case (`usually_kana`, `jlpt_n1`); the web app writes them for display. Optional strings are `null` when not set, and lists are `[]`, never `null`.

## Rows

A search, a Word Link and a Kanji page list Words and Kanji as rows: what a result row shows, and the `id` to open it.

A Word row:

```json
{
  "id": "5f1c2a7e9b1d4c0012ab34cd",
  "word": "足跡",
  "hiragana": "あしあと",
  "spanish": "huella, pisada",
  "group": "jlpt_n2"
}
```

- `word`: the kanji spelling, or the kana if the Word has none.
- `hiragana`: the reading, or `null` when `word` is already kana.
- `group`: the key of the Word's Group: `jlpt_n1`, `jlpt_n2`, `jlpt_n3`, `basic`, `literature`, `gaming`, `anime`, `movie` or `miscellaneous`.

A Kanji row:

```json
{ "id": "5f1c2b0e9b1d4c0012ab3501", "kanji": "足", "spanish": "pie" }
```

- `spanish`: its meaning, which is also its Heisig keyword.

## Search

### `GET /api/japanese/search?q={q}`

`200`: Words and Kanji that match `q`, Words first:

```json
{ "words": [ /* Word rows */ ], "kanji": [ /* Kanji rows */ ] }
```

- A Word matches if its `word`, `hiragana` or `spanish` contains `q`. Searching 足 finds 足 and 足跡; searching あし finds 足 (あし) and 明日 (あした).
- A Kanji matches if its character is in `q`, or its `spanish` contains `q`. Searching 足跡 finds the Kanji 足 and 跡.
- Spanish ignores case and accents: `corazon`, `Corazón` and `CORAZON` all find `corazón`. There's no romaji search.
- Within each list, exact matches (the whole `word`, `hiragana` or `spanish` is `q`, or the Kanji's character is in `q`) come first, then the rest, shorter before longer (by the length of whatever matched). Kanji found by character come in the order they appear in `q`, and Kanji of equal rank by their Heisig number.
- At most 50 Words and 20 Kanji.
- An empty or blank `q` finds nothing: `{ "words": [], "kanji": [] }`.
- An accent or dakuten sent as a separate combining mark matches like the composed letter (`corazo\u0301n` finds `corazón`).

`400`: `q` is longer than 100 characters.

## Words

A Word is a Japanese word or Expression the Owner is learning.

### `GET /api/japanese/words/{id}`

`200`: everything about the Word.

```json
{
  "id": "5f1c2a7e9b1d4c0012ab34ce",
  "word": "期待",
  "hiragana": "きたい",
  "spanish": "expectativa",
  "type": { "noun": 1, "suru": 1 },
  "group": {
    "key": "jlpt_n2",
    "name": "JLPT N2",
    "imageUrl": "https://storage.googleapis.com/benkyo/jlpt_n2.png"
  },
  "tags": ["common"],
  "notes": null,
  "sentences": [
    { "sentence": "期待に胸を膨らませる。", "translation": "Tener el corazón lleno de ilusión." }
  ],
  "wordLinks": [
    {
      "kind": "synonym",
      "note": "Más intenso y escrito.",
      "nuanceTags": ["written", "literary"],
      "word": {
        "id": "5f1c2a7e9b1d4c0012ab34cf",
        "word": "待ち望む",
        "hiragana": "まちのぞむ",
        "spanish": "esperar con ansia",
        "group": "jlpt_n1"
      }
    }
  ],
  "hints": [{ "text": "楽しみ", "note": null }],
  "kanji": [
    { "id": "5f1c2b0e9b1d4c0012ab3510", "kanji": "期", "spanish": "período" },
    { "id": "5f1c2b0e9b1d4c0012ab3511", "kanji": "待", "spanish": "esperar" }
  ]
}
```

- `type`: its parts of speech as benkyo stored them: each key (`noun`, `suru`, `no_adj`, `na_adj`, `i_adj`, `adv`, `verb`, `adj_noun`, `adv_noun`, `counter`) with `1`, or `4` for its と form, or `5` for its 的 form.
- `group`: its Group, with the Group's display `name` and picture (`imageUrl`).
- `tags`: its Word Tags.
- `notes`: free text, or `null`.
- `sentences`: example sentences, each with its Spanish translation.
- `wordLinks`: this Word's sides of its Word Links, in the order they were made. `kind` is `synonym`, `antonym` or `related`. `note` and `nuanceTags` are this side's: about the other Word, and how it differs from this one. `word` is the other Word as a row, to show and open it. A Word Link to a Word that is no longer stored is left out.
- `hints`: its Hints, each the `text` of an everyday word that isn't stored as a Word, with an optional `note`. A Hint has no other side and can't be opened.
- `kanji`: the Kanji in its spelling that are stored, as rows, in the order they first appear in `word`.

`404`: there's no such Word.

## Kanji

A Kanji is one character the Owner studies, numbered by its place in Heisig's Remembering the Kanji.

### `GET /api/japanese/kanji/{id}`

`200`: the Kanji, its Components, and the Words spelled with it.

```json
{
  "id": "5f1c2b0e9b1d4c0012ab3520",
  "kanji": "海",
  "spanish": "mar",
  "on": "カイ",
  "kun": "うみ",
  "v1": 491,
  "v2": null,
  "story": "Las gotas de agua de cada río acaban en el mar.",
  "components": [
    { "name": "gotas de agua", "kanji": null },
    { "name": "cada", "kanji": { "id": "5f1c2b0e9b1d4c0012ab3521", "kanji": "毎", "spanish": "cada" } }
  ],
  "words": [ /* Word rows */ ]
}
```

- `on`, `kun`: its readings, as stored, or `null`.
- `v1`: its Heisig number in volume 1. `v2`: in volume 2, or `null`.
- `story`: the story to remember it by, or `null`.
- `components`: its Components in order, each by its keyword (`name`). `kanji` is the Kanji the Component is, as a row to open it, or `null` for an irregular shape. A keyword stored as an irregular shape is that shape, even if a Kanji has the same keyword, as benkyo worked out radicals; any other keyword is the Kanji with that `spanish` if one is stored.
- `words`: up to 50 Words whose `word` contains the Kanji, as rows, shorter first.

`404`: there's no such Kanji.

## Anki

The Owner studies their Words in Anki. A deck file fills the Owner's note type, Benkyo Note Type V2, documented with its card templates in [`anki/`](anki/README.md); it is the same file benkyo's `GET /words/csv/jlpt` made for the same Words, with two header lines more.

### `GET /api/japanese/anki/jlpt.csv`

`200`: the JLPT deck, as a CSV file to import into Anki, served as `text/csv; charset=utf-8` with `Content-Disposition: attachment; filename="jlpt-<YYYY-MM-DD>.csv"`, dated today in the Owner Time Zone. The name ends in `.csv` so AnkiMobile offers to open it.

It starts with header lines that set up Anki's import screen:

```
#separator:Comma
#html:true
#columns:Id,Word,Pronunciation,Definition,Type,Notes,Japanese Sentence,Translation Sentence,Common,JLPT N1,Expression,Onomatopoeic,Transitive,Intransitive,Related Front,Related Back,Usage
#notetype:Benkyo Note Type V2
#deck:JLPT Benkyo Deck V2
```

Then one row per Word studied for the JLPT: every Word in the JLPT N3, N2 and N1 Groups, and any other Word with the `jlpt_n1` Word Tag, oldest first. A field holding a comma, a quote or a line break is quoted, with its quotes doubled; lines end in `\n`. With no such Words, the file is just the header. The fields:

- `Id`: the Word's id, so re-importing updates the Word's note.
- `Word`, `Pronunciation`: its spelling and reading. A Word with the `usually_kana` Word Tag is exported in kana: its reading as `Word`, and no `Pronunciation`.
- `Definition`: its Spanish meaning.
- `Type`: its parts of speech in the order they are stored, joined by `、`: `名` noun, `する` suru, `の形` no_adj, `な形` na_adj, `い形` i_adj, `副` adv, `動` verb, `名形` adj_noun, `副名` adv_noun, `回` counter, followed by `と` or `的` for those forms (`副と`, `な形的`).
- `Notes`: its notes, without the relation text benkyo once wrote into them (`rel: …`, `sinonimo: …`, `antonimo: …`, up to the next `;`) or the separators that leaves at either end.
- `Japanese Sentence`, `Translation Sentence`: its first example sentence and its translation.
- `Common`, `JLPT N1`, `Expression`, `Onomatopoeic`, `Transitive`, `Intransitive`: `True` if the Word has that Word Tag (`common`, `jlpt_n1`, `expression`, `onomatopoeic`, `transitive`, `intransitive`), or empty.
- `Usage`: every other Word Tag except `usually_kana`, each as `<span class="tag usage">Label</span>`, the label being the tag in words with its first letter capitalized (`Elderly speech`), or `Not Joyo` for `joyogai`.
- `Related Front`, `Related Back`: its Word Links and Hints, in the order they are stored, as `<ul class="related-list">` with one `<li class="rel rel-<kind>">` each. An entry shows the kind's mark (`=` synonym, `✕` antonym, `~` related), the other Word (its kanji spelling, as stored, or its kana if it has none) or the Hint's text, the other Word's JLPT level (`N3` or `N2` for those Groups, `N1` for the JLPT N1 Group or the `jlpt_n1` Word Tag) and this side's Nuance Tags. `Related Front`, on the definition card's front, masks with `■` each Kanji the entry shares with `Word` and leaves out the notes; `Related Back` shows the entry whole, with this side's note. A Word Link to a Word that is no longer stored is left out. Both are empty for a Word with no Word Links or Hints.

The Usage and Related fields are HTML, written as stored, unescaped.
