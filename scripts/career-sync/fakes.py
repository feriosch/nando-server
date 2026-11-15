"""Fake MCP servers standing in for Nando and the LinkedIn reader in tests."""

from contextlib import asynccontextmanager
from typing import Any

from mcp import Client
from mcp.server.mcpserver import MCPServer


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
        def career_record_sync(threads: list[dict[str, Any]] | None = None, problem: dict[str, Any] | None = None) -> dict[str, Any]:
            call: dict[str, Any] = {"threads": threads or []}
            if problem is not None:
                call["problem"] = problem
            self.recorded.append(call)
            return {"sync": f"sync{len(self.recorded)}", "stored": 0}

    def client(self) -> Client:
        return Client(self.server)


class FakeReader:
    """The LinkedIn reader's Tools, with an inbox and conversations a test sets."""

    def __init__(self) -> None:
        self.started = 0
        self.calls: list[str] = []
        self.server = MCPServer("linkedin")

        @self.server.tool()
        def send_message(linkedin_username: str, message: str) -> dict[str, Any]:
            self.calls.append("send_message")
            return {"status": "sent"}

    @asynccontextmanager
    async def start(self):
        self.started += 1
        async with Client(self.server) as client:
            yield client
