# /// script
# requires-python = ">=3.12"
# dependencies = ["mcp==2.3.0", "keyring==25.7.0"]
# ///
"""The Career Sync: hands the Owner's new LinkedIn messages to Nando (ADR 0007).

launchd runs it every 15 minutes. It asks Nando whether a Sync is due and, only
then, reads LinkedIn with the reader (mcp-server-linkedin) and sends Nando the
new messages. It calls no model. See README.md.

    uv run --script sync.py            one run
    uv run --script sync.py --connect  approve it as a Connector, once
"""

import argparse
import asyncio
import functools
import json
import logging
import os
import random
import re
import socket
import stat
import subprocess
import sys
import time as clock
import unicodedata
import webbrowser
from collections.abc import AsyncGenerator
from contextlib import AsyncExitStack, asynccontextmanager
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


async def run_sync(nando: Client, start_reader, *, now: datetime, pause, import_session) -> None:
    """One run: a Sync, if Nando says one is due. import_session
    imports the LinkedIn session of the Owner's Chrome and says whether it
    did (import_chrome_session)."""
    status = await call(nando, "career_sync_status")
    if not status["due"]:
        log.info("No Sync is due.")
        return
    log.info("A Sync is due: starting the reader.")
    async with AsyncExitStack() as stack:
        opened_inbox = await open_inbox(stack, start_reader, import_session)
        if isinstance(opened_inbox, dict):
            log.warning("Reporting a problem to Nando: %s", opened_inbox)
            await call(nando, "career_record_sync", {"threads": [], "problem": opened_inbox})
            return
        reader, inbox = opened_inbox
        held = {t["threadId"]: t for t in status["threads"]}
        since = datetime.combine(date.fromisoformat(status["since"]), time(), now.tzinfo)
        updates = []
        problem = None
        # Whether it stopped before reading every thread it had to.
        stopped_early = False
        opened = 0
        # Set once a thread Nando holds has nothing new. The inbox is in order
        # of activity, so the held threads after it have nothing new either,
        # as long as the last Sync read them all up to date. One that stopped
        # early may have taken a newer message from a thread above one it
        # never reached: then the held threads after it are opened too.
        caught_up = False
        read_through = status.get("readThrough", True)
        names = inbox_names(inbox)
        for thread_id, name in names.items():
            thread = held.get(thread_id)
            if thread and (thread["ignored"] or (caught_up and read_through)):
                continue
            if opened == MAX_OPENED:
                # Early only before a held thread with nothing new. After one,
                # the threads left are the backlog, which every Sync opens,
                # and held threads a Sync that didn't read through opens
                # too, from the top: another such Sync would open the same
                # 30 again, and again every Sync after.
                stopped_early = not caught_up
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
            text = conversation.get("sections", {}).get("conversation", "")
            ad = is_ad(text)
            if ad:
                # Sent whole as one message: the run decides whether it is
                # about a role, and ignores it otherwise.
                message = ad_message(text, today=now.date(), tz=now.tzinfo)
                messages = [message] if message else []
            else:
                messages = parse_conversation(text, other=name, today=now.date(), tz=now.tzinfo)
            if not messages:
                # Every thread has a message, so the page isn't laid out as
                # parse_conversation expects. Tell Nando rather than take it
                # for a thread with nothing new.
                problem = {"kind": "other", "text": f"couldn't read the messages of LinkedIn thread {thread_id}: has its page changed?"}
                break
            newest = datetime.fromisoformat(thread["newest"]) if thread else since
            new = [m for m in messages if datetime.fromisoformat(m["sentAt"]) >= since and datetime.fromisoformat(m["sentAt"]) > newest]
            if not new:
                if ad:
                    # LinkedIn keeps ads near the top, out of order.
                    continue
                if thread:
                    # Threads Nando doesn't hold yet after it are the backlog
                    # an earlier Sync didn't reach: still read them.
                    caught_up = True
                    continue
                # Nothing since `since`: every thread after it is older still.
                break
            updates.append(
                {
                    "threadId": thread_id,
                    "url": f"{LINKEDIN}/messaging/thread/{quote(thread_id, safe='')}/",
                    "person": person(conversation, name),
                    "messages": new,
                }
            )
        # A problem ends a Sync early too. Nando learns of it from the problem,
        # sent after the batches, but the batches say so as well, should it
        # never arrive.
        stopped_early = stopped_early or problem is not None
        sync_id = None
        for start in range(0, len(updates), BATCH):
            batch: dict[str, Any] = {"threads": updates[start : start + BATCH]}
            # Every batch after the first names the Sync the first started,
            # so that Nando counts them as one Sync.
            if sync_id:
                batch["sync"] = sync_id
            elif stopped_early:
                # On the first batch, which starts the Sync, so that the next
                # Sync reads the held threads this one didn't reach.
                batch["stoppedEarly"] = True
            recorded = await call(nando, "career_record_sync", batch)
            sync_id = recorded["sync"]
            log.info("Sent %d threads; Nando stored %d new messages.", len(updates[start : start + BATCH]), recorded["stored"])
        log.info("Opened %d threads, %d with new messages.", opened, len(updates))
        if problem:
            log.warning("Reporting a problem to Nando: %s", problem)
            await call(nando, "career_record_sync", {"threads": [], "problem": problem})
        elif not updates:
            # A Sync that found nothing still counts.
            await call(nando, "career_record_sync", {"threads": [], **({"stoppedEarly": True} if stopped_early else {})})


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


# The first line of an ad LinkedIn lists among the threads: an InMail ad, or
# its own offer of Premium.
AD_LABELS = frozenset({"Sponsored", "LinkedIn Offer"})


def is_ad(text: str) -> bool:
    """Whether get_conversation's page text is an ad, not a conversation."""
    first = next((line.strip() for line in text.splitlines() if line.strip()), "")
    return first in AD_LABELS


def ad_message(text: str, *, today: date, tz: tzinfo) -> dict[str, str] | None:
    """An ad as one message from the other person: its whole text, at the
    start of the Day it names, so that every Sync sends it alike and Nando
    stores it once. None if it names no Day."""
    day = next((d for line in text.splitlines() if (d := parse_day(line.strip(), today))), None)
    if day is None:
        return None
    body = re.sub(r"\n\s*\n(\s*\n)+", "\n\n", "\n".join(line.strip() for line in text.splitlines())).strip()
    return {"from": "other", "sentAt": datetime.combine(day, time(), tz).isoformat(), "text": body}


def landed_on(conversation: dict[str, Any]) -> str | None:
    """The thread ID of the page get_conversation read, if it is a thread."""
    path = THREAD_PATH.match(urlparse(conversation.get("url", "")).path)
    return unquote(path["id"]) if path else None


# The end of a link to someone's profile: "Jane’s profile", or "Chris’
# profile" for a name ending in s.
POSSESSIVE_PROFILE = r"[’']s? profile$"
# How the references label a link to someone's profile: "Open Jane
# Recruiter’s profile", as on 2026-11-16 (#94).
PROFILE_LABEL = re.compile(r"^(?:Open |View )?(?P<name>.+?)" + POSSESSIVE_PROFILE)


def person(conversation: dict[str, Any], name: str) -> dict[str, str]:
    """The other person in a conversation: their name, and their profile if
    the page links to it under their name."""
    found = {"name": name}
    if not name:
        # Every reference would match, the Owner's own profile first.
        return found
    people = []
    for ref in conversation.get("references", {}).get("conversation", []):
        if ref.get("kind") == "person":
            label = PROFILE_LABEL.match(ref.get("text", ""))
            people.append((label["name"] if label else ref.get("text", ""), ref["url"]))
    # The same name first: the Owner's own link comes first, and their name
    # may start with the other person's.
    match = next((url for label, url in people if normal_name(label) == normal_name(name)), None)
    match = match or next((url for label, url in people if same_person(label, name)), None)
    if match:
        found["profileUrl"] = urljoin(LINKEDIN, match)
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


async def open_inbox(stack: AsyncExitStack, start_reader, import_session) -> tuple["Reader", Any] | dict[str, str]:
    """Starts the reader, signed in to LinkedIn, on stack, and reads the
    inbox. Answers the reader and the inbox, or the problem to tell Nando.

    With no session saved, or a saved one LinkedIn turns away, it imports the
    session of the Owner's Chrome, at most once, and starts the reader again
    on it. It never asks for the inbox with no session saved: the reader would
    open a LinkedIn login window on the Mac, every run."""
    imported = False
    while True:
        attempt = AsyncExitStack()
        kept = False
        try:
            reader = Reader(await attempt.enter_async_context(start_reader()))
            # The reader checks only that a session is saved, never LinkedIn.
            session = await reader.call("get_session_status")
            if session["session"]["stored"]:
                try:
                    inbox = await reader.call("get_inbox", {"limit": INBOX_LIMIT})
                except ToolError as e:
                    if e.code != "auth_required" or imported:
                        log.warning("The reader couldn't read the inbox: %s", e)
                        return reader_problem(e)
                    # The reader moves the dead session aside.
                    log.info("LinkedIn turned the saved session away.")
                else:
                    stack.push_async_exit(attempt)
                    kept = True
                    return reader, inbox
            elif imported:
                return {"kind": "session_expired"}
            else:
                log.info("No LinkedIn session is saved.")
        finally:
            if not kept:
                await attempt.aclose()
        log.info("Importing the LinkedIn session of the Owner's Chrome.")
        if not await import_session():
            log.warning("Chrome has no LinkedIn session to import: log in with the reader's --login.")
            return {"kind": "session_expired"}
        imported = True


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


# A group heading for screen readers, which LinkedIn writes when the sender
# changes, right above the first message's profile link.
GROUP_HEADING = re.compile(r"sent the following messages? at \d{1,2}:\d{2}\s?[AP]M$", re.IGNORECASE)
# Each message's own line: the sender's name, their pronouns if they show
# them, and the time, two spaces or more apart.
SENDER_LINE = re.compile(r"^(?P<name>\S.*?)\s{2,}(?:\([^)]*\)\s{2,})?(?P<time>\d{1,2}:\d{2}\s?[AP]M)$", re.IGNORECASE)
# The link to the sender's profile, right above their line.
# "View Jane’s profile", or "View Chris’ profile".
PROFILE_LINK = re.compile(r"^View .+" + POSSESSIVE_PROFILE)
# LinkedIn's quick replies under an InMail: this line, then the reply itself.
QUICK_REPLY = re.compile(r"^Reply to conversation with [“\"](?P<reply>.*)[”\"]$")
# The Owner's reaction under a message: this line, then the emoji and a count.
REACTIONS = "Remove reaction"
# The text of a message that has none on the page, such as an image.
NO_TEXT = "(no text)"


def parse_conversation(text: str, *, other: str, today: date, tz: tzinfo) -> list[dict[str, str]]:
    """The messages in get_conversation's page text, oldest first.

    Each message starts with its sender line, right under a link to their
    profile:

        OCT 13
        Jane Recruiter sent the following messages at 4:43 PM
        View Jane’s profile
        Jane Recruiter  (She/Her)  4:43 PM

        Would you have any feedback?

    A day heading, in capitals, comes when the Day changes, and a group
    heading when the sender does. other is the other person's name, as the
    inbox gives it: a message under another name is the Owner's. today and tz
    date the day headings, which are in the Mac's time zone.
    """
    lines = [line.strip() for line in text.splitlines()]
    nonempty = [i for i, line in enumerate(lines) if line]
    # The next and previous non-empty lines' indexes.
    following = {i: nonempty[k + 1] if k + 1 < len(nonempty) else None for k, i in enumerate(nonempty)}
    preceding = {i: nonempty[k - 1] if k else None for k, i in enumerate(nonempty)}

    def line_after(i: int) -> str:
        j = following.get(i)
        return lines[j] if j is not None else ""

    def is_profile_link(i: int) -> bool:
        """A profile link, right above a sender line."""
        return bool(PROFILE_LINK.match(lines[i]) and SENDER_LINE.match(line_after(i)))

    def is_group_heading(i: int) -> bool:
        j = following.get(i)
        return bool(GROUP_HEADING.search(lines[i]) and j is not None and is_profile_link(j))

    def starts_message(i: int) -> bool:
        """A group heading or profile link: what comes right after a day
        heading."""
        return is_group_heading(i) or is_profile_link(i)

    messages: list[dict[str, Any]] = []
    day: date | None = None
    current: dict[str, Any] | None = None
    reacting = False
    quick_reply: str | None = None
    for i, line in enumerate(lines):
        if not line:
            if current is not None and current["lines"]:
                current["lines"].append(line)
            continue
        sender = SENDER_LINE.match(line)
        p = preceding[i]
        if sender and p is not None and is_profile_link(p):
            at = datetime.combine(day, parse_time(sender["time"]), tz) if day else None
            current = {"from": "other" if same_person(sender["name"], other) else "owner", "at": at, "lines": []}
            messages.append(current)
            reacting = False
            continue
        # A day heading is in capitals, comes right before a message, and
        # never inside a message's text: a line or a reaction ends that first.
        # So a message saying just "Monday", or ending in "FRIDAY", stays so.
        follows_text = i > 0 and bool(lines[i - 1]) and current is not None and bool(current["lines"]) and not reacting
        j = following[i]
        if line == line.upper() and not follows_text and j is not None and starts_message(j):
            heading_day = parse_day(line, today)
            if heading_day:
                day = heading_day
                continue
        if is_group_heading(i) or is_profile_link(i):
            continue
        if current is None:
            continue
        if line == REACTIONS:
            reacting = True
            continue
        if reacting and is_reaction(line):
            continue
        reacting = False
        if offered := QUICK_REPLY.match(line):
            quick_reply = offered["reply"]
            continue
        if line == quick_reply:
            quick_reply = None
            continue
        quick_reply = None
        current["lines"].append(line)
    return [
        {"from": m["from"], "sentAt": m["at"].isoformat(), "text": "\n".join(m["lines"]).strip() or NO_TEXT}
        for m in messages
        if m["at"] is not None
    ]


def is_reaction(line: str) -> bool:
    """An emoji or a count, under REACTIONS."""
    return line.isdigit() or all(not ch.isalnum() for ch in line)


def same_person(sender: str, inbox_name: str) -> bool:
    """Whether a sender line's name is the other person's, as the inbox gives
    it. Either may add a reading in brackets, an emoji or a title the other
    leaves out, so one only has to start the other, up to a word's end."""
    a, b = normal_name(sender), normal_name(inbox_name)
    if not a or not b:
        return False
    shorter, longer = sorted((a, b), key=len)
    return longer == shorter or (longer.startswith(shorter) and not longer[len(shorter)].isalnum())


def normal_name(name: str) -> str:
    return " ".join(unicodedata.normalize("NFKC", name).casefold().split())


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


# How the reader is started for a Sync. Never --import-from-browser or its
# auto-import: left to itself, a failed import falls back to a LinkedIn login
# window. The Sync imports Chrome's session itself (import_chrome_session).
# Never waiting on a login window either: the Owner logs in with --login.
READER_ARGS = [READER, "--transport", "stdio", "--no-auto-import", "--login-inline-wait", "0"]

# No remote tool catalog or update check: the script calls only its three
# Tools, and an update note would be added to their answers.
READER_ENV = {"LINKEDIN_MCP_REMOTE_TOOLS": "off", "LINKEDIN_MCP_CHECK_FOR_UPDATES": "off"}

# The reader's one-shot import of the LinkedIn session in the Owner's Chrome,
# and only Chrome: never another browser's, perhaps another account's. It runs
# once and exits, 0 when it imported a session LinkedIn accepts, and never
# opens a login window.
IMPORT_ARGS = [READER, "--import-from-browser", "chrome"]

# How long the import may take: it may download its browser on a first run,
# and the Keychain may wait on the Owner's answer.
IMPORT_TIMEOUT = 180


@asynccontextmanager
async def start_reader():
    """Starts the reader over stdio, for one Sync."""
    stay_awake()
    params = StdioServerParameters(command="uvx", args=READER_ARGS, env=READER_ENV)
    async with Client(params) as client:
        yield client


async def import_chrome_session() -> bool:
    """Imports the LinkedIn session of the Owner's Chrome into the reader's
    profile, and says whether it did. The first time, macOS may ask to let it
    read "Chrome Safe Storage" from the Keychain."""
    proc = await asyncio.create_subprocess_exec(
        "uvx", *IMPORT_ARGS, env={**os.environ, **READER_ENV}, stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.STDOUT
    )
    try:
        out, _ = await asyncio.wait_for(proc.communicate(), IMPORT_TIMEOUT)
    except TimeoutError:
        proc.kill()
        await proc.wait()
        log.warning("Importing Chrome's LinkedIn session took over %d s: stopped it.", IMPORT_TIMEOUT)
        return False
    for line in out.decode(errors="replace").splitlines():
        if line.strip():
            log.info("Import: %s", line.strip())
    return proc.returncode == 0


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


def awake() -> bool:
    """Whether the Mac is fully awake, rather than in a dark wake: with the
    lid closed, Power Nap wakes it every 15 to 20 minutes for a few seconds,
    without its screen, and launchd starts the runs it missed then. A run
    started there would carry on in slices of a few seconds over hours, the
    reader's browser couldn't start without a screen, and the access token
    could expire between checking it and using it. Only a full wake has the
    Graphics capability. If pmset can't tell, the Mac counts as awake."""
    try:
        out = subprocess.run(["pmset", "-g", "systemstate"], capture_output=True, text=True, timeout=10).stdout
    except (OSError, subprocess.SubprocessError):
        return True
    capabilities = re.search(r"Capabilities are:(.*)", out)
    return capabilities is None or "Graphics" in capabilities.group(1).split()


@functools.cache
def stay_awake() -> None:
    """Keeps the Mac from sleeping when idle until this run ends, so that a
    Sync isn't cut into slices by sleep. Closing the lid still sleeps it.
    Once a run, when the reader first starts: a run with no Sync due ends
    in seconds."""
    try:
        subprocess.Popen(
            ["caffeinate", "-i", "-w", str(os.getpid())],
            stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
    except OSError as e:
        log.info("Couldn't start caffeinate (%s): the Mac may sleep during this run.", e)


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

# How long before the access token expires a run refreshes it, in seconds.
EXPIRY_MARGIN = 300


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
    run from launchd can't. So it reads the metadata itself before a refresh. It
    refreshes EXPIRY_MARGIN early, and once more if Nando turns a token away
    anyway (_auth_flow), before asking for --connect.

    It leans on mcp 2.3.0's internals (_initialize, _auth_flow,
    _refresh_token and context): test_auth.py checks them when the SDK is
    updated."""

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
        self._expire_early()

    async def _handle_token_response(self, response: httpx2.Response) -> None:
        await super()._handle_token_response(response)
        self._expire_early()

    async def _handle_refresh_response(self, response: httpx2.Response) -> bool:
        refreshed = await super()._handle_refresh_response(response)
        self._expire_early()
        return refreshed

    def _expire_early(self) -> None:
        """Takes the access token as expired EXPIRY_MARGIN early, however it
        was got: loaded, exchanged or refreshed. Saved tokens keep the real
        expiry."""
        if self.context.token_expiry_time:
            self.context.token_expiry_time -= EXPIRY_MARGIN

    async def _auth_flow(self, request: httpx2.Request) -> AsyncGenerator[httpx2.Request, httpx2.Response]:
        """The SDK's flow, except that when Nando turns away an access token
        the script took for valid, it refreshes the token and tries once
        more before asking for --connect. The SDK would go straight to the
        browser. A token expires unseen when the Mac sleeps between checking
        it and sending it, or when the Mac's clock and Nando's disagree."""
        retried = False
        flow = super()._auth_flow(request)
        try:
            outgoing = await flow.__anext__()
            while True:
                response = yield outgoing
                if not retried and outgoing is request and response.status_code == 401 and self.context.can_refresh_token():
                    retried = True
                    await flow.aclose()
                    log.info("Nando turned the access token away: refreshing it.")
                    # In the past, not 0: the SDK takes no expiry time as never expiring.
                    self.context.token_expiry_time = clock.time() - 1
                    # The SDK adds a header only for a token it holds as valid,
                    # and never removes one: if the refresh fails, the request
                    # goes without the token Nando just turned away.
                    request.headers.pop("Authorization", None)
                    flow = super()._auth_flow(request)
                    outgoing = await flow.__anext__()
                    continue
                outgoing = await flow.asend(response)
        except StopAsyncIteration:
            return
        finally:
            await flow.aclose()

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
            raise NandoUnavailable(f"Couldn't read Nando's OAuth metadata ({e}): the next run tries again.") from e
        if metadata is None or not issuers_match(str(metadata.issuer), issuer):
            raise NandoUnavailable(
                f"Couldn't read Nando's OAuth metadata ({response.status_code} at {url}): the next run tries again."
            )
        return metadata


class NotConnected(Exception):
    """Nando needs the Owner to approve the script again."""


class NandoUnavailable(Exception):
    """Nando couldn't be asked how to refresh the access token. The tokens
    are kept, and the next run tries again."""


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
        await run_sync(nando, start_reader, now=datetime.now(local_zone()), pause=random_pause, import_session=import_chrome_session)


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
    if not awake():
        log.info("The Mac is in a dark wake: the next run tries again.")
        return 0
    if not online():
        log.info("No network: the next run tries again.")
        return 0
    try:
        asyncio.run(hourly())
    except Exception as e:
        messages = [why_nando_ended(error) for error in leaves(e)]
        if all(messages):
            for message in dict.fromkeys(messages):
                log.error("%s", message)
        else:
            log.exception("The run failed.")
        return 1
    return 0


def why_nando_ended(error: BaseException) -> str | None:
    """One line for an error that only means Nando couldn't be used this
    run, or None for any other, which is logged with its traceback."""
    if isinstance(error, (NotConnected, NandoUnavailable)):
        return str(error)
    # Such as a connection reset just after a wake, before the network is
    # back. Not a timeout reading an answer, a bad URL or a proxy: those
    # won't pass by themselves, so they keep their traceback.
    if isinstance(error, (httpx2.NetworkError, httpx2.RemoteProtocolError, httpx2.ConnectTimeout)):
        return f"Lost the connection to Nando ({type(error).__name__}): the next run tries again."
    return None


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
