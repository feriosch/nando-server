import pytest

from fakes import FakeNando, FakeReader
from sync import Reader, ReaderToolRefused, run_sync

pytestmark = pytest.mark.anyio


@pytest.fixture
def anyio_backend():
    return "asyncio"


async def test_nothing_is_opened_when_no_sync_is_due():
    nando = FakeNando(due=False)
    reader = FakeReader()

    async with nando.client() as client:
        await run_sync(client, reader.start)

    assert reader.started == 0
    assert nando.recorded == []


@pytest.mark.parametrize(
    "tool",
    ["send_message", "reply_to_conversation", "archive_conversation", "connect_with_person", "dismiss_job", "search_conversations"],
)
async def test_the_reader_refuses_any_tool_but_its_three_read_tools(tool):
    linkedin = FakeReader()

    async with linkedin.start() as client:
        with pytest.raises(ReaderToolRefused):
            await Reader(client).call(tool, {"linkedin_username": "someone", "message": "hi"})

    assert linkedin.calls == []
