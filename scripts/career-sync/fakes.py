"""Fake MCP servers standing in for Nando and the LinkedIn reader in tests.

The reader's answers follow mcp-server-linkedin 4.30.0, as README.md records.
"""

import json
from contextlib import asynccontextmanager
from dataclasses import dataclass
from typing import Any
from urllib.parse import quote, unquote

from mcp import Client
from mcp.server.mcpserver import MCPServer
from mcp.shared.exceptions import MCPError
from mcp_types import CallToolResult, TextContent


class FakeNando:
    """Nando's two Sync Tools, answering with what a test sets."""

    def __init__(self, *, due: bool, threads: list[dict[str, Any]] | None = None) -> None:
        self.due = due
        self.threads = threads or []
        self.recorded: list[dict[str, Any]] = []
        self.server = MCPServer("nando")

        @self.server.tool()
        def career_sync_status() -> dict[str, Any]:
            return {"due": self.due, "since": "2025-01-01", "threads": self.threads}

        @self.server.tool()
        def career_record_sync(
            threads: list[dict[str, Any]] | None = None, problem: dict[str, Any] | None = None, sync: str | None = None
        ) -> dict[str, Any]:
            call: dict[str, Any] = {"threads": threads or []}
            if problem is not None:
                call["problem"] = problem
            if sync is not None:
                call["sync"] = sync
            self.recorded.append(call)
            # A later batch is added to the Sync its first batch started.
            return {"sync": sync or f"sync{len(self.recorded)}", "stored": 0}

    def client(self) -> Client:
        return Client(self.server)


def answer(result: dict[str, Any]) -> CallToolResult:
    """A reader Tool's answer: its JSON as text and as structured content."""
    return CallToolResult(content=[TextContent(type="text", text=json.dumps(result))], structured_content=result)


def reader_error(code: str, text: str) -> CallToolResult:
    """A reader Tool's failure, with its error code."""
    return CallToolResult(
        content=[TextContent(type="text", text=text)],
        is_error=True,
        _meta={"linkedin.dev/error": {"v": 1, "code": code}},
    )


def auth_required() -> CallToolResult:
    """What the reader answers when LinkedIn's session has expired."""
    return reader_error(
        "auth_required",
        "Session expired. A login browser window has been opened. Sign in with your LinkedIn credentials there, then retry this tool.",
    )


def rate_limited() -> CallToolResult:
    return reader_error("rate_limited", "Rate limit detected. Wait 30 seconds before trying again.")


@dataclass
class Thread:
    """One LinkedIn thread in the fake reader's inbox."""

    thread_id: str
    name: str
    # text is get_conversation's page text for it.
    text: str
    profile: str = "/in/ACoAAOther/"
    # error, if set, is what get_conversation answers instead.
    error: CallToolResult | None = None
    # landed, if set, is the thread LinkedIn opened instead of this one.
    landed: str | None = None
    # broken, if set, makes get_conversation fail as a protocol error, as when
    # the reader stops answering, instead of as a Tool's error.
    broken: bool = False


class FakeReader:
    """The LinkedIn reader's Tools, with a session, inbox and conversations a test sets."""

    def __init__(
        self,
        *,
        stored: bool = True,
        inbox_error: CallToolResult | None = None,
        inbox: list["Thread"] | None = None,
        chrome_session: bool = False,
    ) -> None:
        self.stored = stored
        # chrome_session is whether the Owner's Chrome holds a LinkedIn
        # session that import_session can import.
        self.chrome_session = chrome_session
        self.imports = 0
        # inbox_error, if set, is what get_inbox answers instead.
        self.inbox_error = inbox_error
        # inbox is the threads get_inbox lists, most recent first.
        self.inbox = inbox or []
        self.started = 0
        self.calls: list[str] = []
        self.opened: list[str] = []
        self.server = MCPServer("linkedin")

        @self.server.tool()
        def get_session_status() -> dict[str, Any]:
            self.calls.append("get_session_status")
            return {"scope": "serving_process", "role": "direct", "session": {"stored": self.stored, "profile": self.stored, "cookies": self.stored}}

        @self.server.tool()
        def get_inbox(limit: int = 20) -> CallToolResult:
            self.calls.append("get_inbox")
            if self.inbox_error:
                return self.inbox_error
            refs = [
                {"kind": "conversation", "url": f"/messaging/thread/{quote(t.thread_id, safe='')}/", "context": "inbox", "text": t.name}
                for t in self.inbox[:limit]
            ]
            # The inbox's own text isn't read, so any will do.
            return answer({"url": "https://www.linkedin.com/messaging/", "sections": {"inbox": "Messaging"}, "references": {"inbox": refs}})

        @self.server.tool()
        def get_conversation(linkedin_username: str | None = None, thread_id: str | None = None, index: int = 0) -> CallToolResult:
            self.calls.append("get_conversation")
            thread_id = unquote(thread_id or "")
            self.opened.append(thread_id)
            thread = next(t for t in self.inbox if t.thread_id == thread_id)
            if thread.error:
                return thread.error
            if thread.broken:
                raise MCPError(code=-32603, message="the reader stopped answering")
            return answer(
                {
                    "url": f"https://www.linkedin.com/messaging/thread/{quote(thread.landed or thread_id, safe='')}/",
                    "sections": {"conversation": thread.text},
                    "references": {
                        "conversation": [
                            # As LinkedIn labels the links on 2026-11-16 (#94).
                            {"kind": "person", "url": "/in/ACoAAOwner/", "context": "conversation", "text": "Open Sam Owner’s profile"},
                            {"kind": "person", "url": thread.profile, "context": "conversation", "text": f"Open {thread.name}’s profile"},
                        ]
                    },
                }
            )

        @self.server.tool()
        def send_message(linkedin_username: str, message: str) -> dict[str, Any]:
            self.calls.append("send_message")
            return {"status": "sent"}

    async def import_session(self) -> bool:
        """The reader's --import-from-browser chrome: with a session in Chrome,
        a fresh one is saved, which LinkedIn accepts."""
        self.imports += 1
        if self.chrome_session:
            self.stored = True
            self.inbox_error = None
        return self.chrome_session

    @asynccontextmanager
    async def start(self):
        self.started += 1
        async with Client(self.server) as client:
            yield client
