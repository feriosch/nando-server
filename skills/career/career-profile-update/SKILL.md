---
name: career-profile-update
description: >-
  Build or change the Owner's Career Profile in Nando: their resumes, current situation, what they're looking for and their deal-breakers, which Claude uses to judge Opportunities and write Suggested Replies. Use when the Owner shares a resume or CV, or says something about their job, what they want next, or what they'd never accept (e.g. "here's my resume", "I got promoted", "no more fully onsite jobs"), even if they don't mention Nando.
---

# Update the Career Profile

The Career Profile is what Claude knows about the Owner's working life. The cloud run reads it to write each Opportunity's fit note and to write Suggested Replies that sound like the Owner. It is one Markdown document, read and written only through the Career Tools of the Nando Connector. The app never shows it.

## 1. Read what's there

`career_get_profile` answers `{"markdown"}`, `""` until it is first saved.

## 2. Work in what the Owner gives

From resumes (PDF, text, a screenshot) and from what the Owner says. Keep the Profile in these sections, in this order:

```markdown
## Current situation
Company, role, since when, team and stack, pay (annual JPY) and conditions, visa, why they might move.

## Looking for
Roles, seniority, stack, Company kind and size, pay range, remote and office, location, working language.

## Deal-breakers
What the Owner would turn down outright.

## Experience
Each job, newest first: Company, role, dates, what they did and achieved, stack.

## Skills and languages
Languages with levels (Japanese at JLPT N2), technical skills, certifications.

## Education

## Notes
Anything else Claude should keep in mind.
```

- **Resumes:** take the facts from them into **Experience**, **Skills and languages** and **Education**. Merge with what's there; don't duplicate a job. Several resumes (English and Japanese, or of different years) are one Profile. Never store the file itself, or paste its text whole.
- **What the Owner says** goes where it belongs: a promotion changes **Current situation** and **Experience**; "no more fully onsite jobs" is a **Deal-breaker**.
- Keep it short and factual, in English, so the run can read it in one go. Where something is unclear (a date, whether a wish is a deal-breaker or a preference), ask.

## 3. Show, then save

Show the Owner what changes, section by section, as before → after (the whole draft when building it the first time). Save once they agree: `career_save_profile` with `markdown`, the whole document, which replaces the old one.
