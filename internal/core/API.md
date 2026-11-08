# Core API

Identity: passkeys, Enrollment Codes and sessions ([ADR 0002](../../docs/adr/0002-passkey-only-identity.md)).
Settings and body measurements: the Owner Time Zone, height and Weigh-ins ([ADR 0003](../../docs/adr/0003-body-measurements-in-core.md)).
Notifications: Push Subscriptions, the push messages Core sends, and the scheduler's tick ([ADR 0001](../../docs/adr/0001-cloud-run-hosting.md)).

The web app calls the API with `credentials: "include"`. Bodies are JSON. Errors are a status code with a short plain-text message.

## Signing in

These routes need no session. Each ceremony is a `begin` that returns the options for the browser, then a `finish` that takes what the browser returned. A ceremony can finish only once, within 5 minutes of its `begin`.

Both `finish` routes answer `204` and set the session cookie, which signs the device in:

| Cookie          | Value                                                                                                  |
| --------------- | ------------------------------------------------------------------------------------------------------ |
| `nando_session` | random token; `Domain=<DOMAIN>; Path=/; Max-Age=7776000` (90 days); `HttpOnly; Secure; SameSite=Lax` |

### `POST /auth/register/begin`

Registers a new passkey. This is the only way in without an existing passkey.

```json
{ "code": "BY95P-1SQMF" }
```

`code` is an Enrollment Code from `nando-server enroll`. It must be typed exactly as printed, dash and all.

- `200`: `{ "publicKey": PublicKeyCredentialCreationOptionsJSON }`. Pass `publicKey` to `PublicKeyCredential.parseCreationOptionsFromJSON` and then to `navigator.credentials.create({ publicKey })`. The passkey is discoverable and needs user verification (Face ID). The options exclude the Owner's existing passkeys, so a device that already has one refuses with `InvalidStateError` instead of silently replacing it.
- `401`: the code is unknown, used or expired.

### `POST /auth/register/finish`

Body: the `PublicKeyCredential` from `navigator.credentials.create`, as `credential.toJSON()`.

- `204`: the passkey is stored and the device is signed in. The Enrollment Code is used up only now, so a cancelled Face ID prompt doesn't waste it.
- `400`: the body isn't a `PublicKeyCredential`, or its challenge was for a sign-in.
- `401`: the challenge is unknown, used or expired; the passkey couldn't be verified; or the Enrollment Code expired or was used in the meantime.

### `POST /auth/signin/begin`

No body.

- `200`: `{ "publicKey": PublicKeyCredentialRequestOptionsJSON }`. It lists no credentials, so the Owner picks any of their passkeys. Pass `publicKey` to `PublicKeyCredential.parseRequestOptionsFromJSON` and then to `navigator.credentials.get({ publicKey })`.

### `POST /auth/signin/finish`

Body: the `PublicKeyCredential` from `navigator.credentials.get`, as `credential.toJSON()`.

- `204`: the device is signed in.
- `400`: the body isn't a `PublicKeyCredential`, or its challenge was for a registration.
- `401`: the challenge is unknown, used or expired; or the passkey is unknown (for example, deleted) or couldn't be verified.

## Everything under `/api/`

Every `/api/...` route, Core's and every Module's, needs a valid session cookie and answers `401` without one. A session lasts 90 days from its last renewal. A session in use is renewed for another 90 days at most once a day, and the response that renews it sets the cookie again with a fresh `Max-Age`.

### `GET /api/sessions`

`200`: every signed-in device, oldest first.

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

`userAgent` is the browser's from when it signed in. `current` marks the session making this request.

### `DELETE /api/sessions/{id}`

`204`: signs that device out; its next request gets `401`. Deleting the `current` session signs this device out, and the response also deletes its cookie (`Max-Age=0`). Deleting a session that doesn't exist also answers `204`.

### `GET /api/passkeys`

`200`: the Owner's passkeys, oldest first.

```json
[{ "id": "q1w2…", "createdAt": "2026-10-22T10:00:00Z" }]
```

`id` is the credential ID, base64url without padding (the `id` of the browser's `PublicKeyCredential`).

### `DELETE /api/passkeys/{id}`

`204`: that passkey can no longer sign in. Devices already signed in with it stay signed in until their sessions are deleted. Deleting the last passkey is allowed; `nando-server enroll` is the way back in.

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

- `title`, `body`: shown as-is.
- `url`: the path in the web app to open when the Owner taps the Notification, such as `/calories`. The service worker opens `/` (Home) if it is missing or `null`.
- The service worker falls back to a default for any field that is missing or `null` (`title` → "Nando"). A push message with no payload, or one that isn't JSON, still shows a plain "Nando" Notification, since Safari revokes a Push Subscription whose push message shows nothing.

A push service keeps an undelivered message for up to 12 hours.

Core's own Notification is the Weigh-in reminder: `{ "title": "Weigh-in", "body": "Time to log this week's weight", "url": "/" }`, sent once a week at the `weighInReminder`'s weekday and time while it is `enabled`.

### `POST /internal/tick`

Called only by Cloud Scheduler, every 30 minutes at :00 and :30. It needs no session, but its `Authorization: Bearer` token must be a Google-signed OIDC token for `TICK_AUDIENCE` issued to `SCHEDULER_SERVICE_ACCOUNT`.

At each tick Core asks every Notification provider (Core's own and every Module's) which Notifications are due in the half hour starting at :00 or :30, and sends each one that hasn't been sent before. Every Notification has an ID, such as `calories:evening-summary:2026-10-20`, that Core remembers for 30 days in `sent_notifications`.

- `204`: done. A push service that failed to take a message doesn't fail the tick; that Notification isn't retried.
- `401`: no token, or not the scheduler's.
- `500`: a provider failed. The other providers' Notifications were still sent, and Cloud Scheduler's retry won't send them again.
