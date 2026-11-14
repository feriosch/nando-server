---
name: word-requests
description: >-
  Work through the Owner's Word Requests in Nando: add each word or improve each Word they asked for, and record the outcome. Use when a scheduled task's prompt points here, or when the Owner asks to work through their Word Requests.
---

# Work through the Word Requests

The Owner asks for words in Nando, and Nando wakes a scheduled task whose prompt points here. **No Owner is present during the run:** everything that would go to the Owner goes into the Word Request's outcome instead, and they read it in Nando's Pending strip.

Every lookup and write goes through the Japanese Tools of the Nando Connector. Each Word Request is handled with the `add-word` skill: read it now from `skills/japanese/add-word/SKILL.md` in this repo, all of it, since a scheduled task may not load skills by itself. This skill only says how a run differs from a chat with the Owner.

## 1. List

Call `japanese_list_word_requests`. It claims up to 3 Word Requests for this run, for 15 minutes. If it answers none, stop: the run is over.

Each Word Request has an `id`, a `kind` (`add` or `improve`), and:
- `add`: `text`, as the Owner typed it, maybe conjugated or in kana, and `group`, the Group they found it in.
- `improve`: `wordId`, the stored Word to improve, and its spelling as `text`.
- `question`, `choices` and `answer` when the Owner has answered a question an earlier run asked. The `answer` settles that `question`, as if the Owner had replied in chat. It may be one of the `choices` or their own words.

## 2. Work each Word Request with add-word

**`add`:**
- Resolve `text` to the dictionary-form Word, as add-word's step 1 says for conjugated input, and take the kanji spelling from jisho for kana input (add-word's **Kanji spelling**). Work on that Word from then on.
- `group` is what the Owner was doing: wherever add-word's step 2 needs the Owner's activity Group (a Word tagged N1 or with no JLPT tag, and its unknown Parts), use `group`. `jlpt_n1` means they were studying for JLPT N1. N2, N3, N4 and N5 Words still take their Group from jisho.
- When `japanese_get_word` finds the Word already stored, or `japanese_add_word` refuses it as stored, improve it instead with add-word's **Repeated Words**, and resolve it `done` with that Word's `id`.
- A phrase jisho doesn't list is a collocation: write it as a note on the Word it's built from, as add-word step 3 says, and resolve `done` with that Word's `id`.

**`improve`:** run add-word's **Repeated Words** review on `wordId`, and look hard for missing Word Links between it and the Words already stored. `done` names that same `wordId`: `japanese_resolve_word_request` refuses any other Word. Improvements to other Words you come across go in `suggestions`.

**Destructive Tools are the Owner's alone.** Never call `japanese_delete_word`, `japanese_merge_words` or `japanese_change_group`, whatever add-word's step 6 allows after asking: there is no Owner to ask. When a merge, a deletion or a Group change looks right, finish everything else and write it as a suggestion the Owner can act on in Claude Code: "Move 掛ける to jlpt_n2", "Merge どこ into 何処: the same Word, stored twice".

## 3. Ask instead of guessing

Wherever add-word would ask the Owner, ask through the Word Request, **before writing anything for it**: resolve it `needs_answer` with a short `question` and 2 to 4 `choices` the Owner can tap. Nando sends it back with their `answer`. The cases:
- a homograph: which of the jisho entries they meant (`"Which かける?"`, `["掛ける (to hang)", "賭ける (to bet)"]`);
- which reading gets a kanji spelling that several readings share;
- the activity Group, when `group` is `jlpt_n1` but jisho gives the Word no JLPT level: offer the likeliest activity Groups;
- whether the Owner knows a Part, when add-word's rules leave it unclear;
- the meaning they care about, when the senses are too far apart to store as one Word.

Keep each choice short enough to read on a phone. Everything add-word says to tell the Owner afterwards (an assumption, a Part kept in the Expression's notes, the reading that got the kanji spelling) goes in the `summary` or the `suggestions`.

## 4. Resolve

Resolve each Word Request with `japanese_resolve_word_request` as soon as its writes are done, well inside its 15-minute claim:

- **`done`:** `wordId` is the Word asked for (for an Expression, the Expression), and `summary` one line for the Owner: `"Added 食べる · 3 Word Links · 1 Hint"`, `"Improved 掛ける: Spanish meaning tightened, +2 Word Links"`. `suggestions` are what the Owner should do in Claude Code, one per item.
- **`failed`:** the word can't be added: no such word, even unconjugated; a regular conjugation of a stored or basic Word that add-word says to add nothing for (name the Word it comes from); an `improve` whose Word is no longer stored. `summary` says why, for the Owner.
- **Neither:** a Word Request you can't finish for another reason (a Tool keeps failing, jisho is unreachable) stays unresolved. Leave it: its claim runs out and a later run takes it.

If a resolve is refused because the Word Request isn't working any more, its claim ran out; move on to the next.

## 5. Next

Once every Word Request from the last list is resolved or left, go back to step 1 while this run has resolved fewer than 10 Word Requests. At 10 or more, stop: Nando starts the next run itself, so each run's context stays short.
