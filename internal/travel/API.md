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

## Fare Checks and Fares

A **Fare Check** is one pass in which the Travel Assistant searches for Fares: `daily` or `weekly`, which Nando asks for, or `asked`, by the Owner in the chat. The run starts one, records the Fares it finds in it, and finishes it.

```json
{
  "id": "6af05d493ea403a78bf5997c",
  "kind": "daily",
  "startedAt": "2026-11-22T12:00:00Z",
  "finishedAt": "2026-11-22T12:20:00Z",
  "sources": ["kiwi", "slicktrip"],
  "summary": "2 new lows",
  "errors": ["expedia timed out"]
}
```

A **Fare** is one priced round trip found for a Watching Trip and one of its Parties. Fares are kept as long as their Trip.

```json
{
  "id": "6af05d493ea403a78bf5997d",
  "tripId": "6af05d493ea403a78bf5997b",
  "party": "with_companion",
  "checkId": "6af05d493ea403a78bf5997c",
  "foundAt": "2026-11-22T12:05:00Z",
  "source": "kiwi",
  "departDate": "2027-05-12",
  "returnDate": "2027-05-19",
  "nights": 7,
  "outbound": {"flights": [{"carrier": "XX", "number": "XX1", "from": "AAA", "to": "BBB", "departs": "2027-05-12T10:00", "arrives": "2027-05-12T22:00"}], "stops": 0, "durationMinutes": 480},
  "inbound": {"flights": [{"carrier": "XX", "number": "XX2", "from": "BBB", "to": "AAA", "departs": "2027-05-19T10:00", "arrives": "2027-05-20T14:00"}], "stops": 0, "durationMinutes": 540},
  "separateTickets": false,
  "cabin": "economy",
  "bagsIncluded": true,
  "currency": "JPY",
  "total": 220461,
  "perPerson": 110231,
  "bookingUrl": "https://fares.example.com/1",
  "leaveNeeded": {"days": 5, "withRemoteDays": 3}
}
```

- `source`: `kiwi`, `expedia`, `skiplagged`, `slicktrip`, `price_alert` or `other`.
- `departDate`, `returnDate`: the departure must fall in one of the Trip's windows, the stay (`nights`, which Nando works out) in its range, and the whole trip clear of its `avoid` ranges.
- `stops`: each way, at most the Trip's `maxStops`. `cabin`: the Trip's or a better one.
- `separateTickets`: only on a Trip that welcomes them.
- `bagsIncluded`: whether `total` includes the Trip's checked bags, which not every source prices.
- `total`: JPY, more than 0, for the whole Party. Nando works out `perPerson`, rounded, from the Party's adults.
- `bookingUrl`: an `https://` link, or empty.
- `leaveNeeded`: needed on a `with_companion` Fare, and only there: the Companion's days off, and with the remote days the settings allow (`0 ≤ withRemoteDays ≤ days`). The Travel Assistant works it out from their calendars; Nando never reads them.

## Price Verdicts

A **Price Verdict** is Google's judgement of a Trip's Party's price on some dates, from Google Flights. Nando fetches it itself, through SerpApi, with a key that never leaves the server ([ADR 0013](../../docs/adr/0013-travel-watches-fares-through-the-travel-assistant.md)). Prices are JPY, for the whole Party.

```json
{
  "tripId": "6af05d493ea403a78bf5997b",
  "party": "owner",
  "departDate": "2027-05-12",
  "returnDate": "2027-05-19",
  "level": "low",
  "lowestPrice": 221000,
  "typicalRange": [240000, 300000],
  "history": [[1795000000, 250000], [1795172800, 221000]],
  "cheapestPrice": 221000,
  "cheapestAirlines": ["Air Z"],
  "fetchedAt": "2026-11-22T12:00:00Z"
}
```

- `level`: `low`, `typical` or `high`, as Google says. `history` is Google's price history, `[Unix seconds, price]`, its last 60 points.
- `cheapestPrice`, `cheapestAirlines`: the cheapest offer Google showed with it.
- Only a Watching Trip's verdicts are fetched, and only for dates leaving today or later.
- Asking again the same Day, in the Owner Time Zone, for the same Trip, Party and dates answers the verdict already fetched, without calling SerpApi, unless the Trip's airports, cabin or adults changed since.
- Each call to SerpApi counts toward the settings' `serpApiMonthlyLimit` for the calendar month, in the Owner Time Zone, answered or not, as SerpApi bills it. Past it, a new verdict is refused, saying when the month turns.
- Without `SERPAPI_KEY`, every new verdict is refused. Fares are still recorded.
- A call SerpApi refuses, fails, or answers without a verdict is refused, and nothing is kept.

## Tools

Each Tool answers JSON. A Tool refuses what it can't do with a message saying why, and changes nothing.

### Look up (`readOnlyHint`)

| Tool | Arguments | What it does |
| --- | --- | --- |
| `travel_get_settings` | none | Answers the settings |
| `travel_list_trips` | `status`: `watching` (when left out), `booked`, `dropped` or `all` | Answers `{trips}`, by name, ignoring case |
| `travel_get_trip` | `id` | Answers the Trip |
| `travel_list_price_verdicts` | `tripId`, `party` | Answers `{verdicts}`, the latest 20 fetched, newest first |
| `travel_fare_stats` | `tripId`, `party` | Answers what that Party's Fares say: `lowestEver`; `bestThisWeek` and `bestLastWeek`, by when they were found, weeks running Monday to Sunday in the Owner Time Zone (each `null` when there is none); `byWeek`, `[{week, fare}]`, the cheapest Fare departing in each week, by its Monday; and `top`, the 3 cheapest itineraries found in the last 7 days, each once however many checks found it |

### Write

| Tool | Arguments | What it does |
| --- | --- | --- |
| `travel_save_settings` | `home`; `companion` `{name, calendars, workWeekdays, holidayCalendar, usualLeaveDays, remoteDaysAbroad}`; `serpApiMonthlyLimit` | Changes the settings. Only the fields sent change, within `companion` too, and at least one must come. Refuses the whole save if any field is wrong. Answers the settings |
| `travel_save_trip` | `id`; any Trip field | Without `id`, creates a Watching Trip: `name`, `destinations`, `nights` and `windows` are needed, and the rest start as above. With `id`, changes only the fields sent. Refuses the whole save if the Trip it would make is wrong. Answers the Trip |
| `travel_start_fare_check` | `kind`: `daily`, `weekly` or `asked` | Starts a Fare Check. Answers `{id, lastFinished}`: when the last finished check of each kind finished, or `null` |
| `travel_record_fares` | `checkId`, `fares` | Records each Fare sent (without the fields Nando sets) in that unfinished Fare Check, or refuses it with why. Answers `{recorded: [{index, id}], refused: [{index, reason}]}`, by place in the list sent |
| `travel_finish_fare_check` | `id`; `sources`, `summary`, `errors` | Finishes the Fare Check, once. Answers it |
| `travel_get_price_verdict` | `tripId`, `party`, `departDate`, `returnDate` | Answers today's Price Verdict on those dates, fetching it from SerpApi when there is none yet |
| `travel_set_trip_status` | `id`, `status` | Sets the Trip to `watching`, `booked` or `dropped`, from today, changing nothing else. A Trip is watched again only while one of its windows isn't over. The Travel Assistant asks the Owner before dropping one. Answers the Trip |

### Delete (`destructiveHint`)

The Travel Assistant asks the Owner before this, and prefers `booked` or `dropped`.

| Tool | Arguments | What it does |
| --- | --- | --- |
| `travel_delete_trip` | `id` | Deletes the Trip for good, with its Fares and Price Verdicts. Answers `{deleted: id}` |
