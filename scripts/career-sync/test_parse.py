from datetime import date
from zoneinfo import ZoneInfo

from sync import parse_conversation

TOKYO = ZoneInfo("Asia/Tokyo")
SUNDAY = date(2026, 11, 15)


def parse(text: str):
    return parse_conversation(text, other="Jane Recruiter", today=SUNDAY, tz=TOKYO)


def test_a_message_from_the_other_person():
    text = """TODAY
Jane Recruiter sent the following message at 10:42 AM
View Jane Recruiter’s profile
Jane Recruiter
Jane Recruiter
 10:42 AM
Hi, I have a backend role in Tokyo.
Would you be open to a call?"""

    assert parse(text) == [
        {
            "from": "other",
            "sentAt": "2026-11-15T10:42:00+09:00",
            "text": "Hi, I have a backend role in Tokyo.\nWould you be open to a call?",
        }
    ]


def test_messages_across_days_and_the_owners_reply():
    text = """DEC 12, 2024
Jane Recruiter sent the following message at 9:00 AM
Jane Recruiter
 9:00 AM
Hello from last year.
MAR 3
Jane Recruiter sent the following message at 6:15 PM
Jane Recruiter
 6:15 PM
Are you still looking?
THURSDAY
Sam Owner sent the following message at 11:05 PM
View Sam Owner’s profile
Sam Owner
Sam Owner
 11:05 PM
Sorry for the delay, yes.
YESTERDAY
Jane Recruiter sent the following messages at 8:30 AM
Jane Recruiter
 8:30 AM
Great!"""

    assert [(m["from"], m["sentAt"], m["text"]) for m in parse(text)] == [
        ("other", "2024-12-12T09:00:00+09:00", "Hello from last year."),
        ("other", "2026-03-03T18:15:00+09:00", "Are you still looking?"),
        ("owner", "2026-11-12T23:05:00+09:00", "Sorry for the delay, yes."),
        ("other", "2026-11-14T08:30:00+09:00", "Great!"),
    ]


def test_a_day_without_a_year_after_today_is_last_year():
    text = """DEC 24
Jane Recruiter sent the following message at 9:00 AM
Merry Christmas!"""

    assert parse(text)[0]["sentAt"] == "2025-12-24T09:00:00+09:00"


def test_a_message_saying_just_a_day_stays_a_message():
    text = """TODAY
Jane Recruiter sent the following message at 10:00 AM
When could you talk?
Sam Owner sent the following message at 10:05 AM
Monday
Jane Recruiter sent the following message at 10:06 AM
Great"""

    assert [m["text"] for m in parse(text)] == ["When could you talk?", "Monday", "Great"]


def test_messages_above_the_first_day_heading_are_left_out():
    # The reader scrolls up only three times, so a long thread's oldest
    # messages may arrive without their day heading.
    text = """Jane Recruiter sent the following message at 7:00 AM
An old message without its Day.
TODAY
Jane Recruiter sent the following message at 10:00 AM
A new one."""

    assert [m["text"] for m in parse(text)] == ["A new one."]


def test_lines_like_the_groups_header_are_kept_in_its_text():
    text = """TODAY
Jane Recruiter sent the following message at 10:42 AM
View Jane Recruiter’s profile
Jane Recruiter
10:42 AM
The role is on our careers page.
View the full job profile
Best,
Jane Recruiter"""

    assert parse(text)[0]["text"] == "The role is on our careers page.\nView the full job profile\nBest,\nJane Recruiter"
