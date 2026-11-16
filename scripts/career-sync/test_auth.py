"""The Connector's OAuth with Nando, across runs: launchd starts each hourly
run as a new process, with only the tokens and registration saved."""

import json
from urllib.parse import parse_qs, urlparse

import httpx2
import pytest

import sync
from mcp.shared.auth import AuthorizationCodeResult

pytestmark = pytest.mark.anyio

ORIGIN = "https://api.nanders.app"
METADATA = "/.well-known/oauth-authorization-server"


@pytest.fixture
def anyio_backend():
    return "asyncio"


class MemoryStore(sync.Store):
    """A Store without the Keychain or a file."""

    def __init__(self, state: dict | None = None) -> None:
        self.text: str | None = json.dumps(state) if state else None

    def save(self, state):
        self.text = json.dumps(state)

    def _read(self):
        return self.text

    def state(self) -> dict:
        return json.loads(self.text)


class FakeAuthServer:
    """Nando's OAuth endpoints and /mcp, as served by internal/core: the token
    endpoint is /oauth/token, and nothing is served at the SDK's fallback
    /token. It records every request."""

    def __init__(self) -> None:
        self.requests: list[tuple[str, str]] = []
        self.valid_access: set[str] = set()
        self.metadata_status = 200
        self.tokens_issued = 0

    def grants(self) -> list[str]:
        return [form for method, form in self.requests if method == "grant"]

    def __call__(self, request: httpx2.Request) -> httpx2.Response:
        path = request.url.path
        self.requests.append((request.method, path))
        if path.startswith("/.well-known/oauth-protected-resource"):
            return httpx2.Response(200, json={"resource": f"{ORIGIN}/mcp", "authorization_servers": [ORIGIN]})
        if path == METADATA:
            if self.metadata_status != 200:
                return httpx2.Response(self.metadata_status)
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
        if path == "/oauth/register" and request.method == "POST":
            body = json.loads(request.content)
            return httpx2.Response(201, json={**body, "client_id": "career-sync"})
        if path == "/oauth/token" and request.method == "POST":
            grant = parse_qs(request.content.decode())["grant_type"][0]
            self.requests.append(("grant", grant))
            self.tokens_issued += 1
            n = self.tokens_issued
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
    auth = sync.NandoAuth(sync.StoredTokens(store), interactive=True, transport=httpx2.MockTransport(server))
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
    auth = sync.NandoAuth(sync.StoredTokens(store), interactive=False, transport=httpx2.MockTransport(server))
    async with httpx2.AsyncClient(auth=auth, transport=httpx2.MockTransport(server)) as http:
        return await http.post(f"{ORIGIN}/mcp")


def expire_access_token(store: MemoryStore) -> None:
    state = store.state()
    state["expiresAt"] = sync.clock.time() - 1
    store.text = json.dumps(state)


async def test_a_later_run_reads_nandos_metadata_and_refreshes_at_its_token_endpoint():
    store, server = MemoryStore(), FakeAuthServer()
    await connect(store, server)
    expire_access_token(store)
    server.requests.clear()

    response = await hourly_request(store, server)

    assert response.status_code == 200
    assert server.requests == [
        ("GET", METADATA),
        ("POST", "/oauth/token"),
        ("grant", "refresh_token"),
        ("POST", "/mcp"),
    ]
    assert store.state()["tokens"]["access_token"] == "access-2"


async def test_a_store_the_script_saved_before_this_fix_refreshes_without_connect():
    """As on the Owner's Mac: tokens, registration and expiry, nothing else,
    and a refresh token Nando never saw used."""
    server = FakeAuthServer()
    store = MemoryStore(
        {
            "client": {"client_id": "career-sync", "redirect_uris": [sync.REDIRECT_URI], "token_endpoint_auth_method": "none"},
            "tokens": {"access_token": "access-0", "token_type": "Bearer", "expires_in": 3600, "refresh_token": "refresh-0"},
            "expiresAt": sync.clock.time() - 8 * 3600,
        }
    )

    response = await hourly_request(store, server)

    assert response.status_code == 200
    assert ("POST", "/token") not in server.requests
    assert server.grants() == ["refresh_token"]


async def test_a_later_run_with_a_valid_access_token_neither_reads_the_metadata_nor_refreshes():
    store, server = MemoryStore(), FakeAuthServer()
    await connect(store, server)
    server.requests.clear()

    response = await hourly_request(store, server)

    assert response.status_code == 200
    assert server.requests == [("POST", "/mcp")]


async def test_metadata_nando_cant_serve_ends_the_run_and_keeps_the_tokens():
    store, server = MemoryStore(), FakeAuthServer()
    await connect(store, server)
    expire_access_token(store)
    saved = store.text
    server.metadata_status = 503

    with pytest.raises(sync.NandoUnavailable, match="503"):
        await hourly_request(store, server)

    assert server.grants() == ["authorization_code"]
    assert store.text == saved


async def test_a_run_nando_turns_away_asks_for_connect_again():
    store, server = MemoryStore(), FakeAuthServer()
    await connect(store, server)
    server.valid_access.clear()

    with pytest.raises(sync.NotConnected):
        await hourly_request(store, server)


CONNECT = "Nando wants the Connector approved: run sync.py --connect"
UNAVAILABLE = "Couldn't read Nando's OAuth metadata (503 at …): the next hour tries again."


def run_main(monkeypatch, error: BaseException) -> int:
    async def hourly() -> None:
        raise error

    monkeypatch.setattr(sync, "online", lambda: True)
    monkeypatch.setattr(sync, "hourly", hourly)
    monkeypatch.setattr(sync.sys, "argv", ["sync.py"])
    return sync.main()


@pytest.mark.parametrize(
    "error, message",
    [
        (sync.NotConnected(CONNECT), CONNECT),
        # As it reaches main: the MCP client's task groups wrap it.
        (ExceptionGroup("unhandled errors in a TaskGroup", [ExceptionGroup("inner", [sync.NotConnected(CONNECT)])]), CONNECT),
        (ExceptionGroup("unhandled errors in a TaskGroup", [sync.NandoUnavailable(UNAVAILABLE)]), UNAVAILABLE),
    ],
)
def test_a_run_ended_by_nando_logs_one_line_saying_why(monkeypatch, caplog, error, message):
    assert run_main(monkeypatch, error) == 1

    errors = [r for r in caplog.records if r.levelname == "ERROR"]
    assert [(r.getMessage(), r.exc_info) for r in errors] == [(message, None)]


def test_any_other_error_beside_it_is_logged_with_its_traceback(monkeypatch, caplog):
    error = ExceptionGroup("unhandled errors in a TaskGroup", [sync.NotConnected(CONNECT), RuntimeError("the reader crashed")])

    assert run_main(monkeypatch, error) == 1

    errors = [r for r in caplog.records if r.levelname == "ERROR"]
    assert [r.getMessage() for r in errors] == ["The run failed."]
    assert "the reader crashed" in caplog.text
