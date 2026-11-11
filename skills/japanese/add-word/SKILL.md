---
name: add-word
description: >-
  Add a Japanese word or expression to the Owner's Words in Nando, or review and improve it if it is already stored. Use whenever the Owner says they found, learned, saw or want to add or save a Japanese word or expression (e.g. "new word: 囁く", "add 首にされる, I'm reading", "/add-word 懸賞"), even if they don't mention Nando.
---

# Add a Word to Nando

Input: one or more Japanese words or Expressions, plus what the Owner tells you: what they were doing when they found it (reading, gaming, watching anime, a movie, studying for JLPT N1…), which meaning they care about, where they saw it. If no word is given, ask for one.

Every lookup and write goes through the Japanese Tools of the Nando Connector. Nando enforces the structure itself, so this skill is about judgment:

- **One Word per spelling and reading.** `japanese_add_word` refuses a Word already stored; another reading of the same spelling is a different Word.
- **Readings.** A spelling with a Kanji needs its reading in kana; a kana Word has none, and a reading equal to its kana spelling counts as none.
- **Word Links are two-sided.** `japanese_add_word_link` writes both sides and gives the way back the opposite Nuance Tags (`formal`↔`casual`, `spoken`↔`written`, `expression`↔`part`). It skips a side that already exists.
- **Groups and tags.** A Word goes in a stored Group; Word Tags and Nuance Tags are snake_case.

A Tool that refuses says why; read its message and adjust.

## 1. Look it up

- **Nando:** `japanese_get_word` with `word` set to the input finds it by spelling or reading. If it is already stored, go to **Repeated Words** below instead of stopping.
- **jisho.org:** `japanese_lookup_jisho` with the word. Take the entry whose `slug` or `japanese[].word` matches. It gives the readings, `jlpt`, `isCommon`, the parts of speech, jisho's sense `tags` (Archaic, Rare, Colloquial, Female term, Slang, Kansai dialect…) and the English meanings. If several homographs match, ask which one.
- **Conjugations:** if the input is just a regular conjugation of a basic or known Word (causative 死なせる ← 死ぬ, passive, potential, causative-passive…), add nothing, even when jisho lists it as its own entry. Tell the Owner which Word it comes from and move on.
- **Conjugated input from a picture or screenshot:** when the Owner sends a picture of a conjugated verb or adjective (e.g. 震撼させる), write the **dictionary form** everywhere: `word`, `notes`, `spanish` and Word Link notes. The conjugated form belongs only where it is almost exclusive to an Expression (身を翻す, 屈託のない), or inside a note or Word Link note as part of an Expression or an example (世界を震撼させる, 気絶させる as an example of use). The Owner knows the conjugation rules; leave them out.

## 2. Pick the Group

Use jisho's JLPT tag, then what the Owner was doing:

| jisho JLPT | Group | extra Word Tag |
|---|---|---|
| N2 | `jlpt_n2` (whatever the Owner was doing) | — |
| N3 | `jlpt_n3` (whatever the Owner was doing) | — |
| N4 / N5 | `basic` (whatever the Owner was doing) | — |
| N1 | the Owner's activity Group, **or** `jlpt_n1` only if they were studying for JLPT N1 | `jlpt_n1` |
| none | the Owner's activity Group | — |

Activity → Group: reading (books, manga, articles) → `literature`, games → `gaming`, anime → `anime`, movies, dramas or series → `movie`, anything else → `miscellaneous`.

If the Word is N1 or has no JLPT tag and the Owner didn't say what they were doing, ask. N4 and N5 Words go in `basic`: there are no N4 or N5 Groups.

## 3. Expressions and their Parts

- **Confirm with jisho first.** The Owner often sends a phrase without knowing whether it's an Expression. If `japanese_lookup_jisho` lists it as an entry (身を翻す, 色気より食い気, 屈託のない), it is an Expression. If jisho doesn't list it (難問に直面する), it's a common collocation: add no Word for it, and write it as a note on the Word or Words it's built from, in the form "X に Y する: <meaning> (collocación común)".

An Expression is a set phrase such as 首にされる or 腹が立つ. Store it as one Word: `word: "首にされる"`, with the `expression` Word Tag and the `type` of what it acts as (首にされる → `verb`). Its Group follows step 2 with the Expression's own JLPT tag, which usually means the activity Group.

Then look at its main **Parts** (首, する):

- **All Parts are basic or known** (N4/N5 on jisho, or already stored): add **only** the Expression.
- **One or more Parts are unknown** (not stored and above N4): add **each unknown Part as its own Word** (steps 1–5) **plus** the Expression, and link them with `japanese_add_word_link` from the Expression to the Part: `kind: "related"`, `nuanceTags: ["part", …]`, `note` saying what the Part means on its own, and `noteBack` saying what the Expression means. The way back gets `expression` on its own.
  - **Exception: Parts that barely live outside the Expression.** If an unknown Part is rarely used on its own and almost only appears in this Expression or compound (通牒 → 最後通牒, 屈託 → 屈託のない), store only the Expression or compound and explain the Part in its `notes` (e.g. `屈託: preocupación (casi solo se usa en esta expresión)`). The Owner wants no useless standalone Words. Mention the choice afterwards so they can ask for the Part.
- Also link the Expression to any of its Parts already stored outside `basic`, the same way.
- **Sentences:** a Part's example sentences show the Part used on its own. Uses of the Expression go only in the Expression's sentences.

If it's unclear whether the Owner knows a Part, say what you assumed so they can correct it.

## 4. Build the Word

`japanese_add_word` takes the Word's own fields, shaped like the stored Words:

```json
{
  "word": "囁く",
  "hiragana": "ささやく",
  "spanish": "susurrar (a alguien, en voz baja)",
  "type": { "verb": 1 },
  "group": "jlpt_n2",
  "tags": ["common", "usually_kana"],
  "notes": "JLPT N2",
  "sentences": [{ "sentence": "彼は耳元で囁いた。", "translation": "He whispered in my ear." }]
}
```

- `word`: always the kanji spelling if one exists (see **Kanji spelling**).
- `spanish`: in Spanish, short, comma-separated senses, context in parentheses. It drives the Anki definition card, so it must tell this Word apart from its near-synonyms (`premio (objeto)` vs `premio (dinero)`).
- `notes`: `JLPT Nx` when jisho has a level; otherwise a short Spanish note, or none.
- `sentences`: 2 natural sentences at the Word's level, with English translations.
- It answers the new Word's `id`. Word Links and Hints come after, in step 5.

### Kanji spelling

If the Word has a kanji spelling, store it in kanji, however rare, obscure or unused that kanji is: `何処`, read どこ. If jisho marks it "Usually written using kana alone", add the `usually_kana` Word Tag; the Anki deck then shows the kana form on the card. The Owner often types the kana form (どこ), so take the kanji spelling from jisho's `japanese[].word`, the first one if there are several. Only Words with no kanji spelling at all are stored in kana, with no `hiragana`.

**Exception: kanji spellings shared by several readings.** The same kanji spelling sometimes has more than one reading: `包む` is both つつむ and くるむ. Only the most common reading gets the kanji spelling: `包む` (つつむ). The other is stored in kana: `くるむ`. To find the most common reading:
- look up the kanji spelling itself with `japanese_lookup_jisho` and compare the entries that use it;
- the reading of the top-ranked entry wins, and `isCommon` breaks ties;
- if `japanese_get_word` finds that kanji spelling stored with another reading, the stored one is normally the common one.

State this choice to the Owner, since they may disagree. Example sentences spell the Word the way it's naturally written (usually kana for `usually_kana` Words).

Spelling follows jisho: when jisho's headword is kana (ぱちり, こそばゆい, listed only in kana), store that kana as `word`, even if games or manga usually write it in katakana.

### Type

Each part of speech maps to the form the cards show, not a count: `1` or `2` plain, `3` ～に, `4` ～と, `5` ～的, `6` ～的に, `7` ～たる, `8` ～を. E.g. 根本 is `{ "noun": 1, "na_adj": 5, "adv": 6 }` (根本的, 根本的に).

From jisho's parts of speech:
- Godan / Ichidan verbs → `verb`
- "Suru verb" → `noun` + `suru`
- "Na-adjective" → `na_adj`
- "No-adjective" → `no_adj`
- "I-adjective" → `i_adj`
- "Adverb" → `adv`
- "Adverb taking the 'to' particle" → `adv: 1`, with `notes` set to `adv. と`. jisho's tag means と is *allowed*, not required: most mimetic adverbs work with or without it (ほんのり甘い / ほんのりと甘い, じわじわ, じっくり, うっすら). Use `adv: 4` (～と) only when と is required for adverbial use, which is mostly ～たる kango (敢然と, 堂々と, 颯爽と: ✗敢然立ち向かう).
- "Counter" → `counter`

### Word Tags: when and how to use the Word

Word Tags tell the Owner, who meets Words they have never heard, **whether and when to use the Word**. Add every Word Tag that helps.

**Basic Word Tags:** `common`, `usually_kana`, `transitive`, `intransitive`, `expression`, `honorific`, `humble`, `onomatopoeic`, `jargon`, `ateji`, `joyogai`, `jlpt_n1`.

**Jōyō check (every new Word, and every new spelling):** `japanese_check_joyo` with the spelling. Any Kanji with `joyo: false` gets the Word the `joyogai` Word Tag (捏 in 捏造, 嘔 in 嘔吐). `joyo: null` means kanjiapi.dev couldn't tell yet: check again a little later, and tag only on an answer.

**Usage Word Tags,** from jisho's sense tags and your own knowledge:
- Register: `formal`, `casual`, `polite`, `business`
- Medium: `written`, `spoken`, `literary`, `poetic`, `technical`
- Age and frequency: `archaic`, `obsolete`, `dated`, `rare`
- Social: `slang`, `internet_slang`, `vulgar`, `derogatory`, `euphemistic`
- Who uses it: `feminine`, `masculine`, `elderly_speech`, `youth_speech`, `childish`
- Dialect: `kansai_dialect` (and other `<region>_dialect`)
- Other: `idiomatic`, `yojijukugo`

You may invent a Word Tag when none of these captures something useful. First check `japanese_list_tags`, and reuse an existing tag over a near-duplicate (`rare`, not `uncommon`). Keep tags lowercase snake_case English, one idea each.

Usage Word Tags show on the Anki cards, in the `Usage` field.

## 5. Word Links

Word Links, and their Nuance Tags, are the Owner's most valuable study tool in Anki. Put real effort here.

Find candidates:
- Words sharing a Kanji: `japanese_search` with `in: "spelling"` and each Kanji of the Word as `q`;
- Words with an overlapping meaning: `japanese_search` with `in: "spanish"`;
- jisho's `seeAlso` and `antonyms`;
- the Expression ↔ Part links from step 3.

For each real synonym, antonym, near-synonym or Expression ↔ Part pair, call `japanese_add_word_link` from the new Word to the other:

- `kind`: `synonym`, `antonym` or `related`.
- `note`, in English: how **the other** Word differs from this one. Leave out its JLPT level: the Anki card shows it as a badge.
- `nuanceTags`: how **the other** Word is used compared with this one. The common ones are `formal`, `casual`, `spoken`, `written`, `technical`, `literary`, `polite`, `archaic`, `humble`; every usage Word Tag from step 4 is welcome too (`rare`, `feminine`, `elderly_speech`, `dated`…), and so are `expression` and `part`. Anki shows them on both sides of the card. Be generous where a tag genuinely tells the two Words apart.
- `noteBack`: the way back's note, about this Word, when it needs its own wording; it is the same note otherwise. The way back's Nuance Tags are set for you.

Skip plain compounds that only share the Kanji (～家, ～客).

An everyday N4/N5 word that isn't stored gets a **Hint** instead: `japanese_add_hint` with the word (天気) and an optional note. A Hint is one-way; a stored Word always gets a Word Link.

## Repeated Words

If the Word is already stored, review the whole Word and improve it:
- Is the `spanish` accurate, and does it tell the Word apart from its synonyms?
- Are the type, Group and Word Tags right under the current rules? Add any missing usage Word Tags.
- Is it stored in kana although it has a kanji spelling? Move it to the kanji spelling with its reading and `usually_kana` (see **Kanji spelling**), unless another, more common reading owns that spelling.
- Are the sentences natural and at the right level? Does a Part use its own Expression in its sentences?
- Is `notes` clean?
- Look for missing Word Links, missing ways back, and weak or missing Word Link notes and Nuance Tags.

Apply the improvements directly: `japanese_update_word` changes only the fields you send (`tags` and `sentences` are whole lists), `japanese_update_word_link` changes one side of a Word Link, and `japanese_add_word_link` adds what's missing. Show the Owner a before → after of what changed: `japanese_get_word` before, and each Tool answers the Word as it is after.

## 6. Approval

**Adding and improving needs no approval.** Go straight to step 7 and show the Owner what was written afterwards when:
- you didn't need to ask the Owner anything (Group, homograph, meaning, which reading gets the kanji spelling, known or unknown Parts);
- the changes are new Words, Word Links, Hints, and improvements to stored Words (spanish, type, Word Tags, notes, sentences, spelling or reading, Word Links and their notes and Nuance Tags).

**Ask first** for the destructive Tools, which the Claude app is also set to confirm: deleting a Word (`japanese_delete_word`), merging two (`japanese_merge_words`: merge first, then set the kept Word's fields with `japanese_update_word`), splitting one into two (大家 おおや/たいか), moving a Word to another Group (`japanese_change_group`), and any change across many Words at once.

When you ask, show one compact draft of everything:
- each Word's spelling, reading, spanish, type, Group (and why), Word Tags, notes and sentences;
- each Word Link, with both sides' notes and Nuance Tags;
- your assumptions (known or unknown Parts, the activity Group).

Apply the Owner's edits, and show the draft again if anything changed substantially.

## 7. Write

In this order, so that Words in the same batch can link to each other:

1. `japanese_add_word` for each new Word, keeping each `id`.
2. `japanese_add_word_link` for each Word Link, and `japanese_add_hint` for each Hint.
3. `japanese_update_word` and `japanese_update_word_link` for the improvements to stored Words.

Each Tool answers what it wrote. Other chats may be adding Words at the same moment: a refused add means the Word is stored now, so look at it with `japanese_get_word` and review it instead. If a call fails partway through, tell the Owner exactly what was and wasn't written.

Then tell the Owner what was written: the Words with their `id`s, the Word Links (both sides are written for you), the Hints and the edits.
