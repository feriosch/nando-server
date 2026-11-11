# Skills

Skills Claude follows to work with the Owner's data in Nando, through the Nando Connector's Tools. They are written for the Claude app (chat on the iPhone, with no filesystem) and work the same in Claude Code.

| Skill | What it does |
| --- | --- |
| [`japanese/add-word`](japanese/add-word/SKILL.md) | Adds a Japanese word or Expression, or reviews and improves a stored Word |
| [`japanese/suggest-related`](japanese/suggest-related/SKILL.md) | Checks a Word Link the Owner thought of while studying, and adds it if it holds |

`internal/japanese/tests/skills_test.go` checks that every Tool a skill names is one Nando serves, so a renamed Tool fails the build until its skills follow.

## Use them in the Claude app

1. **Zip each skill folder**, so that the zip holds the folder with its `SKILL.md` inside:

   ```sh
   cd skills/japanese
   zip -r add-word.zip add-word
   zip -r suggest-related.zip suggest-related
   ```

2. **Upload them:** in claude.ai, open **Settings**, then **Capabilities**, and under **Skills** upload each zip. Skills need **Code execution and file creation** turned on there. A changed skill is uploaded again the same way.

## Use them in Claude Code

Link each skill folder into your personal skills, so they follow this repo as it changes:

```sh
mkdir -p ~/.claude/skills
ln -s "$PWD/skills/japanese/add-word" ~/.claude/skills/add-word
ln -s "$PWD/skills/japanese/suggest-related" ~/.claude/skills/suggest-related
```

## Add the Nando Connector

The skills need the Connector to reach the Tools. Connecting sends you to Nando's consent page, where you approve with your Passkey (Face ID). That page, `https://web.nanders.app/connect`, is still to be built in nando-web; until then, connecting stops there.

- **Claude app:** in claude.ai, open **Settings**, then **Connectors**, choose **Add custom connector**, and give it the URL `https://api.nanders.app/mcp`. Then **Connect**, and approve on Nando's page.
- **Claude Code:**

  ```sh
  claude mcp add --transport http nando https://api.nanders.app/mcp
  ```

  Then run `/mcp` in Claude Code, choose `nando`, and authenticate: your browser opens Nando's page to approve it.

In the Connector's settings in claude.ai, set the three destructive Tools to ask every time: `japanese_delete_word`, `japanese_merge_words` and `japanese_change_group`. The skills ask before using them too.

A Connector is listed in Nando's Settings, where removing it cuts it off at once.
