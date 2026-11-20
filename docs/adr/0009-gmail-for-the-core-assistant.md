# Gmail is an Outside Service the Core Assistant manages, and sends from on the Owner's yes

The Owner wants their Gmail handled in Telegram too: asking what came in, finding a mail, summarizing a thread, drafting answers, tidying the inbox, and a short digest of new mail each morning. So Gmail becomes the second Outside Service, after Google Calendar (ADR 0008). The Assistants' scheduled task gets the claude.ai **Gmail** connector beside the Nando and Google Calendar ones, and Nando itself still never touches Gmail or calls a model. This amends ADR 0008 in two places: the Core Assistant may **send** mail, and a Notification may now **wake a run**.

## Decisions

- **Every Assistant reads mail; only the Core Assistant changes it.** Others read it to answer a question (a Recruiter's email, a receipt) and point the Owner to the Core Assistant for anything that changes the mailbox, as they do for the calendar's events outside their Module.
- **The Core Assistant reads, searches and summarizes, drafts, and organizes** (labels, archive, read or unread, new labels) at once, saying what it did.
- **It sends only on the Owner's yes.** It writes the mail as a Gmail draft, shows the Owner exactly what would go and to whom, and stops. A later run, reading the Owner's yes in the Talk, sends that same draft by its ID, unchanged. Any change to the draft means showing it again. It never forwards, and never replies straight away: a reply is a draft first, like any other mail.
- **It never trashes mail, marks it as spam or deletes labels.** The Owner does that in Gmail.
- **The Mail Digest** is a daily Notification in the Core Assistant's chat, at a time in the Owner's settings, off until the Owner turns it on. Unlike other Notifications it *asks* for work: it waits in the Talk like the Owner's message, and wakes a run, which reads the mail that came since the last digest and answers with the digest.

## Considered Options

- **Drafts only, never sent** (ADR 0008 as it stood): rejected by the Owner. Opening Gmail to press send on a draft Claude wrote is the chore they want gone. The yes in a later message, and sending the shown draft unchanged, keep the Owner in control of every mail that leaves.
- **Send with `reply` or `send_message` from the text the Owner approved**: rejected. The text could drift between what was shown and what was sent. Sending the draft by its ID sends exactly what the Owner saw, and the draft is also in Gmail for them to check.
- **Every Assistant manages its own mail**, such as the Career Assistant answering a Recruiter's email: rejected for now. It blurs the lanes, and mail that sends needs one careful owner.
- **The digest as a cloud scheduled task on its own schedule**: rejected. A second task to set up by hand, outside the Talk, so the Owner couldn't answer the digest ("archive the newsletters") in the same chat.
- **Nando reading Gmail itself, through the Gmail API**: rejected. It would need its own Google OAuth client and token, and a model to summarize, which Nando never calls (ADR 0004, ADR 0008).

## Consequences

- Core's settings gain the Mail Digest (`enabled`, `time` on the hour or half hour, off at 08:00 by default). A settings write that leaves it out keeps the saved one, so a web app that doesn't know it yet can't turn it off.
- A Notification can **ask** for work. One that asks is sent only in the Messenger and only by its own area's Assistant's bot, never by Web Push or another Assistant's bot. Nando keeps it in the Talk as a waiting message `from` `nando`, and wakes a run under ADR 0008's wake rules. A reply answers it like the Owner's messages: Telegram numbers the bot's and the Owner's messages in one sequence. It gets no 👀. If no run answers it after three claims, the bot says it couldn't do it, without asking the Owner to send anything again.
- The Talk skill's "never send anything to anyone but the Owner" gets one exception: mail the Core Assistant sends on the Owner's yes. Suggested Replies for Recruiters stay drafts.
- The Assistants' scheduled task gets the **Gmail** connector in claude.ai, a manual step. No one watches the run to approve a Tool, so Gmail's sending Tools can't be set to ask first: the guard against sending without the Owner's yes is the skill alone, the same trade-off ADR 0006 and ADR 0008 made for destructive Tools.
