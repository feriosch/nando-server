# Career Sync

The script on the Owner's MacBook that hands their new LinkedIn messages to Nando ([ADR 0007](../../docs/adr/0007-linkedin-synced-from-the-owners-mac.md)). It calls no model. It reads LinkedIn with [`stickerdaniel/linkedin-mcp-server`](https://github.com/stickerdaniel/linkedin-mcp-server) (the reader), pinned to `mcp-server-linkedin==4.30.0`, and calls Nando's Tools as a Connector.

## How it works

`launchd` runs `sync.py` at minute 0 of every hour, once when the Owner logs in, and once on wake for the hours the Mac slept through. Each run:

1. Exits quietly if Nando can't be reached. The next hour tries again.
2. Asks Nando `career_sync_status`. Nando decides when a Sync is due: at most twice a Day, 6 hours apart, not before 07:00. Nearly every run stops here, without starting the reader.
3. Starts the reader over stdio and calls `get_session_status`. With no LinkedIn session saved, it tells Nando `session_expired` and stops.
4. Calls `get_inbox` for the 50 most recent threads. If LinkedIn turns the session away, it tells Nando `session_expired` and stops.
5. Opens the threads with `get_conversation`, most recent first, at most 30, waiting 3–10 s between them. It skips threads Nando marked ignored, and stops at the first thread with nothing newer than what Nando holds: the inbox is in order of activity, so no later thread has anything new either. It keeps only messages since 2025-01-01.
6. Sends the new messages with `career_record_sync`, 10 threads at a time. Every batch after the first names, as `sync`, the Sync the first batch started, so that Nando counts them as one. A Sync with nothing new is recorded too, so that it counts.

If the reader fails or stops answering, LinkedIn opens another page than the thread asked for, or a thread's page can't be read, the script tells Nando with `problem: other` and the reason, after sending what it had read. Such a Sync doesn't count, so the next hour tries again.

The script calls only `get_session_status`, `get_inbox` and `get_conversation`. An allow-list in `sync.py` refuses any other reader Tool before it reaches the reader.

## Installing

On the MacBook, with [`uv`](https://docs.astral.sh/uv/) installed:

1. Log in to LinkedIn with the reader (below). This also installs the reader and its browser.
2. Approve the script as a Connector with `--connect` (below).
3. Install the LaunchAgent:

   ```bash
   ./install.sh
   ```

   It fills in the paths in `app.nando.career-sync.plist`, copies it to `~/Library/LaunchAgents/` and loads it. It runs once straight away, then every hour. Run it again after moving the folder or updating `uv`. `./uninstall.sh` removes it.

## Logging in to LinkedIn

```bash
uvx mcp-server-linkedin==4.30.0 --login
```

A browser window opens. Sign in to LinkedIn there. The session is saved under `~/.linkedin-mcp/` and stays on the Mac. To see whether LinkedIn still accepts it:

```bash
uvx mcp-server-linkedin==4.30.0 --status
```

When the session expires, the next Sync tells Nando and the Owner logs in again the same way. The reader also opens a LinkedIn login window by itself when it finds the session expired. Signing in there works too.

## `--connect`

```bash
uv run --script sync.py --connect
```

It registers the script with Nando, opens the browser on Nando's consent page, where the Owner approves it with a Passkey, and waits for the browser to come back to `http://127.0.0.1:8765/callback`. The tokens are kept in the login Keychain (service `app.nando.career-sync`), or, if `launchd` can't reach the Keychain, in `~/.nando/career-sync.json`, readable only by the Owner. The access token lasts an hour. Each hourly run is a new process, so before refreshing it, the script reads Nando's OAuth metadata (`/.well-known/oauth-authorization-server`) for the token endpoint (`/oauth/token`): the MCP SDK reads it only on the way to the browser. If Nando can't serve it, the run logs so, keeps the tokens, and the next hour tries again. The Connector appears in Nando's Settings like any other. Removing it there cuts the script off, and the log then says to run `--connect` again.

## Keeping Claude Code away from the reader's writing Tools

If the reader is ever added to Claude Code on the Mac, deny its writing Tools in `~/.claude/settings.json`. `linkedin` is the name the reader was given in Claude Code's MCP settings. Use that name if it's different:

```json
{
  "permissions": {
    "deny": [
      "mcp__linkedin__send_message",
      "mcp__linkedin__reply_to_conversation",
      "mcp__linkedin__archive_conversation",
      "mcp__linkedin__connect_with_person",
      "mcp__linkedin__dismiss_job"
    ]
  }
}
```

## Reading the log

`launchd` writes the script's output to `~/Library/Logs/nando-career-sync.log`. Open it in Console.app, or:

```bash
tail -n 50 ~/Library/Logs/nando-career-sync.log
```

Most runs log `No Sync is due.` A Sync logs how many threads it opened and sent, and how many new messages Nando stored. `WARNING` lines are problems also sent to Nando. `ERROR` lines are runs that failed before reaching Nando, such as `run sync.py --connect`. To run one by hand: `launchctl kickstart gui/$(id -u)/app.nando.career-sync`.

## The reader, as checked at 4.30.0

Read from the source of `mcp-server-linkedin` 4.30.0. When updating the reader, read its source again, change `READER` in `sync.py` and the commands above, and update this section.

- **Its Tools.** It offers 25 Tools. Five of them write: `send_message`, `reply_to_conversation`, `archive_conversation`, `connect_with_person` and `dismiss_job`.
- **`get_session_status`** reads only the files on the Mac, never LinkedIn. It answers `{"session": {"stored", "profile", "cookies"}, ...}`. An expired session still reads `stored: true`. Only a Tool that opens LinkedIn finds out.
- **An expired session.** `get_inbox` and `get_conversation` then answer with an error whose `_meta["linkedin.dev/error"].code` is `auth_required`. The reader keeps its error codes stable, unlike its messages. It also moves the dead session aside, so `stored` is `false` from then on, and opens a login window.
- **`get_inbox`** takes only `limit`, 1–50, and has no paging. It lists the most recent threads, after at most 5 scrolls, so **it can't reach back to 2025-01-01** on a busy inbox. A Sync reads what the 50 most recent threads hold. It answers `{"url", "sections": {"inbox": text}, "references": {"inbox": [...]}}`. Each thread is a reference `{"kind": "conversation", "url": "/messaging/thread/<id>/", "text": "<the other person's name>"}`. No reference gives a date. To find thread IDs, it clicks each row, which marks threads as read on LinkedIn.
- **`get_conversation(thread_id)`** doesn't check that LinkedIn opened that thread, so the script compares the answer's `url` with it. It answers `{"url", "sections": {"conversation": text}, "references": {"conversation": [...]}}`. The text is the page's text, from the line after the conversation's header to the compose box. The references are links to the people in it, `{"kind": "person", "url": "/in/<id>/", "text": ...}`, the Owner included. It gives **no message IDs**, so the script sends none, and Nando tells messages apart by sender, time and text. It scrolls up only three times, so a long thread's oldest messages may be missing. Opening a thread marks it as read.
- **Thread IDs** look like `2-<base64url>==`. A thread's URL is `https://www.linkedin.com/messaging/thread/<id>/`, with `=` written as `%3D`.
- **Rate limits.** The reader doesn't pace itself. A rate-limited page gives the code `rate_limited`.
- **Started for a Sync** with `--no-auto-import --login-inline-wait 0`, so it never imports a session from the Mac's browsers or waits on a login, and with `LINKEDIN_MCP_REMOTE_TOOLS=off` and `LINKEDIN_MCP_CHECK_FOR_UPDATES=off`, so it fetches no remote Tool catalog and adds no update notes to answers.

### Reading a conversation's text

`parse_conversation` follows LinkedIn's messaging page as the first Sync found it, on 2026-11-16 (#94). `test_parse.py` copies its layout from real threads, with names and text made up:

```
Visit my website
OCT 13
Jane Recruiter sent the following messages at 4:43 PM
View Jane’s profile
Jane Recruiter  (She/Her)  4:43 PM

Would you have any feedback?

OCT 15
Sam Owner sent the following message at 10:15 AM
View Sam’s profile
Sam Owner   10:15 AM

Hello Jane!
```

- **Each message** starts with its own sender line: the name, the pronouns if they show them, and the time, two spaces or more apart. It comes right under a link to their profile (`View <first name>’s profile`). A message under the other person's name, as the inbox gives it, is theirs. Any other is the Owner's.
- **A day heading**, in capitals, comes when the Day changes: `TODAY`, `YESTERDAY`, a weekday, `OCT 13` (this year, or last year if that is still to come), `OCT 23, 2025`. A message saying just "Monday" stays a message.
- **A group heading** for screen readers, `<name> sent the following message(s) at <time>`, comes only when the sender changes. A follow-up on a later Day has just a day heading and its own sender line.
- **Under a message**, the Owner's reaction (`Remove reaction`, the emoji, a count) isn't part of its text. An attachment's name, size and `Download` are.
- The time is in the Mac's time zone. Messages above the first day heading are left out, as their Day is unknown.

**Ads** are listed among the threads too: Sponsored Messages (InMail ads), whose text starts with `Sponsored`, and LinkedIn's own offers of Premium, which start with `LinkedIn Offer`. A Sync opens one, skips it and carries on, since it says nothing about the threads after it. It isn't sent to Nando.

**If LinkedIn changes the page**, the script reads no messages from a thread and tells Nando `problem: other`, "couldn't read the messages of LinkedIn thread …", instead of sending nothing. Then compare a thread's text (`get_conversation` from Claude Code, with the deny rules above) with the tests in `test_parse.py`, and fix `parse_conversation` with a test first.

## Tests

```bash
uv run --project scripts/career-sync pytest scripts/career-sync
```

They run `run_sync` against fake MCP servers for Nando and the reader (`fakes.py`), whose answers follow the reader at 4.30.0 as above. `test_auth.py` runs the Connector's OAuth against a fake of Nando's endpoints, from `--connect` to a later hourly run that refreshes the access token. `NandoAuth` leans on the internals of `mcp` 2.3.0 (`_initialize`, `_refresh_token` and its context): when updating the SDK, run these tests and read its `oauth2.py` again. CI runs them on PRs that touch this folder.
