# Passkeys are the only way to sign in; there is no password

The Owner signs in with a passkey (Face ID on iPhone, stored in Google Password Manager or iCloud Keychain; the server holds only public keys). There is deliberately no password, no login form and no password reset. A new passkey can only be registered with an Enrollment Code: a single-use code, valid for about 15 minutes, printed by a server CLI command (`nando-server enroll`) that needs access to the production database. The same command is the recovery path. We chose this because the Owner wants Face ID, will never change a password, and a password fallback would be extra code and attack surface for a login nobody uses.

## Consequences

- Signing in produces a server-side session: a random ID in an `HttpOnly`, `Secure`, `SameSite=Lax` cookie scoped to the parent domain, so it covers both `web.` and `api.`. Only the ID's hash is stored. It lasts 90 days and renews on use, so Face ID is needed only when it expires. Deleting a session signs that device out.
- The passkey's relying-party ID is the parent domain, so passkeys survive any change to the `web.`/`api.` subdomain layout but not a change of domain. A new domain means enrolling again with an Enrollment Code.
- The iOS Home Screen app keeps its cookies separately from Safari, so each needs its own sign-in.
