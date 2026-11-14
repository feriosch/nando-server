---
name: word-requests
description: >-
  Work through the Owner's Word Requests in Nando, unattended: add each requested word, or improve the requested Word, following the add-word skill, and record each outcome for the Owner. Use when a scheduled task's prompt points here.
---

# Work through the Word Requests

A Word Request is the Owner's ask, made in Nando, to add a word (`kind: "add"`, with the `text` they typed and the `group` they found it in) or to improve a stored Word (`kind: "improve"`, with its `wordId`). This run is unattended: the Owner is not reading it. They see only what you record with `japanese_resolve_word_request`, in the Pending strip of Japanese search, so every outcome, question and suggestion goes there.

If the Owner is in the chat with you, this skill isn't for that: follow `add-word` with them directly, asking as it says.

Each Word Request follows the `add-word` skill. Read all of it now from `skills/japanese/add-word/SKILL.md` in the attached nando-server repo; scheduled tasks may not load skills on their own. Every lookup and write goes through the Japanese Tools of the Nando Connector.

## 1. Claim

`japanese_list_word_requests` claims the oldest waiting Word Request for this run, for 15 minutes. If it answers none, stop: the run is done.

## 2. Work each one through add-word

Resolve the claimed Word Request (step 4) before claiming the next, so its 15 minutes cover only that word.

**`add`**
- `text` is as the Owner typed it, maybe conjugated (食べた) or in kana (どこ). Resolve it to the dictionary-form Word as add-word step 1 and **Kanji spelling** say, and add that Word. If it is stored already, run add-word's **Repeated Words** on it instead.
- `group` is what the Owner was doing: it is the activity Group wherever add-word step 2 needs one (an N1 Word, or one with no JLPT tag). `jlpt_n1` means they were studying for JLPT N1. jisho.org's JLPT level still decides N2, N3 and `basic`.
- `japanese_add_word` refusing because the Word is stored means another chat added it meanwhile: run **Repeated Words** on it and resolve `done`.

**`improve`**
- Run add-word's **Repeated Words** on the Word with id `wordId` (`japanese_get_word` with `id`), including Word Links missing between it and other stored Words.
- Word Links of this Word, both sides, are part of improving it. Changes to another Word's own fields go in `suggestions`.

**An `answer`** comes with the `question` and `choices` you asked before. It settles that question; carry on from there. The Owner may answer in their own words.

**Approval.** There is no Owner to approve a draft, so skip add-word step 6's draft: adding and improving go straight to writing (add-word step 7). Anything add-word would tell the Owner afterwards, such as storing a Part only inside its Expression, goes in the `summary` or `suggestions`.

## 3. The destructive Tools are the Owner's

Deleting, merging and moving a Word to another Group are the Owner's decisions, made in Claude Code. NEVER call `japanese_delete_word`, `japanese_merge_words` or `japanese_change_group`, even when add-word's rules say a Word needs it. Finish everything else, and write the change as a suggestion the Owner can act on: "Move 掛ける to jlpt_n2", "Merge くるむ into 包む".

## 4. Resolve

`japanese_resolve_word_request` with the Word Request's `id` and one outcome:

- **`done`**, once everything is written: `wordId` is the Word asked for (for an Expression, the Expression, not its Parts; for an `improve`, the same `wordId`, which the Tool insists on). `summary` is one line, in English, saying what changed:
  - "Added 食べる (from 食べた) · 3 Word Links · 1 Hint"
  - "Added 首にされる + Part 首 · 2 Word Links"
  - "Improved 掛ける: Spanish meaning tightened, +2 Word Links"

  `suggestions` hold what only the Owner can do (step 3), and changes you'd make to other Words.
- **`needs_answer`**, wherever add-word would ask the Owner: which homograph, which reading gets a kanji spelling several readings share, which Group when the Owner's doesn't tell, or whether the Owner knows a Part when add-word's rule leaves it unclear. Write nothing for that Word Request first. `question` is short; `choices` are 2 to 4 answers the Owner can tap, each clear on its own: "掛ける (to hang)", "架ける (to bridge)". The Word Request comes back with their answer.
- **`failed`**, when the word can't be added or improved at all: jisho.org has no such word even unconjugated, the `text` isn't Japanese, or the Word to improve is no longer stored. `summary` says why, for the Owner.

When you can't finish a Word Request for a passing reason (jisho.org or a Tool keeps failing, a write fails partway), leave it unresolved. Its claim runs out, and a later run takes it and finds whatever was written. A Word Request claimed 3 times with no outcome fails on its own.

If `japanese_resolve_word_request` refuses because the Word Request isn't `"working"`, its claim ran out: leave it and go on.

## 5. Repeat

Back to step 1 until it answers none, or until this run has resolved 10 Word Requests: then stop. Nando starts another run for any left, at the Owner's next ask or within half an hour.
