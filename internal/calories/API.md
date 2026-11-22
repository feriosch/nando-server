# Calories API

The Calories Module: Meals, the Entries that record what the Owner ate each Day, the Daily Limit each Day is judged against, the Owner's weekly Weigh-ins ([ADR 0012](../../docs/adr/0012-weigh-ins-belong-to-calories.md)), the Calories and Weight Home Cards, and the Calories Notifications. Every route is under `/api/calories/` and, like all of `/api/`, needs a valid Session cookie (see [Core's API](../core/API.md)). Bodies are JSON. Errors are a status code with a short plain-text message. The web app only reads: every route here is a `GET`. The Calories Assistant writes, through the Module's [Tools](#tools), which also read the same data ([#141](https://github.com/feriosch/nando-server/issues/141)).

A Day is a date in the Owner Time Zone, written `YYYY-MM-DD`.

## Meals

A Meal is something the Owner eats repeatedly, saved by name with its kcal so it can be logged in one tap. Editing a Meal changes only what is logged from it later: Entries keep the name and kcal they were logged with.

```json
{
  "id": "6af05d493ea403a78bf5997b",
  "name": "Tuna mayo onigiri",
  "kcal": 230,
  "store": "7-Eleven",
  "emoji": "🍙",
  "imageUrl": null,
  "tags": ["snack"],
  "pinned": false,
  "archived": false
}
```

- `name`: 1 to 100 characters, trimmed.
- `kcal`: per serving, a whole number from 0 to 10,000.
- `store`: at most 100 characters, or `null`.
- `emoji`: one emoji (at most 8 code points, so flags and family emoji fit), or `null`.
- `imageUrl`: an `https:` URL of a photo, set by hand (for example, a file in the Meal photos bucket), or `null`.
- `tags`: free-form labels, each 1 to 30 characters, at most 20. They are trimmed and repeats are dropped. A Meal Tag never changes how a Meal behaves.
- `pinned`: pinned Meals lead `calories_list_meals`.
- `archived`: `true` once the Meal is archived (`calories_archive_meal`).

### `GET /api/calories/meals`

`200`: the Meals in use, by name. With `?archived=true`, the archived ones instead.

## Entries

An Entry records that the Owner ate something on a Day: a name, a kcal per serving and a whole number of Servings. Its `kcal` is `kcalPerServing × servings`.

```json
{
  "id": "6af05d8a1400fbb2b9e973e3",
  "day": "2026-10-21",
  "mealId": null,
  "name": "Sushi",
  "kcalPerServing": 45,
  "servings": 12,
  "kcal": 540,
  "loggedAt": "2026-10-21T12:00:00Z"
}
```

- `mealId`: the Meal it was logged from, or `null` for a one-off Entry. The Meal may since have been edited or archived; the Entry keeps its own `name` and `kcalPerServing`.
- `servings`: a whole number from 1 to 100.
- `loggedAt`: when it was logged, in UTC.

Only Days in the **Editable Window**, today and the 7 Days before it, can be changed. Every Day can be read. The Tools refuse to log, edit or delete an Entry on any other Day, including a future one.

### `GET /api/calories/days/{day}/entries`

`200`: the Day's Entries in the order they were logged. `[]` for an Unlogged Day.

`400`: `day` isn't `YYYY-MM-DD`.

## Daily Limit

The Daily Limit is the most kcal the Owner intends to consume in a Day: a ceiling to stay under, not a goal to reach. Each change applies from the Day it is made (today), so past Days keep the limit they had. A second change on the same Day replaces the first. The very first Daily Limit also covers every Day before it. Until one is set, there is none, and no Day is over it.

### `GET /api/calories/daily-limit`

`200`: the Daily Limit in force now, or `null` if none was ever set.

```json
{ "kcal": 1800, "effectiveFrom": "2026-10-23" }
```

- `effectiveFrom`: the Day it was set, from which it applies.

## Days

Consumed Calories are always derived from a Day's Entries. An **Unlogged Day**, one with no Entries, means "not recorded", not "ate nothing": its `consumedCalories`, `remaining` and `overLimit` are `null`, never `0` or `false`, since it is never over or under the Daily Limit. A Day whose Entries add up to 0 kcal is logged, with `consumedCalories: 0`.

The Day view, the history, the Home Card and the Tools all describe a Day with the same fields:

```json
{ "day": "2026-10-21", "consumedCalories": 1000, "dailyLimit": 2000, "remaining": 1000, "overLimit": false }
```

- `consumedCalories`: the sum of the Day's Entries' `kcal`, or `null` for an Unlogged Day.
- `dailyLimit`: the Daily Limit the Day had, or `null` if none was ever set.
- `remaining`: `dailyLimit − consumedCalories`, negative once over; `null` for an Unlogged Day or without a Daily Limit.
- `overLimit`: whether `consumedCalories` is more than `dailyLimit`. Reaching it exactly isn't over. `false` without a Daily Limit, and `null` for an Unlogged Day.

### `GET /api/calories/days/{day}`

`200`: the Day view: the fields above, and the Day's `entries` in the order they were logged, as `GET /api/calories/days/{day}/entries` lists them.

```json
{
  "day": "2026-10-21",
  "consumedCalories": 1000,
  "dailyLimit": 2000,
  "remaining": 1000,
  "overLimit": false,
  "entries": [ … ]
}
```

`400`: `day` isn't `YYYY-MM-DD`.

### `GET /api/calories/history?from={day}&to={day}`

`200`: every Day from `from` to `to`, inclusive, newest first, each with the fields above.

```json
[
  { "day": "2026-10-22", "consumedCalories": null, "dailyLimit": 1800, "remaining": null, "overLimit": null },
  { "day": "2026-10-21", "consumedCalories": 1800, "dailyLimit": 1800, "remaining": 0, "overLimit": false },
  { "day": "2026-10-19", "consumedCalories": 2100, "dailyLimit": 2000, "remaining": -100, "overLimit": true }
]
```

`400`: `from` or `to` is missing or isn't `YYYY-MM-DD`, `from` is after `to`, the range is more than 366 Days, or `to` is after today.

## Calories Home Card

### `GET /api/calories/home-card`

`200`: today's Consumed Calories against the Daily Limit, with the fields above, today being the current Day in the Owner Time Zone.

```json
{ "day": "2026-10-22", "consumedCalories": 460, "dailyLimit": 2000, "remaining": 1540, "overLimit": false }
```

## Weigh-ins

A Weigh-in is the Owner's weight for one week, Monday to Sunday in the Owner Time Zone, in kg to 1 decimal. Each week has at most one, and a later one replaces it. A week is named by the Monday it starts on, as `YYYY-MM-DD`. Only this week's and last week's Weigh-ins can be logged, replaced or deleted, through the Tools. Older weeks are read-only.

### `GET /api/calories/weigh-ins`

`200`: every Weigh-in, newest week first.

```json
[
  { "week": "2026-10-19", "kg": 72.4 },
  { "week": "2026-10-12", "kg": 72.8 }
]
```

## Weight Home Card

### `GET /api/calories/weight-card`

`200`: what the Weight Home Card shows.

```json
{
  "latest": { "week": "2026-10-19", "kg": 72.4 },
  "bmi": 23.6,
  "change": -0.4
}
```

- `latest`: the newest week's Weigh-in, or `null` if there are none. Then `bmi` and `change` are `null` too.
- `bmi`: `latest`'s body mass index to 1 decimal, from the Owner's height in Core's settings, or `null` until the height is set. It is always derived, never stored.
- `change`: `latest`'s kg minus the kg of the week before it, or `null` if that week has no Weigh-in.

## Settings

When the Calories Notifications are sent. There is no Calories Settings Section: the Calories Assistant changes them with `calories_save_settings`.

### `GET /api/calories/settings`

`200`: the settings. Until the Owner first saves them, every Notification is off: the daily ones set for 14:00 and 21:00, the Weigh-in reminder for Monday at 08:00. The Weigh-in reminder the Owner had saved in Core before ADR 0012 was taken over at startup.

```json
{
  "noEntriesReminder": { "enabled": false, "time": "14:00" },
  "eveningSummary": { "enabled": false, "time": "21:00" },
  "weighInReminder": { "enabled": false, "weekday": "monday", "time": "08:00" }
}
```

- `noEntriesReminder`: the Notification reminding the Owner that today has no Entries yet.
- `eveningSummary`: the Notification telling the Owner how today went.
- `weighInReminder`: the weekly Notification reminding the Owner to weigh in. `weekday` is `"monday"` to `"sunday"`.
- `time`: `"HH:MM"` in the Owner Time Zone, on the hour or half hour (`"00:00"` to `"23:30"`).

## Notifications

Core's tick asks the Calories Module for its Notifications every 5 minutes, for the half hour it is in (see [Core's API](../core/API.md)). Each is sent only while switched on, at its `time` in the settings (the Weigh-in reminder only on its `weekday`), at most once a Day (the Weigh-in reminder once a week), titled `Calories`, in the Calories Assistant's chat in Telegram (Core's Messenger):

- **No-Entries reminder** (`noEntriesReminder`): `Nothing logged yet today`, only if today has no Entries at that time, and not if the evening summary is set for the same time (it says the same).
- **Evening summary** (`eveningSummary`): today's Consumed Calories against the Daily Limit today has: `Today: 1460 / 2000 kcal`, or `Today: 1460 kcal` if no Daily Limit was ever set. On an Unlogged Day it is `Nothing logged today`, never 0 kcal.
- **Weigh-in reminder** (`weighInReminder`): `Time to log this week's weight`. The Owner's reply is logged by the Calories Assistant.

## Tools

Core serves these to every Connector at `/mcp` (see [Core's API](../core/API.md)), for the Calories Assistant in Telegram ([ADR 0008](../../docs/adr/0008-assistants-on-telegram.md)) and the Claude app. Arguments and answers are JSON. A `day` is `YYYY-MM-DD` in the Owner Time Zone, and leaving it out means today. Only Days in the Editable Window can be changed. A refused call is a Tool error that says what is wrong, and changes nothing.

### The shapes they answer

- **Entry**, **Meal**, **Day view** and **Daily Limit**: as the routes above answer them.
- **Day**: the fields of a Day without its Entries, `{"day", "consumedCalories", "dailyLimit", "remaining", "overLimit"}`.
- **Weigh-ins**: every Weigh-in with the Weight Home Card's fields, `{"weighIns": [Weigh-in], "latest", "bmi", "change"}`, as `GET /api/calories/weigh-ins` and `GET /api/calories/weight-card` answer them.

### Look up (`readOnlyHint`)

| Tool | Arguments | What it does |
| --- | --- | --- |
| `calories_get_day` | `day` | Answers the Day view, as `GET /api/calories/days/{day}` does |
| `calories_get_daily_limit` | none | Answers `{"dailyLimit": Daily Limit}`, `null` if none was ever set |
| `calories_list_meals` | `archived` | The Meals in use, pinned ones first, then the most logged in the last 30 Days (today and the 29 before it, counting Entries, not Servings), then by name. With `archived: true`, the archived ones by name. Answers `{"meals": [Meal]}` |
| `calories_get_settings` | none | Answers the settings, as `GET /api/calories/settings` does |
| `calories_list_weigh_ins` | none | Answers Weigh-ins |

### Write

| Tool | Arguments | What it does |
| --- | --- | --- |
| `calories_log_entry` | `day`, `servings`, and `mealId` or a one-off's `name` and `kcalPerServing`; `saveAsMeal` | Logs an Entry: from a Meal, copying its `name` and `kcal`, or a one-off. `servings` is a whole number from 1 to 100; a one-off's `name` and `kcalPerServing` follow a Meal's rules. `saveAsMeal: true` also saves the one-off as a new Meal and logs the Entry from it; it can't come with `mealId`. An archived Meal can't be logged. Answers `{"entry": Entry, "day": Day}` |
| `calories_update_entry` | `id`; `name`, `kcalPerServing`, `servings` | Edits an Entry: only the values sent change, with the rules for logging. Its `day` and `mealId` can't change. Answers `{"entry": Entry, "day": Day}` |
| `calories_save_meal` | `id` to change one, or none to save a new one; `name`, `kcal`, `store`, `emoji`, `tags`, `pinned` | Saves a new Meal, which needs `name` and `kcal`, or changes one: only the fields sent change, and `tags` replaces all its Meal Tags. Entries already logged from it don't change. Answers the Meal |
| `calories_restore_meal` | `id` | Brings an archived Meal back into use. Answers the Meal |
| `calories_set_daily_limit` | `kcal` | Sets the Daily Limit from today, a whole number from 1 to 10,000. A second change on the same Day replaces the first, and earlier Days keep theirs. Answers `{"dailyLimit": Daily Limit}`, the one now in force |
| `calories_save_settings` | `noEntriesReminder`, `eveningSummary`, each `{enabled, time}`; `weighInReminder` `{enabled, weekday, time}` | Changes when the Calories Notifications are sent. Only the Notifications sent change, and within one, only the fields sent; at least one must come. `time` is on the hour or half hour. Answers the settings |
| `calories_log_weigh_in` | `kg`; `day` | Logs `kg` (more than 0 and at most 500, to 1 decimal) as the Weigh-in for the week `day` is in, replacing any it had. Only this week and last week. Answers Weigh-ins |

### Delete (`destructiveHint`)

The Calories Assistant asks the Owner before each of these.

| Tool | Arguments | What it does |
| --- | --- | --- |
| `calories_delete_entry` | `id` | Deletes the Entry, which can't be undone. Answers `{"day": Day}`, the Day without it |
| `calories_archive_meal` | `id` | Archives the Meal: it leaves the lists and can't be logged, its Entries stay, and `calories_restore_meal` brings it back. Answers the Meal |
| `calories_delete_weigh_in` | `day` | Deletes the Weigh-in for the week `day` is in, which can't be undone. Only this week and last week; a week with none also succeeds. Answers Weigh-ins |
