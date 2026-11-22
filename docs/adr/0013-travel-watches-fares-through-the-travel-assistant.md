# Travel watches fares through the Travel Assistant, with Google's Price Verdict fetched by Nando

The Owner wants to take some trips next year, and has no time to keep checking fare sites and price emails around work. So Nando gets a **Travel** Module, flights only for now, and a **Travel Assistant** with its own chat in the Messenger. The Assistant searches for Fares for each Watching Trip, judges them, and tells the Owner when one is worth acting on. Its searches use claude.ai connectors to fare sites. Nando keeps every Fare and fetches Google's **Price Verdict** itself through SerpApi. Nando still never calls a model, and the Owner always books.

## Decisions

- **Flights only.** Hotels are a later decision, once fares work. So are award seats: the Owner holds no miles worth searching.
- **Trips, Parties and the Companion are the Owner's data, held only in the Travel database.** The repo is public: its code, Tools, skills and docs never name a destination, a date, the Companion or their calendars. The Owner sets these up in the Travel chat, or in any Claude session with the Travel Tools, and the skill reads them from Nando at every run.
- **A Trip is a round trip from where the Owner lives**, with:
  - a range of nights, windows of dates and dates to avoid;
  - a cabin (economy by default), checked bags and how many stops it allows;
  - one or two **Parties**: the Owner alone, and the Owner with the **Companion**, watched side by side.
  - It is Watching until the Owner says it is Booked or Dropped. Only a Watching Trip is searched. The Assistant asks before dropping or deleting a Trip.
- **What a Fare must be:**
  - **Separate Tickets** are shown, always marked and beside the best Fare on one ticket. They never change airport at the connection and leave at least 3 hours between flights.
  - A better cabin is mentioned when it costs at most 1.3 times the cheapest economy Fare on the same dates (premium economy), or 1.5 times (business).
- **The Companion's calendar is read, never changed.** Through Travel's settings the Assistant knows:
  - which calendars are theirs and their work week (Monday to Friday, with public holidays off, by default);
  - how many days of leave one trip may usually take, and that they may work remotely from the destination for at most 2 days.
  - An all-day or multi-day event in their calendar blocks those dates for the Party with the Companion. An evening event doesn't block anything, and their own leave counts as days already off.
  - Every Fare for that Party shows its **Leave Needed**.
- **Where Fares come from:**
  - **Searches**: the Assistant uses the fare sites' claude.ai connectors (Kiwi.com first; Expedia, Skiplagged and SlickTrip once a run shows each works from the scheduled task), and records each Fare through a Travel Tool.
  - **Price Alerts**: the Owner turns on price tracking at sites that email it, such as Google Flights. The Core Assistant files those emails under `Travel Log/Alerts` in its Gmail Filing (ADR 0011). The Travel Assistant reads only that label. It never labels, archives or stars mail.
  - **Price Verdicts**: a Travel Tool asks SerpApi's Google Flights API for Google's verdict on a Fare's route and dates, and Nando records the answer. Nando makes this call itself, the way Japanese looks Words up on jisho.org: it is an outside service, not a model. The key stays in Secret Manager, never in a connector's URL, and Nando refuses calls past the monthly limit in Travel's settings, which starts at SerpApi's free 250 searches.
- **Fare Checks are asked for by Notifications that ask for work** (ADR 0009), in the Travel chat, answered by the Assistants' scheduled task (ADR 0008), as the Gmail Career Sync is (ADR 0010). There is no Travel scheduled task of its own.
  - The **Daily Fare Check** comes at a time in Travel's settings, after the Gmail Digest's, so the night's Price Alerts are filed first. It reads the new Price Alerts, searches again only where something moved, and fetches a Price Verdict for the best new Fares.
  - The **Weekly Fare Check**, on Sunday, searches every Watching Trip's whole window for each Party, and ends with the **Weekly Travel Summary**.
  - The Owner can also ask for either in the chat at any time.
- **How the Assistant judges a Fare** without a target from the Owner. A Fare is worth a **Fare Alert** when any of these hold, and the alert names which:
  - it is the lowest seen for its Trip and Party;
  - its Price Verdict is low;
  - it dropped sharply since last week's best.
  - After about a month of Fares, the Assistant suggests a **Target Fare** for each Trip and Party, which holds only once the Owner agrees.
- **How often it speaks:**
  - At most one Travel message a day, after the Daily Fare Check, with the day's Fare Alerts, plus the Weekly Travel Summary.
  - A Fare Alert that needs action within hours goes at once, at any hour. Examples: a sale that ends today, or few seats left at a Fare that is the lowest seen with a low Price Verdict.
  - A day with nothing worth saying stays quiet.
- **The Owner always books.** The Assistant never books, holds, pays, or enters the Owner's or the Companion's personal details anywhere. It may create price tracking on the Owner's behalf only where a connector offers it, such as SlickTrip's alerts, and says so.
- **The web app only reads Travel**, like Calories (#175):
  - a Travel screen per Trip, with the best Fare per person over time, the cheapest Fare by departure week, and the top Fares with their booking links;
  - one Home Card per Watching Trip, with its best Fare now and the trend.

## Considered Options

- **Nando searches fares itself, on its tick**: rejected. Few flight APIs are open to one person. Amadeus closed its self-service API in July 2026, Kiwi.com's and Skyscanner's are for partners, and Duffel wants a business. SerpApi alone is open, and its free 250 searches a month cover Price Verdicts but not whole windows for every Trip. The fare sites' connectors are free and already shaped for Claude.
- **Only Price Alerts, no searches**: rejected. An email says a price moved but not whether it is good. It covers only what the Owner set up at the site, and Google Flights' any-dates tracking prices a single traveller.
- **Only searches, no Price Alerts**: rejected. Sites and airlines announce sales that a weekly search would find late, and tracking costs nothing.
- **SerpApi through its own MCP server, from the run**: rejected. Its key would sit in the connector's URL in claude.ai, Nando couldn't hold the monthly limit, and the readings wouldn't be recorded unless the run remembered to.
- **A Travel scheduled task of its own, with its own trigger**: rejected for a Notification that asks for work. The run's answer is a message in the Travel chat anyway, and the Assistants' task already has Gmail and the calendar. A new task would be one more trigger, token and set-up by hand for the same thing.
- **Trips written into the skill**: rejected. The repo is public, and a Trip changes as the Owner's plans do.
- **The Companion's calendar decides the dates outright**: rejected. Most of their calendar says nothing about work, so the Assistant prefers dates with little Leave Needed and lets the Owner and the Companion choose.

## Consequences

- A new `travel` database, added to `scripts/backup.sh`. It holds the settings, Trips with their Parties, Fare Checks, Fares, Price Verdicts and Fare Alerts sent, so the Assistant never repeats itself and the summary can say what changed.
- Core's list of Assistants gains `travel`, with its own Telegram bot, `TELEGRAM_BOT_TOKEN_TRAVEL`, from Secret Manager. The SerpApi key, `SERPAPI_KEY`, comes from Secret Manager too, and without it Price Verdict calls are refused, but searches go on.
- The Assistants' scheduled task gains the fare sites' connectors, which every Assistant then has. Only the Travel skill uses them, as only its skill names them.
- The Core skill's `Travel Log` label gains its `Alerts` sub-label, for Price Alerts only.
- Fares a year ahead move slowly, and sites publish schedules only about 11 months out. A Fare missing from a search isn't sold out, and Trips far ahead show few Fares until their dates open.
- Each Fare Check uses some of the Owner's Claude usage. The weekly one is the larger, and how large is measured on the first runs.
