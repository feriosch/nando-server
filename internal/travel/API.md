# Travel API

The Travel Module watches flight prices for the Owner's Trips, through the Travel Assistant ([ADR 0013](../../docs/adr/0013-travel-watches-fares-through-the-travel-assistant.md)). The Travel Assistant reads and writes everything through the Module's [Tools](#tools), over the Owner's Connector. There are no routes under `/api/travel/` yet: the web app's read-only routes come with #187.

The repo is public, so nothing here names a real Trip, place, date or person. Those are the Owner's data, held only in the `travel` database.

## Settings

One document, the Owner's own:

```json
{
  "home": ["AAA"],
  "companion": {
    "name": "Sam",
    "calendars": ["sam@example.com"],
    "workWeekdays": ["monday", "tuesday", "wednesday", "thursday", "friday"],
    "holidayCalendar": "holidays@example.com",
    "usualLeaveDays": 7,
    "remoteDaysAbroad": 2
  },
  "serpApiMonthlyLimit": 250
}
```

- `home`: the airport or city codes the Owner's Trips leave from unless a Trip says otherwise. Each is 3 capital letters, such as an IATA airport or city code, and none comes twice. Empty until set.
- `companion`: who travels with the Owner in a Party with the Companion. The Travel Assistant reads their `calendars` and `holidayCalendar` (Google Calendar IDs) to work out Leave Needed, and never changes them. There is no Companion while `name` is empty, as it is until set.
  - `workWeekdays`: the weekdays they work, each once: Monday to Friday until changed.
  - `usualLeaveDays`: how many days off one trip may usually take, 0 to 30: 7 until changed.
  - `remoteDaysAbroad`: how many workdays they may work remotely from the destination, 0 to 2: 2 until changed.
- `serpApiMonthlyLimit`: the most Price Verdicts Nando fetches from SerpApi in a calendar month, 0 to 10,000: SerpApi's free 250 until changed.

## Trips

A Trip is a journey the Owner may take: a round trip from `origins` to `destinations`, watched until the Owner books it or drops it. Trips are the Owner's own data, set up through the Tools below.

```json
{
  "id": "6af05d493ea403a78bf5997b",
  "name": "Beach week",
  "origins": ["AAA"],
  "destinations": ["BBB"],
  "nights": {"min": 6, "max": 9},
  "windows": [{"from": "2027-05-10", "to": "2027-06-30", "note": ""}],
  "avoid": [{"from": "2027-05-29", "to": "2027-06-06", "reason": "a wedding"}],
  "cabin": "economy",
  "checkedBags": 1,
  "maxStops": 1,
  "separateTickets": true,
  "parties": [{"key": "owner", "adults": 1}, {"key": "with_companion", "adults": 2}],
  "status": "watching",
  "statusChangedOn": "2026-11-22",
  "notes": ""
}
```

- `name`: 1 to 100 characters, trimmed.
- `origins`, `destinations`: codes as in the settings' `home`, at least one each. `origins` is the settings' `home` unless sent; a new Trip needs one or the other.
- `nights`: the stay, `1 ≤ min ≤ max ≤ 60`.
- `windows`: at least one range of departure dates, `from` to `to` included, written `YYYY-MM-DD`. A window sent must not be over before today.
- `avoid`: ranges of dates the trip must not touch, which must leave a Day to depart on in every window, alone or together.
- `cabin`: `economy` (until changed), `premium_economy`, `business` or `first`.
- `checkedBags`: per person, 0 to 2: 1 until changed. `maxStops`: each way, 0 to 3: 1 until changed.
- `separateTickets`: whether Fares on Separate Tickets are welcome: `true` until changed.
- `parties`: one or two, each once: `owner`, the Owner alone as 1 adult (the only Party until changed), and `with_companion`, 2 to 9 adults, which needs a Companion in the settings when `parties` is sent.
- `status`: `watching`, `booked` or `dropped`, since `statusChangedOn`, a Day in the Owner Time Zone. Only a Watching Trip is searched.
- `notes`: free text the Travel Assistant keeps for itself.

## Tools

Each Tool answers JSON. A Tool refuses what it can't do with a message saying why, and changes nothing.

### Look up (`readOnlyHint`)

| Tool | Arguments | What it does |
| --- | --- | --- |
| `travel_get_settings` | none | Answers the settings |
| `travel_list_trips` | `status`: `watching` (when left out), `booked`, `dropped` or `all` | Answers `{trips}`, by name, ignoring case |
| `travel_get_trip` | `id` | Answers the Trip |

### Write

| Tool | Arguments | What it does |
| --- | --- | --- |
| `travel_save_settings` | `home`; `companion` `{name, calendars, workWeekdays, holidayCalendar, usualLeaveDays, remoteDaysAbroad}`; `serpApiMonthlyLimit` | Changes the settings. Only the fields sent change, within `companion` too, and at least one must come. Refuses the whole save if any field is wrong. Answers the settings |
| `travel_save_trip` | `id`; any Trip field | Without `id`, creates a Watching Trip: `name`, `destinations`, `nights` and `windows` are needed, and the rest start as above. With `id`, changes only the fields sent. Refuses the whole save if the Trip it would make is wrong. Answers the Trip |
| `travel_set_trip_status` | `id`, `status` | Sets the Trip to `watching`, `booked` or `dropped`, from today, changing nothing else. A Trip is watched again only while one of its windows isn't over. The Travel Assistant asks the Owner before dropping one. Answers the Trip |

### Delete (`destructiveHint`)

The Travel Assistant asks the Owner before this, and prefers `booked` or `dropped`.

| Tool | Arguments | What it does |
| --- | --- | --- |
| `travel_delete_trip` | `id` | Deletes the Trip for good. Answers `{deleted: id}` |
