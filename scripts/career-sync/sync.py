# /// script
# requires-python = ">=3.12"
# dependencies = ["mcp==2.3.0", "keyring==25.7.0"]
# ///
"""The Career Sync: hands the Owner's new LinkedIn messages to Nando (ADR 0007).

launchd runs it every hour. It asks Nando whether a Sync is due and, only
then, reads LinkedIn with the reader (mcp-server-linkedin) and sends Nando the
new messages. It calls no model. See README.md.

    uv run --script sync.py            one hourly run
    uv run --script sync.py --connect  approve it as a Connector, once
"""

import argparse
import asyncio
import json
import logging
import os
import random
import re
import socket
import stat
import sys
import time as clock
import webbrowser
from contextlib import asynccontextmanager
from datetime import date, datetime, time, tzinfo
from pathlib import Path
from typing import Any
from urllib.parse import parse_qs, quote, unquote, urljoin, urlparse
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError

import httpx2
from mcp import Client, StdioServerParameters
from mcp.client.auth import OAuthClientProvider
from mcp.client.auth.utils import (
    build_oauth_authorization_server_metadata_discovery_urls,
    create_oauth_metadata_request,
    handle_auth_metadata_response,
    issuers_match,
)
from mcp.client.streamable_http import streamable_http_client
from mcp.shared.exceptions import MCPError
from mcp.shared.auth import (
    AuthorizationCodeResult,
    OAuthClientInformationFull,
    OAuthClientMetadata,
    OAuthMetadata,
    OAuthToken,
)

log = logging.getLogger("nando-career-sync")

async def call(client: Client, tool: str, arguments: dict[str, Any] | None = None) -> Any:
    """Calls tool and answers its JSON result, or raises with the Tool's error."""
    result = await client.call_tool(tool, arguments or {})
    text = "".join(c.text for c in result.content if c.type == "text")
    if result.is_error:
        error = (result.meta or {}).get("linkedin.dev/error") or {}
        raise ToolError(tool, text, error.get("code"))
    if result.structured_content is not None:
        return result.structured_content
    return json.loads(text)


class ToolError(Exception):
    """A Tool answered with an error."""

    def __init__(self, tool: str, text: str, code: str | None = None) -> None:
        super().__init__(f"{tool}: {text}")
        self.tool = tool
        self.text = text
        # code is the reader's error code, such as auth_required, which it
        # keeps stable across releases, unlike its messages.
        self.code = code


async def run_sync(nando: Client, start_reader, *, now: datetime, pause) -> None:
    """One hourly run: a Sync, if Nando says one is due."""
    status = await call(nando, "career_sync_status")
    if not status["due"]:
        log.info("No Sync is due.")
        return
    log.info("A Sync is due: starting the reader.")
    async with start_reader() as client:
        reader = Reader(client)
        session = await reader.call("get_session_status")
        # The reader checks only that a session is saved, never LinkedIn. With
        # none, get_inbox would open a login window instead.
        if not session["session"]["stored"]:
            log.warning("No LinkedIn session is saved: log in with the reader's --login.")
            await call(nando, "career_record_sync", {"threads": [], "problem": {"kind": "session_expired"}})
            return
        try:
            inbox = await reader.call("get_inbox", {"limit": INBOX_LIMIT})
        except ToolError as e:
            log.warning("The reader couldn't read the inbox: %s", e)
            await call(nando, "career_record_sync", {"threads": [], "problem": reader_problem(e)})
            return
        held = {t["threadId"]: t for t in status["threads"]}
        since = datetime.combine(date.fromisoformat(status["since"]), time(), now.tzinfo)
        updates = []
        problem = None
        opened = 0
        names = inbox_names(inbox)
        for thread_id, name in names.items():
            thread = held.get(thread_id)
            if thread and thread["ignored"]:
                continue
            if opened == MAX_OPENED:
                break
            if opened:
                await pause()
            try:
                conversation = await reader.call("get_conversation", {"thread_id": thread_id})
            except ToolError as e:
                problem = reader_problem(e)
                break
            opened += 1
            if landed_on(conversation) != thread_id:
                # The reader doesn't check where LinkedIn took it, and another
                # thread's messages must never be filed under this one.
                problem = {"kind": "other", "text": f"LinkedIn opened {conversation.get('url')} for thread {thread_id}"}
                break
            messages = parse_conversation(
                conversation.get("sections", {}).get("conversation", ""), other=name, today=now.date(), tz=now.tzinfo
            )
            if not messages:
                # Every thread has a message, so the page isn't laid out as
                # parse_conversation expects. Tell Nando rather than take it
                # for a thread with nothing new.
                problem = {"kind": "other", "text": f"couldn't read the messages of LinkedIn thread {thread_id}: has its page changed?"}
                break
            newest = datetime.fromisoformat(thread["newest"]) if thread else since
            new = [m for m in messages if datetime.fromisoformat(m["sentAt"]) >= since and datetime.fromisoformat(m["sentAt"]) > newest]
            if not new:
                # The inbox lists the most recent threads first, so no later
                # one has anything new either.
                break
            updates.append(
                {
                    "threadId": thread_id,
                    "url": f"{LINKEDIN}/messaging/thread/{quote(thread_id, safe='')}/",
                    "person": person(conversation, name),
                    "messages": new,
                }
            )
        sync_id = None
        for start in range(0, len(updates), BATCH):
            batch: dict[str, Any] = {"threads": updates[start : start + BATCH]}
            # Every batch after the first names the Sync the first started,
            # so that Nando counts them as one Sync.
            if sync_id:
                batch["sync"] = sync_id
            recorded = await call(nando, "career_record_sync", batch)
            sync_id = recorded["sync"]
            log.info("Sent %d threads; Nando stored %d new messages.", len(updates[start : start + BATCH]), recorded["stored"])
        log.info("Opened %d threads, %d with new messages.", opened, len(updates))
        if problem:
            log.warning("Reporting a problem to Nando: %s", problem)
            await call(nando, "career_record_sync", {"threads": [], "problem": problem})
        elif not updates:
            # A Sync that found nothing still counts.
            await call(nando, "career_record_sync", {"threads": []})


LINKEDIN = "https://www.linkedin.com"
THREAD_PATH = re.compile(r"^/messaging/thread/(?P<id>[^/]+)/?$")


def inbox_names(inbox: dict[str, Any]) -> dict[str, str]:
    """The other person's name in each thread get_inbox listed, by thread ID,
    most recent first."""
    names: dict[str, str] = {}
    for ref in inbox.get("references", {}).get("inbox", []):
        path = THREAD_PATH.match(ref.get("url", ""))
        if ref.get("kind") == "conversation" and path:
            names.setdefault(unquote(path["id"]), ref.get("text", ""))
    return names


def landed_on(conversation: dict[str, Any]) -> str | None:
    """The thread ID of the page get_conversation read, if it is a thread."""
    path = THREAD_PATH.match(urlparse(conversation.get("url", "")).path)
    return unquote(path["id"]) if path else None


def person(conversation: dict[str, Any], name: str) -> dict[str, str]:
    """The other person in a conversation: their name, and their profile if
    the page links to it under their name."""
    found = {"name": name}
    if not name:
        # Every reference would match, the Owner's own profile first.
        return found
    for ref in conversation.get("references", {}).get("conversation", []):
        if ref.get("kind") == "person" and ref.get("text", "").startswith(name):
            found["profileUrl"] = urljoin(LINKEDIN, ref["url"])
            break
    return found


def reader_problem(e: ToolError) -> dict[str, str]:
    """The problem to tell Nando about when a reader Tool failed."""
    if e.code == "auth_required":
        return {"kind": "session_expired"}
    return {"kind": "other", "text": str(e)}


# The most threads get_inbox lists: it has no paging.
INBOX_LIMIT = 50

# The most threads one Sync opens with get_conversation.
MAX_OPENED = 30

# The most threads sent in one career_record_sync.
BATCH = 10

# The only reader Tools a Sync calls. Every other one, such as send_message,
# is refused here before it reaches the reader (ADR 0007).
READER_TOOLS = frozenset({"get_session_status", "get_inbox", "get_conversation"})


class ReaderToolRefused(Exception):
    """A Sync tried a reader Tool that isn't one of READER_TOOLS."""


class Reader:
    """The LinkedIn reader, limited to the Tools in READER_TOOLS."""

    def __init__(self, client: Client) -> None:
        self._client = client

    async def call(self, tool: str, arguments: dict[str, Any] | None = None) -> Any:
        if tool not in READER_TOOLS:
            raise ReaderToolRefused(f"a Sync never calls the reader's {tool}")
        try:
            return await call(self._client, tool, arguments)
        except MCPError as e:
            # The reader stopped answering, or answered out of turn: to a
            # Sync, the same as its Tool failing.
            raise ToolError(tool, e.message) from e


# A message group's heading, which LinkedIn writes for screen readers before
# the messages one person sent at one time.
GROUP_HEADING = re.compile(r"^(?P<name>.+?) sent the following messages? at (?P<time>\d{1,2}:\d{2}\s?[AP]M)$", re.IGNORECASE)


def parse_conversation(text: str, *, other: str, today: date, tz: tzinfo) -> list[dict[str, str]]:
    """The messages in get_conversation's page text, oldest first.

    other is the other person's name, as the inbox gives it: a message under
    another name is the Owner's. today and tz date LinkedIn's day headings,
    which are in the Mac's time zone.
    """
    lines = [line.strip() for line in text.splitlines()]
    # following[i] is the first non-empty line after line i.
    following = [""] * len(lines)
    for i in range(len(lines) - 2, -1, -1):
        following[i] = lines[i + 1] or following[i + 1]
    messages: list[dict[str, Any]] = []
    day: date | None = None
    current: dict[str, Any] | None = None
    for i, line in enumerate(lines):
        heading = GROUP_HEADING.match(line)
        if heading:
            at = datetime.combine(day, parse_time(heading["time"]), tz) if day else None
            name = heading["name"]
            current = {
                "from": "other" if name == other else "owner",
                "at": at,
                "lines": [],
                "skip": {name, heading["time"]},
            }
            messages.append(current)
            continue
        # A day heading comes only between groups, right before a group's
        # heading, and never as a group's first line: a group always has its
        # text. So a message saying just "Monday" stays a message.
        if line and (current is None or current["lines"]) and GROUP_HEADING.match(following[i]):
            heading_day = parse_day(line, today)
            if heading_day:
                day = heading_day
                continue
        if current is not None:
            # The sender's profile link, name and time come before the text,
            # so only lines before the text can be them.
            is_header = line in current["skip"] or (line.startswith("View ") and line.endswith("profile"))
            if not current["lines"] and (is_header or not line):
                continue
            current["lines"].append(line)
    return [
        {"from": m["from"], "sentAt": m["at"].isoformat(), "text": "\n".join(m["lines"]).strip()}
        for m in messages
        if m["at"] is not None
    ]


WEEKDAYS = ["monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"]


def parse_day(line: str, today: date) -> date | None:
    """The Day a LinkedIn day heading names, such as TODAY, THURSDAY, MAR 3 or
    DEC 12, 2024, or None if line isn't one."""
    word = line.lower()
    if word == "today":
        return today
    if word == "yesterday":
        return date.fromordinal(today.toordinal() - 1)
    if word in WEEKDAYS:
        # The latest such Day before today: LinkedIn names the past week's Days.
        back = (today.weekday() - WEEKDAYS.index(word) - 1) % 7 + 1
        return date.fromordinal(today.toordinal() - back)
    for pattern in ("%b %d, %Y", "%B %d, %Y"):
        try:
            return datetime.strptime(line.title(), pattern).date()
        except ValueError:
            pass
    for pattern in ("%b %d", "%B %d"):
        try:
            # With the year: parsing Feb 29 without one fails.
            parsed = datetime.strptime(f"{line.title()} {today.year}", pattern + " %Y").date()
        except ValueError:
            continue
        # Without a year, it is this year, unless that is still to come.
        return parsed if parsed <= today else parsed.replace(year=today.year - 1)
    return None


def parse_time(text: str) -> time:
    return datetime.strptime(text.replace(" ", "").upper(), "%I:%M%p").time()


# Running it: the reader, Nando as a Connector, and the command line.

# The reader's release. Never @latest: updating it is a deliberate change,
# after checking its Tools again (README.md).
READER = "mcp-server-linkedin==4.30.0"

NANDO_MCP = os.environ.get("NANDO_MCP_URL", "https://api.nanders.app/mcp")


@asynccontextmanager
async def start_reader():
    """Starts the reader over stdio, for one Sync."""
    params = StdioServerParameters(
        command="uvx",
        args=[
            READER,
            "--transport",
            "stdio",
            # Never import a session from the Mac's browsers, and never wait
            # on a login window: the Owner logs in with --login themselves.
            "--no-auto-import",
            "--login-inline-wait",
            "0",
        ],
        # No remote tool catalog or update check: the script calls only its
        # three Tools, and an update note would be added to their answers.
        env={"LINKEDIN_MCP_REMOTE_TOOLS": "off", "LINKEDIN_MCP_CHECK_FOR_UPDATES": "off"},
    )
    async with Client(params) as client:
        yield client


async def random_pause() -> None:
    """Waits between two conversations, as a person reading them would."""
    await asyncio.sleep(random.uniform(3, 10))


def online() -> bool:
    """Whether Nando can be reached at all."""
    host = urlparse(NANDO_MCP).hostname or ""
    try:
        socket.create_connection((host, 443), timeout=5).close()
        return True
    except OSError:
        return False


# The Connector's tokens and registration, in the login Keychain, or in a
# 0600 file if launchd can't reach the Keychain.
KEYCHAIN_SERVICE = "app.nando.career-sync"
KEYCHAIN_ACCOUNT = "connector"
TOKEN_FILE = Path.home() / ".nando" / "career-sync.json"

# Where the browser comes back to after the Owner approves the Connector. A
# fixed loopback port, so the redirect URI is the same at registration,
# authorization and the code exchange.
CALLBACK_PORT = 8765
REDIRECT_URI = f"http://127.0.0.1:{CALLBACK_PORT}/callback"


class Store:
    """The Connector's tokens and registration, with when the access token
    expires, so that a later run refreshes it instead of using it stale."""

    def load(self) -> dict[str, Any]:
        text = self._read()
        return json.loads(text) if text else {}

    def save(self, state: dict[str, Any]) -> None:
        text = json.dumps(state)
        try:
            import keyring

            keyring.set_password(KEYCHAIN_SERVICE, KEYCHAIN_ACCOUNT, text)
            TOKEN_FILE.unlink(missing_ok=True)
            return
        except Exception as e:
            log.info("The Keychain isn't reachable (%s): keeping the tokens in %s.", e, TOKEN_FILE)
        TOKEN_FILE.parent.mkdir(mode=0o700, exist_ok=True)
        fd = os.open(TOKEN_FILE, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, stat.S_IRUSR | stat.S_IWUSR)
        with os.fdopen(fd, "w") as f:
            f.write(text)

    def _read(self) -> str | None:
        try:
            import keyring

            text = keyring.get_password(KEYCHAIN_SERVICE, KEYCHAIN_ACCOUNT)
            if text:
                return text
        except Exception as e:
            log.info("The Keychain isn't reachable (%s): trying %s.", e, TOKEN_FILE)
        try:
            return TOKEN_FILE.read_text()
        except FileNotFoundError:
            return None


class StoredTokens:
    """The SDK's TokenStorage, kept in a Store."""

    def __init__(self, store: Store) -> None:
        self.store = store

    async def get_tokens(self) -> OAuthToken | None:
        tokens = self.store.load().get("tokens")
        return OAuthToken.model_validate(tokens) if tokens else None

    async def set_tokens(self, tokens: OAuthToken) -> None:
        state = self.store.load()
        state["tokens"] = tokens.model_dump(mode="json", exclude_none=True)
        state["expiresAt"] = clock.time() + tokens.expires_in if tokens.expires_in else None
        self.store.save(state)

    async def get_client_info(self) -> OAuthClientInformationFull | None:
        info = self.store.load().get("client")
        return OAuthClientInformationFull.model_validate(info) if info else None

    async def set_client_info(self, client_info: OAuthClientInformationFull) -> None:
        state = self.store.load()
        state["client"] = client_info.model_dump(mode="json", exclude_none=True)
        self.store.save(state)

    def expires_at(self) -> float | None:
        return self.store.load().get("expiresAt")


class NandoAuth(OAuthClientProvider):
    """OAuth with Nando that a later run, a new process, can carry on with.
    The SDK reloads saved tokens, but neither when the access token expires
    nor Nando's authorization server metadata, which it reads only on the way
    to the browser. Without the expiry it would use an expired token until
    Nando turned it away. Without the metadata it would refresh at the
    origin's /token, which Nando doesn't serve: its token endpoint is
    /oauth/token. Either way it would start over in the browser, which an
    hourly run can't. So it reads the metadata itself before a refresh.

    It leans on mcp 2.3.0's internals (_initialize, _refresh_token and
    context): test_auth.py checks them when the SDK is updated."""

    def __init__(
        self, storage: StoredTokens, *, interactive: bool, transport: httpx2.AsyncBaseTransport | None = None
    ) -> None:
        super().__init__(
            server_url=NANDO_MCP,
            client_metadata=OAuthClientMetadata(
                client_name="Career Sync on the Owner's Mac",
                redirect_uris=[REDIRECT_URI],
                grant_types=["authorization_code", "refresh_token"],
                response_types=["code"],
                token_endpoint_auth_method="none",
            ),
            storage=storage,
            redirect_handler=open_browser if interactive else needs_connect,
            callback_handler=wait_for_callback if interactive else needs_connect_callback,
        )
        self._storage = storage
        self._transport = transport

    async def _initialize(self) -> None:
        await super()._initialize()
        self.context.token_expiry_time = self._storage.expires_at()

    async def _refresh_token(self) -> httpx2.Request:
        if self.context.oauth_metadata is None:
            self.context.oauth_metadata = await self._read_metadata()
        return await super()._refresh_token()

    async def _read_metadata(self) -> OAuthMetadata:
        """Nando's authorization server metadata, as the SDK reads it on the
        way to the browser."""
        issuer = self.context.get_authorization_base_url(NANDO_MCP)
        url = build_oauth_authorization_server_metadata_discovery_urls(None, NANDO_MCP)[0]
        try:
            async with httpx2.AsyncClient(transport=self._transport, timeout=30) as http:
                response = await http.send(create_oauth_metadata_request(url))
                _, metadata = await handle_auth_metadata_response(response)
        except httpx2.HTTPError as e:
            raise NandoUnavailable(f"Couldn't read Nando's OAuth metadata ({e}): the next hour tries again.") from e
        if metadata is None or not issuers_match(str(metadata.issuer), issuer):
            raise NandoUnavailable(
                f"Couldn't read Nando's OAuth metadata ({response.status_code} at {url}): the next hour tries again."
            )
        return metadata


class NotConnected(Exception):
    """Nando needs the Owner to approve the script again."""


class NandoUnavailable(Exception):
    """Nando couldn't be asked how to refresh the access token. The tokens
    are kept, and the next hour tries again."""


async def needs_connect(url: str) -> None:
    raise NotConnected("Nando wants the Connector approved: run sync.py --connect")


async def needs_connect_callback() -> AuthorizationCodeResult:
    raise NotConnected("Nando wants the Connector approved: run sync.py --connect")


async def open_browser(url: str) -> None:
    print(f"Approve the Career Sync with your Passkey in the browser:\n{url}")
    webbrowser.open(url)


async def wait_for_callback() -> AuthorizationCodeResult:
    """Waits for the browser to come back to REDIRECT_URI with the code."""
    done: asyncio.Future[AuthorizationCodeResult] = asyncio.get_running_loop().create_future()

    async def answer(reader: asyncio.StreamReader, writer: asyncio.StreamWriter) -> None:
        request = (await reader.readline()).decode(errors="replace").split(" ")
        query = parse_qs(urlparse(request[1] if len(request) > 1 else "").query)
        if "code" in query and not done.done():
            done.set_result(
                AuthorizationCodeResult(code=query["code"][0], state=query.get("state", [None])[0], iss=query.get("iss", [None])[0])
            )
            body = b"Nando's Career Sync is connected. You can close this tab."
        else:
            body = b"No authorization code here: " + ", ".join(query.get("error", ["unknown"])).encode()
        writer.write(b"HTTP/1.1 200 OK\r\nContent-Type: text/plain; charset=utf-8\r\nConnection: close\r\n")
        writer.write(f"Content-Length: {len(body)}\r\n\r\n".encode() + body)
        await writer.drain()
        writer.close()

    server = await asyncio.start_server(answer, "127.0.0.1", CALLBACK_PORT)
    async with server:
        return await asyncio.wait_for(done, timeout=600)


@asynccontextmanager
async def nando_client(*, interactive: bool):
    """Nando's MCP endpoint, as the Connector the Owner approved."""
    auth = NandoAuth(StoredTokens(Store()), interactive=interactive)
    async with httpx2.AsyncClient(auth=auth, timeout=httpx2.Timeout(30, read=300)) as http:
        async with Client(streamable_http_client(NANDO_MCP, http_client=http)) as client:
            yield client


async def connect() -> None:
    """Approves the script as a Connector and checks that it can call Nando."""
    async with nando_client(interactive=True) as nando:
        status = await call(nando, "career_sync_status")
    print(f"Connected to Nando. A Sync is {'due' if status['due'] else 'not due'} now.")


def local_zone() -> tzinfo:
    """The Mac's time zone, by name, so that a day heading across a change
    to or from summer time gets its own offset. /etc/localtime links to it."""
    try:
        return ZoneInfo(os.path.realpath("/etc/localtime").split("/zoneinfo/", 1)[1])
    except (IndexError, ValueError, ZoneInfoNotFoundError):
        return datetime.now().astimezone().tzinfo


async def hourly() -> None:
    async with nando_client(interactive=False) as nando:
        await run_sync(nando, start_reader, now=datetime.now(local_zone()), pause=random_pause)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--connect", action="store_true", help="approve the script as a Connector, in the browser")
    args = parser.parse_args()
    # launchd sends stderr to ~/Library/Logs/nando-career-sync.log.
    logging.basicConfig(stream=sys.stderr, level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
    logging.getLogger("httpx2").setLevel(logging.WARNING)
    logging.getLogger("mcp").setLevel(logging.WARNING)
    if args.connect:
        asyncio.run(connect())
        return 0
    if not online():
        log.info("No network: the next hour tries again.")
        return 0
    try:
        asyncio.run(hourly())
    except Exception as e:
        errors = list(leaves(e))
        if all(isinstance(error, (NotConnected, NandoUnavailable)) for error in errors):
            for message in dict.fromkeys(str(error) for error in errors):
                log.error("%s", message)
        else:
            log.exception("The run failed.")
        return 1
    return 0


def leaves(e: BaseException):
    """The errors that ended a run, out of the ExceptionGroups the MCP
    client's task groups wrap them in."""
    if isinstance(e, BaseExceptionGroup):
        for inner in e.exceptions:
            yield from leaves(inner)
    else:
        yield e


if __name__ == "__main__":
    sys.exit(main())
