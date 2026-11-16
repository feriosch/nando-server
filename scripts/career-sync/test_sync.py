from datetime import datetime
from zoneinfo import ZoneInfo

import pytest

from fakes import FakeNando, FakeReader, Thread, auth_required, reader_error, rate_limited
from sync import Reader, ReaderToolRefused, run_sync

pytestmark = pytest.mark.anyio


@pytest.fixture
def anyio_backend():
    return "asyncio"


NOW = datetime(2026, 11, 15, 12, 0, tzinfo=ZoneInfo("Asia/Tokyo"))


async def no_pause() -> None:
    pass


async def sync(nando: FakeNando, reader: FakeReader) -> None:
    async with nando.client() as client:
        await run_sync(client, reader.start, now=NOW, pause=no_pause)


async def test_nothing_is_opened_when_no_sync_is_due():
    nando = FakeNando(due=False)
    reader = FakeReader()

    await sync(nando, reader)

    assert reader.started == 0
    assert nando.recorded == []


async def test_a_session_linkedin_no_longer_holds_is_reported_as_expired():
    nando = FakeNando(due=True)
    reader = FakeReader(stored=False)

    await sync(nando, reader)

    assert nando.recorded == [{"threads": [], "problem": {"kind": "session_expired"}}]
    assert "get_inbox" not in reader.calls


async def test_a_session_linkedin_turns_away_is_reported_as_expired():
    nando = FakeNando(due=True)
    reader = FakeReader(inbox_error=auth_required())

    await sync(nando, reader)

    assert nando.recorded == [{"threads": [], "problem": {"kind": "session_expired"}}]


async def test_only_new_messages_since_2025_of_threads_not_ignored_are_sent():
    nando = FakeNando(
        due=True,
        threads=[
            {"threadId": "2-known==", "newest": "2026-11-14T09:00:00+09:00", "ignored": False},
            {"threadId": "2-ignored==", "newest": "2026-11-01T09:00:00+09:00", "ignored": True},
        ],
    )
    reader = FakeReader(
        inbox=[
            Thread(
                "2-known==",
                "Jane Recruiter",
                day("YESTERDAY") + message("Jane Recruiter", "9:00 AM", "Already in Nando.") + day("TODAY") + message("Jane Recruiter", "10:00 AM", "New since then."),
            ),
            Thread("2-ignored==", "Pat Friend", day("TODAY") + message("Pat Friend", "9:30 AM", "Lunch?")),
            Thread(
                "2-new==",
                "Kim Hunter",
                day("DEC 30, 2024") + message("Kim Hunter", "8:00 AM", "Before 2025.") + day("JAN 2, 2025") + message("Kim Hunter", "8:00 AM", "A role for you."),
            ),
        ]
    )

    await sync(nando, reader)

    assert "2-ignored==" not in reader.opened
    assert nando.recorded == [
        {
            "threads": [
                {
                    "threadId": "2-known==",
                    "url": "https://www.linkedin.com/messaging/thread/2-known%3D%3D/",
                    "person": {"name": "Jane Recruiter", "profileUrl": "https://www.linkedin.com/in/ACoAAOther/"},
                    "messages": [{"from": "other", "sentAt": "2026-11-15T10:00:00+09:00", "text": "New since then."}],
                },
                {
                    "threadId": "2-new==",
                    "url": "https://www.linkedin.com/messaging/thread/2-new%3D%3D/",
                    "person": {"name": "Kim Hunter", "profileUrl": "https://www.linkedin.com/in/ACoAAOther/"},
                    "messages": [{"from": "other", "sentAt": "2025-01-02T08:00:00+09:00", "text": "A role for you."}],
                },
            ]
        }
    ]


def day(heading: str) -> str:
    """A day heading in a thread's page text."""
    return f"{heading}\n"


def message(name: str, at: str, text: str) -> str:
    """One message in a thread's page text, as LinkedIn lays it out."""
    return f"{name} sent the following message at {at}\nView {name.split()[0]}’s profile\n{name}  (She/Her)  {at}\n\n{text}\n\n"


def today_message(name: str, at: str, text: str) -> str:
    """A thread's page text with one message from name, at a time TODAY."""
    return day("TODAY") + message(name, at, text)


async def test_the_first_thread_with_nothing_new_ends_the_sync():
    nando = FakeNando(due=True, threads=[{"threadId": "2-old==", "newest": "2026-11-15T09:00:00+09:00", "ignored": False}])
    reader = FakeReader(
        inbox=[
            Thread("2-fresh==", "Jane Recruiter", today_message("Jane Recruiter", "11:00 AM", "Hi")),
            Thread("2-old==", "Kim Hunter", today_message("Kim Hunter", "9:00 AM", "Already in Nando.")),
            Thread("2-older==", "Lee Agent", today_message("Lee Agent", "8:00 AM", "Never opened.")),
        ]
    )

    await sync(nando, reader)

    assert reader.opened == ["2-fresh==", "2-old=="]
    assert [t["threadId"] for t in nando.recorded[0]["threads"]] == ["2-fresh=="]


async def test_at_most_30_threads_are_opened_with_a_pause_between_each():
    nando = FakeNando(due=True)
    reader = FakeReader(
        inbox=[Thread(f"2-t{n}==", f"Recruiter {n}", today_message(f"Recruiter {n}", "11:00 AM", "Hi")) for n in range(40)]
    )
    pauses = []

    async def pause() -> None:
        pauses.append(len(reader.opened))

    async with nando.client() as client:
        await run_sync(client, reader.start, now=NOW, pause=pause)

    assert reader.opened == [f"2-t{n}==" for n in range(30)]
    assert pauses == list(range(1, 30))


async def test_new_messages_are_sent_in_batches_of_10_threads():
    nando = FakeNando(due=True)
    reader = FakeReader(
        inbox=[Thread(f"2-t{n}==", f"Recruiter {n}", today_message(f"Recruiter {n}", "11:00 AM", "Hi")) for n in range(25)]
    )

    await sync(nando, reader)

    assert [[t["threadId"] for t in call["threads"]] for call in nando.recorded] == [
        [f"2-t{n}==" for n in range(0, 10)],
        [f"2-t{n}==" for n in range(10, 20)],
        [f"2-t{n}==" for n in range(20, 25)],
    ]


async def test_later_batches_name_the_sync_the_first_started():
    nando = FakeNando(due=True)
    reader = FakeReader(
        inbox=[Thread(f"2-t{n}==", f"Recruiter {n}", today_message(f"Recruiter {n}", "11:00 AM", "Hi")) for n in range(25)]
    )

    await sync(nando, reader)

    assert [call.get("sync") for call in nando.recorded] == [None, "sync1", "sync1"]


async def test_a_sync_with_nothing_new_is_still_recorded():
    nando = FakeNando(due=True, threads=[{"threadId": "2-old==", "newest": "2026-11-15T09:00:00+09:00", "ignored": False}])
    reader = FakeReader(inbox=[Thread("2-old==", "Kim Hunter", today_message("Kim Hunter", "9:00 AM", "Already in Nando."))])

    await sync(nando, reader)

    assert nando.recorded == [{"threads": []}]


async def test_a_thread_whose_messages_cant_be_read_is_reported():
    nando = FakeNando(due=True)
    reader = FakeReader(
        inbox=[
            Thread("2-fresh==", "Jane Recruiter", today_message("Jane Recruiter", "11:00 AM", "Hi")),
            Thread("2-odd==", "Kim Hunter", "A page laid out in a way the script doesn't know"),
            Thread("2-later==", "Lee Agent", today_message("Lee Agent", "8:00 AM", "Never opened.")),
        ]
    )

    await sync(nando, reader)

    assert reader.opened == ["2-fresh==", "2-odd=="]
    assert [[t["threadId"] for t in call["threads"]] for call in nando.recorded] == [["2-fresh=="], []]
    assert nando.recorded[1]["problem"]["kind"] == "other"
    assert "2-odd==" in nando.recorded[1]["problem"]["text"]


async def test_an_inbox_the_reader_cant_read_is_reported():
    nando = FakeNando(due=True)
    reader = FakeReader(inbox_error=reader_error("browser_unavailable", "LinkedIn setup is not complete yet."))

    await sync(nando, reader)

    assert nando.recorded == [
        {"threads": [], "problem": {"kind": "other", "text": "get_inbox: LinkedIn setup is not complete yet."}}
    ]


async def test_a_thread_the_inbox_gives_no_name_for_gets_no_profile():
    nando = FakeNando(due=True)
    reader = FakeReader(inbox=[Thread("2-fresh==", "", today_message("Jane Recruiter", "11:00 AM", "Hi"))])

    await sync(nando, reader)

    assert nando.recorded[0]["threads"][0]["person"] == {"name": ""}


async def test_a_reader_that_stops_answering_is_reported_after_what_was_read():
    nando = FakeNando(due=True)
    reader = FakeReader(
        inbox=[
            Thread("2-fresh==", "Jane Recruiter", today_message("Jane Recruiter", "11:00 AM", "Hi")),
            Thread("2-broken==", "Kim Hunter", "", broken=True),
        ]
    )

    await sync(nando, reader)

    assert [[t["threadId"] for t in call["threads"]] for call in nando.recorded] == [["2-fresh=="], []]
    assert nando.recorded[1]["problem"] == {"kind": "other", "text": "get_conversation: the reader stopped answering"}


async def test_a_thread_linkedin_opens_another_page_for_is_reported():
    nando = FakeNando(due=True)
    reader = FakeReader(
        inbox=[
            Thread("2-fresh==", "Jane Recruiter", today_message("Jane Recruiter", "11:00 AM", "Hi")),
            Thread("2-gone==", "Kim Hunter", today_message("Lee Agent", "10:00 AM", "Someone else's thread."), landed="2-other=="),
        ]
    )

    await sync(nando, reader)

    assert [[t["threadId"] for t in call["threads"]] for call in nando.recorded] == [["2-fresh=="], []]
    assert nando.recorded[1]["problem"]["kind"] == "other"
    assert "2-gone==" in nando.recorded[1]["problem"]["text"]


async def test_a_reader_error_is_reported_after_what_was_read():
    nando = FakeNando(due=True)
    reader = FakeReader(
        inbox=[
            Thread("2-fresh==", "Jane Recruiter", today_message("Jane Recruiter", "11:00 AM", "Hi")),
            Thread("2-limited==", "Kim Hunter", "", error=rate_limited()),
        ]
    )

    await sync(nando, reader)

    assert nando.recorded == [
        {"threads": [nando.recorded[0]["threads"][0]]},
        {"threads": [], "problem": {"kind": "other", "text": "get_conversation: Rate limit detected. Wait 30 seconds before trying again."}},
    ]
    assert nando.recorded[0]["threads"][0]["threadId"] == "2-fresh=="


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


# A Sponsored Message (an InMail ad), laid out as get_conversation gave one on
# 2026-11-16: no "sent the following message" headings. Names and text made up.
SPONSORED = """Sponsored

Nov 13
Owner, your invitation — Example Fellowship
Apply Now
Sam Admissions, PhD
Sam Admissions, PhD

Hi Owner,




I lead Admissions at Example School. I would like to invite you to apply.




Best regards,

Sam Admissions, PhD

Apply Now
Not interested

Example School | Somewhere

"""


async def test_a_sponsored_message_is_skipped_and_the_sync_carries_on():
    nando = FakeNando(due=True)
    reader = FakeReader(
        inbox=[
            Thread("2-ad==", "Sam Admissions, PhD", SPONSORED),
            Thread("2-new==", "Kim Hunter", today_message("Kim Hunter", "10:00 AM", "A role for you.")),
        ]
    )

    await sync(nando, reader)

    assert reader.opened == ["2-ad==", "2-new=="]
    assert nando.recorded == [
        {
            "threads": [
                {
                    "threadId": "2-new==",
                    "url": "https://www.linkedin.com/messaging/thread/2-new%3D%3D/",
                    "person": {"name": "Kim Hunter", "profileUrl": "https://www.linkedin.com/in/ACoAAOther/"},
                    "messages": [{"from": "other", "sentAt": "2026-11-15T10:00:00+09:00", "text": "A role for you."}],
                }
            ]
        }
    ]
