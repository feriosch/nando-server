# Passkeys are the only way to sign in; there is no password

The Owner signs in with a Passkey (Face ID on iPhone, stored in Google Password Manager or iCloud Keychain; the server holds only public keys). There is deliberately no password, no username-and-password form and no password reset. A new Passkey can only be registered with an Enrollment Code: a single-use code, valid for about 15 minutes, printed by a server CLI command (`nando-server enroll`) that needs access to the production database. The same command is the recovery path. We chose this because the Owner wants Face ID, will never change a password, and a password fallback would be extra code and attack surface for a way in nobody uses.

## Considered Options

- **Passkey origins derived from the domain** (only `https://web.<domain>`, plus a separate override list for local development): rejected. `DOMAIN=localhost` would give `https://web.localhost`, but the dev server is `http://localhost:5173`, so the override would be needed anyway, as a second origin list kept in step with `ALLOWED_ORIGINS`. And an origin trusted to call the API with the Owner's Session cookie gains little from also being allowed to sign in.

## Consequences

- Signing in starts a Session, kept on the server: a random token in an `HttpOnly`, `Secure`, `SameSite=Lax` cookie scoped to the parent domain, so it covers both `web.` and `api.`. Only the token's hash is stored; the Session also has a separate ID, so it can be listed and deleted without revealing the token. It lasts 90 days and renews on use, so Face ID is needed only when it expires. Deleting a Session signs it out.
- The Passkey's relying-party ID is the parent domain, so Passkeys survive any change to the `web.`/`api.` subdomain layout but not a change of domain. A new domain means enrolling again with an Enrollment Code.
- Passkeys work from every origin in `ALLOWED_ORIGINS`, the same list that may call the API with credentials (CORS), so adding an origin there also lets it sign in. The server refuses a Passkey answer from any other origin, and one whose relying-party ID isn't the parent domain. It does not itself check that an origin is under the parent domain: browsers do, as they only use a Passkey on pages under its relying-party ID. The exception is WebAuthn Related Origin Requests, where a browser also allows the origins the relying party lists at `https://<domain>/.well-known/webauthn`; Nando publishes no such file.
- The iOS Home Screen app keeps its cookies separately from Safari, so each needs its own sign-in.
