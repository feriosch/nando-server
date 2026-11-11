---
name: suggest-related
description: >-
  Check a related word the Owner thought of while studying an Anki card and add the Word Link to Nando if it holds, or explain why it doesn't. Use whenever the Owner names a word and another word it made them think of, or says a link is missing (e.g. "for 待ち望む I was thinking 期待", "待ち望む reminds me of 期待", "is 杞憂 related to 心配?", "/suggest-related 待ち望む 期待"), even if they don't mention Word Links or Nando.
---

# Check and add a suggested Word Link

Input: the Owner's message. It has the **card Word** (the Word being studied, usually on a definition card) and one or more **suggested words** the Owner associated with it. If it's unclear which is which, ask.

The Owner is studying and wants a quick answer. Lead with the verdict and keep the rest short.

This skill relies on the `add-word` skill for adding new Words and for the rules of Word Link notes and Nuance Tags (its step 5). Every lookup and write goes through the Japanese Tools of the Nando Connector.

## 1. Look both up

- **Nando:** `japanese_get_word` with `word` set to the card Word and to each suggestion. Note each Word's `id`, `spanish`, `group`, Word Tags and Word Links. The card Word is usually stored; if it isn't, ask the Owner which Word they mean.
- **jisho.org:** `japanese_lookup_jisho` for both: the meanings, JLPT level and `isCommon`.

## 2. Judge the association

Decide what the link really is, from jisho's meanings and your own knowledge of usage:

| Verdict | When | Action |
|---|---|---|
| **synonym** | same meaning; often interchangeable in context | add, `kind: "synonym"` |
| **antonym** | opposite meaning | add, `kind: "antonym"` |
| **related** | near-synonym with a clear difference in nuance, register or scope; the same concept seen from another angle; or a common trap worth remembering together | add, `kind: "related"` |
| **not related** | the Owner misremembered a meaning, the Words only share a Kanji (plain compounds), or they're too far apart to help | add nothing; correct the Owner |

Tell the Owner plainly whether they were right. When they were partly right (they thought synonym but it's only related), say what the difference is. When they were wrong, correct them kindly: give both meanings and one short example sentence for each, and suggest a better-fitting word if one exists, stored or not.

## 3. A suggested word that isn't stored

- **N4/N5 or an everyday basic word:** add it as a Hint on the card Word with `japanese_add_hint` (its text, and a note). A Hint is one-way.
- **N3 and above:** add it as a new Word following `add-word` steps 1–7, then link it. The Owner found it while studying their JLPT deck, so an N1 Word goes in the `jlpt_n1` Group, with the `jlpt_n1` Word Tag. If it has no JLPT tag, ask which Group it belongs in.

## 4. Write the Word Link

`japanese_add_word_link` from the card Word to the suggested Word, following `add-word` step 5:
- `note`, in English: how **the suggested** Word differs from the card Word.
- `nuanceTags`: how the suggested Word is used compared with the card Word (`formal`, `spoken`, `literary`…).
- `noteBack` when the way back needs its own wording. Nando writes both sides and gives the way back the opposite Nuance Tags.

Also check:
- **The Word Link already exists:** `japanese_add_word_link` skips a side that exists, and says so. Tell the Owner it was already there, and that the card shows it after the next deck import (step 5). If its note or Nuance Tags are weak, improve them with `japanese_update_word_link`, one side per call.
- **The card Word has few or no Word Links:** while you're there, add other clear missing synonyms, antonyms and related Words that are stored. Find them with `japanese_search` (`in: "spelling"` with each of its Kanji, `in: "spanish"` with its meaning). Only clear ones, and list them separately in the report.

**Approval:** none for adding or improving Word Links between stored Words, or for a new Word that `add-word` step 6 counts as needing none. Ask first if you needed to ask the Owner anything (meaning, homograph, Group), and before removing a Word Link (`japanese_remove_word_link`) or changing its kind.

## 5. Report

1. **Verdict:** right, partly right or not related, with the difference in one or two lines.
2. **Written:** each Word Link as `card Word → Word (kind, Nuance Tags): note`; the way back is implied. List any extra Word Links from step 4 separately.
3. **Anki:** the card shows the new Word Link after the next import of the JLPT deck. The Owner downloads it by opening https://api.nanders.app/api/japanese/anki/jlpt.csv in a browser where they're signed in to Nando, and imports it with **Existing notes: Update**; the `Id` field keeps their review history. Words outside the JLPT deck (Gaming, Literature…) can't be exported from Nando yet.
