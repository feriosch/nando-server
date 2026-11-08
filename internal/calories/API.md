# Calories API

The Calories Module: Meals, and the Entries that record what the Owner ate each Day. Every route is under `/api/calories/` and, like all of `/api/`, needs a valid session cookie (see [Core's API](../core/API.md)). Bodies are JSON. Errors are a status code with a short plain-text message.

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
