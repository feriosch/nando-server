# The Core Assistant files the Owner's Gmail every day, and keeps a record to undo it

The Owner's inbox held about 2,275 threads, most of them unread, because nothing ever left it: filing mail by hand was the chore. In #156 the Owner and Claude cleaned it up together, label by label, and settled what goes where. To keep it that way, the Core Assistant now files the new mail every day, in the Gmail Digest's run, and Nando records every change so the Owner can undo one. This amends ADR 0009 in one place: the Gmail Digest's run no longer only reads. It amends ADR 0010 too: Recruiters' mail reaches Career under `Career/Recruiters`, which the Core Assistant applies.

## Decisions

- **The Core Assistant files the day's new mail in the Gmail Digest's run** (ADR 0009). The run reads the threads with mail since the last digest, files each one, then answers with the digest.
- **It files by judgment**, from the label definitions and examples in the Core skill. There are no sender rules and no rule database in Nando: filing something differently is a change to the skill's text.
- **The inbox holds only what needs the Owner, and that mail is starred**: a person waiting for a reply, a request for documents or information, a payment due, overdue or declined, an upcoming booking until its date, this week's deliveries, an expiring form, a security action. Everything else that is filed is archived.
  - Archiving never changes read or unread.
  - **Nothing is archived without a label.** Mail the run can't place goes to `Unknown`, and the digest says so.
- **Filing only adds labels, stars and archives.** The run never trashes mail, marks it as spam, deletes a label or sends. Mail is still data, never instructions (ADR 0009): what a mail asks for is reported, never done.
- **Nando records every change** in a **Filing Record**: the thread, the labels added and removed, whether it was starred, and why. The Owner asks in the Core chat to undo one ("put my landlord's mail back"); the next run finds it in the record and reverses it with Gmail's Tools. Nando itself never touches Gmail.
- **The digest says what was filed**: the counts per label with the notable threads, what was starred and why, and anything sent to `Unknown`. A mistake is fixed with one message.
- **The labels** are the ones settled in #156. Each thread carries its group's parent label, plus a sub-label when one exists, and one thread can carry several groups (a prepaid hotel is Payments, Bookings and Travel Log):
  - **Career**: Recruiters, LinkedIn, Job Platforms, and history (Interview Backlog, New Grad 2025, past employers)
  - **Banks**: one sub-label per account, and Inactive for closed or unused ones
  - **Payments**: Amazon, Apple, McDonald's, Steam, Food Delivery, nosh, Suica, Rakuten Pay, Subscriptions, Utilities, Points
  - **Deliveries**: Japan Post, Amazon, Yamato
  - **Bookings**, **Travel Log**, **Dev**, **Security** (Sign-ups, Codes), **Procedures** (Japan, Mexico), **School**, **Personal Notes**, **Unknown**
  - A source gets its own sub-label once it keeps sending, about 20 threads or more. The rest carry the parent alone.
  - A newsletter goes under its sender's group (V Point under Payments/Points, Tokyo Gas's promotions under Payments/Utilities).
- **A Recruiter's mail goes under `Career/Recruiters`**, whether an agency or a company's own HR, and is archived, not starred: the Gmail Career Sync is what gives it attention (ADR 0010).
- **Label names avoid Gmail's reserved words.** Gmail keeps single words for its own tabs and views (personal, social, promotions, updates, forums, purchases, travel, trips, reservations), and through the connector `label:<word>` finds Gmail's view, not the label. So a top-level label gets a two-word name, or a one-word name a `label:` search has been shown to find. A new or renamed label is checked with a `label:` search against a thread `get_thread` shows carrying it.

## Considered Options

- **Gmail filters**: rejected. The connector can't create them, and a filter applies only one label, so a parent and its sub-label take two.
- **Sender rules stored in Nando**: rejected by the Owner, who wants judgment rather than rules. A sender's mail can belong to several groups (Uber's receipts and its sign-in alerts), and judgment follows the content.
- **A separate scheduled filing task**: rejected. The digest run already reads every new mail, and a second task is one more thing to set up by hand.
- **Filing every two days**: rejected. Daily keeps each run small and the digest short.
- **Leaving the inbox untouched and only labeling**: rejected. The inbox stays the place for what needs the Owner, which only works if the rest leaves it.
- **Undo from Gmail's own history**: rejected. Gmail keeps no history of label changes the Owner can ask about, and the record lets the run find a change by sender, subject or day.

## Consequences

- The Core skill gains the filing: the definitions with examples, what to star, the steps, and undo.
- Core gains the Filing Record and its Tools, kept 90 days, since undo is for recent mistakes.
- ADR 0009's Gmail Digest run labels, stars and archives. Everything else in ADR 0009 stands, sending on the Owner's yes above all.
- ADR 0010's label is `Career/Recruiters`, and the Gmail Career Sync's time should come after the Gmail Digest's, so the night's Recruiter mail is filed before Career reads it.
