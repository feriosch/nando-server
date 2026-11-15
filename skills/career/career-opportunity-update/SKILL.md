---
name: career-opportunity-update
description: >-
  Update the Owner's Opportunities in Nando from what happened: a Step done or scheduled, dates, salary or offer numbers, Perks, a tip from another network, a PDF or a screenshot; also closes, reopens and re-ranks Opportunities. Use whenever the Owner tells Claude news about a job process, a Company or a Recruiter (e.g. "had the casual interview with Mercari, next is a coding test on the 20th", "they offered 9.5M", "I'm out of the LINE one"), even if they don't mention Nando.
---

# Update an Opportunity

Input: what happened, in the Owner's words, maybe with a pasted message, a PDF (an offer letter, a job description) or a screenshot. Every lookup and write goes through the Career Tools of the Nando Connector.

**Files are never stored.** Take from a file only what belongs in the Opportunity or Company (dates, numbers, conditions, Perks, a few lines of details), and write that. Never save the file, or its text whole, anywhere.

## 1. Find the Opportunity

`career_search` with the Company name, the role, or the Recruiter's name. It finds Opportunities open or closed. Then `career_get_opportunity` with its `id` for the whole Opportunity, and `career_get_company` for its Company when the news is about the Company.

- **Several match:** ask which, naming each by Company, role and Stage.
- **None matches:** it is new. Say so and ask before creating it. One the Owner found themselves has no Recruiter (`recruiterId: null`). A Company that comes back after an Opportunity closed starts a new Opportunity.

## 2. Work out the changes

From what the Owner said, and nothing more:

- **Steps:** each dated event, such as a casual interview, a coding test or a final interview, is a Step: `kind` as the Owner names it, `date` (`YYYY-MM-DD`), `time` (`HH:MM`) if given, and short `notes` on how it went or what to prepare. A Step that moved is the same Step with a new date (`stepId`), not a second one.
- **Stage:** `introduced`, `in_process` (the first Step), `offer`, `closed`. It skips ahead but never goes back; Nando refuses a step back.
- **Closing:** `closed` with an Outcome:
  - `accepted`: the Owner took the offer;
  - `declined`: the Owner turned down an offer;
  - `withdrawn`: the Owner stepped away before an offer, for any reason;
  - `rejected`: the Company said no.

  `faded` is Nando's own, for an Opportunity that went 30 days without a message. A closed Opportunity's Outcome may be corrected later, but never to `faded`.
- **Reopening:** only a `faded` Opportunity can be reopened: `career_reopen_opportunity` puts it back at the Stage it had. Any other closed one stays closed. If the Company comes back, that's a new Opportunity.
- **Interest:** `high`, `normal` or `low`, only when the Owner says how much they want it ("I really want this one", "not excited about it").
- **Pay:** annual, in JPY. A salary first stated by a Recruiter goes in `pay.firstStated`; an offer goes in `pay.offered`. Each is `{"min", "max"}` (the same number twice for a single figure). A monthly figure is ×12; say so in `details`. `bonusIncluded` says whether the figure includes bonus.
- **Conditions:** `remote` (`remote`, `hybrid` with `officeDays`, or `office`), `officeLocation`, `workingLanguage` (`ja`, `en` or `mixed`).
- **Perks:** free-text items beyond pay and conditions ("Housing allowance ¥30,000/month", "Side jobs allowed").
- **Details:** anything else worth remembering about the role, short.
- **Company notes:** what the Owner learned about the Company itself (from an interviewer, a friend, a tip on another network) goes in the Company's `notes`, added to what's there.

## 3. Write

- `career_save_opportunity` with the `id` and only the fields that change. `pay` and `conditions` are replaced whole, so send them complete, starting from what `career_get_opportunity` answered. `perks` is the whole list.
- `career_save_step` with `opportunityId`, and `stepId` to change an existing Step.
- `career_save_company` with the Company's `id` and `notes` (the old notes with the new ones added), or `brief` if the Owner corrects the Company Brief.
- `career_reopen_opportunity` with the `id`, for a `faded` one.

**Ask first** before a destructive Tool, which the Claude app is also set to confirm: deleting a Step (`career_delete_step`) or a whole Opportunity (`career_delete_opportunity`). They are for mistakes only, such as a Step that never existed or an Opportunity made twice. An Opportunity that ended is closed with an Outcome, never deleted.

A Tool that refuses says why: read its message and adjust.

## 4. Confirm

Tell the Owner what changed, in a few lines: the Opportunity (Company, role), its Stage and Outcome if they changed, each Step added or changed, and the other fields, as before → after where something was replaced. Say what you left out of a file on purpose, if anything.
