---
name: career-now-reply
description: >-
  Answer a Recruiter's latest message right now, one the Owner pastes or screenshots from LinkedIn and no Sync has brought to Nando yet: read it with the whole past Conversation, the Opportunity, the Career Profile and Looking, find free slots in Google Calendar when they ask to meet, and write the reply in the Owner's voice for them to copy. Use whenever the Owner shares a new message from a Recruiter and wants to answer it (e.g. "Tanaka just wrote back, help me answer", a LinkedIn screenshot with "what do I say?"), even if they don't mention Nando. To steer the Suggested Reply Nando already holds, follow `career-draft-reply` instead.
---

# Answer the latest message now

The Owner sends every reply on LinkedIn themselves. A Recruiter has written again before the next Sync, and the Owner wants to answer straight away. You bring what a plain chat lacks, through the Career Tools of the Nando Connector: the whole past Conversation, the Opportunity, the Career Profile and whether the Owner is Looking. The reply you write lives only in this chat: the next Sync brings the Recruiter's message, with its exact time, and the Owner's answer.

## 1. Read the latest message

The Owner pastes it as text, shares a screenshot of LinkedIn, or both. Read it and say back, in a line or two, who it's from and what they ask (a time to meet, a CV, the Owner's salary, whether they're interested). Done when the Owner agrees that's the message.

## 2. Gather the context

- `career_search` with the Recruiter's name, or their agency or Company. Each Recruiter it finds comes with their `conversations` and the `opportunities` they brought. Several Recruiters or Conversations match: ask which.
- `career_get_conversation` with the Conversation's `id`: its messages (`from` `"owner"` is the Owner's, `"other"` the Recruiter's) and its `url`, the LinkedIn thread.
- `career_get_opportunity` for each of the Recruiter's open `opportunities`: its Stage and Steps. `career_get_company` with its Company's `id` gives the Company Brief.
- `career_get_settings` for whether the Owner is Looking.
- `career_get_profile` for the Career Profile: what the Owner wants, won't accept, and when they can meet.

A Recruiter whose `opportunities` is empty is a **Lead**, such as one who wants a casual talk to introduce roles. Build the reply from the Conversation, Looking and the Profile alone. A Recruiter Nando doesn't hold at all: say so, and work from the message and the Profile.

Done when you hold the Conversation, Looking and the Profile, and every open Opportunity of this Recruiter.

## 3. Find times, when they ask to meet

Read the Owner's calendar through the Google Calendar connector, **read-only**, and pick 2–3 free slots that:

- start at least 2 days from today;
- clash with nothing on the calendar;
- fall when the Career Profile says the Owner can meet (weekday evenings, say), when it says.

Write them the Japanese way in a Japanese reply, `11月20日（木）19:00〜`, and as `Thu 20 Nov, 19:00` in an English one, in the Owner Time Zone.

With no Google Calendar connector in the chat, say so and ask the Owner for times.

## 4. Write the reply

Write in the voice `career-draft-reply` keeps:

- the language the Recruiter wrote in;
- like the Owner's own earlier messages in the Conversation: their greeting, length and sign-off;
- Japanese at the Owner's level (JLPT N2): polite です/ます, the standard set phrases (お世話になっております, よろしくお願いいたします), one humble form at a time, never stacked 謙譲語, short sentences;
- like a person typing on LinkedIn: short paragraphs of plain prose, straight to the point, with commas and full stops for punctuation, so the Recruiter never notices it's AI;
- Looking: engaged, asking for the job description and salary range, open to meeting. Not Looking: warm and short, keeping in touch. Unless the Owner steers otherwise.

Show it and iterate until the Owner is happy, explaining a Japanese phrase if they ask. The final text sits in the chat, ready to copy. It stays out of `career_set_suggested_reply`: a Suggested Reply answers a message Nando holds, and this one doesn't hold it yet.

## 5. Once a time is agreed

When the Recruiter or the Owner confirms a meeting time:

1. **Ask the Owner** whether to put it in their calendar. Only with their yes, create the Google Calendar event: a title such as "Casual talk · Tanaka Yui (Hays)", the agreed time, and the Conversation's `url` in the description.
2. If the meeting belongs to an Opportunity, offer to save it as a Step: `career_save_step` with `opportunityId`, `kind` (such as `casual interview`), `date`, `time` and short `notes`. Nando's Notification the evening before needs the Step.

A meeting with a Lead gets the calendar event only: Steps belong to Opportunities.

## 6. Offer the rest

Once the reply is settled, check whether the message changes anything Nando holds, and offer it once, writing only with the Owner's yes, the way `career-opportunity-update` does:

- an Opportunity's Stage, Outcome or a salary figure, with `career_save_opportunity` (`pay` is replaced whole: start from what `career_get_opportunity` answered);
- for a Lead whose message now names a Company and a role, a new Opportunity: `career_save_opportunity` with `company`, `role`, the `recruiterId`, and a `fitNote` of two or three sentences on how the role fits the Career Profile: what matches, what clashes with a deal-breaker, what's unknown. It starts `introduced`.

Nothing to change: say so, and you're done.

## Guardrails

The Owner sends every message and accepts every calendar change. So: you never send anything on LinkedIn, you create a calendar event only after the Owner's yes, and you use no destructive Tool. A Step or Opportunity saved by mistake is the Owner's to fix with `career-opportunity-update`.
