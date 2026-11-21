# Core API

Identity: Passkeys, Enrollment Codes and Sessions ([ADR 0002](../../docs/adr/0002-passkey-only-identity.md)).
Settings and body measurements: the Owner Time Zone, height and Weigh-ins ([ADR 0003](../../docs/adr/0003-body-measurements-in-core.md)).
Notifications: sent in the Messenger at the scheduler's tick ([ADR 0001](../../docs/adr/0001-cloud-run-hosting.md), [ADR 0008](../../docs/adr/0008-assistants-on-telegram.md)).
Connectors: Nando as its own OAuth 2.1 authorization server, the consent page's calls, the Tools at `/mcp`, and the Owner's list of Connectors ([ADR 0004](../../docs/adr/0004-connectors-over-oauth.md)).

The web app calls the API with `credentials: "include"`. Bodies are JSON. Errors are a status code with a short plain-text message. Answers to the origins in `ALLOWED_ORIGINS` expose `Content-Disposition` (`Access-Control-Expose-Headers`), which a browser otherwise hides from another origin, so the web app can name a file it downloads, such as the JLPT deck.

## Signing in

These routes need no Session. Passkeys belong to the relying party `<DOMAIN>` and work from every origin in `ALLOWED_ORIGINS`; a `finish` whose answer comes from any other origin is refused. The server doesn't check that those origins are under `<DOMAIN>`, but browsers use a Passkey only on pages under its relying party ([ADR 0002](../../docs/adr/0002-passkey-only-identity.md)). Each ceremony is a `begin` that returns the options for the browser, then a `finish` that takes what the browser returned. A ceremony can finish only once, within 5 minutes of its `begin`.

Both `finish` routes answer `204`, start a Session and set its cookie:

| Cookie          | Value                                                                                                  |
| --------------- | ------------------------------------------------------------------------------------------------------ |
| `nando_session` | random token; `Domain=<DOMAIN>; Path=/; Max-Age=7776000` (90 days); `HttpOnly; Secure; SameSite=Lax` |

### `POST /auth/register/begin`

Registers a new Passkey. This is the only way in without an existing Passkey.

```json
{ "code": "BY95P-1SQMF" }
```

`code` is an Enrollment Code from `nando-server enroll`. It must be typed exactly as printed, dash and all.

- `200`: `{ "publicKey": PublicKeyCredentialCreationOptionsJSON }`. Pass `publicKey` to `PublicKeyCredential.parseCreationOptionsFromJSON` and then to `navigator.credentials.create({ publicKey })`. The Passkey is discoverable and needs user verification (Face ID). The options exclude the Owner's existing Passkeys, so a device that already has one refuses with `InvalidStateError` instead of silently replacing it.
- `401`: the code is unknown, used or expired.

### `POST /auth/register/finish`

Body: the `PublicKeyCredential` from `navigator.credentials.create`, as `credential.toJSON()`.

- `204`: the Passkey is stored and a Session started. The Enrollment Code is used up only now, so a cancelled Face ID prompt doesn't waste it.
- `400`: the body isn't a `PublicKeyCredential`, or its challenge was for a sign-in.
- `401`: the challenge is unknown, used or expired; the Passkey couldn't be verified, for example because it answered from an origin not in `ALLOWED_ORIGINS`; or the Enrollment Code expired or was used in the meantime.

### `POST /auth/signin/begin`

No body.

- `200`: `{ "publicKey": PublicKeyCredentialRequestOptionsJSON }`. It lists no credentials, so the Owner picks any of their Passkeys. Pass `publicKey` to `PublicKeyCredential.parseRequestOptionsFromJSON` and then to `navigator.credentials.get({ publicKey })`.

### `POST /auth/signin/finish`

Body: the `PublicKeyCredential` from `navigator.credentials.get`, as `credential.toJSON()`.

- `204`: a Session is started.
- `400`: the body isn't a `PublicKeyCredential`, or its challenge was for a registration.
- `401`: the challenge is unknown, used or expired; the Passkey is unknown (for example, deleted) or couldn't be verified, for example because it answered from an origin not in `ALLOWED_ORIGINS`; its sign count didn't go up since its last sign-in, which suggests it was copied; or another sign-in with the same Passkey finished first. Synced Passkeys always report a sign count of 0, which is never rejected; this mainly protects hardware security keys. Every refused Passkey gets the same message, so a copied one isn't told why; the server logs the reason.

## Everything under `/api/`

Every `/api/...` route, Core's and every Module's, needs a valid Session cookie and answers `401` without one. A Session lasts 90 days from its last renewal. A Session in use is renewed for another 90 days at most once a day, and the response that renews it sets the cookie again with a fresh `Max-Age`.

### `GET /api/sessions`

`200`: every Session, oldest first.

```json
[
  {
    "id": "HJ6Q…",
    "userAgent": "Mozilla/5.0 (iPhone; …)",
    "createdAt": "2026-10-22T10:00:00Z",
    "expiresAt": "2027-01-20T10:00:00Z",
    "current": true
  }
]
```

`userAgent` is the browser's from when it signed in. `current` marks the Session making this request.

### `DELETE /api/sessions/{id}`

`204`: deletes that Session, which signs its browser out; that browser's next request gets `401`. Deleting the `current` Session also deletes this browser's cookie (`Max-Age=0`). Deleting a Session that doesn't exist also answers `204`.

### `GET /api/passkeys`

`200`: the Owner's Passkeys, oldest first.

```json
[{ "id": "q1w2…", "createdAt": "2026-10-22T10:00:00Z" }]
```

`id` is the credential ID, base64url without padding (the `id` of the browser's `PublicKeyCredential`).

### `DELETE /api/passkeys/{id}`

`204`: that Passkey can no longer sign in. Sessions it already started last until they end or are deleted. Deleting the last Passkey is allowed; `nando-server enroll` is the way back in.

`400`: `id` isn't base64url.

## Settings

Core's own Settings Section.

### `GET /api/settings`

`200`: the settings. Until the Owner first saves them, they are `"UTC"`, no height, and the reminder off, set for Monday at 08:00.

```json
{
  "timeZone": "UTC",
  "heightCm": 175,
  "weighInReminder": { "enabled": false, "weekday": "monday", "time": "08:00" }
}
```

- `timeZone`: the Owner Time Zone, an IANA name such as `"Asia/Tokyo"`. Every Day, and every clock time such as the reminder's, is in this zone. Changing it never moves stored data: a Weigh-in stays in its week. Only which Day is today changes.
- `heightCm`: a whole number of cm, or `null` until it is set.
- `weighInReminder`: the weekly Notification reminding the Owner to weigh in. `weekday` is `"monday"` to `"sunday"`, and `time` is `"HH:MM"` on the hour or half hour (`"00:00"` to `"23:30"`).

### `PUT /api/settings`

Body: all of the settings, as `GET` answers them.

- `204`: saved.
- `400`: the body isn't settings, or one is invalid: an unknown time zone, a height outside 1 to 300 cm or not a whole number, an unknown weekday, or a time that isn't on the hour or half hour. The message says which.

## Weigh-ins

A Weigh-in is the Owner's weight for one week, Monday to Sunday in the Owner Time Zone, in kg to 1 decimal. Each week has at most one. A week is named by the Monday it starts on, as `YYYY-MM-DD`.

Only this week's and last week's Weigh-ins can be logged, replaced or deleted. Older weeks are read-only.

### `GET /api/weigh-ins`

`200`: every Weigh-in, newest week first.

```json
[
  { "week": "2026-10-19", "kg": 72.4 },
  { "week": "2026-10-12", "kg": 72.8 }
]
```

### `PUT /api/weigh-ins/{week}`

Logs the Weigh-in for `week`, replacing any it already has.

```json
{ "kg": 72.4 }
```

- `204`: saved.
- `400`: `week` isn't a Monday written `YYYY-MM-DD`, or `kg` is missing, not more than 0 and at most 500, or has more than 1 decimal.
- `409`: `week` is neither this week nor last week.

### `DELETE /api/weigh-ins/{week}`

- `204`: that week has no Weigh-in now. This is also the answer if it had none.
- `400`: `week` isn't a Monday written `YYYY-MM-DD`.
- `409`: `week` is neither this week nor last week.

## Body Home Card

### `GET /api/body`

`200`: what the Body Home Card shows.

```json
{
  "latest": { "week": "2026-10-19", "kg": 72.4 },
  "bmi": 23.6,
  "change": -0.4
}
```

- `latest`: the newest week's Weigh-in, or `null` if there are none. Then `bmi` and `change` are `null` too.
- `bmi`: `latest`'s body mass index to 1 decimal, or `null` until the height is set. It is always derived, never stored.
- `change`: `latest`'s kg minus the kg of the week before it, or `null` if that week has no Weigh-in.

## Notifications

Core sends every Notification, for a Module or for itself, in the Messenger whenever some Assistant has a bot ([#128](https://github.com/feriosch/nando-server/issues/128), ADR 0008):
- It goes to the chat of the Assistant its ID's area names, such as the Career Assistant's for `career:…`. When that Assistant has no bot, it goes to the Core Assistant's chat, or, if Core has no bot either, to any Assistant's that has one.
- It is sent as plain text: the title, a line break and the body, leaving out either if it is empty.
- It joins that Assistant's Talk as a message from the Assistant, so the Owner can answer it there. It wakes no run on its own.

Each Notification is sent at most once, even if the scheduler retries a tick. With no bot configured, as in local development, nothing is sent or recorded as sent, and the tick says so. Web Push was retired ([#137](https://github.com/feriosch/nando-server/issues/137)): Telegram is the only channel.

Core's own Notification is the Weigh-in reminder, "Weigh-in" / "Time to log this week's weight", sent once a week at the `weighInReminder`'s weekday and time while it is `enabled`.

### `POST /internal/tick`

Called only by Cloud Scheduler, every 5 minutes. It needs no Session, but its `Authorization: Bearer` token must be a Google-signed OIDC token for `TICK_AUDIENCE` issued to `SCHEDULER_SERVICE_ACCOUNT`.

At each tick Core asks every Notification provider (Core's own and every Module's) which Notifications are due in the current half hour, the one starting at :00 or :30 in the Owner Time Zone, and sends each one that hasn't been sent before. So the six ticks of a half hour send each of its Notifications once, at the first tick that runs. Every Notification has an ID, such as `calories:evening-summary:2026-10-20`, that Core remembers for 30 days in `sent_notifications`. Then Core runs every Module's tick job, its work to do at every tick, such as the Japanese Module waking Claude for Word Requests still waiting.

- `204`: done. Telegram failing to take a message doesn't fail the tick; that Notification isn't retried.
- `401`: no token, or not the scheduler's.
- `500`: a provider or a tick job failed, or a Notification couldn't be recorded as sent. Everything else was still sent and every other tick job still ran, and Cloud Scheduler's retry won't send a Notification again.

Each tick logs, to Cloud Logging, one line saying what it sent:

- `tick: sent 2 Notifications in Telegram: calories:no-entries:2026-10-20 (sent), calories:evening-summary:2026-10-20 (refused)`: each Notification sent, by its ID. One Telegram refused shows `(refused)`, with the reason logged on its own line (`send Notification <id> in Telegram: …`). It isn't retried.
- `tick: no Notifications due`: nothing was due, or a retried tick found it all sent already, and nothing went wrong.
- `tick: 1 Notification due, but no Assistant has a bot to send them`: with no bot configured, followed by `, 1 error` when there were errors. Nothing is recorded as sent.
- With errors, the line counts them: `tick: no Notifications sent, 1 error`, or `tick: sent 1 Notification in Telegram, 1 error: …`. Each error is logged on its own line, and the tick answers `500`.

Logs never hold what a Notification says: only its ID.

## Connectors

A Connector, such as the Claude app, uses the Tools of Core and every Module over MCP at `https://api.<DOMAIN>/mcp`. It never signs in. Nando is its own OAuth 2.1 authorization server: authorization code with PKCE (`S256` only) and dynamic client registration, for public clients only. The Owner approves each Connector on the web app's consent page with a Passkey, always, even in a browser with a Session ([ADR 0004](../../docs/adr/0004-connectors-over-oauth.md)).

The OAuth routes (`/.well-known/`, `/oauth/`) and `/mcp` are called by the Connector, not the web app. They need no Session and send no cookie. OAuth errors are JSON, `{ "error": "invalid_grant", "error_description": "…" }`, as RFC 6749 sets out.

Every secret Nando issues is random and stored only as a SHA-256 hash, like a Session token:

| Secret             | Lasts                                                                                              |
| ------------------ | -------------------------------------------------------------------------------------------------- |
| Registration       | a day, unless a Connector is approved for it; then until a day after its last Connector is removed |
| Pending request    | 10 minutes, and it is approved or denied only once                                                 |
| Authorization code | 1 minute; any exchange uses it up, even a refused one                                              |
| Access token       | 1 hour                                                                                             |
| Refresh token      | 90 days unused, like a Session; each refresh replaces it                                           |

### `GET /.well-known/oauth-protected-resource`

`200`: `/mcp`'s protected resource metadata (RFC 9728), where a Connector starts.

```json
{ "resource": "https://api.<DOMAIN>/mcp", "authorization_servers": ["https://api.<DOMAIN>"] }
```

### `GET /.well-known/oauth-authorization-server`

`200`: the authorization server metadata (RFC 8414).

```json
{
  "issuer": "https://api.<DOMAIN>",
  "authorization_endpoint": "https://api.<DOMAIN>/oauth/authorize",
  "token_endpoint": "https://api.<DOMAIN>/oauth/token",
  "registration_endpoint": "https://api.<DOMAIN>/oauth/register",
  "response_types_supported": ["code"],
  "grant_types_supported": ["authorization_code", "refresh_token"],
  "code_challenge_methods_supported": ["S256"],
  "token_endpoint_auth_methods_supported": ["none"]
}
```

### `POST /oauth/register`

Dynamic client registration (RFC 7591).

```json
{
  "client_name": "Claude",
  "redirect_uris": ["https://claude.ai/api/mcp/auth_callback"],
  "grant_types": ["authorization_code", "refresh_token"],
  "response_types": ["code"],
  "token_endpoint_auth_method": "none"
}
```

- `201`: the registration as kept: the fields above plus `client_id` and `client_id_issued_at`. It is always a public client, `token_endpoint_auth_method: "none"` with no `client_secret`, whatever was asked. `client_name` defaults to the first redirect URI's host, and is the Connector's name. `grant_types` defaults to `["authorization_code", "refresh_token"]`, since every Connector gets a refresh token, and `response_types` to `["code"]`.
- `400` `invalid_redirect_uri`: no redirect URI, or one that isn't `https:`, or `http:` to `localhost` or `127.0.0.1` on any port (for local apps such as Claude Code). Fragments and user info are refused.
- `400` `invalid_client_metadata`: the body isn't JSON, more than 10 redirect URIs, a name over 100 characters, or a grant or response type other than those above.

### `GET /oauth/authorize`

The Connector sends the Owner's browser here with `response_type=code`, `client_id`, `redirect_uri` (exactly as registered, except that a loopback one may use any port, as RFC 8252 section 7.3 asks for local apps), `code_challenge`, `code_challenge_method=S256` and `state`. The code exchange must repeat the `redirect_uri` used here, port and all.

- `302` to `https://web.<DOMAIN>/connect?request=<id>`, the consent page, with the pending request.
- `400`, plain text: an unknown or expired `client_id`, or a `redirect_uri` it didn't register. These are never redirected, since the redirect URI can't be trusted.
- `302` back to the redirect URI with `error` and `state`: `unsupported_response_type` for a `response_type` other than `code`, and `invalid_request` for no `response_type`, no `code_challenge`, a method other than `S256`, or no `state`.

### `GET /auth/connect/{request}`

For the consent page, which may run in a browser without a Session. `200`: which app asks, the host it returns to, and what Nando's Tools would let it do: one description per Tool provider with Tools, in the order they were added, each worded to follow "It will be able to" and written by its Module (`model.ToolProvider`'s `ToolsDescription`). `[]` when no provider has Tools.

```json
{
  "clientName": "Claude",
  "redirectHost": "claude.ai",
  "toolsDescriptions": ["search your Words and Kanji, add, change, merge and delete your Words, and work through your Word Requests"]
}
```

`404`: the request is unknown, already approved or denied, or expired.

### `POST /auth/connect/{request}/begin`

Starts approving the request with a Passkey. `200`: `{ "publicKey": PublicKeyCredentialRequestOptionsJSON }`, as `POST /auth/signin/begin` answers. `404` as above.

### `POST /auth/connect/{request}/finish`

Body: the `PublicKeyCredential` from `navigator.credentials.get`, as `credential.toJSON()`. It doesn't start a Session.

- `200`: the Connector is created and its registration kept. Send the browser to `redirect`, the Connector's redirect URI with a single-use authorization code.

  ```json
  { "redirect": "https://claude.ai/api/mcp/auth_callback?code=…&state=…" }
  ```

- `400`: the body isn't a `PublicKeyCredential`, or its challenge wasn't begun for approving this request (a sign-in's, or another request's).
- `401`: the challenge is unknown, used or expired, or the Passkey couldn't be verified.
- `404`: the request was approved, denied or expired in the meantime.

An approval's challenge can't sign in either: `POST /auth/signin/finish` answers it `400`.

### `POST /auth/connect/{request}/deny`

`200`: `{ "redirect": "<redirect_uri>?error=access_denied&state=…" }`. The request is gone. `404` as above.

### `POST /oauth/token`

Form-encoded (`application/x-www-form-urlencoded`). Every answer has `Cache-Control: no-store`.

- `grant_type=authorization_code` with `code`, `redirect_uri`, `client_id` and `code_verifier`: the code must be unused and under a minute old, issued to this `client_id` and `redirect_uri`, and SHA-256 of `code_verifier` must match its `code_challenge`.
- `grant_type=refresh_token` with `refresh_token` and `client_id`: the Connector's current refresh token, used within 90 days.

`200`: new tokens. A refresh replaces both the access token and the refresh token.

```json
{ "access_token": "…", "token_type": "Bearer", "expires_in": 3600, "refresh_token": "…" }
```

- `400` `invalid_grant`: the code or refresh token is unknown, used, expired, another client's, or its Connector was removed. A code exchanged again, or a refresh token used again after it was replaced, removes its Connector and every token it got, since it may have been stolen; the Owner then approves the app again. One exception: the refresh token replaced last, sent again by its own client while the tokens that replaced it were never used, gets new tokens once more, since the answer to its refresh was likely lost (a Mac that slept, a dropped connection). The tokens never used are retired, so using them later removes the Connector.
- `400` `invalid_request`: no `grant_type`, or the body isn't a form.
- `400` `unsupported_grant_type`: any other `grant_type`.

### `POST /mcp`

MCP over Streamable HTTP, stateless, answering `application/json`, so each request can reach any instance. It needs `Authorization: Bearer <access token>` (the scheme in any case), never a Session cookie. It serves `tools/list` and `tools/call` for the Tools Core and every Module add. Each Tool says whether it is read-only (`readOnlyHint`) and whether it deletes, merges or moves data (`destructiveHint`). A Tool's result comes back as JSON text, and also as `structuredContent` when it is a JSON object. A Tool that fails comes back with `isError: true` and its message.

`401` without a valid, unexpired access token, with where to start:

```
WWW-Authenticate: Bearer resource_metadata="https://api.<DOMAIN>/.well-known/oauth-protected-resource"
```

A Connector's access token works only here; under `/api/` it gets `401`.

### `GET /api/connectors`

`200`: every Connector, oldest approved first.

```json
[
  {
    "id": "7XQ2…",
    "name": "Claude",
    "approvedAt": "2026-11-10T10:00:00Z",
    "lastUsedAt": "2026-11-10T12:30:00Z"
  }
]
```

`lastUsedAt` is its last call to `/mcp`, or `null` if it has never made one.

### `DELETE /api/connectors/{id}`

`204`: removes the Connector and its tokens. Its next call to `/mcp` gets `401`, and its refresh token `invalid_grant`. If no other Connector uses its registration, the registration expires a day later, so the app can be connected again with the same `client_id` until then. Deleting one that doesn't exist also answers `204`.

## Messenger

The Owner talks to Assistants in Telegram, one bot per Assistant ([ADR 0008](../../docs/adr/0008-assistants-on-telegram.md)). Telegram, not the web app, calls this route. It needs no Session.

At startup, `main` points every Assistant's bot at its webhook with `setWebhook`, for messages only, keeping the updates still waiting. It does so every time, because Telegram never says which secret a webhook already has.

### `POST /messenger/telegram/{assistant}`

One update from the bot of `{assistant}`, such as `career`.

- `401` unless `X-Telegram-Bot-Api-Secret-Token` is the webhook secret, checked first, so that no one without it learns which bots exist.
- `404` if there's no such Assistant, or its bot has no token.
- `500` if the message couldn't be kept. Telegram delivers it again later.
- `200` otherwise, once the update is dealt with, since Telegram re-delivers anything else.

The update is dropped, with nothing kept and nothing sent, when:
- it isn't a message in the Owner's private chat with the bot, from the Owner;
- it is `/start`;
- the bot has already had it (the same `update_id`, remembered for 7 days).

A photo is kept like text, with its caption, if any, as its text ([#127](https://github.com/feriosch/nando-server/issues/127)). Nando keeps only Telegram's `file_id` for the photo's size up to 1280 px on its long side, never the file.

A file of up to 10 MB is kept too, its caption as its text ([#135](https://github.com/feriosch/nando-server/issues/135)), again only by its `file_id`:
- an image sent as a file, of up to 5 MB and in a type Claude can see (JPEG, PNG, GIF or WebP), is kept as a photo, at its full size;
- a PDF (`application/pdf`, or a name ending in `.pdf`) is kept under the message's files, with its name and size.

Otherwise, nothing is kept, caption and all, and the bot answers instead:
- a larger PDF or image: "That file is too large: I can read PDFs of up to 10 MB and images of up to 5 MB.";
- anything else without text, such as a sticker, a voice note, a Word file or a HEIC photo: "I can read text, photos and PDFs, but not other files, stickers or voice notes yet."

Anything else waits in the Assistant's open Talk, which is opened if there is none. The bot reacts to it with 👀 and shows the typing status. If Telegram refuses either, that is logged and the message is still kept. Then Nando wakes that Assistant's runs (below).

### Waking Claude

Every Assistant's runs come from one cloud scheduled task, woken through its API trigger (`MESSENGER_TRIGGER_URL`, `MESSENGER_TRIGGER_TOKEN`). Each Assistant is woken on its own, under ADR 0006's rules:
- Nando fires when a message waits, unless that Assistant's run is in flight (fired in the last 10 minutes, and no run has claimed or answered its Talk since) or working (claimed or answered it in the last 2 minutes, and hasn't stopped).
- The fire's body is `{}`: a run claims whichever Talk is waiting, so two Assistants can run at once.

At each tick, for each Assistant:
1. If 3 claims passed with no reply and the last one lapsed, the waiting messages fail. The bot says "Sorry, something went wrong and I couldn't answer that. Please send it again." and removes their 👀.
2. If the oldest waiting message is 10 minutes old and no run has claimed it, the bot says, once for those messages, "Claude didn't start (maybe the usage limit). I'll keep trying." The 👀 stays. Without a trigger nothing would try, so nothing is said.
3. Nando fires again if messages still wait, under the rules above.

### Tools for the run

Served to every Connector while any bot has a token (ADR 0004). Each one shows the typing status again in the Assistant's chat while messages still wait there.

- **`messenger_claim_talk`** (`assistant` optional): claims the Talk for 15 minutes and returns:
  - `assistant`, and `skill` (`skills/assistants/<assistant>/SKILL.md`);
  - `summary`;
  - `messages` (`from`, `at`, `text`; `note` on a reply whose run left one; `waiting` on those waiting, `failed` on those no run answered, `photos`, each photo's `id`, and `files`, each PDF's `id` and `name`, on those with them);
  - `otherAssistants`, those with a bot.

  Without `assistant`, it takes the Talk whose oldest waiting message is oldest; with it, only that Assistant's. `{}` means nothing waits, and with `assistant` it records that the run stopped. A claim that lapses leaves the Talk claimable again, until its third.
- **`messenger_reply`** (`assistant`, `text`, `note` optional): sends `text` as plain text, split at the last line break that fits Telegram's 4096 characters. It answers the messages waiting when the Talk was claimed, removes their 👀, and releases the claim. Messages that came since keep waiting. It answers `{"answered": n}`.
  - `note` is a **Note** ([ADR 0009](../../docs/adr/0009-gmail-for-the-core-assistant.md)): what a later run needs and the Owner shouldn't read, such as a draft's ID. Nando keeps it in the Talk, on the reply's first message, and never sends it to Telegram. Later claims show it. It may be at most 1000 characters; a longer one is a Tool error, and nothing is sent.
- **`messenger_get_photo`** (`assistant`, `id`): downloads the photo with `id`, from the claim's `photos`, from Telegram as that Assistant's bot, and answers it as an image (`image/jpeg`). It is read-only. A photo Telegram doesn't have, or no longer keeps, is a Tool error saying to ask the Owner for it again; any other failure, such as a timeout, says to try once more. Neither includes the bot token. Nando downloads up to 5 MB.
- **`messenger_get_file`** (`assistant`, `id`, `pages` optional, such as `3` or `11-20`): downloads the PDF with `id`, from the claim's `files`, from Telegram as that Assistant's bot, and reads it with poppler's `pdfinfo`, `pdftotext` and `pdftoppm`. Each runs as its own process, for at most 30 seconds. It is read-only.
  - It answers `pageCount` and `pages`, the pages read (the first 10 when `pages` isn't given, and never more than 10), each with its `page` number and `text`.
  - A page with no text, such as a scanned one, is marked `image: true` and follows the answer as a JPEG of up to 1280 px, in page order. When none of the pages read has text, as in a scan, the answer also says `scanned: true`.
  - A PDF that takes poppler over 30 seconds is an error that says to read fewer pages.
  - Pages outside the PDF, a broken PDF, or a file Telegram no longer has are Tool errors, which never include the bot token.
- **`messenger_new_talk`** (`assistant`, `summary` optional): closes the open Talk and opens one starting from `summary`. Waiting messages and the run's claim move to it. The bot says "Started a new chat.", or "Started a new chat (kept a short summary)." with a summary. It answers `{"moved": n}`.

Core's database holds:
- `talks`: one document per Talk, with its messages, and at most one open Talk per Assistant;
- `telegram_updates`: each bot's updates, until they expire;
- `messenger_scheduled_task`: one document per Assistant, holding its last fire and when its runs last worked or stopped.
