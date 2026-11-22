# Recruiters' Gmail reaches Career through a label, in a Gmail Career Sync

Most of the Owner's career mail is LinkedIn InMail, which the LinkedIn Sync already reads (ADR 0007). But some Recruiters write by email only, such as an agency or a company's own HR, and the Career Module never saw them. Now that Gmail is an Outside Service (ADR 0009), their email threads become Conversations too. They reach Career through a Gmail label, in a **Gmail Career Sync**: a daily pass in which the Career Assistant files into Nando the new mail labeled `Career`. The LinkedIn Sync stays LinkedIn-only. This amends ADR 0009 in one place: the Career Assistant may create Gmail drafts, in `Career` threads only, and never sends.

## Decisions

- **A Gmail thread with a Recruiter is a Conversation of its own**, keyed by Gmail's thread ID, linked to open in Gmail, and filed under the Recruiter. A Recruiter can have LinkedIn and Gmail Conversations side by side.
- **Mail reaches Career only through the Gmail label `Career`.** The Owner applies it by hand or with a Gmail filter, until #156 decides how it is applied for them. (Amended by ADR 0011: the label is now `Career/Recruiters`, which the Core Assistant applies in the Gmail Digest's run to any mail from a Recruiter, agency or in-house. The Gmail Career Sync reads only that label, so job platforms, LinkedIn's notices and Career's history never reach it, and its time should come after the Gmail Digest's.)
- **The Gmail Career Sync**:
  - Once a day, at a time in Career's Settings Section (off until the Owner turns it on), Nando asks the Career Assistant for one, with a Notification that asks for work (ADR 0009). The Owner can also ask for one in the Career chat at any time.
  - Nando records each pass, apart from the LinkedIn Syncs, so a pass never counts toward a LinkedIn Sync's limits or alerts.
  - The first pass reads labeled threads back to 2025-01-01, as the first LinkedIn Sync did. Each later pass reads every labeled thread Nando doesn't hold yet, so a thread the Owner labels late is still brought in, and the held ones with mail since the last finished pass. A pass that stops partway doesn't move that mark, so the next one picks up what it missed.
  - The run answers in the Career chat with what it filed, any new Suggested Replies, and its questions.
- **Recruiters gain email addresses.** A thread from a known address is filed to its Recruiter. For an unknown one, the run judges from the signature, the company and the role, as `career-sync` does for a new LinkedIn thread, and asks the Owner when unsure. An address is remembered once its thread is filed, whether the run was sure or the Owner confirmed it, and the Owner can correct a wrong one.
- **A labeled thread that isn't about a role is ignored**, as on LinkedIn: only its thread ID is kept, so it isn't read again. The label stays on in Gmail. Fixing labels is #156's job.
- **What is kept:**
  - each message's own new text, without the quoted thread or a long signature;
  - the Owner's own sent mail too, so that a reply they sent clears the Suggested Reply, as a LinkedIn Sync does;
  - an attachment's name only: a job description or an offer is read into the Opportunity, never stored.
- **Replies:** the Career Assistant writes the Suggested Reply in Nando, and also creates it as a Gmail draft in that thread (`create_draft` with `replyToMessageId`), leaving the draft's ID as a Note. It never sends. The Owner sends it from Gmail, or asks the Core Assistant, which finds the draft, shows it, and sends it on their yes under ADR 0009's rules.
- **Career's timing rules apply unchanged**: a follow-up after 14 days of silence after the Owner's message, a Late Reply to a message more than 30 days old, and an Opportunity Faded after 30 days with no message.
- **Names:** the old **Sync** is now the **LinkedIn Sync**, and everything about Gmail is named Gmail (Outlook may come one day). Code names (`syncs`, `career_record_sync`, `career-sync`) stay.

## Considered Options

- **The `career-sync` run also reads Gmail**: rejected by the Owner. That run stays tied to LinkedIn and the LinkedIn Sync's rules, and a label keeps what Career reads to what the Owner chose.
- **The Owner files each thread by hand, in the chat**: kept as an extra way in (asking for a Gmail Career Sync), but not the only one: it would need a new habit.
- **One Conversation per Recruiter across channels**: rejected. It breaks the glossary's "one message thread", and the LinkedIn Sync's key.
- **Emails read on demand, never stored**: rejected. Suggested Replies, follow-ups and Faded all need the messages.
- **The Core Assistant files career mail in the Gmail Digest's run**: rejected. It changes the Career Module from outside its lane, and the digest run changes nothing. (Amended by ADR 0011: the digest run now files the Owner's mail, and applies `Career/Recruiters`. Filing that mail into Career stays the Career Assistant's, in the Gmail Career Sync.)
- **Only the Core Assistant touches Gmail, even for drafts** (ADR 0009 as it stood): rejected. A ready draft in the right thread saves the Owner copying the reply over, and sending stays with the Core Assistant alone.
- **The Career Assistant sends on the Owner's yes**: rejected. ADR 0009's one careful owner for sending stays.
- **LinkedIn's notification emails as a source**: out of scope, for its own decision. It would change ADR 0007's LinkedIn Sync.

## Consequences

- Career's settings gain the Gmail Career Sync (`enabled`, `time`), off by default, saved with the rest of Career's settings, and its switch is in Career's Settings Section.
- Career's Tools gain what the run needs to bring in a Gmail thread, file or ignore it, and find or add a Recruiter's address, plus a record of each pass.
- The lanes test lets the Career skill name Gmail's draft Tools, and only those.
- The web app shows Gmail Conversations beside LinkedIn ones, with a mail icon and "Open in Gmail".
- The Assistants' scheduled task already has the Gmail connector (ADR 0009). The `career-sync` task gets none.
