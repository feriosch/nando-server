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

**Settings Section**:
The part of the single Settings screen that a Module (or Core itself) supplies for its own preferences, such as the Daily Limit for Calories.
_Avoid_: Preferences page, module settings, config

**Editable Window**:
Today plus the previous 7 Days. Only Days inside it can be changed; older Days are read-only.
_Avoid_: Grace period, edit range

## Calories

The Module for tracking what the Owner eats each Day.

**Meal**:
Something the Owner eats repeatedly, saved by name with its kcal (and optionally the store it comes from) so it can be logged in one tap. Not tied to any Day. A snack is a Meal too. A different recipe or size of the same product is a separate Meal, not a new version of one.
_Avoid_: Food, item, product, dish, breakfast/lunch/dinner

**Tag**:
A free-form label on a Meal that adds information, such as "snack". It never changes how a Meal behaves.
_Avoid_: Category, type, kind

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
Proving it is the Owner with a Passkey, either an existing one or one just registered with an Enrollment Code, which starts a Session. Deleting that Session signs it out.
_Avoid_: Log in, login, authenticate

**Session**:
One browser or Home Screen app in which the Owner is signed in, so one iPhone can have two. It ends after 90 days unused or when the Owner deletes it, and is separate from any Push Subscription on the same device.
_Avoid_: Login, token, device

## Notifications

**Notification**:
A short message delivered to the Owner's installed devices. Core always sends it, either on a Module's behalf or for itself (such as the Weigh-in reminder).
_Avoid_: Alert, push, message

**Push Subscription**:
The permission and delivery address of one installed copy of Nando on one device; a Notification is sent to every Push Subscription the Owner has.
_Avoid_: Device, token, registration
