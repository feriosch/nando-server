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
The shared foundation every Module relies on: proving the Owner's identity, delivering Notifications, the Owner's settings and body measurements, and the Home screen. Core knows nothing about any specific Module.
_Avoid_: Common, shared, platform, kernel

**Home**:
The screen Nando opens to, made up of Home Cards from Core and from every Module.
_Avoid_: Dashboard, start page, landing

**Home Card**:
A small summary that a Module (or Core itself) supplies for Home, such as today's Consumed Calories against the Daily Limit, or the latest Weigh-in.
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
Today plus the previous 7 Days. Only Days inside it can be changed; older Days are read-only.
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

## Japanese

The Module for the Owner's Japanese vocabulary and kanji study: Claude adds and improves what is stored, and the Owner searches it and takes it to Anki.

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

**Hint**:
A one-way, text-only pointer from a Word to an everyday word that isn't stored as a Word, such as 天気. It has no other side.
_Avoid_: Basic link, text link, Word Link

**Kanji**:
One character the Owner studies, stored with its Spanish meaning, readings, Components and a story to remember it by, and numbered by its place in Heisig's Remembering the Kanji (volume 1, and volume 2 for some).
_Avoid_: Character, glyph

**Component**:
One of the shapes a Kanji is written with, such as 氵 in 海: either a Kanji itself or an irregular shape that isn't one.
_Avoid_: Part (that is an Expression's)

## Body

Measurements of the Owner, kept in Core so that any Module can read them.

**Weigh-in**:
The Owner's weight for one week (Monday to Sunday in the Owner Time Zone), in kg; at most one per week, and a later one in the same week replaces the earlier.
_Avoid_: Weight log, measurement, entry

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
One browser or Home Screen app in which the Owner is signed in, so one iPhone can have two, and separate from any Push Subscription on the same device. It expires if left unused, and deleting it signs that browser or Home Screen app out.
_Avoid_: Login, token, device

**Connector**:
An outside app, such as Claude, that the Owner allowed, with a Passkey, to use Nando's Tools on their behalf. A Connector never signs in and has no Session; removing it cuts it off at once.
_Avoid_: Integration, client, app, bot, token

## Notifications

**Notification**:
A short message delivered to the Owner's installed devices. Core always sends it, either on a Module's behalf or for itself (such as the Weigh-in reminder).
_Avoid_: Alert, push, message

**Push Subscription**:
The permission and delivery address of one installed copy of Nando on one device; a Notification is sent to every Push Subscription the Owner has.
_Avoid_: Device, token, registration
