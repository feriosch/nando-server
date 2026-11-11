# nando-server

Go API for Nando. Domain language: [`GLOSSARY.md`](GLOSSARY.md). Decisions: [`docs/adr/`](docs/adr/).

## Layout

- `cmd/nando-server`: the program: reads config, connects to MongoDB, wires Core and Modules, serves HTTP.
- `internal/server`: the HTTP API (routes, CORS) and `server.RegisterDatabases`, which opens the MongoDB connection and binds it as singletons in a [golobby container](https://github.com/golobby/container): the `*mongo.Client` and one named `*mongo.Database` each for Core and every Module (`core`, `calories`, `japanese`). `main` fills a `server.Databases` from the container and hands each area its own database.
- `internal/core`: Core, wired in `core.go`. Its HTTP API is documented in [`internal/core/API.md`](internal/core/API.md).
- `internal/calories`: the Calories Module, wired in `calories.go`. Its HTTP API is documented in [`internal/calories/API.md`](internal/calories/API.md).
- `internal/japanese`: the Japanese Module, wired in `japanese.go`. Its read-only HTTP API and the Tools through which Claude adds and improves Words are documented in [`internal/japanese/API.md`](internal/japanese/API.md). `japanese.Config` names the outside services its Tools look Words up on, jisho.org and kanjiapi.dev; `main` passes the defaults and tests set fakes. `internal/japanese/anki` documents the Anki note type its JLPT deck file fills: its card templates, styling and fields. Its database holds the Words and Kanji moved from benkyo (#36): `words`, `kanji`, `irregular_components`, `groups` and `clauses`, which has no API. `words` has a unique index on spelling and reading: one Word per spelling and reading.
- `internal/testmongo`: test helper that starts a real MongoDB container (`URI`, or `Start` for a test that needs the container itself).
- `internal/testpasskey`: test helper, a software Passkey, so tests can sign in.
- `internal/testpush`: test helper, a browser's push service, so tests can see the Notifications Nando delivers.
- `internal/testconnector`: test helper, a Connector the Owner approves with a Passkey, connected to `/mcp` with the official MCP client, so a Module's tests can call its Tools as Claude does.
- `skills/`: the skills Claude follows to work with the Owner's data through the Nando Connector, such as adding a Japanese Word (`skills/japanese/add-word`). [`skills/README.md`](skills/README.md) says how to use them in the Claude app and in Claude Code, and how to add the Connector.
- `scripts/backup.sh`: the weekly backup's dump (see [Backups](#backups)). `scripts/backup_test.go` runs it inside `mongo:8` and restores what it wrote with the commands below.

Core and every Module are split into the same three layers:

- `model/`: the area's data and one repository interface per collection, declared next to the data it stores (`MealsRepository` in `calories/model/meals.go`, `EntriesRepository` in `entries.go`). Core's also declares `CoreService` (`core/model/core_service.go`). Core and every Module also declare their JSON shapes there, exported: what their API answers with and is sent, and what Core's push messages carry. Each goes next to the data it shows (`MealJSON` in `calories/model/meals.go`, `WordJSON` in `japanese/model/words.go`, `SettingsJSON` in `core/model/settings.go`, `PushMessageJSON` in `core/model/notifications.go`), or in a file of its own when it shows no single collection (`DayJSON` in `calories/model/days.go`, `SearchJSON` in `japanese/model/search.go`, `BodyCardJSON` in `core/model/body.go`, `CreatedJSON` in `core/model/created.go`). `service/` declares none: a request body decoded inline in a handler may stay unnamed there, but no handler answers with an unnamed struct. Tests never decode into these types: each declares its own copy, so a changed `json` tag fails a test: see [JSON shapes are the API](#json-shapes-are-the-api). Japanese's `AnkiWord` (`anki.go`) is a Word as the JLPT deck file reads it, kept apart from `Word` because the deck depends on details of how benkyo stored Words that nothing else shows.
- `service/`: request handling and validation, one service per collection it serves, each in the file named after it with its own routes (`MealsService` in `calories/service/meals.go`, `EntriesService` in `entries.go`). A service holds only the repositories it uses, which may include another collection's, such as the Entries one for the quick list's counts. A service may also ask another service of its area, as Core's Weigh-ins service asks its Settings service for Today. Depends only on `model/` interfaces; never touches the database. Core's services that serve no single collection are named for what they do: `IdentityService` (`identity.go`) checks it is the Owner with a Passkey under `/auth/`, with the Enrollment Codes, Ceremonies and Passkeys repositories: to sign in, then having the `SessionsService` start a Session, or to approve a Connector, then having the `OAuthService` create it; `OAuthService` (`oauth.go`) is Nando's own OAuth authorization server for Connectors, with the OAuth Clients, Authorization Requests, Authorization Codes and Connectors repositories; `MCPService` (`mcp.go`) serves every Tool provider's Tools at `/mcp` to Connectors, checking their access tokens with the Connectors repository; `BodyService` (`body.go`) answers the Body Home Card from Weigh-ins and settings; `NotificationsService` (`notifications.go`) runs the tick, reading Push Subscriptions and recording which Notifications were sent. Calories' `DaysService` (`days.go`) answers the Day view, the preview, the history and the Calories Home Card from Entries and Daily Limits, asking the `EntriesService` which Days can be logged on. Calories' `NotificationsService` (`notifications.go`) is the Module's Notification provider, which `main` adds to Core's tick: it holds no repository, asking the `SettingsService` when the no-Entries reminder and the evening summary are due and the `DaysService` how today went. Japanese's `SearchService` (`search.go`) serves no single collection either: it answers the search screen from Words and Kanji in one ranked response, so it sits on its own rather than in the Words or Kanji service. `WordsService` (`words.go`) answers the Word page, also reading Groups and Kanji, and `KanjiService` (`kanji.go`) the Kanji page, also reading irregular Components and Words. `AnkiService` (`anki.go`) exports Words as Anki deck files, asking Core for today's Day to date them, so it too is named for what it does, and `LookupService` (`lookup.go`) looks Words and Kanji up outside Nando, on jisho.org and kanjiapi.dev. Each Japanese service also offers the Tools for its own data (`WordsService` the Word, Word Link and Hint Tools), and `Module.ToolProvider` gathers them for Core.
- `repository/`: all database code, one repository per collection, over the area's own `*mongo.Database`. Each file holds one collection's repository and all the code for it, including its constructor and indexes (e.g. `calories/repository/meals.go` → `meals`, `entries.go` → `entries`). Turning MongoDB's "nothing found" into `ErrNotFound` is shared through Core's `core/utils/errors.go` (`utils.NotFound`, `utils.Matched`, `utils.Deleted`); a Module checks for Core's `utils.ErrNotFound` rather than declaring its own. A method that reads more than one collection belongs to the repository of the collection it is mainly about, as with a SQL join: counting Entries per Meal is the Entries repository's.

No layer has a file for what its files share (no `model.go`, `service.go` or `repository.go`): that goes in `utils/`. Core and a Module may have one: helpers their own layers share. Core's are also for Modules to use. Examples: Core's `ErrNotFound` and MongoDB error helpers (`core/utils/errors.go`), the response helpers every part of the API answers with (`core/utils/responses.go`), the expiry index and filter for documents that expire (`core/utils/expiry.go`), the half-hour clock times Notifications can be set for, since the tick runs every half hour, and whether a tick is at one (`core/utils/clock_times.go`), Calories' checks on what the Owner sends and their limits (`calories/utils/checks.go`), the request and JSON helpers Calories' services share, such as reading the Day in a path and writing Entries (`calories/utils/requests.go`, `calories/utils/json.go`), the Editable Window rule (`calories/utils/editable_window.go`), and Japanese's search pattern that ignores case and accents, with the query parts that rank matches (`japanese/utils/search.go`). A `utils/` package runs no queries and handles no requests, though it may build a query's parts or write a response.

Core's and every Module's tests live in their `tests/` folder (`core/tests/weigh_ins_test.go`, `calories/tests/meals_test.go`). They use only what Core and the Module export, through the HTTP API, so they don't need to sit next to the code. Japanese has two exceptions, for files the Module ships outside its code. `anki_note_type_test.go` also reads the note type documented in `japanese/anki`, to check its card templates and README against the fields the JLPT deck file serves. `skills_test.go` also reads the skills in `skills/japanese`, to check every Tool they name is one Nando serves.

A file about something there can be many of is named in the plural in every layer and in tests, matching its collection: `model/meals.go`, `service/meals.go`, `repository/meals.go`, `meals_test.go`. Names with no plural form, such as `settings.go`, `identity.go` or `body.go`, stay as they are.

The one difference: Core also hands every Module's service a `model.CoreService`, answered by Core's Settings service (`OwnerTimeZone`, `Today`) and Weigh-ins service (`LatestWeighIn`) through a small type in `core.go`. When a Module needs Core's data (such as Weigh-ins), its service asks it and then uses that data itself or passes it to its own repository. A Module never receives Core's repository, nor another Module's service, repository or database. Reaching another database from a repository (for example via `db.Client()`) is possible with the driver, so this rule is kept by convention.

Core and every Module may also supply Core with providers, which `main` adds once, at startup, before serving: a Notification provider (`model.NotificationProvider`, added with `AddNotificationProvider`), which Core asks at every tick for the Notifications due, and a Tool provider (`model.ToolProvider`, added with `AddToolProvider`), whose `Tools()` Core serves to every Connector at `/mcp`. A `model.Tool` is a name, a description, a JSON Schema for its arguments, whether it is read-only or destructive, and a handler that takes the arguments as JSON and returns a result Core sends back as JSON. Core serves Tools without knowing what they do. A Module's Tool names start with the Module's name (`japanese_search`). `AddToolProvider` panics at startup on a Tool it can't serve: one without a name, a handler or an object schema, or whose name another Tool already has.

Imports follow the same direction. A Module may import what Core exports, such as Core's `model` types and Core's `utils` (`utils.ErrNotFound`, and the response helpers `utils.WriteJSON` and `utils.ServerError`, so every part of the API answers alike). Core never imports a Module, and a Module never imports another Module. Core's data still reaches a Module only through `model.CoreService`, never by calling Core's services or repositories directly. `internal/imports_test.go` fails the build if Core imports a Module, a Module imports another Module, or a Module imports Core's repositories or services. It treats every folder under `internal/` as a Module except `core`, `server` and the `test…` helpers.

## Run locally

Requires Go 1.25+ and Docker.

```sh
docker compose up -d   # MongoDB on localhost:27017

MONGO_URI=mongodb://localhost:27017 \
DOMAIN=localhost \
ALLOWED_ORIGINS=http://localhost:5173 \
VAPID_PUBLIC_KEY=<public> VAPID_PRIVATE_KEY=<private> \
PUSH_CONTACT=mailto:you@example.com \
TICK_AUDIENCE=http://localhost:8080 \
SCHEDULER_SERVICE_ACCOUNT=none \
go run ./cmd/nando-server

curl localhost:8080/health
```

| Env var           | Meaning |
| ----------------- | --- |
| `MONGO_URI`       | MongoDB connection string (required)                             |
| `DOMAIN`          | Parent domain of `web.` and `api.`, such as `nanders.app` (required): the Passkeys' relying-party ID and the Session cookie's domain. Changing it means enrolling again. |
| `ALLOWED_ORIGINS` | Comma-separated web app origins (required), each exactly `scheme://host[:port]` (no trailing slash, no `*`). They may call the API with credentials and are the only origins Passkeys work from ([ADR 0002](docs/adr/0002-passkey-only-identity.md)), so adding one also lets it sign in. |
| `PORT`            | Port to listen on (default `8080`; Cloud Run sets it)            |
| `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY` | The key pair that signs push messages (required to serve), base64url. Generate one with `webpush.GenerateVAPIDKeys`. Changing it invalidates every Push Subscription. In production the private key comes from Secret Manager. |
| `PUSH_CONTACT`    | A `mailto:` or `https:` URL where a push service can reach Nando's operator (required to serve; Apple rejects push messages without one). |
| `TICK_AUDIENCE`   | The audience the Cloud Scheduler job's OIDC token is for (required to serve), exactly as the job sets it: `https://api.nanders.app` in production. |
| `SCHEDULER_SERVICE_ACCOUNT` | The email of the service account Cloud Scheduler signs its OIDC token as (required to serve). `/internal/tick` accepts no other caller. |

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

## Deploy

Every push to `main` deploys to Cloud Run (`.github/workflows/deploy.yml`; also runnable by hand from the Actions tab). It signs in through Workload Identity Federation, pushes the image to Artifact Registry tagged with the commit, deploys it with its env vars and Secret Manager secrets, and ends by checking that the API's `/health` answers as nando-server. That workflow is the source of truth for the service's env vars and secrets.

If the new revision doesn't start (for example, MongoDB is unreachable), the deploy fails and traffic stays on the previous revision.

## Backups

Every Sunday at 03:17 Tokyo time, `.github/workflows/backup.yml` (also runnable by hand from the Actions tab) dumps the `core`, `calories` and `japanese` databases with `scripts/backup.sh` and uploads them as `nando-<UTC date and time>.tar` (one gzipped BSON file per collection) to the private bucket named by the repo variable `GCP_BACKUP_BUCKET` (`gs://nanders-backups`). It signs in through Workload Identity Federation as the backup service account (`GCP_BACKUP_SA`). That account can read the `nandoproject-mongo` secret and create objects in the bucket, but can't read, overwrite or delete them.

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
