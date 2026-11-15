# Career API

The Career Module: the Owner's LinkedIn Conversations with Recruiters, the Opportunities they bring, and whether the Owner is Looking. The script on the Owner's MacBook reads LinkedIn and hands each Sync to Nando through the Tools below ([ADR 0007](../../docs/adr/0007-linkedin-synced-from-the-owners-mac.md)). Every route is under `/api/career/` and, like all of `/api/`, needs a valid Session cookie (see [Core's API](../core/API.md)). Bodies are JSON. Errors are a status code with a short plain-text message.

## Settings

```json
{ "looking": false }
```

- `looking`: whether the Owner is Looking for a new role right now. It sets the tone of every Suggested Reply. `false` until the Owner first changes it.

### `GET /api/career/settings`

`200`: the settings.

### `PUT /api/career/settings`

Replaces the settings. `204`. `400` when `looking` is missing or isn't `true` or `false`.

## Syncs

A Sync is one pass in which the Mac's script picks up the Owner's new LinkedIn messages. Nando, not the script, decides when one is due:

- not before 07:00 in the Owner Time Zone;
- at most 2 on a Day;
- at least 6 hours after the last.

Only Syncs that read LinkedIn count. One that reports a problem doesn't. A Sync sent in batches counts once however many batches it took, as long as each batch after the first names the Sync the first started (see `career_record_sync`).

Each LinkedIn thread is one Conversation (collection `conversations`, unique on the thread ID), holding a copy of its messages. A Conversation isn't filed under a Recruiter yet: Claude does that in the run a Sync wakes, which isn't built yet (#89). That run will also mark threads that aren't about a role as ignored.

## Tools

Core serves these to every Connector at `/mcp` (see [Core's API](../core/API.md)). Arguments and answers are JSON. Times are RFC 3339.

### Look up (`readOnlyHint`)

| Tool | Arguments | What it does |
| --- | --- | --- |
| `career_get_settings` | none | Answers `{"looking": bool}` |
| `career_sync_status` | none | For the Mac's script, before it reads LinkedIn. Answers `{"due": bool, "since": "2025-01-01", "threads": [{"threadId", "newest", "ignored"}]}`. `due` follows the rules under Syncs. `since` is the oldest Day whose messages a Sync reads. `threads` lists every thread Nando holds, with when its newest message was sent, so that the script opens only threads with newer ones and never sends an ignored thread's messages |

### Write

| Tool | Arguments | What it does |
| --- | --- | --- |
| `career_record_sync` | `threads`; `sync` on every batch after a Sync's first; `problem` only when the script couldn't read LinkedIn | Records a Sync. Each thread is `{"threadId", "url", "person": {"name", "profileUrl"}, "messages": [{"id", "from", "sentAt", "text"}]}`, where `from` is `"owner"` or `"other"` and `id` is LinkedIn's message id, if the reader gave one. Each thread's new messages are added to its Conversation, which is made if new. A message Nando already holds is skipped: the same `id`, or when either has none, the same `from`, `sentAt` and `text`. `problem` is `{"kind": "session_expired"}` when LinkedIn wants the Owner to log in again on the Mac, or `{"kind": "other", "text"}`. A Sync sent in batches is one Sync: the first batch is sent without `sync`, and every later one with `sync` set to the id the first was answered with, which adds its threads to that Sync and answers the same id. `sync` is refused when Nando holds no such Sync, when that Sync reported a problem, when it started more than 10 minutes ago (a new run starts a Sync of its own), or together with `problem`, which is always a Sync of its own. Answers `{"sync": id, "stored": how many new messages were stored}` |
