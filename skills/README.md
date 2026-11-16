# Skills

Skills Claude follows to work with the Owner's data in Nando, through the Nando Connector's Tools. The chat skills are written for the Claude app (chat on the iPhone, with no filesystem) and work the same in Claude Code. `word-requests` and `career-sync` are for cloud scheduled tasks that Nando wakes, with no Owner present.

### Japanese

| Skill | What it does |
| --- | --- |
| [`japanese/add-word`](japanese/add-word/SKILL.md) | Adds a Japanese word or Expression, or reviews and improves a stored Word |
| [`japanese/suggest-related`](japanese/suggest-related/SKILL.md) | Checks a Word Link the Owner thought of while studying, and adds it if it holds |
| [`japanese/word-requests`](japanese/word-requests/SKILL.md) | Works through the Owner's Word Requests, following `add-word` for each, and records what came of them (scheduled task) |

### Career

| Skill | What it does |
| --- | --- |
| [`career/career-opportunity-update`](career/career-opportunity-update/SKILL.md) | Updates an Opportunity from what happened (a Step, dates, an offer, a tip, a PDF or screenshot): Steps, Stage, Outcome, Interest, pay, Perks and Company notes |
| [`career/career-draft-reply`](career/career-draft-reply/SKILL.md) | Shows the Suggested Reply a Sync's run wrote for a Recruiter and rewrites it as the Owner steers, in their voice |
| [`career/career-profile-update`](career/career-profile-update/SKILL.md) | Builds or changes the Career Profile from resumes or what the Owner says |
| [`career/career-interview-prep`](career/career-interview-prep/SKILL.md) | Prepares for a Step from the Company Brief and further public research, and keeps useful findings as Company notes |
| [`career/career-sync`](career/career-sync/SKILL.md) | Sorts what each Sync brought, keeps Opportunities and Company Briefs, and writes every Suggested Reply (scheduled task) |

Each name carries `career-` because skills uploaded to the Claude app have no prefix.

`internal/japanese/tests/skills_test.go` and `internal/career/tests/skills_test.go` check that every Tool a skill names is one Nando serves, so a renamed Tool fails the build until its skills follow. They also check that each unattended skill bans every destructive Tool, and that the `nando` plugin carries exactly the four Career chat skills.

## Use them in the Claude app

1. **Zip each chat skill's folder**, so that the zip holds the folder with its `SKILL.md` inside:

   ```sh
   cd skills/japanese
   zip -r add-word.zip add-word
   zip -r suggest-related.zip suggest-related
   cd ../career
   for s in career-opportunity-update career-draft-reply career-profile-update career-interview-prep; do zip -r "$s.zip" "$s"; done
   ```

   `word-requests` and `career-sync` are never uploaded: their scheduled tasks read them from the repo.

2. **Upload them:** in claude.ai, open **Settings**, then **Capabilities**, and under **Skills** upload each zip. Skills need **Code execution and file creation** turned on there. A changed skill is uploaded again the same way.

   **Once, after the rename:** `career-draft-reply` was called `career-help-reply`. Delete `career-help-reply` from your Skills, then upload `career-draft-reply.zip`.

## Use them in Claude Code

### Career: the `nando` plugin

This repo is a Claude Code plugin marketplace holding one plugin, `nando`, with the four Career chat skills ([`.claude-plugin/`](../.claude-plugin/)). `career-sync` isn't in it. In Claude Code:

```text
/plugin marketplace add feriosch/nando-server
/plugin install nando@nando
```

The skills are then `/nando:career-opportunity-update`, `/nando:career-draft-reply`, `/nando:career-profile-update` and `/nando:career-interview-prep`, and Claude also uses them on its own when the chat calls for one. The plugin has no version, so `/plugin marketplace update nando` brings in each change pushed to `main`.

**Once, after the rename** of `career-help-reply` to `career-draft-reply`: run `/plugin marketplace update nando` so the plugin carries the new name.

The marketplace entry's `source` is the repo root, `"./"`, and it lists the four skill folders in `skills`. A root-source entry that lists `skills` loads only those folders, without scanning `skills/` for others. A new chat skill is added to that list, and the skills test checks it.

### Japanese

Link each skill folder into your personal skills, so they follow this repo as it changes:

```sh
mkdir -p ~/.claude/skills
ln -s "$PWD/skills/japanese/add-word" ~/.claude/skills/add-word
ln -s "$PWD/skills/japanese/suggest-related" ~/.claude/skills/suggest-related
```

## Use `word-requests` in a scheduled task

The Owner adds or improves a word from Nando as a Word Request, and Nando wakes a cloud scheduled task (Claude Code on the web) through its API trigger to work through them ([ADR 0006](../docs/adr/0006-word-requests-through-a-scheduled-task.md)). The task has **no schedule**, only the API trigger, so it runs only when Nando wakes it and an empty queue costs nothing. It has this repo attached and the Nando Connector, and its prompt points at the skill:

```text
Follow skills/japanese/word-requests/SKILL.md
```

The run reads the skill, and `add-word` with it, from the attached repo, so nothing is uploaded. With no Owner to approve, it never deletes, merges or moves a Word: it leaves those as suggestions on the Word Request, for the Owner to do in Claude Code. Setting the task up is issue #71.

## Use `career-sync` in a scheduled task

The Owner's MacBook hands each Sync's LinkedIn messages to Nando, and Nando wakes a second cloud scheduled task through its API trigger to work through them ([ADR 0007](../docs/adr/0007-linkedin-synced-from-the-owners-mac.md)), as for Word Requests. Set it up like `word-requests`'s:

- **No schedule**, only the API trigger. Nando calls it with `CAREER_SYNC_TRIGGER_URL` and `CAREER_SYNC_TRIGGER_TOKEN` (see [Career's API](../internal/career/API.md)).
- **This repo attached**, so the run reads the skill from it, and nothing is uploaded.
- **The Nando Connector.**
- **Web access**, for the Company Briefs it writes from public information.
- **Its prompt** points at the skill:

  ```text
  Follow skills/career/career-sync/SKILL.md
  ```

With no Owner present, it never deletes a Step or an Opportunity, and closes an Opportunity only as `withdrawn` once the Owner's own message declines it. It stops after 30 Items, and Nando wakes another run for the rest. Setting the task up is issue #94.

## Add the Nando Connector

The skills need the Connector to reach the Tools. Connecting sends you to Nando's consent page, `https://web.nanders.app/connect`, where you approve with your Passkey (Face ID).

- **Claude app:** in claude.ai, open **Settings**, then **Connectors**, choose **Add custom connector**, and give it the URL `https://api.nanders.app/mcp`. Then **Connect**, and approve on Nando's page.
- **Claude Code:**

  ```sh
  claude mcp add --transport http nando https://api.nanders.app/mcp
  ```

  Then run `/mcp` in Claude Code, choose `nando`, and authenticate: your browser opens Nando's page to approve it.

In the Connector's settings in claude.ai, set the destructive Tools to ask every time:

- Japanese: `japanese_delete_word`, `japanese_merge_words` and `japanese_change_group`;
- Career: `career_delete_step` and `career_delete_opportunity`.

A Connector is listed in Nando's Settings, where removing it cuts it off at once.
