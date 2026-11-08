# Calories API

The Calories Module: Meals, the Entries that record what the Owner ate each Day, the Daily Limit each Day is judged against, and the Calories Home Card and Settings Section. Every route is under `/api/calories/` and, like all of `/api/`, needs a valid session cookie (see [Core's API](../core/API.md)). Bodies are JSON. Errors are a status code with a short plain-text message.

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
- `tags`: free-form labels, each 1 to 30 characters, at most 20. They are trimmed and repeats are dropped. A Tag never changes how a Meal behaves.
- `pinned`: pinned Meals lead the quick list.
- `archived`: read-only. `true` once the Meal is deleted.

### `GET /api/calories/meals`

`200`: the Meals in use, by name. With `?archived=true`, the archived ones instead.

### `GET /api/calories/meals/quick`

`200`: the Meals in use, for one-tap logging: pinned ones first, then the most-logged in the last 30 Days (today and the 29 before it, counting Entries, not Servings), then by name. Archived Meals aren't listed.

### `POST /api/calories/meals`

Body: a Meal without `id` and `archived`.

- `201`: `{ "id": "…" }`.
- `400`: the body isn't a Meal, or a field is invalid. The message says which.

### `PUT /api/calories/meals/{id}`

Body: the whole Meal, as for `POST`. An archived Meal can be edited too; it stays archived.

- `204`: saved. Entries already logged from it don't change.
- `400`: as for `POST`.
- `404`: there's no such Meal.

### `DELETE /api/calories/meals/{id}`

Archives the Meal: it leaves both lists and can't be logged, but its Entries stay, and it can be restored.

- `204`: archived (also if it already was).
- `404`: there's no such Meal.

### `POST /api/calories/meals/{id}/restore`

- `204`: the Meal is back in use (also if it wasn't archived).
- `404`: there's no such Meal.

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

Only Days in the **Editable Window**, today and the 7 Days before it, can be changed. Every Day can be read. Logging, editing or deleting an Entry on any other Day, including a future one, answers `409`.

### `GET /api/calories/days/{day}/entries`

`200`: the Day's Entries in the order they were logged. `[]` for an Unlogged Day.

`400`: `day` isn't `YYYY-MM-DD`.

### `POST /api/calories/days/{day}/entries`

Logs an Entry on `day`, either from a Meal:

```json
{ "mealId": "6af05d493ea403a78bf5997b", "servings": 2 }
```

which copies the Meal's `name` and `kcal`, or as a one-off:

```json
{ "name": "Sushi", "kcalPerServing": 45, "servings": 12, "saveAsMeal": false }
```

`saveAsMeal: true` also saves the one-off as a new Meal (with only its name and kcal) and logs the Entry from it. It can't be sent with `mealId`.

- `201`: `{ "id": "…" }`.
- `400`: `day` isn't `YYYY-MM-DD`; the body isn't an Entry; `servings` is missing or not a whole number from 1 to 100; the Meal doesn't exist or is archived; `saveAsMeal` was sent with `mealId`; or a one-off's `name` or `kcalPerServing` is missing or invalid (as for a Meal's `name` and `kcal`).
- `409`: `day` isn't in the Editable Window.

### `PUT /api/calories/entries/{id}`

Edits an Entry. Body: all three of its values:

```json
{ "name": "Sushi (salmon)", "kcalPerServing": 50, "servings": 10 }
```

Its `day` and `mealId` can't change.

- `204`: saved.
- `400`: a value is missing or invalid, as for logging.
- `404`: there's no such Entry.
- `409`: its Day isn't in the Editable Window.

### `DELETE /api/calories/entries/{id}`

- `204`: deleted. This is also the answer if there's no such Entry.
- `409`: its Day isn't in the Editable Window.

## Daily Limit

The Daily Limit is the most kcal the Owner intends to consume in a Day: a ceiling to stay under, not a goal to reach. Each change applies from the Day it is made (today), so past Days keep the limit they had. A second change on the same Day replaces the first. The very first Daily Limit also covers every Day before it. Until one is set, there is none, and no Day is over it.

### `GET /api/calories/daily-limit`

`200`: the Daily Limit in force now, or `null` if none was ever set.

```json
{ "kcal": 1800, "effectiveFrom": "2026-10-23" }
```

- `effectiveFrom`: the Day it was set, from which it applies.

### `PUT /api/calories/daily-limit`

Sets the Daily Limit from today.

```json
{ "kcal": 1800 }
```

- `204`: saved.
- `400`: `kcal` is missing or not a whole number from 1 to 10,000.

## Days

Consumed Calories are always derived from a Day's Entries. An **Unlogged Day**, one with no Entries, means "not recorded", not "ate nothing": its `consumedCalories` and `remaining` are `null`, never `0`, and it is never over or under the Daily Limit. A Day whose Entries add up to 0 kcal is logged, with `consumedCalories: 0`.

### `GET /api/calories/days/{day}`

`200`: the Day view.

```json
{
  "day": "2026-10-21",
  "entries": [ … ],
  "consumedCalories": 1000,
  "dailyLimit": 2000,
  "remaining": 1000
}
```

- `entries`: the Day's Entries in the order they were logged, as `GET /api/calories/days/{day}/entries` lists them.
- `consumedCalories`: the sum of the Entries' `kcal`, or `null` for an Unlogged Day.
- `dailyLimit`: the Daily Limit the Day had, or `null` if none was ever set.
- `remaining`: `dailyLimit − consumedCalories`, negative once over; `null` for an Unlogged Day or without a Daily Limit.

`400`: `day` isn't `YYYY-MM-DD`.

### `GET /api/calories/days/{day}/preview?kcal={kcal}`

What logging an Entry of `kcal` on `day` would make of it, so the web app can warn before it goes over the Daily Limit. Nothing is logged.

```json
{ "consumedCalories": 2001, "dailyLimit": 2000, "remaining": -1, "overLimit": true }
```

- `consumedCalories`: the Day's Consumed Calories with the Entry (just its `kcal` on an Unlogged Day).
- `dailyLimit`, `remaining`: as in the Day view, with the Entry. `remaining` is `null` only without a Daily Limit.
- `overLimit`: `true` if `consumedCalories` would be more than `dailyLimit`. Reaching it exactly isn't over. Always `false` without a Daily Limit.

`400`: `day` isn't `YYYY-MM-DD`, or `kcal` is missing or not a whole number from 0 to 1,000,000 (the most one Entry can have: 10,000 kcal per serving × 100 Servings).

### `GET /api/calories/history?from={day}&to={day}`

`200`: every Day from `from` to `to`, inclusive, newest first, with its Consumed Calories against the Daily Limit it had.

```json
[
  { "day": "2026-10-22", "consumedCalories": null, "dailyLimit": 1800, "overLimit": null },
  { "day": "2026-10-21", "consumedCalories": 1800, "dailyLimit": 1800, "overLimit": false },
  { "day": "2026-10-19", "consumedCalories": 2100, "dailyLimit": 2000, "overLimit": true }
]
```

- `consumedCalories`: `null` for an Unlogged Day.
- `dailyLimit`: `null` if none was ever set.
- `overLimit`: whether the Day went over its Daily Limit; `false` without one, and `null` for an Unlogged Day.

`400`: `from` or `to` is missing or isn't `YYYY-MM-DD`, `from` is after `to`, or the range is more than 366 Days.

## Calories Home Card

### `GET /api/calories/home-card`

`200`: today's Consumed Calories against the Daily Limit, today being the current Day in the Owner Time Zone. The fields are as in the Day view.

```json
{ "day": "2026-10-22", "consumedCalories": 460, "dailyLimit": 2000, "remaining": 1540 }
```

## Settings

The Calories Settings Section: when its Notifications are sent. The Daily Limit is set on its own route, above. Sending the Notifications themselves is described with them.

### `GET /api/calories/settings`

`200`: the settings. Until the Owner first saves them, both Notifications are off, set for 14:00 and 21:00.

```json
{
  "noEntriesReminder": { "enabled": false, "time": "14:00" },
  "eveningSummary": { "enabled": false, "time": "21:00" }
}
```

- `noEntriesReminder`: the Notification reminding the Owner that today has no Entries yet.
- `eveningSummary`: the Notification telling the Owner how today went.
- `time`: `"HH:MM"` in the Owner Time Zone, on the hour or half hour (`"00:00"` to `"23:30"`).

### `PUT /api/calories/settings`

Body: all of the settings, as `GET` answers them.

- `204`: saved.
- `400`: the body isn't settings, one of them is missing, or a `time` isn't on the hour or half hour. The message says which.
