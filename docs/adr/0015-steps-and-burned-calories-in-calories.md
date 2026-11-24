# Steps and Burned Calories live in Calories, synced from the iPhone with a Sync Key

The Owner wants to see how much they moved next to what they ate: the Day's **Steps** and an estimate of the kcal they burned. They come from two places. Most Days, the iPhone counts steps and Apple Health estimates active kcal. The treadmill is the exception: the Owner leaves the iPhone on the console, so Health never sees those steps, and the treadmill's screen shows a kcal estimate but not reliably steps. So a Day's activity is a **Health Sync**, sent by an iOS Shortcuts automation, plus any **Treadmill Walks** the Owner tells the Calories Assistant about. A web app can't read HealthKit, and a Shortcut can't sign in with a Passkey or do OAuth, so the Shortcut reaches Nando with a **Sync Key**.

## Decisions

- **It belongs to Calories**, which becomes the Module for what the Owner eats *and burns* each Day. ADR 0012's rule applies: it lives where the habit is tracked, and the Owner wants Burned Calories beside Consumed Calories, which only works inside one Module.
- **It informs, it never changes the Daily Limit.** Steps and Burned Calories are shown next to Consumed Calories. Walking never raises the Daily Limit or "earns" kcal back: the Daily Limit stays a ceiling, and Burned Calories are rough estimates that would undo a deficit if eaten back.
- **Two records per Day, summed:**
  - **Health Sync**: one per Day, with the Day's steps and active kcal from Apple Health. Each sync replaces the Day's last one, and it is never edited by hand: the next sync is the correction.
  - **Treadmill Walk**: any number per Day, each with km, Steps and kcal. They are added to the Health Sync's numbers, never instead of them, since the iPhone is never carried on the treadmill.
  - **Steps** and **Burned Calories** are the Day's sums, always derived, never entered. Burned Calories are always labelled an estimate.
  - A Day with neither record has no Steps and no Burned Calories (`null`, shown "—"), never 0, like an Unlogged Day.
- **A Treadmill Walk's Steps come from the Owner's height:** `km × 100,000 / (0.415 × height in cm)`, the usual walking stride. They are worked out when the walk is logged and kept, as an Entry keeps its kcal, so a later change of height never rewrites old walks. With no height in Core's settings, the walk is logged without Steps and the Assistant asks the Owner to set it.
- **A Treadmill Walk's kcal is the one on the treadmill's screen.** Without it, the Calories Assistant estimates about 0.5 kcal per kg per km from the latest Weigh-in, and says so. With no Weigh-in either, it asks for the screen's kcal instead of guessing.
- **Burned Calories add the two as they are.** Health's active kcal excludes resting burn and most treadmills include it, so the sum runs a little high. That is accepted for a number that changes nothing.
- **Only the Calories Assistant writes Treadmill Walks**, like the rest of Calories: `calories_log_walk`, `calories_update_walk`, and `calories_delete_walk` (destructive, asked first). They follow the Editable Window, like Entries. The web app only reads.
- **The Health Sync comes from an iOS Shortcuts automation**, free and built into the iPhone:
  - Personal automations run at a few times a day (12:00, 18:00 and 23:55), without asking. Each sends today's steps and active kcal, and only today's: steps walked after the last run of a Day are not counted.
  - iOS hides Health data while the iPhone is locked, so a run on a locked phone finds nothing and sends nothing, leaving the Day's last Health Sync as it was. The other runs make up for it.
  - The Shortcut first sent yesterday too, to catch steps walked late at night. Building that in Shortcuts proved fiddly (its date filters and variables), so the Owner keeps it to today. Nando still takes yesterday, so a Day can be resent by hand the morning after.
  - Nando takes only today and yesterday, and leaves out any other Day sent, so that abroad, where the iPhone's Days aren't Nando's, the Days that match still land.
  - The Shortcut sums the Day in the iPhone's time zone, not the Owner Time Zone. Away from it, a Day's Health Sync can be a few hours off. That is accepted: it only matters on travel days.
  - It sends steps and active kcal only. Walking distance and resting kcal are left out until something uses them.
- **A Sync Key lets the Shortcut in, and Core owns it.**
  - The Owner creates one in Core's Settings Section, approving with a Passkey, as for a Connector. It is shown once and stored hashed, like Session and Connector tokens.
  - It reaches only the routes a Module marks as open to Sync Keys, as Modules supply Tools today: first `POST /api/calories/health-sync`. It can't read anything, can't call `/mcp`, and starts no Session.
  - It is listed in Settings and removing it cuts it off at once.
  - This amends ADR 0004, which rejected a long-lived key, for the Claude app, because Claude does OAuth. A Shortcut can't, and the key's reach is one write route, so the risk is someone overwriting a Day's steps.
- **A Notification says when syncing stops.** If no Health Sync has arrived for 48 hours, the Calories Assistant's chat gets one `Steps haven't synced since …`, once per gap, with no setting: it only fires when something is broken, such as a removed Sync Key or a disabled automation.

## Where it shows

- The **Calories Home Card** gets one line: today's Steps and Burned Calories (est.).
- The **Day view** gets an Activity section: the Health Sync (steps, kcal, when it last synced) and each Treadmill Walk.
- The **history** carries each Day's Steps and Burned Calories next to Consumed Calories.
- The **evening summary** adds a line with the Day's Steps and Burned Calories, as of the last Health Sync.
- `calories_get_day` answers with them, so the Calories Assistant includes them when the Owner asks how the Day went.

## Considered Options

- **Raise the Daily Limit by what was burned**: rejected. It turns the ceiling into a goal that moves, on estimates.
- **A separate Activity Module**: rejected. Calories couldn't show Burned Calories beside Consumed Calories, since Modules never read each other. If a future Gym Module needs Steps, this needs revisiting, as ADR 0012 says for weight.
- **One number per Day that both paths overwrite or add to**: rejected. A wrong Treadmill Walk couldn't be fixed without disturbing what Health sent.
- **Health Auto Export or another paid app**: rejected. A Shortcut does the same for free, with the same key question.
- **The Shortcut sends a Telegram message to the Calories Assistant**: rejected. Every sync would wake a Claude run, three a day, and it isn't certain that Telegram's Shortcut action runs without a prompt.
- **A Sync Key owned by Calories**: rejected. Credentials belong to identity, in Core. Core still knows nothing about what a route does, and a later Shortcut, such as one for Money, can reuse the mechanism.
- **Recalculate a walk's Steps from the current height**: rejected for keeping what was logged, as Entries do.

## Consequences

- The glossary's Calories section grows: Steps, Burned Calories, Health Sync and Treadmill Walk. Identity gains the Sync Key.
- Core gains Sync Keys: their own collection, a Passkey-approved create, list and delete in Core's Settings Section, and a way for a Module to mark a route as open to them.
- The Owner builds the Shortcut and its automations by hand on the iPhone, following `docs/health-sync-shortcut.md`.
