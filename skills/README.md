# Skills

Skills Claude follows to work with the Owner's data in Nando, through the Nando Connector's Tools. They are written for the Claude app (chat on the iPhone, with no filesystem) and work the same in Claude Code.

| Skill | What it does |
| --- | --- |
| [`japanese/add-word`](japanese/add-word/SKILL.md) | Adds a Japanese word or Expression, or reviews and improves a stored Word |
| [`japanese/suggest-related`](japanese/suggest-related/SKILL.md) | Checks a Word Link the Owner thought of while studying, and adds it if it holds |
| [`japanese/word-requests`](japanese/word-requests/SKILL.md) | Works through the Owner's Word Requests from Nando, with no Owner present |

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

## Use word-requests in a scheduled task

The Owner's Word Requests are worked through by a cloud scheduled task in Claude Code on the web, which Nando wakes through its API trigger ([ADR 0006](../docs/adr/0006-word-requests-through-a-scheduled-task.md)). The task has the nando-server repo attached and the Nando Connector on, and its prompt points at the skill: "Follow skills/japanese/word-requests/SKILL.md". The run reads that file, and `add-word` with it, from the repo, so neither is uploaded or linked for it.

## Add the Nando Connector

The skills need the Connector to reach the Tools. Connecting sends you to Nando's consent page, `https://web.nanders.app/connect`, which shows what each Module's Tools let the Connector do; approve there with your Passkey (Face ID).

- **Claude app:** in claude.ai, open **Settings**, then **Connectors**, choose **Add custom connector**, and give it the URL `https://api.nanders.app/mcp`. Then **Connect**, and approve on Nando's page.
- **Claude Code:**

  ```sh
  claude mcp add --transport http nando https://api.nanders.app/mcp
  ```

  Then run `/mcp` in Claude Code, choose `nando`, and authenticate: your browser opens Nando's page to approve it.

In the Connector's settings in claude.ai, set the three destructive Tools to ask every time: `japanese_delete_word`, `japanese_merge_words` and `japanese_change_group`.

A Connector is listed in Nando's Settings, where removing it cuts it off at once.
