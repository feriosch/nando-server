# Core API

Identity: Passkeys, Enrollment Codes and Sessions ([ADR 0002](../../docs/adr/0002-passkey-only-identity.md)).
Settings and body measurements: the Owner Time Zone, height and Weigh-ins ([ADR 0003](../../docs/adr/0003-body-measurements-in-core.md)).
Notifications: Push Subscriptions, the push messages Core sends, and the scheduler's tick ([ADR 0001](../../docs/adr/0001-cloud-run-hosting.md)).
Connectors: Nando as its own OAuth 2.1 authorization server, the consent page's calls, the Tools at `/mcp`, and the Owner's list of Connectors ([ADR 0004](../../docs/adr/0004-connectors-over-oauth.md)).

The web app calls the API with `credentials: "include"`. Bodies are JSON. Errors are a status code with a short plain-text message.

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

Core sends every Notification, for a Module or for itself, to every Push Subscription. Each one is sent at most once, even if the scheduler retries a tick.

### `GET /api/push-subscriptions/vapid-public-key`

`200`: the key to pass as `applicationServerKey` to `registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey })`. It is base64url, without padding.

```json
{ "publicKey": "BNc…" }
```

### `POST /api/push-subscriptions`

Registers this copy of Nando for Notifications. Body: the `PushSubscription` from `pushManager.subscribe` (or `getSubscription`), as `subscription.toJSON()`.

```json
{
  "endpoint": "https://web.push.apple.com/QG…",
  "expirationTime": null,
  "keys": { "p256dh": "BOr…", "auth": "Ys5…" }
}
```

- `204`: registered. Registering the same `endpoint` again replaces its keys, so it is safe to send on every launch.
- `400`: the body isn't a `PushSubscription`: `endpoint` isn't an absolute URL, `keys.p256dh` isn't a P-256 public key, or `keys.auth` isn't 16 bytes (both base64url). The message says which.

When a push service says it no longer knows a Push Subscription (`404` or `410`, for example after the Owner turned Notifications off or removed the app), Core deletes it. Registering again brings it back.

### `DELETE /api/push-subscriptions?endpoint={endpoint}`

Unregisters the Push Subscription with this `endpoint` (URL-encoded), for example after `subscription.unsubscribe()`.

- `204`: that endpoint gets no more Notifications. This is also the answer if it wasn't registered.
- `400`: `endpoint` is missing.

### The push message

Each push message carries one Notification as JSON, encrypted for the Push Subscription (`aes128gcm`) and signed with the VAPID key above:

```json
{ "title": "Calories", "body": "Nothing logged today", "url": "/calories" }
```

- `title`, `body`: shown as-is. Core leaves out a field that is empty, so the service worker's default applies.
- `url`: the path in the web app to open when the Owner taps the Notification, such as `/calories`. The service worker opens `/` (Home) if it is missing or `null`.
- The service worker falls back to a default for any field that is missing or `null` (`title` → "Nando"). A push message with no payload, or one that isn't JSON, still shows a plain "Nando" Notification, since Safari revokes a Push Subscription whose push message shows nothing.

A push service keeps an undelivered message for up to 12 hours.

Core's own Notification is the Weigh-in reminder: `{ "title": "Weigh-in", "body": "Time to log this week's weight", "url": "/" }`, sent once a week at the `weighInReminder`'s weekday and time while it is `enabled`.

### `POST /internal/tick`

Called only by Cloud Scheduler, every 30 minutes at :00 and :30. It needs no Session, but its `Authorization: Bearer` token must be a Google-signed OIDC token for `TICK_AUDIENCE` issued to `SCHEDULER_SERVICE_ACCOUNT`.

At each tick Core asks every Notification provider (Core's own and every Module's) which Notifications are due in the current half hour, the one starting at :00 or :30 in the Owner Time Zone, and sends each one that hasn't been sent before. Every Notification has an ID, such as `calories:evening-summary:2026-10-20`, that Core remembers for 30 days in `sent_notifications`.

- `204`: done. A push service that failed to take a message doesn't fail the tick; that Notification isn't retried.
- `401`: no token, or not the scheduler's.
- `500`: a provider failed, or a Notification couldn't be recorded as sent. Everything else was still sent, and Cloud Scheduler's retry won't send it again.

## Connectors

A Connector, such as the Claude app, uses the Tools of Core and every Module over MCP at `https://api.<DOMAIN>/mcp`. It never signs in. Nando is its own OAuth 2.1 authorization server: authorization code with PKCE (`S256` only) and dynamic client registration, for public clients only. The Owner approves each Connector on the web app's consent page with a Passkey, always, even in a browser with a Session ([ADR 0004](../../docs/adr/0004-connectors-over-oauth.md)).

The OAuth routes (`/.well-known/`, `/oauth/`) and `/mcp` are called by the Connector, not the web app. They need no Session and send no cookie. OAuth errors are JSON, `{ "error": "invalid_grant", "error_description": "…" }`, as RFC 6749 sets out.

Every secret Nando issues is random and stored only as a SHA-256 hash, like a Session token:

| Secret             | Lasts                                                                          |
| ------------------ | ------------------------------------------------------------------------------ |
| Registration       | a day, unless a Connector is approved for it; then until it is removed         |
| Pending request    | 10 minutes, and it is approved or denied only once                             |
| Authorization code | 1 minute, and any exchange uses it up, even a refused one                      |
| Access token       | 1 hour                                                                         |
| Refresh token      | 90 days unused, like a Session; each refresh replaces it                       |

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

- `201`: the registration as kept: the fields above plus `client_id` and `client_id_issued_at`. It is always a public client, `token_endpoint_auth_method: "none"` with no `client_secret`, whatever was asked. `client_name` defaults to the first redirect URI's host, and is the Connector's name. `grant_types` defaults to `["authorization_code"]` and `response_types` to `["code"]`.
- `400` `invalid_redirect_uri`: no redirect URI, or one that isn't `https:`, or `http:` to `localhost` or `127.0.0.1` on any port (for local apps such as Claude Code). Fragments and user info are refused.
- `400` `invalid_client_metadata`: the body isn't JSON, more than 10 redirect URIs, a name over 100 characters, or a grant or response type other than those above.

### `GET /oauth/authorize`

The Connector sends the Owner's browser here with `response_type=code`, `client_id`, `redirect_uri` (exactly as registered), `code_challenge`, `code_challenge_method=S256` and `state`.

- `302` to `https://web.<DOMAIN>/connect?request=<id>`, the consent page, with the pending request.
- `400`, plain text: an unknown or expired `client_id`, or a `redirect_uri` it didn't register. These are never redirected, since the redirect URI can't be trusted.
- `302` back to the redirect URI with `error` and `state`: `unsupported_response_type` for a `response_type` other than `code`, and `invalid_request` for no `response_type`, no `code_challenge`, a method other than `S256`, or no `state`.

### `GET /auth/connect/{request}`

For the consent page, which may run in a browser without a Session. `200`: which app asks.

```json
{ "clientName": "Claude", "redirectHost": "claude.ai" }
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

- `400` `invalid_grant`: the code or refresh token is unknown, used, expired, another client's, or its Connector was removed. A refresh token that was already replaced, used again, removes its Connector, since it may have been stolen; the Owner then approves the app again.
- `400` `invalid_request`: no `grant_type`, or the body isn't a form.
- `400` `unsupported_grant_type`: any other `grant_type`.

### `POST /mcp`

MCP over Streamable HTTP, stateless, answering `application/json`, so each request can reach any instance. It needs `Authorization: Bearer <access token>`, never a Session cookie. It serves `tools/list` and `tools/call` for the Tools Core and every Module add. Each Tool says whether it is read-only (`readOnlyHint`) and whether it deletes, merges or moves data (`destructiveHint`). A Tool's result comes back as JSON text, and also as `structuredContent` when it is a JSON object. A Tool that fails comes back with `isError: true` and its message.

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

`204`: removes the Connector and its tokens. Its next call to `/mcp` gets `401`, and its refresh token `invalid_grant`. Deleting one that doesn't exist also answers `204`.
