"""The Connector's OAuth with Nando, across runs: launchd starts each hourly
run as a new process, with only what --connect saved."""

import json
from urllib.parse import parse_qs, urlparse

import httpx2
import pytest

import sync
from mcp.shared.auth import AuthorizationCodeResult

pytestmark = pytest.mark.anyio

ORIGIN = "https://api.nanders.app"


@pytest.fixture
def anyio_backend():
    return "asyncio"


class MemoryStore(sync.Store):
    """A Store without the Keychain or a file."""

    def __init__(self) -> None:
        self.text: str | None = None

    def save(self, state):
        self.text = json.dumps(state)

    def _read(self):
        return self.text


class FakeAuthServer:
    """Nando's OAuth endpoints and /mcp, as served by internal/core: the token
    endpoint is /oauth/token, not the SDK's fallback /token."""

    def __init__(self) -> None:
        self.grants: list[tuple[str, str]] = []
        self.valid_access: set[str] = set()

    def __call__(self, request: httpx2.Request) -> httpx2.Response:
        path = request.url.path
        if path.startswith("/.well-known/oauth-protected-resource"):
            return httpx2.Response(200, json={"resource": f"{ORIGIN}/mcp", "authorization_servers": [ORIGIN]})
        if path == "/.well-known/oauth-authorization-server":
            return httpx2.Response(
                200,
                json={
                    "issuer": ORIGIN,
                    "authorization_endpoint": f"{ORIGIN}/oauth/authorize",
                    "token_endpoint": f"{ORIGIN}/oauth/token",
                    "registration_endpoint": f"{ORIGIN}/oauth/register",
                    "response_types_supported": ["code"],
                    "grant_types_supported": ["authorization_code", "refresh_token"],
                    "code_challenge_methods_supported": ["S256"],
                    "token_endpoint_auth_methods_supported": ["none"],
                },
            )
        if path == "/oauth/register":
            body = json.loads(request.content)
            return httpx2.Response(201, json={**body, "client_id": "career-sync"})
        if path == "/oauth/token":
            form = {k: v[0] for k, v in parse_qs(request.content.decode()).items()}
            self.grants.append((form["grant_type"], path))
            n = len(self.grants)
            self.valid_access.add(f"access-{n}")
            return httpx2.Response(
                200,
                json={"access_token": f"access-{n}", "token_type": "Bearer", "expires_in": 3600, "refresh_token": f"refresh-{n}"},
            )
        if path == "/mcp":
            if request.headers.get("Authorization", "").removeprefix("Bearer ") in self.valid_access:
                return httpx2.Response(200, json={})
            return httpx2.Response(
                401,
                headers={"WWW-Authenticate": f'Bearer resource_metadata="{ORIGIN}/.well-known/oauth-protected-resource/mcp"'},
            )
        return httpx2.Response(404)


async def connect(store: sync.Store, server: FakeAuthServer) -> None:
    """--connect, with the Owner approving in the browser at once."""
    auth = sync.NandoAuth(sync.StoredTokens(store), interactive=True)
    approved: list[str] = []

    async def redirect(url: str) -> None:
        approved.append(parse_qs(urlparse(url).query)["state"][0])

    async def callback() -> AuthorizationCodeResult:
        return AuthorizationCodeResult(code="code", state=approved[0])

    auth.context.redirect_handler = redirect
    auth.context.callback_handler = callback
    async with httpx2.AsyncClient(auth=auth, transport=httpx2.MockTransport(server)) as http:
        assert (await http.post(f"{ORIGIN}/mcp")).status_code == 200


async def hourly_request(store: sync.Store, server: FakeAuthServer) -> httpx2.Response:
    """One request of an hourly run: a new process, not interactive."""
    auth = sync.NandoAuth(sync.StoredTokens(store), interactive=False)
    async with httpx2.AsyncClient(auth=auth, transport=httpx2.MockTransport(server)) as http:
        return await http.post(f"{ORIGIN}/mcp")


def expire_access_token(store: MemoryStore) -> None:
    state = json.loads(store.text)
    state["expiresAt"] = sync.clock.time() - 1
    store.text = json.dumps(state)


async def test_a_later_run_refreshes_an_expired_access_token_at_nandos_token_endpoint():
    store, server = MemoryStore(), FakeAuthServer()
    await connect(store, server)
    expire_access_token(store)

    response = await hourly_request(store, server)

    assert response.status_code == 200
    assert server.grants == [("authorization_code", "/oauth/token"), ("refresh_token", "/oauth/token")]
    assert json.loads(store.text)["tokens"]["access_token"] == "access-2"


async def test_a_later_run_with_a_valid_access_token_doesnt_refresh():
    store, server = MemoryStore(), FakeAuthServer()
    await connect(store, server)

    response = await hourly_request(store, server)

    assert response.status_code == 200
    assert server.grants == [("authorization_code", "/oauth/token")]


async def test_a_run_nando_turns_away_asks_for_connect_again():
    store, server = MemoryStore(), FakeAuthServer()
    await connect(store, server)
    server.valid_access.clear()

    with pytest.raises(sync.NotConnected):
        await hourly_request(store, server)


@pytest.mark.parametrize(
    "error",
    [
        sync.NotConnected("Nando wants the Connector approved: run sync.py --connect"),
        # As it reaches main: the MCP client's task group wraps it.
        ExceptionGroup("unhandled errors in a TaskGroup", [sync.NotConnected("Nando wants the Connector approved: run sync.py --connect")]),
    ],
)
def test_an_hourly_run_that_needs_connect_logs_one_line_saying_so(monkeypatch, caplog, error):
    async def hourly() -> None:
        raise error

    monkeypatch.setattr(sync, "online", lambda: True)
    monkeypatch.setattr(sync, "hourly", hourly)
    monkeypatch.setattr(sync.sys, "argv", ["sync.py"])

    assert sync.main() == 1

    errors = [r for r in caplog.records if r.levelname == "ERROR"]
    assert [(r.getMessage(), r.exc_info) for r in errors] == [("Nando wants the Connector approved: run sync.py --connect", None)]
