"""parse_conversation, on get_conversation's page text as LinkedIn laid it out
on 2026-11-16 (#94). The layout is copied from real threads; names and text
are made up."""

from datetime import date
from zoneinfo import ZoneInfo

from sync import ad_message, is_ad, parse_conversation

TOKYO = ZoneInfo("Asia/Tokyo")
SUNDAY = date(2026, 11, 15)


def parse(text: str):
    return parse_conversation(text, other="Jane Recruiter", today=SUNDAY, tz=TOKYO)


def test_a_message_from_the_other_person():
    text = """Visit my website
TODAY
Jane Recruiter sent the following message at 10:42 AM
View Jane’s profile
Jane Recruiter  (She/Her)  10:42 AM

Hi, I have a backend role in Tokyo.
Would you be open to a call?
"""

    assert parse(text) == [
        {
            "from": "other",
            "sentAt": "2026-11-15T10:42:00+09:00",
            "text": "Hi, I have a backend role in Tokyo.\nWould you be open to a call?",
        }
    ]


def test_each_message_of_a_group_has_its_own_day_and_time():
    # A group heading comes only when the sender changes: the other person's
    # follow-ups on later Days come with just a day heading and their own
    # sender line.
    text = """OCT 1
Jane Recruiter sent the following messages at 11:15 AM
View Jane’s profile
Jane Recruiter  (She/Her)  11:15 AM

Any update?

OCT 5
View Jane’s profile
Jane Recruiter  (She/Her)  12:34 PM

@Sam Owner

OCT 13
View Jane’s profile
Jane Recruiter  (She/Her)  4:43 PM

Would you have any feedback?
"""

    assert [(m["from"], m["sentAt"], m["text"]) for m in parse(text)] == [
        ("other", "2026-10-01T11:15:00+09:00", "Any update?"),
        ("other", "2026-10-05T12:34:00+09:00", "@Sam Owner"),
        ("other", "2026-10-13T16:43:00+09:00", "Would you have any feedback?"),
    ]


def test_the_owners_replies_and_two_messages_on_one_day():
    text = """OCT 15
Sam Owner sent the following messages at 10:15 AM
View Sam’s profile
Sam Owner   10:15 AM

Hello Jane!
Thank you as always.

Jane Recruiter sent the following messages at 1:06 PM
View Jane’s profile
Jane Recruiter  (She/Her)  1:06 PM

Thanks.

View Jane’s profile
Jane Recruiter  (She/Her)  1:09 PM

What about these roles?

Example株式会社
バックエンドエンジニア
https://example.com/jobs/1
"""

    assert [(m["from"], m["sentAt"], m["text"]) for m in parse(text)] == [
        ("owner", "2026-10-15T10:15:00+09:00", "Hello Jane!\nThank you as always."),
        ("other", "2026-10-15T13:06:00+09:00", "Thanks."),
        ("other", "2026-10-15T13:09:00+09:00", "What about these roles?\n\nExample株式会社\nバックエンドエンジニア\nhttps://example.com/jobs/1"),
    ]


def test_day_headings_with_a_year_a_weekday_or_none_after_today():
    text = """OCT 23, 2025
Jane Recruiter sent the following messages at 11:17 AM
View Jane’s profile
Jane Recruiter  (She/Her)  11:17 AM

Last year.

DEC 24
View Jane’s profile
Jane Recruiter  (She/Her)  9:00 AM

Merry Christmas!

WEDNESDAY
View Jane’s profile
Jane Recruiter  (She/Her)  1:01 PM

Any feedback?

YESTERDAY
View Jane’s profile
Jane Recruiter  (She/Her)  8:30 AM

Great!
"""

    assert [m["sentAt"] for m in parse(text)] == [
        "2025-10-23T11:17:00+09:00",
        "2025-12-24T09:00:00+09:00",
        "2026-11-11T13:01:00+09:00",
        "2026-11-14T08:30:00+09:00",
    ]


def test_a_reaction_is_left_out_of_the_text_and_an_attachment_kept():
    text = """OCT 15
Jane Recruiter sent the following message at 12:16 PM
View Jane’s profile
Jane Recruiter  (She/Her)  12:16 PM

Great to hear!

Remove reaction
👍
1
OCT 16
Sam Owner sent the following message at 10:48 AM
View Sam’s profile
Sam Owner   10:48 AM

Here is my resume.

Resume.pdf

37 KB

Download
"""

    assert [m["text"] for m in parse(text)] == ["Great to hear!", "Here is my resume.\n\nResume.pdf\n\n37 KB\n\nDownload"]


def test_a_message_saying_just_a_day_or_like_a_header_stays_a_message():
    text = """TODAY
Jane Recruiter sent the following message at 10:00 AM
View Jane’s profile
Jane Recruiter  (She/Her)  10:00 AM

When could you talk?

Sam Owner sent the following message at 10:05 AM
View Sam’s profile
Sam Owner   10:05 AM

Monday

Jane Recruiter sent the following message at 10:06 AM
View Jane’s profile
Jane Recruiter  (She/Her)  10:06 AM

Great. View the full job profile
View the full job profile
Jane Recruiter
"""

    assert [m["text"] for m in parse(text)] == [
        "When could you talk?",
        "Monday",
        "Great. View the full job profile\nView the full job profile\nJane Recruiter",
    ]


def test_messages_above_the_first_day_heading_are_left_out():
    # The reader scrolls up only three times, so a long thread's oldest
    # messages may arrive without their day heading.
    text = """View Jane’s profile
Jane Recruiter  (She/Her)  7:00 AM

An old message without its Day.

TODAY
View Jane’s profile
Jane Recruiter  (She/Her)  10:00 AM

A new one.
"""

    assert [m["text"] for m in parse(text)] == ["A new one."]


SPONSORED = """Sponsored

Nov 13
Owner, your invitation — Example Fellowship
Apply Now
Sam Admissions, PhD
Sam Admissions, PhD

Hi Owner,

Apply Now
Not interested
"""

LINKEDIN_OFFER = """LinkedIn Offer

Nov 8
LinkedIn
NOV 8
sent the following messages at 11:05 AM
LinkedIn
LinkedIn  11:05 AM

Hi there! We would like to offer you a free month of Premium.

Try for free
Not interested
"""


def test_sponsored_messages_and_linkedin_offers_are_ads():
    assert is_ad(SPONSORED)
    assert is_ad(LINKEDIN_OFFER)
    assert not is_ad("""TODAY
Jane Recruiter sent the following message at 10:00 AM
View Jane’s profile
Jane Recruiter  (She/Her)  10:00 AM

Sponsored
""")


def test_the_other_person_is_matched_by_their_name_as_the_inbox_gives_it_or_its_start():
    # The inbox may add a reading in brackets, an emoji or a title the
    # sender line doesn't show, or the other way round.
    text = """TODAY
Jane Recruiter sent the following message at 10:00 AM
View Jane’s profile
Jane Recruiter  (She/Her)  10:00 AM

Hello!

Sam Owner sent the following message at 10:05 AM
View Sam’s profile
Sam Owner   10:05 AM

Hi Jane.
"""

    for inbox_name in ["Jane Recruiter", "Jane Recruiter (ジェーン)", "Jane Recruiter 🇯🇵", "ｊａｎｅ  recruiter"]:
        messages = parse_conversation(text, other=inbox_name, today=SUNDAY, tz=TOKYO)
        assert [m["from"] for m in messages] == ["other", "owner"], inbox_name


def test_a_message_ending_in_a_day_in_capitals_keeps_it_and_its_day():
    text = """TODAY
Jane Recruiter sent the following messages at 10:00 AM
View Jane’s profile
Jane Recruiter  (She/Her)  10:00 AM

Can we talk on
FRIDAY
View Jane’s profile
Jane Recruiter  (She/Her)  10:01 AM

Or next week.
"""

    assert [(m["sentAt"], m["text"]) for m in parse(text)] == [
        ("2026-11-15T10:00:00+09:00", "Can we talk on\nFRIDAY"),
        ("2026-11-15T10:01:00+09:00", "Or next week."),
    ]


def test_text_after_a_reaction_is_kept():
    text = """TODAY
Jane Recruiter sent the following message at 10:00 AM
View Jane’s profile
Jane Recruiter  (She/Her)  10:00 AM

Great to hear!

Remove reaction
👍
1
Edited
"""

    assert [m["text"] for m in parse(text)] == ["Great to hear!\n\nEdited"]


def test_lines_like_a_name_a_sender_line_or_a_group_heading_stay_in_the_text():
    text = """TODAY
Jane Recruiter sent the following message at 10:00 AM
View Jane’s profile
Jane Recruiter  (She/Her)  10:00 AM

Our CEO sent the following message at 9:00 AM
Jane Recruiter
Jane Recruiter  10:30 AM works for me.
Jane Recruiter  10:30 AM
"""

    assert [m["text"] for m in parse(text)] == [
        "Our CEO sent the following message at 9:00 AM\nJane Recruiter\nJane Recruiter  10:30 AM works for me.\nJane Recruiter  10:30 AM"
    ]


def test_a_message_without_text_says_so():
    # An image or sticker leaves no text on the page.
    text = """TODAY
Jane Recruiter sent the following message at 10:00 AM
View Jane’s profile
Jane Recruiter  (She/Her)  10:00 AM

"""

    assert [m["text"] for m in parse(text)] == ["(no text)"]


def test_an_ad_is_one_message_from_the_other_person_on_its_date():
    assert ad_message(SPONSORED, today=SUNDAY, tz=TOKYO) == {
        "from": "other",
        "sentAt": "2026-11-13T00:00:00+09:00",
        "text": "Sponsored\n\nNov 13\nOwner, your invitation — Example Fellowship\nApply Now\nSam Admissions, PhD\nSam Admissions, PhD\n\nHi Owner,\n\nApply Now\nNot interested",
    }
    assert ad_message(LINKEDIN_OFFER, today=SUNDAY, tz=TOKYO)["sentAt"] == "2026-11-08T00:00:00+09:00"
    assert ad_message("Sponsored\nNo date anywhere", today=SUNDAY, tz=TOKYO) is None


def test_an_inmail_from_a_name_ending_in_s_with_its_quick_replies_left_out():
    # As thread 15 of the first Sync: "View Chris’ profile", a subject line
    # right under the sender line, and LinkedIn's quick replies at the end.
    text = """View my services
MAR 10
Chris Betrand sent the following messages at 10:47 AM
View Chris’ profile
Chris Betrand   10:47 AM
Explore QA roles in Tokyo

Dear Owner,
I would love to talk.

MAR 13
View Chris’ profile
Chris Betrand   10:47 AM
Just checking in

Any interest?

Reply to conversation with “Yes, interested...”
Yes, interested...
Reply to conversation with “No thanks...”
No thanks...
"""

    assert [(m["from"], m["sentAt"], m["text"]) for m in parse_conversation(text, other="Chris Betrand", today=SUNDAY, tz=TOKYO)] == [
        ("other", "2026-03-10T10:47:00+09:00", "Explore QA roles in Tokyo\n\nDear Owner,\nI would love to talk."),
        ("other", "2026-03-13T10:47:00+09:00", "Just checking in\n\nAny interest?"),
    ]
