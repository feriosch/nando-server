# Career API

The Career Module: the Owner's LinkedIn Conversations with Recruiters, the Opportunities they bring, and whether the Owner is Looking. The app reads them through the routes below and changes only Looking and whether the Owner replied to a Suggested Reply. Everything else is changed through the Tools. The script on the Owner's MacBook reads LinkedIn and hands each Sync to Nando through the Tools below ([ADR 0007](../../docs/adr/0007-linkedin-synced-from-the-owners-mac.md)). A cloud Claude run then sorts what it brought, and the Owner's chat skills keep the rest, through the same Tools. Every route is under `/api/career/` and, like all of `/api/`, needs a valid Session cookie (see [Core's API](../core/API.md)). Bodies are JSON. Errors are a status code with a short plain-text message.

## Settings

```json
{ "looking": false }
```

- `looking`: whether the Owner is Looking for a new role right now. It sets the tone of every Suggested Reply. `false` until the Owner first changes it.

### `GET /api/career/settings`

`200`: the settings.

### `PUT /api/career/settings`

Replaces the settings. `204`. `400` when `looking` is missing or isn't `true` or `false`.

## What the app reads

The Home Card, the Career screen and its pages, all read-only (the Owner's mark that they replied is under [Marking a Suggested Reply replied](#marking-a-suggested-reply-replied)). None of them lists Leads as such, closed Opportunities, or the Career Profile, which only the Tools read. Times are RFC 3339. Step dates are `YYYY-MM-DD` and times `HH:MM` (or `null`), in the Owner Time Zone.

### The top Opportunities

Lists show open Opportunities only, top first:

1. by Interest: `high`, then `normal`, then `low`;
2. within the same Interest, by Stage: `offer`, then `in_process` by when its next Step is, by date then time (one with none left after the others), then `introduced`;
3. then the one that changed most recently through the Tools first.

The fit note is never used to rank. An Opportunity's **next Step** is its first Step, by date then time, dated today or later in the Owner Time Zone. Past Steps are never next. The rule is `Rank` in `utils/ranking.go`.

Each list shows an Opportunity as:

```json
{
  "id": "6740…",
  "company": { "id": "6741…", "name": "Kumo Pay" },
  "role": "Backend Engineer",
  "stage": "in_process",
  "interest": "high",
  "nextStep": { "kind": "Final interview", "date": "2026-11-27", "time": "18:00" }
}
```

`nextStep` is `null` when no Step is left.

### Waiting and Late Replies

A Recruiter is **waiting** when one of their Conversations has a Suggested Reply of kind `answer` or `follow_up`. One whose Suggested Reply is `late` is in Late Replies instead. A Suggested Reply the Owner marked replied is in neither, nor counted on the Career screen or the Home Card. A Lead is listed like any other Recruiter. Each list shows a Recruiter once: waiting from their newest message waiting for an answer or a follow-up, Late Replies from their oldest message waiting for a Late Reply. Ties are listed in the same order on every request:

```json
{
  "id": "6742…",
  "name": "Tanaka Yui",
  "organisation": "Hays Japan",
  "lastMessageAt": "2026-11-13T01:15:00Z",
  "firstLine": "Thank you for reaching out!"
}
```

- `lastMessageAt`: when the Recruiter's last message in that Conversation came, not the Owner's after it.
- `firstLine`: the first line of the Suggested Reply.

### `GET /api/career/home-card`

`200`: the Career Home Card.

```json
{ "looking": false, "waiting": 2, "top": [] }
```

- `looking`: whether the Owner is Looking, as in the settings.
- `waiting`: how many Recruiters are waiting.
- `top`: the top 3 open Opportunities.

### `GET /api/career`

`200`: the Career screen.

```json
{ "waiting": [], "top": [], "open": 7, "lateReplies": 1 }
```

- `waiting`: the Recruiters waiting, by `lastMessageAt`, newest first.
- `top`: the top 5 open Opportunities.
- `open`: how many Opportunities are open.
- `lateReplies`: how many Recruiters are in Late Replies.

### `GET /api/career/opportunities`

`200`: every open Opportunity, top first, as the lists show them.

### `GET /api/career/opportunities/{id}`

`200`: everything about the Opportunity, open or closed. `404` when no Opportunity has the id.

```json
{
  "id": "6740…",
  "role": "Backend Engineer",
  "stage": "in_process",
  "outcome": null,
  "interest": "high",
  "fitNote": "Go and payments: close to what the Owner does now.",
  "details": "Ledger team.",
  "pay": { "firstStated": { "min": 9000000, "max": 12000000 }, "offered": null, "bonusIncluded": true },
  "conditions": { "remote": "hybrid", "officeDays": 2, "officeLocation": "Shibuya", "workingLanguage": "en" },
  "perks": ["Housing allowance"],
  "steps": [
    { "id": "6743…", "kind": "Casual interview", "date": "2026-11-10", "time": "18:00", "notes": "", "next": false },
    { "id": "6744…", "kind": "Final interview", "date": "2026-11-27", "time": null, "notes": "", "next": true }
  ],
  "recruiter": { "id": "6742…", "name": "Tanaka Yui" },
  "company": { "id": "6741…", "name": "Kumo Pay", "linkedInUrl": "", "websiteUrl": "https://kumopay.example.com", "brief": "Payments for small shops in Japan, about 200 people." }
}
```

- `outcome`: how a closed one ended, otherwise `null`.
- `pay`, `conditions`: as the Tools answer them (below).
- `steps`: by date, then time. `next` marks the next Step, if one is left.
- `recruiter`: who brought it, or `null`.
- `company`: its Company, with the Company Brief as `brief`.

### `GET /api/career/recruiters/{id}`

`200`: the Recruiter and each of their Conversations, the one with the newest message first. `404` when no Recruiter has the id.

```json
{
  "id": "6742…",
  "name": "Tanaka Yui",
  "profileUrl": "https://www.linkedin.com/in/tanaka-yui/",
  "kind": "agency",
  "organisation": "Hays Japan",
  "conversations": [
    {
      "id": "6745…",
      "url": "https://www.linkedin.com/messaging/thread/2-tanaka/",
      "messages": [{ "id": "m7", "from": "other", "sentAt": "2026-11-16T05:00:00Z", "text": "Are you free for a call this week?" }],
      "suggestedReply": {
        "text": "Yes! Thursday evening works for me.",
        "language": "en",
        "kind": "answer",
        "notes": [{ "wrote": "Sounds interesting.", "natural": "That sounds interesting!", "why": "warmer" }],
        "markedRepliedAt": null
      }
    }
  ]
}
```

- `url`: the Conversation on LinkedIn.
- `messages`: its last 5, by when they were sent, oldest first. `from` is `owner` or `other`.
- `suggestedReply`: `null`, or its text, language (`ja` / `en`), kind (`answer` / `follow_up` / `late`), its language notes on the Owner's last message, and `markedRepliedAt`: when the Owner marked it replied, or `null`. A marked one is still shown here, so the page can offer Undo.

### `GET /api/career/late-replies`

`200`: the Recruiters in Late Replies, by `lastMessageAt`, oldest first.

### Marking a Suggested Reply replied

A Suggested Reply leaves the lists on its own only once a Sync brings the Owner's own message. The Owner may mark it **replied** sooner, once they've sent it in LinkedIn. The Suggested Reply then gets a `markedRepliedAt`:

- It is left out of the waiting Recruiters (`GET /api/career` and the Home Card's `waiting`), Late Replies and the Career screen's `lateReplies`, and the "Recruiters waiting" Notifications, Looking or not.
- It stays on the Recruiter's page, with `markedRepliedAt`.
- A Sync that brings an Owner message newer than the one it answers removes it, as any Suggested Reply. When the Recruiter has written again since, that Sync also queues a `reply` Item.
- `career_set_suggested_reply` replaces it with one that isn't marked.
- The tick's rules: it never turns `late`, and for the follow-up it counts as the Owner's last message, sent at `markedRepliedAt` (see [The tick's rules](#the-ticks-rules)).

#### `POST /api/career/conversations/{id}/replied`

Marks the Conversation's current Suggested Reply replied, now. `204`. Marking it again keeps the first time. `404` when no Conversation has the id. `409` when it has no Suggested Reply.

#### `DELETE /api/career/conversations/{id}/replied`

Undo: clears the mark. A `follow_up` Item the tick queued for the mark and no run has written yet is dropped with it: the Owner hasn't sent the reply after all. `204`, also when nothing was marked. `404` when no Conversation has the id.

### Paths the web app uses

These are the web app's own paths, for Notifications to open (#91) and nando-web to serve:

| Path | Shows |
| --- | --- |
| `/career` | the Career screen |
| `/career/recruiters/{id}` | a Recruiter's page |
| `/career/opportunities` | every open Opportunity |
| `/career/opportunities/{id}` | an Opportunity's page |
| `/career/late` | Late Replies |

## Syncs

A Sync is one pass in which the Mac's script picks up the Owner's new LinkedIn messages. Nando, not the script, decides when one is due:

- not before 07:00 in the Owner Time Zone;
- at most 2 on a Day;
- at least 6 hours after the last.

Only Syncs that read LinkedIn count. One that reports a problem doesn't. A Sync sent in batches counts once however many batches it took, as long as each batch after the first names the Sync the first started (see `career_record_sync`).

Each LinkedIn thread is one Conversation (collection `conversations`, unique on the thread ID), holding a copy of its messages. A Conversation a Sync brings is unsorted until the cloud run (below) files it under a Recruiter or ignores it. An ignored thread keeps only its thread ID: later Syncs drop its messages.

A Sync that brings a message from the Owner newer than the message a Conversation's Suggested Reply answers removes that Suggested Reply, marked replied or not: the Owner has replied themselves.

## The run's work

What a Sync brings waits as Items (collection `work`) for a cloud Claude run, which follows `skills/career/career-sync` ([ADR 0006](../../docs/adr/0006-word-requests-through-a-scheduled-task.md), [ADR 0007](../../docs/adr/0007-linkedin-synced-from-the-owners-mac.md)). A Conversation has at most one open Item of each kind:

| Kind | Queued when | Done by |
| --- | --- | --- |
| `sort` | a Sync stores messages in a Conversation not filed yet | `career_ignore_thread` or `career_file_conversation` |
| `reply` | a Sync stores a message from the Recruiter in a filed Conversation | `career_set_suggested_reply` |
| `late_reply` | an `answer` or `follow_up` Suggested Reply turns `late` (see The tick's rules) | `career_set_suggested_reply` |
| `follow_up` | an Opportunity waits on its Recruiter for 14 days (see The tick's rules) | `career_set_suggested_reply` |

`career_list_work` claims up to 5 Items at a time, oldest first, each for 15 minutes. A claim that lapses lets another run take the Item. After 3 claims with no outcome, the Item is set aside as failed, and that is logged.

### Waking the run

Nando wakes the run by calling the `/fire` API trigger of a cloud scheduled task (`CAREER_SYNC_TRIGGER_URL` and `CAREER_SYNC_TRIGGER_TOKEN`), when a Sync stores new messages and at each tick while an Item waits. It follows the same rules as Japanese's Word Requests, shared through Core's `utils`:

- It fires only while an Item is claimable, and never while a run holds a claim.
- A fire is in flight until its run claims or does an Item, for up to 10 minutes: no Sync and no tick starts a second run meanwhile.
- A run that claimed or did an Item in the last 2 minutes may list the work again, so nothing fires until it stops.
- A trigger that fails is logged and fails nothing. The next Sync or tick tries again.

Without both variables, Nando never wakes the run. The last fire and the run's activity are kept in `scheduled_task`, one document.

## The tick's rules

At every tick (Core's `/internal/tick`, every 5 minutes), before it sets aside failed Items and wakes the run, the Module applies these rules by the clock. None needs a model. What they queue wakes the run under the rules above, at the same tick.

- **Faded.** An open Opportunity closes with Outcome `faded` once 30 days have passed with no new message (from either side) in its Recruiter's Conversations and no change to it through the Tools. One with no Recruiter counts only changes through the Tools. It keeps the Stage it had, for `career_reopen_opportunity`, which counts as a change.
- **Late Reply.** A Suggested Reply of kind `answer` or `follow_up` that answers a message more than 30 days old becomes kind `late`, and a `late_reply` Item is queued for the run to rewrite it from the late templates. A reply the run rewrote in the meantime is left alone, and so is one the Owner marked replied.
- **Follow-up.** For an Opportunity in `in_process` or `offer`, each of its Recruiter's Conversations whose last message is the Owner's, at least 14 days old, with no Suggested Reply, gets a `follow_up` Item. A Suggested Reply the Owner marked replied counts as the Owner's last message, sent at `markedRepliedAt`, until a Sync brings the real one; the run's follow-up then answers `markedRepliedAt` and replaces it. A message gets one at most, ever: not again if no run writes it and it is set aside, nor when a Sync later brings the message a mark stood for.

## Notifications

Core asks the Module at every tick (see [Core's API](../core/API.md)), and sends each ID once.

| When | Title / body | Opens | ID |
| --- | --- | --- | --- |
| **Looking**, every Item a Sync queued is done or set aside, and an `answer` Suggested Reply now answers a message it brought, within 48 hours of the Sync | "Recruiters waiting" / "N Recruiters are waiting for your reply" | `/career` | `career:replies:<sync id>` |
| **Not Looking**, Monday and Thursday in the 19:00 slot, when any Recruiter is waiting | the same | `/career` | `career:replies:<YYYY-MM-DD>` |
| The 20:00 slot, for each Step on the next Day in an open Opportunity | "Tomorrow: \<Step kind\>" / "\<Company\>, \<HH:MM\>", or only the Company with no time | `/career/opportunities/{id}` | `career:step:<opportunity id>:<step id>:<YYYY-MM-DD>` |
| A Sync reported `session_expired`: once for the first such report since the last Sync that read LinkedIn, as the Mac reports it again every hour, and only within 7 days of it | "Log in to LinkedIn again" / "Open the Mac and run the Sync's login" | `/career` | `career:sync-problem:<sync id>` |
| No Sync has read LinkedIn for 36 hours, sent within the 7 days after that (a Sync that reported a problem doesn't count) | "No LinkedIn Sync since \<weekday\>" / "Check that the Mac is on and its Sync job is loaded" | `/career` | `career:no-sync:<last sync id>` |

A Recruiter is waiting when one of their Conversations has a Suggested Reply of kind `answer` or `follow_up` to a message at most 30 days old, not marked replied. A Sync whose only answers are marked replied causes none. Late Replies never cause a Notification, nor a reply about to turn late: Core sends the tick's Notifications before the rules run. The Sync Notifications stop after 7 days, well within the 30 days Core remembers a sent ID, so none is sent twice. Once a Sync reads LinkedIn, the "no Sync" Notification can fire again only after a later 36-hour gap. Before the first Sync, there is none.

## Tools

Core serves these to every Connector at `/mcp` (see [Core's API](../core/API.md)). Arguments and answers are JSON. Times are RFC 3339. A refused call is a Tool error that says what is wrong, and changes nothing.

### The shapes they answer

- **Conversation**: `{"id", "threadId", "url", "person": {"name", "profileUrl"}, "messages": [{"id", "from", "sentAt", "text"}], "ignored", "recruiterId", "suggestedReply"}`. `recruiterId` is `null` until it is filed. `suggestedReply` is `null` or `{"text", "language", "kind", "answers", "notes": [{"wrote", "natural", "why"}], "writtenAt", "markedRepliedAt"}`, where `answers` is the `sentAt` of the message it answers, and `markedRepliedAt` is when the Owner marked it replied, or `null`: the Owner already sent it in LinkedIn.
- **Recruiter**: `{"id", "name", "profileUrl", "kind", "organisation"}`, where `kind` is `agency` or `in_house`. A Lead is worked out from their Opportunities, never stored.
- **Opportunity in short**: `{"id", "company": {"id", "name"}, "role", "recruiterId", "stage", "outcome", "interest"}`.
- **Opportunity**: the short shape and `"fitNote"`, `"details"`, `"pay": {"firstStated": {"min", "max"}, "offered": {"min", "max"}, "bonusIncluded"}`, `"conditions": {"remote", "officeDays", "officeLocation", "workingLanguage"}`, `"perks": []`, `"steps": [{"id", "kind", "date", "time", "notes"}]`, `"createdAt"`, `"updatedAt"`. Salaries are annual, in JPY. `remote` is `remote`, `hybrid` or `office`. `workingLanguage` is `ja`, `en` or `mixed`. Steps are by date then time, with dates as `YYYY-MM-DD` and times as `HH:MM` (or `null`), in the Owner Time Zone.
- **Company**: `{"id", "name", "linkedInUrl", "websiteUrl", "brief", "notes", "opportunities": [Opportunity in short]}`. `brief` is the Company Brief.

### Look up (`readOnlyHint`)

| Tool | Arguments | What it does |
| --- | --- | --- |
| `career_get_settings` | none | Answers `{"looking": bool}` |
| `career_sync_status` | none | For the Mac's script, before it reads LinkedIn. Answers `{"due": bool, "since": "2025-01-01", "threads": [{"threadId", "newest", "ignored"}]}`. `due` follows the rules under Syncs. `since` is the oldest Day whose messages a Sync reads. `threads` lists every thread Nando holds, with when its newest message was sent, so that the script opens only threads with newer ones and never sends an ignored thread's messages |
| `career_search` | `query` | Recruiters by name or organisation, Companies by name, and Opportunities by role or Company name, open or closed, ignoring case. Each Recruiter comes with their `conversations`, `[{"id", "hasSuggestedReply"}]`, so that a chat can read one with `career_get_conversation`, and the `opportunities` they brought, in short (`[]` for a Lead). Answers `{"recruiters", "companies": [{"id", "name"}], "opportunities": [Opportunity in short]}` |
| `career_get_conversation` | `id` | Answers the Conversation |
| `career_get_opportunity` | `id` | Answers the Opportunity, open or closed |
| `career_get_company` | `id` | Answers the Company |
| `career_get_profile` | none | The Career Profile, one Markdown document, `""` until first saved. Answers `{"markdown"}`. The Tools are the only way to read it |

### Write

| Tool | Arguments | What it does |
| --- | --- | --- |
| `career_record_sync` | `threads`; `sync` on every batch after a Sync's first; `problem` only when the script couldn't read LinkedIn | Records a Sync. Each thread is `{"threadId", "url", "person": {"name", "profileUrl"}, "messages": [{"id", "from", "sentAt", "text"}]}`, where `from` is `"owner"` or `"other"` and `id` is LinkedIn's message id, if the reader gave one. Each thread's new messages are added to its Conversation, which is made if new, and queue the run's work. An ignored thread's messages are dropped. A message Nando already holds is skipped: the same `id`, or when either has none, the same `from`, `sentAt` and `text`. `problem` is `{"kind": "session_expired"}` when LinkedIn wants the Owner to log in again on the Mac, or `{"kind": "other", "text"}`. When it stores new messages, it wakes the run. A Sync sent in batches is one Sync: the first batch is sent without `sync`, and every later one with `sync` set to the id the first was answered with, which adds its threads to that Sync and answers the same id. `sync` is refused when Nando holds no such Sync, when that Sync reported a problem, when it started more than 10 minutes ago (a new run starts a Sync of its own), or together with `problem`, which is always a Sync of its own. Answers `{"sync": id, "stored": how many new messages were stored}` |
| `career_list_work` | none | For the run: claims up to 5 Items for 15 minutes, of the kinds under The run's work. Answers `{"items": [{"id", "kind", "conversation", "recruiter"}], "looking", "profile", "liveSince"}`. `recruiter` is `null` for an unfiled Conversation, or the Recruiter with their other `conversations` and their `opportunities` (in short). `profile` is the Career Profile. `liveSince` is the time of the first Sync that read LinkedIn: nothing before it becomes an Opportunity |
| `career_ignore_thread` | `conversationId` | The thread isn't about a role: its messages, person, URL, Recruiter and Suggested Reply are deleted, and only its thread ID stays, marked ignored. Does all its open Items. Answers the Conversation |
| `career_file_conversation` | `conversationId`, and `recruiterId` or `recruiter`: `{"name", "profileUrl", "kind", "organisation"}` | Files the Conversation under an existing Recruiter, or a new one. An ignored Conversation can't be filed. Does its `sort` Item. Answers `{"conversation", "recruiter"}` |
| `career_set_suggested_reply` | `conversationId`, `text`, `language` (`ja` / `en`), `kind` (`answer` / `follow_up` / `late`), `answers`, `notes` (up to 3 `{"wrote", "natural", "why"}`) | Replaces the Conversation's Suggested Reply. `answers` is the `sentAt` of one of its messages: for an `answer` or a `late` one, the Recruiter's newest. A `follow_up` may instead answer the current Suggested Reply's `markedRepliedAt`. The new one isn't marked replied. Does its `reply`, `late_reply` or `follow_up` Item. Answers the Conversation |
| `career_save_opportunity` | `id` to update, or none to create; `company` (`{"id"}`, or `{"name"}` for a new one), `role`, `recruiterId` (`null` for none), `stage`, `outcome`, `interest`, `fitNote`, `details`, `pay`, `conditions`, `perks` | Creates an Opportunity, which needs `company` and `role` and starts `introduced` with `normal` Interest, or updates one: only the fields sent change, and `pay` and `conditions` are replaced whole. A Company `name` Nando already holds, ignoring case, is that Company. The Stage (`introduced`, `in_process`, `offer`, `closed`) may skip ahead but never go back. `closed` needs an Outcome (`accepted`, `declined`, `withdrawn`, `rejected`, `faded`), and only a closed one has one. A closed Opportunity's Outcome may be corrected, but never to `faded`, which only one that went quiet while open gets. Answers the Opportunity |
| `career_reopen_opportunity` | `id` | Only a `faded` Opportunity: back to the Stage it had when it closed, with no Outcome. Answers the Opportunity |
| `career_save_step` | `opportunityId`, `stepId` to replace one, `kind`, `date`, `time`, `notes` | Adds or replaces a Step. Answers the Opportunity |
| `career_save_company` | `id` to update, or none to create; `name`, `linkedInUrl`, `websiteUrl`, `brief`, `notes` | Creates a Company, which needs a name no Company has, or updates one: only the fields sent change. Answers the Company |
| `career_save_profile` | `markdown` | Replaces the whole Career Profile. Answers `{"markdown"}` |

### Delete (`destructiveHint`)

These can't be undone. They are for the Owner's mistakes in chat, never for the run.

| Tool | Arguments | What it does |
| --- | --- | --- |
| `career_delete_step` | `opportunityId`, `stepId` | Deletes the Step. Answers the Opportunity |
| `career_delete_opportunity` | `id` | Deletes the Opportunity and its Steps. Answers `{"deleted": id}` |
