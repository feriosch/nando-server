# Core API

Identity: passkeys, Enrollment Codes and sessions ([ADR 0002](../../docs/adr/0002-passkey-only-identity.md)).

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
