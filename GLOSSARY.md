# Nando

A personal app for managing the Owner's daily routines, installed on their iPhone's Home Screen and able to send them notifications. This is the canonical glossary for both `nando-server` and `nando-web`.

## Language

**Owner**:
The single person who uses Nando. There are no other users.
_Avoid_: User, account, customer

**Owner Time Zone**:
The single time zone, set in the Owner's settings, in which all of Nando's dates and clock times are interpreted, regardless of where the Owner's device currently is.
_Avoid_: Local time, device time, user time zone

**Day**:
One calendar date in the Owner Time Zone, from 00:00 to 24:00.
_Avoid_: Date (when meaning the period), 24 hours

**Module**:
A self-contained area of functionality (such as calorie tracking) that exists as its own slice of both the server and the web app. A Module depends only on Core, never on another Module.
_Avoid_: Feature, app, plugin

**Core**:
The shared foundation every Module relies on: proving the Owner's identity, delivering Notifications, the Owner's settings (including their height), and the Home screen. Core knows nothing about any specific Module.
_Avoid_: Common, shared, platform, kernel

**Home**:
The screen Nando opens to, made up of Home Cards from Core and from every Module.
_Avoid_: Dashboard, start page, landing

**Home Card**:
A small summary that a Module (or Core itself) supplies for Home, such as today's Consumed Calories against the Daily Limit, or the latest Weigh-in. A Module may supply more than one.
_Avoid_: Widget, tile, panel

**Tool**:
An action Core or a Module offers to Connectors, such as searching Words or adding one. Core serves every Tool to every Connector without knowing what any Module's Tools do.
_Avoid_: Command, endpoint, function, skill

**Settings Section**:
The part of the single Settings screen that a Module (or Core itself) supplies for its own preferences, such as the Daily Limit for Calories.
_Avoid_: Preferences page, module settings, config

**Appearance**:
Whether Nando shows its light or dark colours: Light, Dark, or System (following the device's own setting). It is chosen and kept on each device separately, not stored with the Owner's settings on the server.
_Avoid_: Dark mode, color scheme, theme

**Editable Window**:
Today plus the previous 7 Days. Only Days inside it can be changed; older Days are read-only. Money has none: its Statements can change for any month (ADR 0014).
_Avoid_: Grace period, edit range

## Calories

The Module for tracking what the Owner eats each Day.

**Meal**:
Something the Owner eats repeatedly, saved by name with its kcal (and optionally the store it comes from) so it can be logged in one tap. Not tied to any Day. A snack is a Meal too. A different recipe or size of the same product is a separate Meal, not a new version of one.
_Avoid_: Food, item, product, dish, breakfast/lunch/dinner

**Meal Tag**:
A free-form label on a Meal that adds information, such as "snack". It never changes how a Meal behaves.
_Avoid_: Tag (alone), category, type, kind

**Entry**:
One record that the Owner ate something on a particular Day: a name, a kcal per serving and a number of Servings, either taken from a Meal or typed in for a one-off item. It keeps those values from when it was logged, so later changes to a Meal never alter it.
_Avoid_: Log, record, consumption, meal (for the instance)

**Servings**:
The whole number of units eaten in one Entry (1 or more), such as 12 pieces of sushi; the Entry's kcal is its kcal per serving times its Servings.
_Avoid_: Quantity, portion, amount

**Consumed Calories**:
The sum of the kcal of a Day's Entries. Always derived from the Entries, never entered directly.
_Avoid_: Daily total, intake, calories eaten

**Unlogged Day**:
A Day with no Entries. It means "not recorded", not "ate nothing", so it is never treated as 0 kcal or as being under the Daily Limit.
_Avoid_: Empty day, zero day, fasting day

**Daily Limit**:
The most kcal the Owner intends to consume in a Day; a ceiling to stay under, not a goal to reach. It is set once and stays in force until the Owner changes it, and each change applies from the Day it is made, so past Days are judged against the limit they had. The very first Daily Limit also covers every Day before it.
_Avoid_: Target, goal, budget, allowance

**Weigh-in**:
The Owner's weight for one week (Monday to Sunday in the Owner Time Zone), in kg; at most one per week, and a later one in the same week replaces the earlier. It belongs to Calories (ADR 0012), which also derives its BMI from the Owner's height in Core's settings.
_Avoid_: Weight log, measurement, entry

## Japanese

The Module for the Owner's Japanese vocabulary and kanji study: Claude adds and improves what is stored, either in the Claude app or by working through the Owner's Word Requests, and the Owner searches it and takes it to Anki.

**Word**:
A Japanese word or Expression the Owner is learning, stored once in its kanji spelling (if it has one) with its reading, Spanish meaning, Group, Word Tags, example sentences and Word Links.
_Avoid_: Vocabulary, entry, card, note, item

**Expression**:
A set phrase, such as 首にされる or 腹が立つ, stored as one Word and treated as the part of speech it acts as. A phrase that jisho.org doesn't list is a collocation, not an Expression.
_Avoid_: Phrase, idiom, collocation

**Part**:
A Word that an Expression is built from, such as 首 in 首にされる. A Part the Owner doesn't know becomes a Word of its own, joined to the Expression by a Word Link, unless it is hardly used outside that Expression.
_Avoid_: Component (that is a Kanji's)

**Group**:
The one place a Word is filed. A Word that jisho.org tags N2 or N3 goes in JLPT N2 or JLPT N3, and one tagged N4 or N5 goes in Basic; any other Word goes in the Group for what the Owner was doing when they found it (Literature, Gaming, Anime, Movie or Miscellaneous), or in JLPT N1 if they were studying for N1. Basic Words are kept so that other Words can be linked to them, not to be studied. Each Group has a display name and a picture.
_Avoid_: Collection, deck, category, level

**Word Tag**:
A label on a Word, from a mostly fixed list, saying when or how to use it (formal, archaic, spoken…) or a fact about it (common, usually written in kana, uses a kanji outside the jōyō list, JLPT N1). Unlike a Meal Tag, some Word Tags change how the Word is exported to Anki.
_Avoid_: Tag (alone), label, category

**Word Link**:
A two-way connection between two Words, of one of three kinds: Synonym, Antonym or Related. Each side has its own note and Nuance Tags about the other Word. Changing one side leaves the other as it is; removing a Word Link removes both sides.
_Avoid_: Link (alone), relation, related word, URL

**Nuance Tag**:
A label on one side of a Word Link saying how the other Word differs from this one, such as formal or written. When a Word Link is made, the other side gets the opposite Nuance Tags (formal and casual, spoken and written, Expression and Part swap).
_Avoid_: Tag (alone), Word Tag

**Word Request**:
The Owner's ask, made in Nando, for Claude to add a word (with the Group the Owner found it in) or to improve a stored Word. It waits until a Claude run takes it, then holds the outcome: done, with a short summary and any suggestions for the Owner; a question for the Owner to answer; or failed.
_Avoid_: Pending word, job, task, topic, inbox item

**Hint**:
A one-way, text-only pointer from a Word to an everyday word that isn't stored as a Word, such as 天気. It has no other side.
_Avoid_: Basic link, text link, Word Link

**Kanji**:
One character the Owner studies, stored with its Spanish meaning, readings, Components and a story to remember it by, and numbered by its place in Heisig's Remembering the Kanji (volume 1, and volume 2 for some).
_Avoid_: Character, glyph

**Component**:
One of the shapes a Kanji is written with, such as 氵 in 海: either a Kanji itself or an irregular shape that isn't one.
_Avoid_: Part (that is an Expression's)

**Drawing**:
A kanji the Owner draws by finger, stroke by stroke, to find out which one it is, usually because they can't type or read it.
_Avoid_: Sketch, handwriting, canvas, image

**Candidate**:
One of the up to 8 kanji recognised for a Drawing, best guess first. A Candidate may be a stored Kanji or one the Owner hasn't studied.
_Avoid_: Result, prediction, match, suggestion

**Kanji Lookup**:
What Nando finds out about a kanji the Owner hasn't stored, from kanjiapi.dev and jisho.org: its English meanings, readings, strokes, grade and JLPT level, and some common words with it. It is read when asked for, never stored, and never made for a stored Kanji.
_Avoid_: Preview, external Kanji, unstored Kanji page

## Career

The Module for keeping up with the Opportunities that Recruiters bring the Owner, from the first message until each one closes. Claude reads the Owner's messages and suggests replies; the Owner always sends them.

**Opportunity**:
One role at one Company put to the Owner, from the first message about it until it closes. A Recruiter who pitches three Companies brings three Opportunities, two roles at the same Company are two Opportunities, and a Company that comes back after an Opportunity closed starts a new one. It has at most one Recruiter, the one who brought it; one the Owner finds themselves has none.
_Avoid_: Application, process, job, deal

**Stage**:
Where an Opportunity stands overall: Introduced, In Process, Offer, or Closed with an outcome. It can skip ahead but never goes back.
_Avoid_: Status, phase, step

**Outcome**:
How a Closed Opportunity ended: Accepted, Declined (the Owner turned down an Offer), Withdrawn (the Owner stepped away before an Offer, for any reason), Rejected (the Company said no) or Faded (30 days passed with no message about it).
_Avoid_: Result, resolution, status

**Step**:
One dated event inside an Opportunity's process, of any kind, such as a casual interview, a take-home assignment or a final interview. An Opportunity has any number of Steps, in any order, and none are required.
_Avoid_: Round, phase, interview (for any Step), stage

**Perk**:
Something an Opportunity offers beyond its pay and working conditions, written freely, such as a housing allowance or side jobs being allowed.
_Avoid_: Benefit, extra

**Interest**:
How much the Owner wants an Opportunity: High, Normal or Low, Normal until the Owner says otherwise. The Owner's most relevant Opportunities are ranked by Interest first, then by how far along they are.
_Avoid_: Priority, rating, score, fit

**Company**:
An employer that has been put to the Owner in at least one Opportunity. It keeps what Nando knows about the employer across all of its Opportunities.
_Avoid_: Employer, firm, client

**Company Brief**:
Claude's summary of a Company from public information, written when the Company's first Opportunity appears. It stops where only the Owner could add more, and the Owner and Claude take it further together in a chat.
_Avoid_: Research, report, profile, dossier

**Recruiter**:
A person who contacts the Owner about roles, either from an agency or from the hiring Company itself, on LinkedIn or by email. One Recruiter can bring many Opportunities. Nando knows them by their LinkedIn profile and by the email addresses their filed Gmail threads came from.
_Avoid_: Headhunter, agent, contact, HR

**Lead**:
A Recruiter who has never brought the Owner an Opportunity. It is worked out from the Recruiter's Opportunities, never set by hand, and a Recruiter who brings one stops being a Lead for good.
_Avoid_: Prospect, cold contact, inactive recruiter

**Conversation**:
One message thread between the Owner and exactly one Recruiter, of which Nando keeps a copy: a LinkedIn conversation, which a LinkedIn Sync brings, or a Gmail thread, which a Gmail Career Sync brings. A Recruiter can have both.
_Avoid_: Thread, chat, inbox, message (for the whole thread)

**Suggested Reply**:
Claude's draft of the Owner's next message in a Conversation, written to sound like the Owner: an answer when the last message is the Recruiter's, or a follow-up when an open Opportunity has had no message for 14 days after the Owner's. It follows whether the Owner is Looking. Only the Owner ever sends it, or the Core Assistant on their yes for a Gmail Conversation; the Career Assistant never sends anything. It is gone from the Owner's lists once they mark it replied in the app, and for good once a LinkedIn Sync or a Gmail Career Sync sees the Owner's reply.
_Avoid_: Draft, auto-reply, response, answer

**Late Reply**:
A Suggested Reply to a Recruiter's message more than 30 days old, apologising for the delay. Late Replies are kept apart from the others so that they never bury new ones.
_Avoid_: Revival, catch-up, old reply

**LinkedIn Sync**:
One pass in which Nando picks up the Owner's new LinkedIn messages from the Owner's MacBook, at most twice a Day. A Day on which the MacBook is never on has no LinkedIn Sync.
_Avoid_: Sync (alone), pull, fetch, scrape, import, read

**Gmail Career Sync**:
One pass in which the Career Assistant files into Nando the new mail the Owner's Gmail holds under the label `Career/Recruiters`, once a Day at the time in Career's settings, or whenever the Owner asks. It reads only that label, which the Core Assistant applies in its Gmail Filing (ADR 0011), and never LinkedIn.
_Avoid_: Mail Sync, Gmail Sync, Sync (alone), import, scan

**Career Profile**:
What Claude knows about the Owner's working life, to judge Opportunities and write Suggested Replies: their resumes, current situation, what they are looking for and their deal-breakers.
_Avoid_: Resume (for the whole), CV, preferences, about me

**Looking**:
Whether the Owner is looking for a new role right now, set by the Owner as Looking or Not Looking. It changes the tone of every Suggested Reply and how often the Owner hears about them.
_Avoid_: Open to Work (that is LinkedIn's), job hunting, active, available

## Travel

The Module that watches flight prices for the Owner's Trips, so the Owner never has to check fares or price emails themselves. Claude searches and judges; the Owner always books (ADR 0013).

**Trip**:
A journey the Owner may take: a round trip from where the Owner lives to one destination, with a range of nights, the windows of dates it may fall in, the dates it must avoid, and the cabin, bags and stops it allows. It is Watching until the Owner says it is Booked or Dropped, and only a Watching Trip is searched. Trips are the Owner's data, held only in Nando's database, never named in the code or docs.
_Avoid_: Plan, itinerary, route, search, watch

**Party**:
Who a Trip is priced for: the Owner alone, or the Owner with the Companion. A Trip has one or two Parties, watched side by side, so the Owner can weigh going alone against going together.
_Avoid_: Travellers, passengers, group, version

**Companion**:
The person who travels with the Owner in a Party. Nando reads their calendar, never changes it, to know which dates they can travel and what Leave Needed a Fare would cost them. Who they are and which calendars are theirs are held only in Travel's settings.
_Avoid_: Partner (in code and docs), passenger, guest, plus-one

**Leave Needed**:
How many days off work the Companion would take for a Fare: the workdays it covers, less public holidays and leave they have already booked. It is shown with and without up to 2 days the Companion could work remotely from the destination, which counts only when staying longer is worth it.
_Avoid_: PTO, vacation days, days off (alone)

**Fare**:
One priced round trip found for a Trip and Party: its dates, flights, stops and journey time, its price per person and in total, whether that includes checked bags (not every source prices them), where it was found, and a link to book it.
_Avoid_: Flight, offer, deal, price, result

**Separate Tickets**:
A Fare made of flights sold apart, which the Owner connects between themselves: if the first is late, no airline has to carry them on. It is always shown beside the best Fare on one ticket, never alone, and never changes airport at the connection.
_Avoid_: Self-transfer, hacker fare, virtual interlining

**Price Verdict**:
Google's judgement of a Fare's price for its route and dates: low, typical or high, with the typical range. Nando fetches it from Google Flights through SerpApi, within a monthly limit in Travel's settings.
_Avoid_: Rating, score, price level, insight

**Price Alert**:
An email from a fare site or an airline about prices, such as Google Flights tracking or an airline's sale. The Core Assistant files it under `Travel Log/Alerts` in its Gmail Filing, and the next Fare Check reads it. Nando keeps the Fares it yields, never the email.
_Avoid_: Fare Alert (that is Nando's own message), deal email, newsletter

**Fare Check**:
One pass in which the Travel Assistant searches for Fares and records them. The **Daily Fare Check**, after the Gmail Digest, reads the new Price Alerts and searches again only where something moved. The **Weekly Fare Check**, on Sunday unless Travel's settings name another weekday, searches every Watching Trip's whole window for each Party, and ends with the Weekly Travel Summary. The Owner can also ask for one at any time, for the Trips they name or every Watching Trip.
_Avoid_: Search, scan, scrape, poll, sync

**Fare Alert**:
The Travel Assistant's message about a Fare worth acting on, saying why: the lowest seen for its Trip and Party, a low Price Verdict, a big drop since last week, or below the Target Fare. It comes in the day's one Travel message, unless it needs action within hours (a sale ending, few seats left at a good price), when it is sent at once, at any hour.
_Avoid_: Price Alert (that is a site's email), deal alert, notification (alone)

**Target Fare**:
The price per person below which the Owner wants to hear about a Trip and Party. The Travel Assistant suggests one after about a month of Fares, and it holds only once the Owner agrees.
_Avoid_: Budget, goal, limit, threshold

**Weekly Travel Summary**:
The Travel Assistant's message at the end of the Weekly Fare Check on how each Watching Trip stands: the best Fares for each Party, their trend, the cheapest stretches of its windows, and any Target Fare it suggests.
_Avoid_: Report, digest (that is Gmail's), newsletter

## Money

The Module that keeps the Owner's money across Japan and Mexico: what they spend, earn and hold, Transaction by Transaction, so the Money Assistant can find trends and spending the Owner could do without. The banks offer no API to one person, so the Owner sends each Statement in the Money chat and Claude reads it (ADR 0014).

**Account**:
One place the Owner's money sits or is spent from, in one currency: a bank account, a credit card, a savings pot or an investment account. Accounts are the Owner's data, held only in Nando's database, never listed in the code.
_Avoid_: Wallet, source, bank (for a card)

**Statement**:
One record of an Account for one period that the Owner sent, as a CSV, a PDF or screenshots, with the total or closing balance it states. Nando keeps its Transactions and a copy of its file. Sending a Statement for the same Account and period again replaces it, so nothing is counted twice.
_Avoid_: Upload, import, file, bill

**Transaction**:
One line of a Statement: an **Expense**, an **Income** or a **Transfer**, in its original currency, counted in the month of its purchase date, whatever month it was billed in.
_Avoid_: Movement, entry, charge, record

**Transfer**:
A Transaction that moves money between two of the Owner's own Accounts, such as a bank paying a card's bill or money put into savings. It never counts as spending or income. Taking cash out or topping up an e-money app is not a Transfer, since neither is an Account: it is an Expense.
_Avoid_: Payment (for a card's bill), movement

**Category**:
Where an Expense or Income belongs, from one fixed list, so that months always compare. A Refund lowers spending in the Category it came back to rather than counting as Income.
_Avoid_: Tag, label, type, bucket

**Subcategory**:
A finer place inside a Category, such as Fashion or Furniture inside Shopping. Unlike Categories, Subcategories grow as needed: Claude creates one when nothing fits and says so, and the Owner can rename or merge them.
_Avoid_: Tag, sub-tag, child category

**Essential**:
Spending the Owner needs, as opposed to **Optional** spending they choose. Each Subcategory is one or the other, and a single Transaction can say otherwise. Optional spending is where unnecessary expenses are looked for.
_Avoid_: Need/want, necessary, mandatory, discretionary

**Merchant**:
Who a Transaction was with, cleaned up from a Statement's raw text (`ｾﾌﾞﾝ-ｲﾚﾌﾞﾝ…` is 7-Eleven), with the Subcategory its Transactions usually take. It learns from the Owner's corrections. A Merchant that sells everything, such as an online store, has no usual Subcategory, and its Transactions are looked up in the Owner's receipts or asked about.
_Avoid_: Store, shop, vendor, payee

**Unconfirmed**:
A Transaction saved with Claude's best guess at its Merchant, Subcategory or kind while the Owner's answer is awaited. Nothing waits on it: the Statement is saved whole, and the guess becomes final once the Owner answers.
_Avoid_: Pending (that is a card's own word), draft, uncategorized

**Subscription**:
A charge that repeats, monthly or yearly, at one Merchant, such as streaming or an app. Claude keeps the list from the Transactions and the Owner's receipts, and points out new ones, price changes and ones the Owner may no longer use.
_Avoid_: Recurring charge, plan, membership

**Return**:
How much a savings or investment Account grew in a month: its balance, less last month's, less the money Transferred in. It is neither Income nor saving.
_Avoid_: Interest (that is a bank account's Income), profit, yield, gains

**Rate**:
The one exchange rate between yen and pesos that Nando uses for a month: the month's average for Transactions, and the month's last day for balances. Nando fetches it from the European Central Bank's published rates and never changes it afterwards. Every amount keeps its own currency; Rates only turn totals into yen.
_Avoid_: Exchange, FX, conversion

**Complete**:
A month for which every Account has a Statement covering its last day, except an Account set not to hold the month back, such as a card that only pays the rent and closes mid-month. A Transaction that appears late, on the next Statement, still joins its own month.
_Avoid_: Closed, finished, done, reconciled

**Statement Reminder**:
A Money Notification asking for the Statements of one app the Owner opens, such as one bank's app that holds both an account and a card, on its day in Money's settings. It is sent only while those Statements are missing, and again 3 days later if they still are.
_Avoid_: Checklist, to-do, nag

**Monthly Review**:
The Money Assistant's message when a month becomes Complete: spending against last month and the 3-month average, Essential against Optional, the Categories that moved most, Subscriptions, anything worth a look, net worth, Return and the saving rate, and what is still Unconfirmed.
_Avoid_: Report, digest (that is Gmail's), summary, statement (that is an Account's)

## Identity

**Passkey**:
A secret on the Owner's devices, unlocked by the device's own check such as Face ID, that proves it is the Owner. It counts as one Passkey however many devices it is synced to, and it is the only way to sign in.
_Avoid_: Password, key, login

**Enrollment Code**:
A single-use, short-lived code, issued only from the server's command line, that lets the Owner register a new Passkey; it is the only way into Nando without an existing Passkey.
_Avoid_: Invite, setup token, recovery code, password

**Sign in**:
Proving it is the Owner with a Passkey, either an existing one or one just registered with an Enrollment Code, which starts a Session. Unlike the other terms it is a verb, written in lowercase ("sign in", and "sign-in" as a noun).
_Avoid_: Log in, login, authenticate

**Session**:
One browser or Home Screen app in which the Owner is signed in, so one iPhone can have two. It expires if left unused, and deleting it signs that browser or Home Screen app out.
_Avoid_: Login, token, device

**Connector**:
An outside app, such as Claude, that the Owner allowed, with a Passkey, to use Nando's Tools on their behalf. A Connector never signs in and has no Session; removing it cuts it off at once.
_Avoid_: Integration, client, app, bot, token

## Messenger

**Messenger**:
The outside chat app (Telegram) in which the Owner talks to Claude about their daily life in Nando, with one chat for each Assistant.
_Avoid_: Chat app, channel, integration

**Talk**:
The running exchange between the Owner and one Assistant in its chat in the Messenger. It goes on until the Owner asks for a new one, or until Claude decides it has grown too long and starts a new one to replace it.
_Avoid_: Conversation (that is a Recruiter thread), chat, thread, session

**Assistant**:
Claude in one of the Owner's chats in the Messenger, looking after one Module, such as the Career Assistant. It can read everything in Nando and in the Outside Services, but changes only its own Module, Core and the Outside Services, and points the Owner to another Assistant for anything else.
_Avoid_: Bot, agent, Connector

**Core Assistant**:
The Assistant for Core, and for anything the Owner asks that belongs to no Module, such as adding a friend's birthday to their calendar.
_Avoid_: Nando Assistant, general bot, main bot

**Outside Service**:
An app outside Nando that serves the Owner's whole life rather than one Module: Google Calendar and Gmail so far, and perhaps documents later. Every Assistant reads the Outside Services, and only the Core Assistant changes the Owner's Gmail, but for the drafts the Career Assistant creates in `Career/Recruiters` threads (ADR 0010, ADR 0011).
_Avoid_: Integration, external app, Connector (that is the reverse: an outside app using Nando)

**Note**:
What a run leaves in a Talk for later runs with its reply, such as the ID of a draft the Owner was shown. Nando keeps it in the Talk, and carries the latest ones into a new Talk, but never sends it to the Owner.
_Avoid_: Memo, comment, metadata, hidden message

**Gmail Digest**:
The Core Assistant's short daily summary of the mail that came since the last one, at the time in the Owner's settings. Nando only asks for it, with a Notification; Claude reads the mail, files it in a Gmail Filing, and writes the digest, saying what it filed and what it starred.
_Avoid_: Mail Digest (its old name), inbox summary, newsletter, briefing, report

**Gmail Filing**:
The Core Assistant's daily filing of the Owner's new mail, in the Gmail Digest's run (ADR 0011): it labels each thread by judgment, stars what needs the Owner, and archives the rest. It never archives mail without a label, and never trashes, marks spam or sends.
_Avoid_: Auto-labeling, rules, filters, sorting, triage

**Filing Record**:
What Nando keeps of each change a Gmail Filing makes, for 90 days: the thread, the labels added and removed, whether it was starred, and why. The Owner can ask the Core Assistant to undo any of it.
_Avoid_: Log, history, audit trail

**Starred**:
In the Owner's Gmail, the mark for mail that needs the Owner: a reply someone waits for, a request, a payment, an upcoming booking, a delivery this week. A Recruiter's mail is never starred: it goes to `Career/Recruiters`, where the Gmail Career Sync picks it up. A Gmail Filing leaves starred mail in the inbox and archives the rest; the Owner unstars it once it's done.
_Avoid_: Important (that is Gmail's own marker), flagged, pinned, to-do

## Notifications

**Notification**:
A short message Nando sends the Owner first, without being asked. Core always sends it, either on a Module's behalf (such as Calories' Weigh-in reminder) or for itself, in the Messenger: in the chat of that Module's Assistant, or the Core Assistant's when the Module has none. It becomes part of that Assistant's Talk, so the Owner can answer it there. A Notification can also ask that Assistant for work, such as the Gmail Digest: then it waits in the Talk like the Owner's message, and a run answers it.
_Avoid_: Alert, push, message

