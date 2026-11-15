---
name: career-interview-prep
description: >-
  Prepare with the Owner for a Step in one of their Opportunities, such as a casual interview, a technical interview or a final interview: start from the Company Brief, research the Company further from public information, and draft likely questions and questions to ask. Use when the Owner wants to get ready for an interview or meeting with a Company (e.g. "prep me for the Mercari interview tomorrow", "what should I ask SmartNews?"), even if they don't mention Nando.
---

# Prepare for a Step

Every lookup and write goes through the Career Tools of the Nando Connector. This is done together with the Owner: research, then talk it through, rather than one long answer.

## 1. Start from what Nando holds

- `career_search` with the Company name or role, then `career_get_opportunity` with the Opportunity's `id`. If several are open, ask which.
- The Step to prepare for is the one the Owner names, or else the next one by date in `steps`. Its `kind` and `notes` say what it is.
- `career_get_company` for the Company Brief and the Company's `notes`.
- `career_get_profile` for the Owner's Career Profile: their experience, what they want, their deal-breakers.

Tell the Owner in a few lines what you start from: the role, the Stage, the Step, and the Company Brief's main points and open questions.

## 2. Research further

From public information only: the Company's site, its LinkedIn page, its engineering blog and talks, its job posts, news, and employee reviews. Look for:

- the product and team the role sits in, and the tech they use;
- recent news, such as funding, launches, layoffs or reorganisations;
- how its interviews go, as candidates describe them;
- answers to the Company Brief's open questions.

Share what you find as you go, saying where it comes from, and let the Owner steer what to dig into.

## 3. Draft

- **Likely questions** for this kind of Step, fitted to the role and the Company: from the job details, the Company's product and tech, and the Owner's own experience in their Career Profile. For each, a few points the Owner could answer with from their own experience.
- **Questions to ask** them, chosen for what the Owner still needs to know to decide: the team, how they work, growth, pay and conditions not yet stated, the next Steps. Skip what the Opportunity already answers.
- For a Japanese-language Step, give the key questions and phrases in Japanese at the Owner's level (JLPT N2), polite です/ます.

## 4. Keep what's useful

Save findings worth keeping across Opportunities in the Company's notes: `career_save_company` with its `id` and `notes`, the old notes with the new ones added, each short and with its source. Tell the Owner what you added.

If the Step's date, time or kind changed while you talked, or the Owner learned something about pay or conditions, update the Opportunity as `career-opportunity-update` says: `career_save_step` and `career_save_opportunity`.
