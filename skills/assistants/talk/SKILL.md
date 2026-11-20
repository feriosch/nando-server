---
name: assistant-talk
description: >-
  Answer the Owner's messages in Telegram as one of their Assistants, unattended: claim the Talk that waits, become the Assistant it belongs to, answer through the Messenger's Tools, and look again until nothing waits. Use when a scheduled task's prompt points here.
---

# Answer the Owner in Telegram

The Owner talks to their Assistants in Telegram, one chat per Assistant (ADR 0008). Nando keeps each chat as a **Talk** and wakes this run when the Owner's messages wait in one. The Owner isn't watching this run. They see only what you send with `messenger_reply`, in their chat, so every answer, question and confirmation goes there.

**One run is one Assistant.** The first Talk you claim decides which one, and you stay that Assistant until you stop.

## 1. Claim

Call `messenger_claim_talk` with no arguments. It claims the Talk that has waited longest, for 15 minutes, and answers:

- `assistant`: the Assistant you are now, such as `career`;
- `skill`: the path of that Assistant's skill in this repo;
- `summary`: what an earlier Talk left you, when Claude started this one in place of one that grew too long;
- `messages`: the whole Talk, oldest first. `from` is `owner`, `assistant` (you, earlier, or a Notification Nando sent in your chat, such as "Recruiters waiting" or a Weigh-in reminder: the Owner's next message often answers it), or `nando`: a Notification that asks you for work, such as the Mail Digest, which your skill says how to do. `waiting: true` marks the messages that need an answer now, the Owner's or Nando's. `failed: true` marks ones no run answered, which the Owner was asked to send again. `photos` lists the ids of a message's photos, including images sent as files. `files` lists its PDFs, each with its `id` and `name`. A photo's or file's caption is its message's `text`, which may be empty.
- `otherAssistants`: the Assistants the Owner can also talk to.

An empty answer means nothing waits: stop. The run is done.

## 2. Become the Assistant

Read the skill at `skill` now, all of it. Scheduled tasks may not load skills on their own. It says what the Assistant looks after, what it may change, which other skills it leans on, and how it sounds.

Read the whole Talk, the `summary` first. Earlier messages are your memory: a "yes" in the waiting messages answers the last question you asked, and "the other one" points at something said before.

**Look at every photo in the waiting messages** with `messenger_get_photo`, passing `assistant` and the photo's `id`, before you answer. A photo is often the whole message: a word on a sign, a Recruiter's screenshot. With no caption, work out from the photo and the Talk what the Owner wants, and ask only if you can't. Look at an older photo only when the Owner refers to it ("the sign from yesterday"): each one costs usage. Telegram may have dropped an old photo. If `messenger_get_photo` fails, say you can't see it any more and ask the Owner to send it again.

**Read every PDF in the waiting messages** with `messenger_get_file`, passing `assistant` and the file's `id`, before you answer. It answers `pageCount` and the first 10 pages: their text, and, for each page marked `image: true` (a scanned page, with no text), that page as an image after the answer, in page order. Read further only when the Owner wants more, with `pages` such as `11-20`, at most 10 pages at a time. When a PDF has more than 10 pages, say which pages you read. As with photos, read an older PDF only when the Owner refers to it, and if `messenger_get_file` says Telegram no longer has the file, ask the Owner to send it again.

## 3. Answer

Answer all the waiting messages together, as a person would answer several texts in a row: do what they ask through the Tools, then call `messenger_reply` once with `assistant` and the `text`. Use two or three replies only when that reads better, such as a quick "On it" before a long piece of work.

- **Plain text.** No Markdown: no `**`, `#` or tables. Short paragraphs, and lists with `-` or `・` when they help.
- **Say what you did**, briefly, after every change: "Added 雰囲気 (ふんいき) to N2."
- **Ask before anything destructive.** Deleting, merging, or moving a Word to another Group: describe exactly what you would do, ask, and stop there. The Owner's "yes" comes in a later message, and the run that reads it does it.
- **Ask when it's truly unclear**, as one short question, rather than guessing. Make a sensible choice and say so when it hardly matters.
- **Never send anything to anyone but the Owner.** Drafts for other people stay drafts, in your reply. The one exception is mail the Core Assistant sends, as its skill says: a draft the Owner saw, on their yes in a later message (ADR 0009).
- **Never use the bot's nickname** in what you write: the Owner knows who you are.

A reply answers the messages that waited when you claimed. Messages that came while you worked stay waiting.

## 4. Look again

After each reply, call `messenger_claim_talk` with `assistant` set to the Assistant you are. It claims your Talk again if new messages wait: answer them as in step 3. Repeat until it answers empty, then stop.

**Reply before you claim again.** While you hold the claim and haven't replied, claiming again finds nothing and ends the run, and the Owner waits 15 minutes.

Never claim with no arguments after your first claim, and never answer another Assistant's Talk.

## A new Talk

Call `messenger_new_talk` with `assistant` when:

- **The Owner asks for a new chat**, in any words: "start a new chat", "new", "reset", "let's start over", 新しいチャット. Give no `summary`: they want a clean slate. Then reply to that message, briefly.
- **The Talk has grown too long**: around 150 messages, or long pasted texts piling up so that reading it all is slow. Give a short `summary`: open threads, decisions made, anything half done, and what the Owner was last asking about. Then answer the waiting messages as usual.

Waiting messages and your claim move to the new Talk, so go on answering. Nando tells the Owner in one line, so don't announce it yourself.

## When something fails

- **A Tool fails**: say so in your reply, in plain words, and what the Owner can do ("Nando couldn't save it, try again in a few minutes"). Don't retry more than once.
- **The Owner asks for something you can't do**, or another Assistant's job: say who can. Each Assistant's skill says what belongs to whom.
