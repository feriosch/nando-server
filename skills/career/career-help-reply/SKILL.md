---
name: career-help-reply
description: >-
  Show the Owner the Suggested Reply Nando holds for a Recruiter, and rewrite it the way they steer ("decline but keep in touch", "ask about salary first", "make it shorter"), in their own voice, then save it. Use whenever the Owner wants to change, discuss or get help with a reply to a Recruiter (e.g. "help me answer Tanaka", "the reply to the Hays guy is too eager"), even if they don't mention Nando.
---

# Help with a reply

The Owner sends every reply on LinkedIn themselves. Nando holds a Suggested Reply per Conversation, which the cloud run wrote following the `career-sync` skill. Here the Owner steers it, and you save the new version for them to copy from the app. Every lookup and write goes through the Career Tools of the Nando Connector.

## 1. Find the Conversation

`career_search` with the Recruiter's name, or their agency or Company. Each Recruiter it finds comes with their `conversations`, each `{"id", "hasSuggestedReply"}`. If the Owner named a Company or role instead, the Opportunity it finds has the `recruiterId` to look for.

- Several Recruiters or Conversations match: ask which.
- `career_get_conversation` with the `id` answers the whole Conversation: its messages (`from` `"owner"` is the Owner's, `"other"` the Recruiter's) and its `suggestedReply`.
- `career_get_settings` says whether the Owner is Looking; `career_get_profile` gives the Career Profile, for what the Owner wants and won't accept.

## 2. Show what's there

The Recruiter's last message or two, then the current Suggested Reply's text, and its notes on the Owner's last message, if any (each "what you wrote → what's natural: why"). With no Suggested Reply, say so and offer to write one.

## 3. Rewrite as the Owner steers

Apply what the Owner asks ("decline but keep in touch", "ask about salary first", "mention I can only meet after 19:00"), and change nothing else. Keep the voice the run writes in:

- the language the Recruiter wrote in (`ja` or `en`);
- like the Owner's own earlier messages in the Conversation: their greeting, length and sign-off;
- Japanese at the Owner's level (JLPT N2): polite です/ます, the standard set phrases (お世話になっております, よろしくお願いいたします), one humble form at a time, never stacked 謙譲語, short sentences;
- like a person typing on LinkedIn: short paragraphs of plain prose, straight to the point, with commas and full stops for punctuation, so the Recruiter never notices it's AI;
- Looking: engaged, asking for the job description and salary range, proposing a casual meeting. Not Looking: warm and short, keeping in touch. Unless the Owner steers otherwise.

Show the new text and iterate until the Owner is happy. Explain a Japanese phrase if they ask.

## 4. Save

`career_set_suggested_reply` with:

- `conversationId`, `text` and `language`;
- `kind`: the same as before (`answer`, `late` or `follow_up`). A new one is an `answer` if the Recruiter wrote last within 30 days, `late` if their message is older, and `follow_up` if the Owner wrote last;
- `answers`: the `sentAt` of the message it answers: the Recruiter's newest for an `answer` or a `late` one, the Owner's last for a `follow_up`, or the old Suggested Reply's `markedRepliedAt` when it has one (the Owner sent it in LinkedIn, and no Sync has brought it yet);
- `notes`: the existing notes on the Owner's last message, unless the Owner asks to change them (up to 3 `{"wrote", "natural", "why"}`).

It replaces the old Suggested Reply. Tell the Owner it's saved and waiting in the app to copy. Nando removes it by itself once a Sync brings the Owner's own reply, and hides it from the lists as soon as the Owner marks it replied in the app.
