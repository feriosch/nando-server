# Anki note type

Benkyo Note Type V2: the Anki note type the JLPT deck file, `GET /api/japanese/anki/jlpt.csv` (see [the Japanese API](../API.md)), fills. These are its templates and styling, as set up in the Owner's Anki.

| File | Where it goes in Anki |
|---|---|
| `styling.css` | Note type → Cards → Styling (shared by both card types) |
| `definition_card/front.html`, `back.html` | Card type 1: Definition → Word |
| `word_card/front.html`, `back.html` | Card type 2: Word → Definition |

## Fields

Create the note type with exactly these fields, in this order:

1. Id
2. Word
3. Pronunciation
4. Definition
5. Type
6. Notes
7. Japanese Sentence
8. Translation Sentence
9. Common
10. JLPT N1
11. Expression
12. Onomatopoeic
13. Transitive
14. Intransitive
15. Related Front
16. Related Back
17. Usage

`Id` is the Word's ObjectId. It is the first field, so Anki uses it as the
note key: re-importing updates the existing note even if the Word or its reading
changed. No template shows it.

## Importing

The file starts with Anki header lines: `#separator`, `#html` and `#columns`, so
the import screen picks the right separator, enables HTML and maps columns to
fields by name, and `#notetype` and `#deck`, so it also starts on this note type
and the JLPT Benkyo Deck V2 deck. Set **Existing notes** to **Update**.
