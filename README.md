# nando-server

Go API for Nando. Domain language: [`GLOSSARY.md`](GLOSSARY.md). Decisions: [`docs/adr/`](docs/adr/).

## Layout

- `cmd/nando-server`: the program: reads config, connects to MongoDB, wires Core and Modules, serves HTTP.
- `internal/server`: the HTTP API (routes, CORS) and `server.RegisterDatabases`, which opens the MongoDB connection and binds it as singletons in a [golobby container](https://github.com/golobby/container): the `*mongo.Client` and one named `*mongo.Database` each for Core and every Module (`core`, `calories`, `japanese`, `career`, `travel`, `money`). `main` fills a `server.Databases` from the container and hands each area its own database.
- `internal/core`: Core, wired in `core.go`. Its HTTP API is documented in [`internal/core/API.md`](internal/core/API.md).
- `internal/calories`: the Calories Module, wired in `calories.go`. Its HTTP API is documented in [`internal/calories/API.md`](internal/calories/API.md).
- `internal/japanese`: the Japanese Module, wired in `japanese.go`. Its HTTP API, read-only for Words and Kanji but where the Owner writes Word Requests, and the Tools through which Claude adds and improves Words are documented in [`internal/japanese/API.md`](internal/japanese/API.md). `japanese.Config` names the outside services its Tools look Words up on, jisho.org and kanjiapi.dev, and the API trigger of the cloud scheduled task it wakes Claude with, with the trigger's bearer token; `main` passes the defaults and the trigger from `WORD_REQUESTS_TRIGGER_URL` and `WORD_REQUESTS_TRIGGER_TOKEN`, and tests set fakes. `internal/japanese/anki` documents the Anki note type its JLPT deck file fills: its card templates, styling and fields. Its database holds the Words and Kanji moved from benkyo (#36): `words`, `kanji`, `irregular_components`, `groups` and `clauses`, which has no API. `words` has a unique index on spelling and reading: one Word per spelling and reading. It also holds the Owner's `word_requests` ([ADR 0006](docs/adr/0006-word-requests-through-a-scheduled-task.md)), with a unique index on what each asks for, limited to the open ones: at most one open Word Request per word to add or Word to improve, and `scheduled_task`, one document holding when Nando last fired the trigger, shared by every instance.
- `internal/career`: the Career Module, wired in `career.go`. Its HTTP API, read-only for the app (the Home Card, the Career screen, Late Replies and the Recruiter and Opportunity pages) but where the Owner sets whether they are Looking and marks a Suggested Reply replied, and the Tools through which the LinkedIn Sync script on the Owner's MacBook hands Nando new LinkedIn messages ([ADR 0007](docs/adr/0007-linkedin-synced-from-the-owners-mac.md)) are documented in [`internal/career/API.md`](internal/career/API.md). It also has the Tools the cloud run that a LinkedIn Sync wakes, and the Owner's chat, work with: sorting Conversations, Opportunities, Companies, Suggested Replies and the Career Profile. `career.Config` is the API trigger of the cloud scheduled task it wakes Claude with, and its bearer token; `main` passes them from `CAREER_SYNC_TRIGGER_URL` and `CAREER_SYNC_TRIGGER_TOKEN`. Its database holds `settings`, one document; `syncs`, one per LinkedIn Sync; `conversations`, one per LinkedIn thread with a unique index on its thread ID; `recruiters`; `companies`; `opportunities`, with their Steps; `profile`, one document; `work`, the run's Items, with a unique index on Conversation and kind limited to the open ones; and `scheduled_task`, as Japanese's.
- `internal/travel`: the Travel Module, wired in `travel.go`, which watches flight prices for the Owner's Trips through the Travel Assistant ([ADR 0013](docs/adr/0013-travel-watches-fares-through-the-travel-assistant.md)). Its Tools, documented in [`internal/travel/API.md`](internal/travel/API.md), are how the Travel Assistant reads and changes it; the web app only reads it, through the routes its `OverviewService` (`overview.go`) serves: the list of Trips, a Trip's page, the Home Cards and the settings, reusing the `FaresService`'s stats. Its `TripsService` (`trips.go`) asks its `SettingsService` for where Trips leave from by default, and deletes a deleted Trip's Fares; its `FaresService` (`fares.go`) runs Fare Checks, checks each Fare against its Trip, and works out what a Trip's Fares say; its `PriceVerdictsService` (`price_verdicts.go`) fetches Google's Price Verdicts from SerpApi itself, at most `serpApiMonthlyLimit` calls a month, with the key `main` passes from `SERPAPI_KEY`. Its `NotificationsService` (`notifications.go`) is the Module's Notification provider, which `main` adds to Core's tick: it asks the Travel Assistant for the Daily and Weekly Fare Checks at their times in the settings, while a Trip is Watching. The repo is public, so Trips and the Companion are the Owner's data, held only in its database, and no code, Tool, skill or doc names them. Its database holds `settings`, one document, `trips`, the Owner's Trips with their Parties; `fare_checks`, one per Fare Check; `fares`, with an index on Trip, Party and price; `price_verdicts`; `serpapi_calls`, one per call to SerpApi, which bills each; and `fare_alerts`, the Fare Alerts sent.
- `internal/money`: the Money Module, wired in `money.go`, which keeps the Owner's money across Japan and Mexico from the Statements the Owner sends the Money Assistant ([ADR 0014](docs/adr/0014-money-reads-statements-the-owner-sends.md)). Its Tools, documented in [`internal/money/API.md`](internal/money/API.md), are how the Money Assistant reads and changes it. `money.Config` names the Cloud Storage bucket Statements' files are kept in, which `main` passes from `MONEY_STATEMENTS_BUCKET`. The repo is public, so Accounts, Merchants and amounts are the Owner's data, held only in its database, and no code, Tool, skill or doc names them. Its `AccountsService` (`accounts.go`) keeps the Accounts, and its `StatementsService` (`statements.go`) the Statements with their Transactions, inside each Statement's document so that saving or replacing one is a single write, and whether a month is Complete. Its `CategoriesService` (`categories.go`) serves the fixed Categories and the Subcategories that grow in them, and merges Subcategories through every repository that points at them; its `MerchantsService` (`merchants.go`) the Merchants Claude learns; and its `SubscriptionsService` (`subscriptions.go`) the Subscriptions, asking Core for today to say which are late. Its database holds `settings`, one document; `accounts`; `statements`, with a unique index on Account and period; `subcategories`; `merchants`; and `subscriptions`.
- `internal/testmongo`: test helper that starts a real MongoDB container (`URI`, or `Start` for a test that needs the container itself).
- `internal/testpasskey`: test helper, a software Passkey, so tests can sign in.
- `internal/testtrigger`: test helper, a fake API trigger of a cloud scheduled task that records each fire, so a Module's tests can see when Nando wakes Claude.
- `internal/testconnector`: test helper, a Connector the Owner approves with a Passkey, connected to `/mcp` with the official MCP client, so a Module's tests can call its Tools as Claude does.
- `skills/`: the skills Claude follows to work with the Owner's data through the Nando Connector, such as adding a Japanese Word (`skills/japanese/add-word`). [`skills/README.md`](skills/README.md) says how to use them in the Claude app and in Claude Code, and how to add the Connector.
- `.claude-plugin/`: makes this repo a Claude Code plugin marketplace (`marketplace.json`) holding the `nando` plugin (`plugin.json`), which carries the Career chat skills. [`skills/README.md`](skills/README.md) says how to install it.
- `scripts/backup.sh`: the weekly backup's dump (see [Backups](#backups)). `scripts/backup_test.go` runs it inside `mongo:8` and restores what it wrote with the commands below.
- `scripts/career-sync/`: the script on the Owner's MacBook that runs each LinkedIn Sync, reading LinkedIn and handing Nando the new messages, checking every 15 minutes from `launchd` ([ADR 0007](docs/adr/0007-linkedin-synced-from-the-owners-mac.md)). [`scripts/career-sync/README.md`](scripts/career-sync/README.md) says how it works, how to install it and what was learned about the LinkedIn reader it uses. Its tests are Python: `uv run --project scripts/career-sync pytest scripts/career-sync`.

Core and every Module are split into the same three layers:

- `model/`: the area's data and one repository interface per collection, declared next to the data it stores (`MealsRepository` in `calories/model/meals.go`, `EntriesRepository` in `entries.go`). Core's also declares `CoreService` (`core/model/core_service.go`). Core and every Module also declare their JSON shapes there, exported: what their API answers with and is sent, and what Core's push messages carry. Each goes next to the data it shows (`MealJSON` in `calories/model/meals.go`, `WordJSON` in `japanese/model/words.go`, `SettingsJSON` in `core/model/settings.go`, `PushMessageJSON` in `core/model/notifications.go`), or in a file of its own when it shows no single collection (`DayJSON` in `calories/model/days.go`, `SearchJSON` in `japanese/model/search.go`, `CreatedJSON` in `core/model/created.go`). `service/` declares none: a request body decoded inline in a handler may stay unnamed there, but no handler answers with an unnamed struct. Tests never decode into these types: each declares its own copy, so a changed `json` tag fails a test: see [JSON shapes are the API](#json-shapes-are-the-api). Japanese's `AnkiWord` (`anki.go`) is a Word as the JLPT deck file reads it, kept apart from `Word` because the deck depends on details of how benkyo stored Words that nothing else shows.
- `service/`: request handling and validation, one service per collection it serves, each in the file named after it with its own routes (`MealsService` in `calories/service/meals.go`, `EntriesService` in `entries.go`). A service holds only the repositories it uses, which may include another collection's, such as the Entries one for the quick list's counts. A service may also ask another service of its area, as Core's Gmail Filings service asks its Settings service for today's Day. Depends only on `model/` interfaces; never touches the database. Core's services that serve no single collection are named for what they do: `IdentityService` (`identity.go`) checks it is the Owner with a Passkey under `/auth/`, with the Enrollment Codes, Ceremonies and Passkeys repositories: to sign in, then having the `SessionsService` start a Session, or to approve a Connector, then having the `OAuthService` create it; `OAuthService` (`oauth.go`) is Nando's own OAuth authorization server for Connectors, with the OAuth Clients, Authorization Requests, Authorization Codes and Connectors repositories; `MCPService` (`mcp.go`) serves every Tool provider's Tools at `/mcp` to Connectors, checking their access tokens with the Connectors repository, and keeps what each provider says its Tools let a Connector do, which the `OAuthService` gives the consent page; `NotificationsService` (`notifications.go`) runs the tick, recording which Notifications were sent and sending them through the `MessengerService`, in Telegram (ADR 0008). Calories' `DaysService` (`days.go`) answers the Day view, the history and the Calories Home Card from Entries and Daily Limits. Calories' `NotificationsService` (`notifications.go`) is the Module's Notification provider, which `main` adds to Core's tick: it holds no repository, asking the `SettingsService` when the no-Entries reminder, the evening summary and the weekly Weigh-in reminder are due and the `DaysService` how today went. Calories' `WeighInsService` (`weigh_ins.go`) serves the Weigh-ins and the Weight Home Card, asking Core for the Owner's height for the BMI. Japanese's `SearchService` (`search.go`) serves no single collection either: it answers the search screen from Words and Kanji in one ranked response, so it sits on its own rather than in the Words or Kanji service. `WordsService` (`words.go`) answers the Word page, also reading Groups and Kanji, and `KanjiService` (`kanji.go`) the Kanji page, also reading irregular Components and Words. `AnkiService` (`anki.go`) exports Words as Anki deck files, asking Core for today's Day to date them, so it too is named for what it does, and `LookupService` (`lookup.go`) looks Words and Kanji up outside Nando, on jisho.org and kanjiapi.dev. `WordRequestsService` (`word_requests.go`) serves the Owner's Word Requests, also reading Words to check the one to improve, asks Core's `utils.ScheduledTask` to wake Claude when one becomes pending or the Owner asks again for one still open, and is also the Module's Notification provider, which `main` adds to Core's tick: a Notification for each Word Request that needs an answer or has failed. That `ScheduledTask` (`core/utils/scheduled_task.go`), shared with Career, fires the scheduled task's API trigger, reading the Word Requests (`WordRequestsQueue`) to see whether any wait for a run and recording each fire in Japanese's own `scheduled_task` repository, beside when a run last claimed or resolved a Word Request and when one last found nothing left and stopped, which the `WordRequestsService` tells it, so that it doesn't wake a second run while one is starting or working; the Module's tick job asks it too. Career's `OverviewService` (`overview.go`) serves no single collection either: it answers the Home Card, the Career screen and Late Replies from Conversations' Suggested Replies, Recruiters and the top Opportunities, which it asks the `OpportunitiesService` for. Career's `TickRulesService` (`tick_rules.go`) applies at each tick the rules that need no model, reading Opportunities and Conversations and queueing the run's Items, and its `NotificationsService` (`notifications.go`) is the Module's Notification provider, which `main` adds to Core's tick, reading Conversations, Opportunities, Companies, LinkedIn Syncs and Items, and asking the `SettingsService` whether the Owner is Looking; `career.go`'s tick job runs the rules, then the `WorkService`'s tick work. Each Japanese and Calories service also offers the Tools for its own data (`WordsService` the Word, Word Link and Hint Tools; Calories' `EntriesService` the Entry Tools, refusing with a `utils.Refusal` whose message the Tool answers; Calories writes only through its Tools, as the web app only reads it), and `Module.ToolProvider` gathers them for Core.
- `repository/`: all database code, one repository per collection, over the area's own `*mongo.Database`. Each file holds one collection's repository and all the code for it, including its constructor and indexes (e.g. `calories/repository/meals.go` → `meals`, `entries.go` → `entries`). Turning MongoDB's "nothing found" into `ErrNotFound` is shared through Core's `core/utils/errors.go` (`utils.NotFound`, `utils.Matched`, `utils.Deleted`); a Module checks for Core's `utils.ErrNotFound` rather than declaring its own. A method that reads more than one collection belongs to the repository of the collection it is mainly about, as with a SQL join: counting Entries per Meal is the Entries repository's.

No layer has a file for what its files share (no `model.go`, `service.go` or `repository.go`): that goes in `utils/`. Core and a Module may have one: helpers their own layers share. Core's are also for Modules to use. Examples: Core's `ErrNotFound` and MongoDB error helpers (`core/utils/errors.go`), the response helpers every part of the API answers with (`core/utils/responses.go`), the expiry index and filter for documents that expire (`core/utils/expiry.go`), the half-hour clock times Notifications can be set for, the step the tick sends Notifications in, and whether a tick is at one (`core/utils/clock_times.go`), waking a Module's cloud scheduled task: the trigger's client and the wake rules every Module follows, with the filter its `scheduled_task` repository claims a fire with (`core/utils/scheduled_task.go`), Calories' checks on what the Owner sends and their limits (`calories/utils/checks.go`), the request and JSON helpers Calories' services share, such as reading the Day in a path and writing Entries (`calories/utils/requests.go`, `calories/utils/json.go`), the Editable Window rule (`calories/utils/editable_window.go`), Career's JSON conversions, id checks for Tools and the 404 for an unknown id in a route's path, name searches and the ranking of the top Opportunities with their next Step (`career/utils/json.go`, `ids.go`, `search.go`, `ranking.go`), and Japanese's search pattern that ignores case and accents, with the query parts that rank matches (`japanese/utils/search.go`). A `utils/` package runs no queries and handles no requests, though it may build a query's parts or write a response.

Core's and every Module's tests live in their `tests/` folder (`core/tests/settings_test.go`, `calories/tests/meals_test.go`). They use only what Core and the Module export, through the HTTP API, so they don't need to sit next to the code. Japanese and Career have exceptions, for files the Module ships outside its code. Japanese's `anki_note_type_test.go` also reads the note type documented in `japanese/anki`, to check its card templates and README against the fields the JLPT deck file serves. Each Module's `skills_test.go` also reads its skills in `skills/japanese` or `skills/career`, to check every Tool they name is one Nando serves; Career's also reads `.claude-plugin/`, to check the `nando` plugin carries exactly the chat skills.

A file about something there can be many of is named in the plural in every layer and in tests, matching its collection: `model/meals.go`, `service/meals.go`, `repository/meals.go`, `meals_test.go`. Names with no plural form, such as `settings.go` or `identity.go`, stay as they are.

The one difference: Core also hands every Module's service a `model.CoreService`, answered by Core's Settings service (`OwnerTimeZone`, `Today`, `HeightCm`) through a small type in `core.go`, which also gives Core's clock (`Now`), so that tests move one clock for Core and the Module. When a Module needs Core's data (such as the Owner's height), its service asks it and then uses that data itself or passes it to its own repository. A Module never receives Core's repository, nor another Module's service, repository or database. Reaching another database from a repository (for example via `db.Client()`) is possible with the driver, so this rule is kept by convention.

Core and every Module may also supply Core with providers, which `main` adds once, at startup, before serving: a Notification provider (`model.NotificationProvider`, added with `AddNotificationProvider`), which Core asks at every tick for the Notifications due, a tick job (`model.TickJob`, added with `AddTickJob`), which Core runs at every tick after sending the Notifications, without knowing what it does (Japanese's wakes Claude for Word Requests still waiting), and a Tool provider (`model.ToolProvider`, added with `AddToolProvider`), whose `Tools()` Core serves to every Connector at `/mcp`, and whose `ToolsDescription()` says what they let a Connector do, worded to follow "It will be able to" (Japanese's: "search your Words and Kanji, add, change, merge and delete your Words, and work through your Word Requests"), which the consent page shows the Owner before they approve one. A `model.Tool` is a name, a description, a JSON Schema for its arguments, whether it is read-only or destructive, and a handler that takes the arguments as JSON and returns a result Core sends back as JSON. Core serves Tools without knowing what they do. A Module's Tool names start with the Module's name (`japanese_search`). `AddToolProvider` panics at startup on a provider with Tools but no description of them, and on a Tool it can't serve: one without a name, a handler or an object schema, or whose name another Tool already has.

Imports follow the same direction. A Module may import what Core exports, such as Core's `model` types and Core's `utils` (`utils.ErrNotFound`, and the response helpers `utils.WriteJSON` and `utils.ServerError`, so every part of the API answers alike). Core never imports a Module, and a Module never imports another Module. Core's data still reaches a Module only through `model.CoreService`, never by calling Core's services or repositories directly. `internal/imports_test.go` fails the build if Core imports a Module, a Module imports another Module, or a Module imports Core's repositories or services. It treats every folder under `internal/` as a Module except `core`, `server` and the `test…` helpers.

## Run locally

Requires Go 1.25+, Docker, and poppler's programs (`pdfinfo`, `pdftotext`, `pdftoppm`, from `poppler-utils` on Debian or Ubuntu, `poppler` on Homebrew), with which Assistants read PDFs.

```sh
docker compose up -d   # MongoDB on localhost:27017

MONGO_URI=mongodb://localhost:27017 \
DOMAIN=localhost \
ALLOWED_ORIGINS=http://localhost:5173 \
TICK_AUDIENCE=http://localhost:8080 \
SCHEDULER_SERVICE_ACCOUNT=none \
go run ./cmd/nando-server

curl localhost:8080/health
```

Notifications go only through the Messenger's Telegram bots (ADR 0008). Without a `TELEGRAM_BOT_TOKEN_*`, none are sent: each tick logs which were due, and records none as sent.

| Env var           | Meaning |
| ----------------- | --- |
| `MONGO_URI`       | MongoDB connection string (required)                             |
| `DOMAIN`          | Parent domain of `web.` and `api.`, such as `nanders.app` (required): the Passkeys' relying-party ID and the Session cookie's domain. Changing it means enrolling again. |
| `ALLOWED_ORIGINS` | Comma-separated web app origins (required), each exactly `scheme://host[:port]` (no trailing slash, no `*`). They may call the API with credentials and are the only origins Passkeys work from ([ADR 0002](docs/adr/0002-passkey-only-identity.md)), so adding one also lets it sign in. |
| `PORT`            | Port to listen on (default `8080`; Cloud Run sets it)            |
| `TICK_AUDIENCE`   | The audience the Cloud Scheduler job's OIDC token is for (required to serve), exactly as the job sets it: `https://api.nanders.app` in production. |
| `SCHEDULER_SERVICE_ACCOUNT` | The email of the service account Cloud Scheduler signs its OIDC token as (required to serve). `/internal/tick` accepts no other caller. |
| `CAREER_SYNC_TRIGGER_URL`, `CAREER_SYNC_TRIGGER_TOKEN` | The API trigger (`…/fire`) of the cloud scheduled task that works through what LinkedIn Syncs bring, and its bearer token ([ADR 0007](docs/adr/0007-linkedin-synced-from-the-owners-mac.md)). Optional: without both, Nando never wakes Claude for them. In production the URL is the repo variable `CAREER_SYNC_TRIGGER_URL` and the token comes from Secret Manager (`nandoproject-career-sync-trigger-token`). |
| `WORD_REQUESTS_TRIGGER_URL`, `WORD_REQUESTS_TRIGGER_TOKEN` | The API trigger (`…/fire`) of the cloud scheduled task that works through Word Requests, and its bearer token ([ADR 0006](docs/adr/0006-word-requests-through-a-scheduled-task.md)). Optional: without both, Nando never wakes Claude. In production the URL is the repo variable `WORD_REQUESTS_TRIGGER_URL` and the token comes from Secret Manager (`nandoproject-word-requests-trigger-token`). |
| `TELEGRAM_BOT_TOKEN_CORE`, `TELEGRAM_BOT_TOKEN_JAPANESE`, `TELEGRAM_BOT_TOKEN_CAREER`, `TELEGRAM_BOT_TOKEN_CALORIES`, `TELEGRAM_BOT_TOKEN_TRAVEL`, `TELEGRAM_BOT_TOKEN_MONEY` | Each Assistant's Telegram bot token, from @BotFather ([ADR 0008](docs/adr/0008-assistants-on-telegram.md)). Optional: an Assistant without one is off, and with none at all the Messenger is off. In production each comes from Secret Manager (`nandoproject-telegram-bot-token-core`, `-career`, `-japanese`, `-calories`, `-travel`, `-money`). |
| `TELEGRAM_OWNER_ID`, `TELEGRAM_WEBHOOK_SECRET` | The Owner's numeric Telegram user ID, whose messages are the only ones kept, and the secret every bot's webhook is set with (1 to 256 letters, digits, `_` or `-`). Both are required with any bot token. In production the ID is the repo variable `TELEGRAM_OWNER_ID` and the secret comes from Secret Manager (`nandoproject-telegram-webhook-secret`). |
| `SERPAPI_KEY` | The key of the SerpApi account Travel fetches Price Verdicts with, from Google Flights ([ADR 0013](docs/adr/0013-travel-watches-fares-through-the-travel-assistant.md)). Optional: without it, Price Verdicts are refused and everything else works. It never reaches a Connector, a log or a Tool's answer. In production it comes from Secret Manager (`nandoproject-serpapi-key`). |
| `MONEY_STATEMENTS_BUCKET` | The Cloud Storage bucket, without `gs://`, where Money keeps every Statement's file for good ([ADR 0014](docs/adr/0014-money-reads-statements-the-owner-sends.md)). Nando's service account may add and read files there, never delete them. Optional: without it, Statements are kept without their files. In production it is `benkyo-ferios-nando-statements`, set in `deploy.yml`. |
| `MESSENGER_TRIGGER_URL`, `MESSENGER_TRIGGER_TOKEN` | The API trigger (`…/fire`) of the cloud scheduled task whose runs answer the Owner as their Assistants, and its bearer token ([ADR 0008](docs/adr/0008-assistants-on-telegram.md)). Optional: without both, Nando never wakes Claude for them, and messages wait. In production the URL is the repo variable `MESSENGER_TRIGGER_URL` and the token comes from Secret Manager (`nandoproject-messenger-trigger-token`). |

## Enroll a Passkey

`nando-server enroll` prints an Enrollment Code that registers one Passkey within 15 minutes ([ADR 0002](docs/adr/0002-passkey-only-identity.md)). It's also the recovery path. It needs the same `MONGO_URI`, `DOMAIN` and `ALLOWED_ORIGINS` as the server (not the Notification ones), so for production:

```sh
MONGO_URI="$(gcloud secrets versions access latest --secret nandoproject-mongo --project <project>)" \
DOMAIN=nanders.app \
ALLOWED_ORIGINS=https://web.nanders.app \
go run ./cmd/nando-server enroll
```

## Test

```sh
go test -count=1 ./...
```

Tests that touch data run against a real MongoDB through `testmongo.URI(t)`: each one starts a fresh `mongo:8` container with testcontainers-go and terminates it when the test ends. Docker must be running; `docker compose` is not needed for tests. `-count=1` stops Go replaying a cached pass, since its test cache can't see the `mongo:8` image or the Docker host's kernel.

### JSON shapes are the API

Core's and every Module's tests read and write the API with their own copies of its JSON shapes, declared in the test files (`settingsJSON` in `core/tests/settings_test.go`, `mealJSON` in `calories/tests/meals_test.go`, `wordJSON` in `japanese/tests/words_test.go`), the way `nando-web` has its own. They never use the `model` JSON types, so a test reads the API as a client does, and a changed `json` tag fails it. `internal/json_copies_test.go` fails the build if a file in a `tests/` folder uses a `model` type named `…JSON`.

> [!NOTE]
> If someone renames `json:"weekday"` to `json:"day"` in `model.ReminderJSON`, the test's `reminderJSON` still reads `weekday`, finds nothing, and fails, just as the web app would break.

So treat every `json` tag in `model/` as part of the API, the same as a route:

- Never rename, remove or retype a `json` tag, or add or drop `omitempty`, as part of a refactor. Renaming the Go field is fine; its tag is what the web app reads.
- When a test fails on a changed tag, the API changed: change the area's `API.md` and `nando-web` in step, in the same piece of work, then the test's copy. Never change only the test's copy to make it pass.
- A test's copy only checks the fields it declares, and a field it leaves out goes unchecked. When the API gains a field, add it to the copy of the test that should check it.
- Go matches JSON keys without regard to case, so no test notices a tag that only changes case (`imageUrl` to `imageURL`), though the web app breaks on it.
- In review, read every diff to a `json:"…"` tag in `model/` as an API change.

Every pull request runs `go vet ./...` and the same `go test` on GitHub Actions (`.github/workflows/pr.yml`); the runner's Docker hosts the test containers. It also builds the container image.

## Container image

```sh
docker build -t nando-server .
```

The image runs the static binary on `debian:12-slim`, as a non-root user, with `poppler-utils` for reading PDFs (ADR 0008) and CA certificates for calling out. Time zones are built into the binary.

## Deploy

Every push to `main` deploys to Cloud Run (`.github/workflows/deploy.yml`; also runnable by hand from the Actions tab). It signs in through Workload Identity Federation, pushes the image to Artifact Registry tagged with the commit, deploys it with its env vars and Secret Manager secrets, and ends by checking that the API's `/health` answers as nando-server. That workflow is the source of truth for the service's env vars and secrets.

If the new revision doesn't start (for example, MongoDB is unreachable), the deploy fails and traffic stays on the previous revision.

## Backups

Every Sunday at 03:17 Tokyo time, `.github/workflows/backup.yml` (also runnable by hand from the Actions tab) dumps the `core`, `calories`, `japanese`, `career`, `travel` and `money` databases with `scripts/backup.sh` and uploads them as `nando-<UTC date and time>.tar` (one gzipped BSON file per collection) to the private bucket named by the repo variable `GCP_BACKUP_BUCKET` (`gs://nanders-backups`). It signs in through Workload Identity Federation as the backup service account (`GCP_BACKUP_SA`). That account can read the `nandoproject-mongo` secret and create objects in the bucket, but can't read, overwrite or delete them.

The bucket's lifecycle rule deletes each `nando-` backup when it's 56 days old, which leaves the last 8 weekly ones. It goes by age, not count: if backups stop, the old ones still expire, and after 8 weeks there are none. So watch for failed runs. GitHub emails a failed scheduled run to whoever last changed the workflow's `cron` line, and in a public repository it turns scheduled workflows off after 60 days without activity (re-enable Backup in the Actions tab).

Anything else in the bucket is kept. `final/vocabulary-2026-11-10.tar` is the last dump of benkyo's `vocabulary` database from the old US-East cluster, taken when it moved into `japanese` (#36). It has the same layout, so the restore below works on it, but it restores into `vocabulary` and includes benkyo's `users`.

`backup.sh` lists the databases it dumps. A new Module's database must be added there.

The backup account was given the secret once (#13); to set it up again:

```sh
gcloud secrets add-iam-policy-binding nandoproject-mongo --project <project> \
  --member "serviceAccount:<GCP_BACKUP_SA>" --role roles/secretmanager.secretAccessor
```

### Restore

Signed in to `gcloud` as yourself (the backup account can't read the bucket), pick a backup and download it as `nando.tar`:

```sh
gcloud storage ls gs://nanders-backups/
gcloud storage cp gs://nanders-backups/nando-<when>.tar nando.tar
```

Point `MONGO_URI` at the database to restore into: production's, or `mongodb://localhost:27017` to look at a backup in the local MongoDB first.

```sh
export MONGO_URI="$(gcloud secrets versions access latest --secret nandoproject-mongo --project <project>)"
```

Then, in the folder holding `nando.tar`, with `mongorestore` installed ([MongoDB Database Tools](https://www.mongodb.com/docs/database-tools/installation/)), unpack and restore it. `--drop` replaces each backed-up collection with its contents in the backup; collections that aren't in the backup are left alone.

<!-- restore: scripts/backup_test.go runs the block below -->
```sh
mkdir nando && tar -xf nando.tar -C nando
mongorestore --uri "$MONGO_URI" --gzip --drop nando
```
