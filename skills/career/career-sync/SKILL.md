---
name: career-sync
description: >-
  Work through what the Owner's LinkedIn Syncs brought to Nando, unattended: sort each new thread (ignore it, or file it under its Recruiter and keep its Opportunities), and write every Suggested Reply in the Owner's voice. Use when a scheduled task's prompt points here.
---

# Work through the Career run's Items

A Sync from the Owner's MacBook hands Nando their new LinkedIn messages, and Nando queues them as Items for this run. This run is unattended: the Owner is not reading it. They see only what you write through the Career Tools of the Nando Connector: Conversations filed under Recruiters, Opportunities, Company Briefs and Suggested Replies, which they read in the app and copy into LinkedIn themselves. Nothing you write is ever sent for them.

If the Owner is in the chat with you, this skill isn't for that: follow `career-draft-reply` or `career-opportunity-update` with them.

## 1. Claim

`career_list_work` claims up to 5 Items for this run, for 15 minutes, and answers:

- `items`: each `{"id", "kind", "conversation", "recruiter"}`. `recruiter` is `null` for an unfiled Conversation; otherwise it is the Recruiter with their other `conversations` and their `opportunities` (in short).
- `looking`: whether the Owner is Looking. It sets the tone of every Suggested Reply (step 4).
- `profile`: the Career Profile, in Markdown: the Owner's resumes, current situation, what they're looking for, and deal-breakers.
- `liveSince`: when the Module went live. Nothing sent before it becomes an Opportunity.

If it answers no Items, stop: the run is done.

Do every Item of the batch before claiming the next. Each one is done by the Tool its kind names below; an Item you can't finish for a passing reason (a Tool keeps failing) stays as it is, and a later run takes it once its claim runs out.

## 2. Each Item

In a Conversation, a message `from` `"owner"` is the Owner's; `"other"` is the person they're talking to. Every Item works on its `conversation`.

### `sort`: a thread Nando hasn't sorted yet

**Ignore** a thread that isn't about a role for the Owner: a friend or ex-colleague saying "my team is hiring", sales pitches, event or webinar invites, networking with no role in it. `career_ignore_thread` with `conversationId`. Nando deletes its text and keeps only its thread ID, so later Syncs never send it again. Someone at the Company who follows up on a friend's tip writes in their own thread, and is a Recruiter.

**An ad** comes as one message whose text starts with `Sponsored` (an InMail ad) or `LinkedIn Offer` (LinkedIn selling Premium), dated the Day it names. Ignore it, unless it advertises a role that could fit the Owner, such as an agency or a Company paying to reach engineers. Then file it under the sender like any Recruiter's thread, and keep the role as a Lead. Write no Suggested Reply to an ad: the Owner answers it, if at all, through its own link.

**Otherwise**, file it under its Recruiter with `career_file_conversation`:

- If the person is a Recruiter Nando holds (`career_search` with their name or organisation; the same `profileUrl` settles it), send `recruiterId`.
- Otherwise send `recruiter`: `{"name", "profileUrl", "kind", "organisation"}`. `kind` is `agency` for an agency's recruiter, `in_house` for one hiring for their own Company. `organisation` is the agency or the Company.

Then keep its Opportunities (step 3), and write its Suggested Reply when the last message is the Recruiter's: an `answer` if that message is at most 30 days old, or a `late` one from the templates (step 5) if older. When the Owner's message is the last, there is nothing to reply to.

### `reply`: a filed Conversation with a new message from its Recruiter

Keep its Opportunities (step 3) with what the new messages say, then write an `answer`, or a `late` reply if the Recruiter's newest message is over 30 days old.

When the Owner's own message is the last (they answered on LinkedIn, and the same Sync brought both), there is nothing to reply to: keep its Opportunities and leave the Item as it is. Nando sets it aside after 3 claims.

### `late_reply`: a Suggested Reply turned late

The Recruiter's message is now over 30 days old. Rewrite the Suggested Reply as a `late` one from the templates (step 5).

### `follow_up`: an open Opportunity silent since the Owner's message

The Owner wrote last, 14 days ago or more, on an Opportunity In Process or at Offer. Write a `follow_up`: a short, friendly nudge asking where things stand, in the Owner's voice (step 4), with nothing about being late. `answers` is the `sentAt` of the Owner's message it follows up. When the Conversation's `suggestedReply` has a `markedRepliedAt`, the Owner sent that reply in LinkedIn and no Sync has brought it yet: follow up that reply, with `answers` set to its `markedRepliedAt`.

## 3. Opportunities

An Opportunity is one role at one Company, from the first message about it until it closes. A Recruiter who pitches three Companies brings three. One the Recruiter brought earlier is in `recruiter.opportunities`: update it rather than making a second one.

**A message sent after `liveSince`** that names a Company and a role creates an Opportunity: `career_save_opportunity` with `company` (`{"id"}` of a Company Nando holds, found with `career_search`, or `{"name"}` for a new one), `role`, `recruiterId`, and:

- `fitNote`: two or three sentences on how the role fits the Career Profile: what matches, what clashes with a deal-breaker, what's unknown. Facts, no ranking.
- `details`: what the Recruiter said about the role, in short: the job description, team, product, anything they stressed.
- `pay` and `conditions`, only what was said: the salary range as `pay.firstStated` (annual, in JPY; a monthly figure is ×12, and say so in `details`), whether bonus is included, `remote` (`remote`, `hybrid` with `officeDays`, or `office`), `officeLocation`, `workingLanguage` (`ja`, `en` or `mixed`).
- `perks`: anything beyond pay and conditions, as short free-text items.

A message **before `liveSince`** never creates an Opportunity, however clear its role is: that Recruiter stays a Lead, and their Conversation gets a reply only.

**A new Company** gets a Company Brief: `career_save_company` with its `id` and `brief`, plus `websiteUrl` and `linkedInUrl` if you find them. Use public information only: the Company's own site, its LinkedIn page, news, and engineering blogs or talks. In a few short paragraphs, say:

- what the Company does and for whom;
- its size, age and funding or listing;
- the tech it uses;
- its reputation as an employer, such as reviews or how it treats foreign engineers;
- any red flags.

Stop where only the Owner could add more, such as what the team is like inside or what the interviews ask: write that as open questions at the end. If nothing public can be found, say so in one line.

**An Opportunity the Recruiter takes further** changes with what they say, all through `career_save_opportunity` (only the fields you send change; `pay` and `conditions` are replaced whole, so start from `career_get_opportunity`) and `career_save_step`:

- a date set for an interview, a call or an assignment is a Step: `kind` as they name it ("Casual interview", "Coding test"), `date`, `time` if they gave one, and short `notes`;
- the first Step makes it `in_process`; an offer makes it `offer`. The Stage skips ahead but never goes back;
- an offered salary goes in `pay.offered`;
- the Company saying no goes in `details`, dated, leaving the Opportunity open: closing it as `rejected` is the Owner's, in chat.

An Opportunity closes as `withdrawn` only once the Owner's own message in the Conversation declines it. Close it no other way: accepting or declining an offer, a rejection, and changing Interest are the Owner's to tell Claude in chat.

## 4. The Owner's voice

Every Suggested Reply must read as the Owner's own message.

- **Language:** reply in the language the Recruiter wrote in: `ja` or `en`.
- **Match the Owner:** read the Owner's own earlier messages in this Conversation, and in the Recruiter's other `conversations`, and write the way they do: their greeting, their length, their sign-off.
- **Japanese**, at the Owner's level (JLPT N2): polite です/ます, and the standard set phrases (お世話になっております, よろしくお願いいたします, ご連絡ありがとうございます). One humble form at a time, never stacked 謙譲語. Shorter sentences, plain words.
- **Like a person typing on LinkedIn:** two or three short paragraphs of plain prose, getting straight to the point and answering what the Recruiter actually asked, with commas and full stops for punctuation. Address the Recruiter as the Owner did, or by first name in English and by family name with 様 in Japanese. Recruiters must never notice it's AI.
- **Looking:** engage. Thank them, show interest in the role, ask for the job description and the salary range if they're missing, and propose a casual meeting.
- **Not Looking:** warm and short. Thank them, say the Owner isn't looking right now, and that they'd like to stay in touch.
- **Further along:** when an Opportunity is In Process or at Offer, answer what the Recruiter asked (dates, documents) plainly, whichever the tone.

Write it with `career_set_suggested_reply`: `conversationId`, `text`, `language`, `kind` (`answer`, `late` or `follow_up`), `answers` (for an `answer` or a `late` reply, the `sentAt` of the Recruiter's newest message), and `notes`.

**`notes`** are the "About your last message" note: up to 3 language notes on the Owner's own last message in the Conversation, each `{"wrote", "natural", "why"}`, only for real mistakes (grammar, a wrong word, unnatural Japanese, wrong keigo). Leave style alone. No mistakes, or no message from the Owner yet: `[]`.

## 5. Late Reply templates

A `late` reply starts from the template for whether the Owner is Looking, in the Recruiter's language. Fill in the Recruiter's name and how long it's been since their message ("few months", "year", "ここ半年", "1年ほど"). Then fit it to the Conversation: name the role or Company if the Recruiter gave one, and change little else.

**Looking, English:**

```text
Hello {first name}!

My deepest apologies for the late response. I had been inactive on LinkedIn for the past {time}.

Due to changes at my company, I am starting to look into new opportunities. I understand this one is most likely no longer available, but if you have anything else to share, I would love to keep hearing from you!

Warm regards,
Fernando
```

**Not Looking, English:**

```text
Hello {first name}!

My deepest apologies for the late response. I had been inactive on LinkedIn for the past {time}.

At the moment I am not actively looking for new opportunities, but I appreciate you sharing this one with me.

I hope we can stay connected in the future.

Warm regards,
Fernando
```

**Looking, Japanese:**

```text
{family name}様

お世話になっております。リオスです。
返信が大変遅くなってしまい、申し訳ありません。{time}ほどLinkedInを開いていませんでした。

会社の状況が変わったこともあり、新しい仕事を探し始めました。こちらのポジションはもう募集が終わっているかと思いますが、ほかにもお話があれば、ぜひ聞かせてください。

よろしくお願いいたします。
```

**Not Looking, Japanese:**

```text
{family name}様

お世話になっております。リオスです。
返信が大変遅くなってしまい、申し訳ありません。{time}ほどLinkedInを開いていませんでした。

今のところ転職は考えていませんが、ご連絡いただきありがとうございました。

今後ともつながっていられたら嬉しいです。よろしくお願いいたします。
```

## 6. What the run never does

Deleting is the Owner's, in chat. NEVER call `career_delete_step` or `career_delete_opportunity`, even to fix your own mistake: correct it with `career_save_opportunity` or `career_save_step` instead. Never close an Opportunity except as `withdrawn` once the Owner's own message declines it (step 3). Never write to the Career Profile or the Settings.

## 7. Repeat

Back to step 1 until it answers no Items, or until this run has done 30 Items: then stop, so the run's context stays short. Nando wakes another run for any left, at the next Sync or tick.
