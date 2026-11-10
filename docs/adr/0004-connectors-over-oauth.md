# Connectors get in through OAuth, approved with a Passkey

So that the Owner can talk to Claude about their Japanese Words from the Claude app, Nando serves its Tools over MCP at `https://api.<domain>/mcp`, and an outside app reaches them as a Connector. A Connector never signs in: Nando acts as a small OAuth 2.1 authorization server (authorization code with PKCE, dynamic client registration), and the authorize step is a Nando web page where the Owner approves the Connector with a Passkey. Nando then issues the Connector its own tokens, which work only on `/mcp`. This amends ADR 0002: a Passkey is still the only way for the Owner to sign in, and it is also the only way to let a Connector in.

## Considered Options

- **No authentication, at a secret URL**: rejected. Anyone who learns the URL could change the Owner's Words, and the URL would sit in the Claude app's settings and logs.
- **A long-lived API key pasted into the Claude app**: rejected. Claude's custom connectors authenticate with OAuth, and a key that never expires is a worse secret than a refresh token that can be revoked.
- **Chat inside Nando via the Anthropic API**: rejected for now. It bills per token on top of the Owner's Claude subscription, and the skills would have to be rebuilt as server code. A Connector uses the subscription the Owner already pays for.

## Consequences

- Core owns `/mcp`, the OAuth endpoints and the Connectors, and each Module supplies its own Tools, as it does Home Cards and Settings Sections. Core serves them without knowing what they do.
- Every Connector gets every Tool. There is one Owner, so there are no per-Module scopes.
- Approving a Connector always asks for a Passkey, even in a browser that already has a Session.
- Connector tokens are stored hashed, like Session tokens. The access token is short-lived. The refresh token renews on use and expires if unused for 90 days, the same as a Session, so a Connector used at least once every three months never needs approving again.
- Connectors are listed in Settings and can be removed there, which revokes their tokens at once.
- Tools that delete, merge or move a Word are marked destructive, so the Claude app can be set to ask before each use.
