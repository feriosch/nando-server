# /// script
# requires-python = ">=3.12"
# dependencies = ["mcp==2.3.0"]
# ///
"""The Career Sync: hands the Owner's new LinkedIn messages to Nando (ADR 0007)."""

import json
from typing import Any

from mcp import Client


async def call(client: Client, tool: str, arguments: dict[str, Any] | None = None) -> Any:
    """Calls tool and answers its JSON result, or raises with the Tool's error."""
    result = await client.call_tool(tool, arguments or {})
    text = "".join(c.text for c in result.content if c.type == "text")
    if result.is_error:
        raise ToolError(tool, text)
    if result.structured_content is not None:
        return result.structured_content
    return json.loads(text)


class ToolError(Exception):
    """A Tool answered with an error."""

    def __init__(self, tool: str, text: str) -> None:
        super().__init__(f"{tool}: {text}")
        self.tool = tool
        self.text = text


async def run_sync(nando: Client, start_reader) -> None:
    """One hourly run: a Sync, if Nando says one is due."""
    status = await call(nando, "career_sync_status")
    if not status["due"]:
        return


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
        return await call(self._client, tool, arguments)
