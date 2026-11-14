# Japanese API

The Japanese Module: searching the Owner's Words and Kanji, the Word and Kanji pages, and the JLPT deck for Anki. Every route is under `/api/japanese/` and, like all of `/api/`, needs a valid Session cookie (see [Core's API](../core/API.md)). Words and Kanji are read-only here: Claude adds and improves Words through the Module's [Tools](#tools), not through this API. Word Requests are the one thing the Owner writes, since they are the Owner's own asks for Claude ([ADR 0006](../../docs/adr/0006-word-requests-through-a-scheduled-task.md)). Bodies are JSON, except the deck file. Errors are a status code with a short plain-text message.

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

### `GET /api/japanese/kanji?kanji={kanji}`

`200`: which of a Drawing's Candidates are stored Kanji, so the Draw screen can show their meaning and number and open them. `kanji` is the Candidates, best guess first, in one string:

```json
[
  { "id": "5f1c2b0e9b1d4c0012ab3530", "kanji": "派", "spanish": "facción", "v1": 1045 },
  { "id": "5f1c2b0e9b1d4c0012ab3531", "kanji": "水", "spanish": "agua", "v1": 137 }
]
```

- The stored Kanji among `kanji`, in the order they first appear there, each once. Asking for 派水氷 with 氷 not stored gives 派 then 水.
- `v1`: its Heisig number in volume 1.
- An empty, blank or missing `kanji` finds nothing: `[]`. Anything that isn't a stored Kanji, such as kana, is left out.

`400`: `kanji` is more than 8 characters, after trimming spaces at either end. A Drawing has at most 8 Candidates.

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

## Kanji Lookup

What Nando finds out about a kanji the Owner hasn't stored (the Glossary's Kanji Lookup), from kanjiapi.dev and jisho.org. A stored Kanji is never looked up outside: it has its own page.

### `GET /api/japanese/lookup/kanji/{kanji}`

The stored Kanji are checked first; kanjiapi.dev and Jisho are only asked, at the same time, when `{kanji}` isn't one of them.

`200`: the Kanji Lookup.

```json
{
  "kanji": "士",
  "meanings": ["gentleman", "scholar", "samurai"],
  "on": ["シ"],
  "kun": ["さむらい"],
  "strokes": 3,
  "grade": 4,
  "joyo": true,
  "jlpt": 1,
  "words": [
    { "word": "武士", "reading": "ぶし", "meanings": ["warrior", "samurai"] },
    { "word": "弁護士", "reading": "べんごし", "meanings": ["lawyer", "attorney"] }
  ]
}
```

- `meanings`: kanjiapi.dev's, in English. The Owner's Spanish keyword exists only for stored Kanji.
- `grade`, `jlpt`: kanjiapi.dev's school grade and JLPT level, or `null` when it has none.
- `joyo`: whether it is jōyō, grade 1 to 8, the same rule as the `japanese_check_joyo` Tool. With no grade it's `false`.
- `words`: up to 5 of Jisho's common words spelled with the kanji, in Jisho's order: the first spelling (or the reading, for one with none), the first reading, and the first sense's English definitions. `[]` when Jisho has none or doesn't answer; the rest doesn't depend on it.

`409`: `{kanji}` is a stored Kanji. Neither outside site is asked. The answer says which Kanji to open instead:

```json
{ "id": "5f1c2b0e9b1d4c0012ab3520" }
```

`400`: `{kanji}` isn't exactly one kanji. Nothing is looked up.

`404`: kanjiapi.dev doesn't know it.

`503`: kanjiapi.dev couldn't be asked, or kept rate-limiting, through its retries: try again later.

## Anki

The Owner studies their Words in Anki. A deck file fills the Owner's note type, Benkyo Note Type V2, documented with its card templates in [`anki/`](anki/README.md); it is the same file benkyo's `GET /words/csv/jlpt` made for the same Words, with two header lines more.

### `GET /api/japanese/anki/jlpt.csv`

`200`: the JLPT deck, as a CSV file to import into Anki, served as `text/csv; charset=utf-8` with `Content-Disposition: attachment; filename="jlpt-<YYYY-MM-DD>.csv"`, dated today in the Owner Time Zone. The name ends in `.csv` so AnkiMobile offers to open it. The web app can read that name from another origin (see [Core's API](../core/API.md)).

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
- `Type`: its parts of speech in the order they are stored, those stored as 1 (or `true`), 4 or 5, joined by `、`: `名` noun, `する` suru, `の形` no_adj, `な形` na_adj, `い形` i_adj, `副` adv, `動` verb, `名形` adj_noun, `副名` adv_noun, `回` counter, followed by `と` or `的` for those forms (`副と`, `な形的`).
- `Notes`: its notes, without the Word Links benkyo once wrote into them as text (`rel: …`, `sinonimo: …`, `antonimo: …`, up to the next `;`) or the separators that leaves at either end.
- `Japanese Sentence`, `Translation Sentence`: its first example sentence and its translation; an example sentence stored without one is skipped for that field.
- `Common`, `JLPT N1`, `Expression`, `Onomatopoeic`, `Transitive`, `Intransitive`: `True` if the Word has that Word Tag (`common`, `jlpt_n1`, `expression`, `onomatopoeic`, `transitive`, `intransitive`), or empty.
- `Usage`: every other Word Tag except `usually_kana`, each as `<span class="tag usage">Label</span>`, the label being the tag in words with its first letter capitalized (`Elderly speech`), or `Not Joyo` for `joyogai`.
- `Related Front`, `Related Back`: its Word Links and Hints, in the order they are stored, as `<ul class="related-list">` with one `<li class="rel rel-<kind>">` each. An entry shows the kind's mark (`=` synonym, `✕` antonym, `~` related or any other kind; an entry stored with no kind is related), the other Word (its kanji spelling, as stored, or its kana if it has none, or `?` if it has neither) or the Hint's text, the other Word's JLPT level (`N3` or `N2` for those Groups, `N1` for the JLPT N1 Group or the `jlpt_n1` Word Tag) and this side's Nuance Tags. `Related Front`, on the definition card's front, masks with `■` each Kanji the entry shares with `Word` and leaves out the notes; `Related Back` shows the entry whole, with this side's note. A Word Link to a Word that is no longer stored is left out. Both are empty for a Word with no Word Links or Hints.

The Usage and Related fields are HTML, written as stored, unescaped.

## Word Requests

A Word Request is the Owner's ask for Claude to add a word or to improve a stored Word ([ADR 0006](../../docs/adr/0006-word-requests-through-a-scheduled-task.md)). The Owner creates, answers, retries and dismisses Word Requests here; a Claude run takes them and records what came of them through the Module's Tools. These routes list them for the Pending strip of Japanese search.

A Word Request:

```json
{
  "id": "6af743080428f77297d429c9",
  "text": "かける",
  "kind": "add",
  "group": "anime",
  "wordId": null,
  "state": "needs_answer",
  "attempts": 1,
  "answer": null,
  "summary": null,
  "suggestions": [],
  "question": "Which かける: to hang, or to call?",
  "choices": ["掛ける (to hang)", "架ける (to call)"]
}
```

- `kind`: `"add"` or `"improve"`.
- `group`: for `"add"`, the Group the Owner found the word in; `null` for `"improve"`.
- `wordId`: for `"improve"`, the Word to improve; once `"done"`, the Word added or improved.
- `state`:
  - `"pending"`: waiting for a Claude run.
  - `"working"`: a Claude run has it.
  - `"done"`: added or improved; `summary` says what Claude did, and `suggestions` what Claude thinks the Owner should do in Claude Code, such as move the Word to another Group.
  - `"needs_answer"`: Claude asks `question`, usually with `choices`.
  - `"failed"`: given up on.
- `attempts`: how many Claude runs have taken it. A Word Request taken 3 times with no outcome fails.
- `answer`: the Owner's answer to `question`, once given.

### `GET /api/japanese/word-requests`

`200`: the Word Requests for the Pending strip, newest first: every open one (`"pending"`, `"working"`, `"needs_answer"`), and every `"done"` or `"failed"` one the Owner hasn't dismissed that finished in the last 7 days.

### `POST /api/japanese/word-requests`

Asks Claude to add a word or to improve a stored Word.

- To add: `{ "kind": "add", "text": "食べた", "group": "gaming" }`. `text` is as the Owner typed it, conjugated or in kana if so; Claude stores the dictionary form. `group` is where the Owner found the word: `literature`, `gaming`, `anime`, `movie`, `miscellaneous` or `jlpt_n1`. The JLPT N2, JLPT N3 and Basic Groups aren't the Owner's to pick: Claude takes them from jisho.org's JLPT level.
- To improve: `{ "kind": "improve", "wordId": "…" }`. The Word Request's `text` is the Word's spelling.

Answers:

- `201`: the new Word Request, `"pending"`.
- `200`: an open Word Request already asks for the same (the same `text` to add, or the same Word to improve, even if its spelling has changed since), and it is returned as it is, with its own `group`, instead of a second one. A finished one doesn't count. This holds when the Owner taps twice at once.
- `400`: blank `text`, a `group` not in the list above, or a `kind` that isn't `"add"` or `"improve"`.
- `404`: no Word with that `wordId`.

### `POST /api/japanese/word-requests/{id}/answer`

Answers Claude's question: `{ "answer": "掛ける (to hang)" }`, one of the `choices` or the Owner's own words. The Word Request goes back to `"pending"` with the `answer` and `attempts` back at 0, as a retry does, and keeps its `question`, so that Claude reads both.

### `POST /api/japanese/word-requests/{id}/retry`

Sends a `"failed"` Word Request back to `"pending"`, with `attempts` back at 0, so that Claude gets 3 fresh tries.

### `POST /api/japanese/word-requests/{id}/dismiss`

Takes a `"done"` or `"failed"` Word Request off the Pending strip, as when the Owner opens it.

These three answer:

- `200`: the Word Request as it is now.
- `400` (answer only): a blank `answer`.
- `404`: no such Word Request.
- `409`: the Word Request isn't in a state this applies to: only a `"needs_answer"` one can be answered, only a `"failed"` one retried, and only a `"done"` or `"failed"` one dismissed.

## Tools

The Module offers these Tools to Connectors, such as Claude, at Core's `/mcp` with a Connector's access token (see [Core's API](../core/API.md#connectors)). Each takes a JSON object of arguments and answers JSON; a Tool that can't do what it's asked fails with a message saying why, for Claude to read. Word and Kanji answers are shaped as the [Word page](#words) and [Kanji page](#kanji) show them, and search answers as the [search](#search). The server enforces the structural rules: both sides of a Word Link, swapped Nuance Tags, stored Groups, one Word per spelling and reading. Judgment, such as which Group or what a note says, is Claude's.

### Look up (`readOnlyHint`)

| Tool | Arguments | Answers |
| --- | --- | --- |
| `japanese_get_word` | `id`, or `word`: a spelling or a reading | `{"words": [Word page]}`, every match, `[]` if none |
| `japanese_search` | `q`; optional `in`: `spelling` or `spanish` | As `/api/japanese/search`. With `in`, only that field of the Words is searched and no Kanji: Word Link candidates sharing a Kanji, or with a similar meaning |
| `japanese_get_kanji` | `kanji`: one character | The Kanji page; fails if it isn't stored |
| `japanese_list_tags` | none | `{"wordTags": [{"tag", "count"}], "nuanceTags": [...]}`, most used first; Nuance Tags on Word Links only |
| `japanese_lookup_jisho` | `keyword` | `{"entries": [...]}`: jisho.org's entries trimmed to `slug`, `isCommon`, `jlpt`, `japanese` (`word`, `reading`) and `senses` (`english`, `partsOfSpeech`, `tags`, `seeAlso`, `antonyms`) |
| `japanese_check_joyo` | `text` | `{"kanji": [{"kanji", "stored", "joyo", "grade"}]}`, each Kanji once. `joyo` is kanjiapi.dev's `grade` 1 to 8; `null` if kanjiapi.dev couldn't tell (it rate-limits), never `false` then |

The server asks jisho.org and kanjiapi.dev itself. It asks kanjiapi.dev at most once a second, and retries a reply with no `grade` up to 3 times.

### Write

| Tool | Arguments | What it does |
| --- | --- | --- |
| `japanese_add_word` | `word`, `hiragana` (left out for a kana Word), `spanish`, `type`, `group`, `tags`, `notes`, `sentences` (`sentence`, `translation`) | Adds a Word and answers `{"id"}`. A spelling with a Kanji needs its reading in kana; a kana Word has none, a reading equal to its spelling counting as none. `type` maps `noun`, `suru`, `no_adj`, `na_adj`, `i_adj`, `adv`, `verb`, `adj_noun`, `adv_noun` or `counter` to a form 1 to 8; `group` must be stored; Word Tags are snake_case. Refused if a Word with the same spelling and reading is stored, by a unique index, so it holds when several chats add at once |
| `japanese_update_word` | `id` and any of the fields above except `group` | Changes only the fields given and answers the Word page. An empty `hiragana` or `notes` removes it. Only the fields given are checked, a new spelling or reading against the other as it will be stored. Refused for a `group`, Word Links, or another Word's spelling and reading |
| `japanese_add_word_link` | `from`, `to`, `kind` (`synonym`, `antonym`, `related`), `note`, `nuanceTags`, optional `noteBack` | Writes both sides. `to` gets `noteBack` (or `note`) and the opposite Nuance Tags: `formal` and `casual`, `spoken` and `written`, `expression` and `part` swap. A side is skipped if that Word already links to the other. Answers `{"fromSideWritten", "backSideWritten"}` |
| `japanese_update_word_link` | `from`, `to`, any of `kind`, `note`, `nuanceTags` | Changes only `from`'s side; the way back is left as it is. Answers the `from` Word page |
| `japanese_remove_word_link` | `from`, `to` | Removes both sides. Answers `{"fromSideRemoved", "backSideRemoved"}`; fails if there's neither |
| `japanese_add_hint` | `word` (an id), `text`, `note` | Adds a Hint, unless the Word has one with that text. Refused for text a stored Word is spelled or read as: that's a Word Link. Answers `{"written"}` |
| `japanese_remove_hint` | `word`, `text` | Removes that Hint and answers the Word page |

### Word Requests

A Claude run works through the Owner's [Word Requests](#word-requests) with these two, following the `word-requests` skill.

| Tool | Arguments | What it does |
| --- | --- | --- |
| `japanese_list_word_requests` | none | Claims up to 3 Word Requests for this run, oldest first, and answers `{"wordRequests": [{"id", "text", "kind", "group", "wordId", "question", "choices", "answer"}]}`, `[]` when none are waiting. A claimed one is `"working"` for 15 minutes, with one more attempt; if no run has resolved it by then, the next list may claim it again, and after its 3rd claim runs out it is `"failed"` instead. Each claim is one write, so runs listing at once never get the same Word Request. One the Owner has answered comes with Claude's earlier `question`, its `choices` and the Owner's `answer` |
| `japanese_resolve_word_request` | `id`, `outcome`, and what that outcome needs | Records what came of a `"working"` Word Request and answers it as the Owner now sees it. `done` needs a `summary` for the Owner and the `wordId` of the stored Word added or improved, with optional `suggestions` (what to do in Claude Code, such as a merge). `needs_answer` needs a `question` and 2 to 4 `choices`. `failed` needs a `summary` saying why. Refused for a Word Request that isn't `"working"` |

### Destructive (`destructiveHint`)

The Owner sets these to ask before each use in the Claude app.

| Tool | Arguments | What it does |
| --- | --- | --- |
| `japanese_delete_word` | `id` | Deletes the Word and every Word Link to it. Answers `{"deleted": id}` |
| `japanese_merge_words` | `keep`, `remove` | Merge first, then set `keep`'s fields with `japanese_update_word`: it can only take `remove`'s spelling and reading once `remove` is gone. Moves `remove`'s Word Links and Hints to `keep`, except those it has and any link between the two. Other Words' sides to `remove` now point at `keep`, or go if they already link to it. Deletes `remove` and answers the `keep` Word page. Each step can run again, so a merge that fails halfway is finished by merging again |
| `japanese_change_group` | `id`, `group` | Moves the Word to that stored Group and answers its page |

