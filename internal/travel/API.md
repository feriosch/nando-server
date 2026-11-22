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

## Tools

Each Tool answers JSON. A Tool refuses what it can't do with a message saying why, and changes nothing.

### Look up (`readOnlyHint`)

| Tool | Arguments | What it does |
| --- | --- | --- |
| `travel_get_settings` | none | Answers the settings |

### Write

| Tool | Arguments | What it does |
| --- | --- | --- |
| `travel_save_settings` | `home`; `companion` `{name, calendars, workWeekdays, holidayCalendar, usualLeaveDays, remoteDaysAbroad}`; `serpApiMonthlyLimit` | Changes the settings. Only the fields sent change, within `companion` too, and at least one must come. Refuses the whole save if any field is wrong. Answers the settings |
