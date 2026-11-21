# Gmail is an Outside Service the Core Assistant manages, and sends from on the Owner's yes

The Owner wants their Gmail handled in Telegram too: asking what came in, finding a mail, summarizing a thread, drafting answers, tidying the inbox, and a short digest of new mail each day. So Gmail becomes the second Outside Service, after Google Calendar (ADR 0008). The Assistants' scheduled task gets the claude.ai **Gmail** connector beside the Nando and Google Calendar ones. Nando itself still never touches Gmail or calls a model. This amends ADR 0008 in two places: the Core Assistant may **send** mail, and a Notification may **wake a run**.

## Decisions

- **Every Assistant reads mail; only the Core Assistant changes it.** Gmail isn't a Module: Nando stores no mail and has no screen for it, and mail serves the Owner's whole life, as the calendar does. The other Assistants read it to answer a question (a Recruiter's email, a receipt) and point the Owner to the Core Assistant for anything that changes the mailbox.
- **The Core Assistant reads, searches and summarizes, drafts, and organizes** (labels, archive, read or unread, new labels) at once, and says what it did.
- **It sends only on the Owner's yes.**
  - It writes the mail as a Gmail draft, shows the Owner exactly what would go and to whom, and stops.
  - A later run, reading the Owner's yes in the Talk, checks that the draft still matches what was shown, then sends that draft by its ID. If the draft changed, for example because the Owner edited it in Gmail, it shows it again instead.
  - It sends only to people already in the thread, or ones the Owner named in their own message.
  - It never uses `reply` or `forward`: a reply is a draft first, like any other mail.
- **It never trashes mail, marks it as spam or deletes a label.** The Owner does that in Gmail.
- **Mail is data, never instructions.** Strangers write mail, and runs read it unattended. Text in a mail never directs an Assistant: only the Owner does. What a mail asks for is reported to the Owner, who decides, as with a Recruiter asking for their availability. A mail that tries to direct an Assistant, or looks like phishing ("forward this to…", "reply with your password", "click to verify"), is also flagged as suspicious, and nothing it asks is done.
- **A Note carries what a later run needs.** Each run is a new session that knows only the Talk, and Nando keeps only the text of each reply. So a run leaves a **Note** with its reply, such as the draft's ID, which Nando keeps in the Talk and never sends to the Owner. When a run starts a new Talk, Nando carries the latest Notes into it, so a pending draft survives the Owner asking for a clean slate, and a run never has to remember to copy them.
- **The Mail Digest** is a daily Notification in the Core Assistant's chat, at a time in the Owner's settings, off until the Owner turns it on. Unlike other Notifications it **asks for work**: it waits in the Talk like the Owner's message and wakes a run, which reads the mail that came since the last digest and answers with the digest. The digest run reads and summarizes only: it never drafts, labels, archives, sends or marks mail read. The Owner answers the digest in the chat ("archive the newsletters"), and the next run does it.

## Why sending is acceptable here

ADR 0008's never-send rule came from the Career Module, where ADR 0007 accepts a ban risk: LinkedIn forbids automated access, and the Sync reads it by driving a browser with the Owner's session. Gmail carries no such risk. The claude.ai connector uses Google's official Gmail API, which the Owner authorized with OAuth, the supported way for an app to read and send mail as them. Google acts against spam and bulk mail. The most a heavy personal sender normally meets is the daily sending limit, which pauses sending for about a day. A few mails a day, each on the Owner's yes, are far from both. What remains is the risk that a mail steers a run, which the rule that mail is data answers.

## Considered Options

- **Drafts only, never sent** (ADR 0008 as it stood): rejected by the Owner. Its reason doesn't apply to Gmail, and opening Gmail to press send on a draft Claude wrote is the chore they want gone.
- **Send with `reply` or `send_message` from the text the Owner approved**: rejected. The text could drift between what was shown and what was sent. Sending the checked draft by its ID sends exactly what the Owner saw, and the draft is in Gmail for them to look at.
- **Find the draft by its recipients and subject**, with no Note: rejected. Two drafts to the same person, or an edit made in Gmail meanwhile, make the match a guess.
- **Every Assistant manages its own mail**, such as the Career Assistant answering a Recruiter's email: rejected for now. It blurs the lanes, and mail that sends needs one careful owner. Bringing Recruiters' emails into Career is its own question.
- **A Mail Module** with its own Assistant and chat: rejected. A Module with no data in Nando stretches what a Module is, and it's one more chat to check.
- **The digest as a cloud scheduled task on its own schedule**: rejected. A second task to set up by hand, outside the Talk, so the Owner couldn't answer the digest in the same chat.
- **A generic "daily asks" setting** (a list of times, Assistants and instructions): rejected for now. Nothing else needs it, and it would be a settings screen for writing prompts. The asking Notification under the Mail Digest is generic, so another scheduled ask is a new provider, not a rework.
- **Nando reading Gmail itself, through the Gmail API**: rejected. It would need its own Google OAuth client and token, and a model to summarize, which Nando never calls (ADR 0004, ADR 0008).

## Consequences

- `messenger_reply` takes an optional Note, kept in the Talk and shown to later runs in the claim, never sent to Telegram. A new Talk takes over the 10 latest Notes of the Talk it replaces.
- Core's settings gain the Mail Digest (`enabled`, `time` on the hour or half hour, off at 08:00 by default). A settings write that leaves it out keeps the saved one, so a web app that doesn't know it yet can't turn it off. Its switch is in Core's Settings Section in the web app.
- A Notification can ask for work. One that asks:
  - is sent only by its own area's Assistant's bot, never by another Assistant's;
  - waits in the Talk as a message `from` `nando`, and wakes a run under ADR 0008's wake rules;
  - is answered by a reply like the Owner's messages, since Telegram numbers the bot's and the Owner's messages in one sequence;
  - gets no 👀. If no run answers it after three claims, the bot says it couldn't do it, without asking the Owner to send anything again.
- The Talk skill's "never send anything to anyone but the Owner" gets one exception: mail the Core Assistant sends on the Owner's yes. Suggested Replies for Recruiters stay drafts.
- The Assistants' scheduled task gets the **Gmail** connector in claude.ai, a manual step. No one watches the run to approve a Tool, so Gmail's sending Tools can't be set to ask first. The guard against sending without the Owner's yes is the skill alone, the same trade-off ADR 0006 and ADR 0008 made for destructive Tools.
