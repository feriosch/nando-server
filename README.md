# nando-server

Go API for Nando. Domain language: [`GLOSSARY.md`](GLOSSARY.md). Decisions: [`docs/adr/`](docs/adr/).

## Layout

- `cmd/nando-server`: the program: reads config, connects to MongoDB, wires Core and Modules, serves HTTP.
- `internal/server`: the HTTP API (routes, CORS) and `server.RegisterDatabases`, which opens the MongoDB connection and binds it as singletons in a [golobby container](https://github.com/golobby/container): the `*mongo.Client` and one named `*mongo.Database` each for Core and every Module (`core`, `calories`). `main` fills a `server.Databases` from the container and hands each area its own database.
- `internal/core`: Core, wired in `core.go`. Its HTTP API is documented in [`internal/core/API.md`](internal/core/API.md).
- `internal/calories`: the Calories Module, wired in `calories.go`. Its HTTP API is documented in [`internal/calories/API.md`](internal/calories/API.md).
- `internal/testmongo`: test helper that starts a real MongoDB container.
- `internal/testpasskey`: test helper, a software passkey, so tests can sign a device in.
- `internal/testpush`: test helper, a browser's push service, so tests can see the Notifications Nando delivers.
- `scripts/backup.sh`: the weekly backup's dump (see [Backups](#backups)). `scripts/backup_test.go` runs it inside `mongo:8` and restores what it wrote with the commands below.

Core and every Module are split into the same three layers:

- `model/`: the area's data and one repository interface per collection, declared next to the data it stores (`MealsRepository` in `calories/model/meals.go`, `EntriesRepository` in `entries.go`). Core's also declares `CoreService` (`core/model/core_service.go`).
- `service/`: request handling and validation, one service per collection it serves, each in the file named after it with its own routes (`MealsService` in `calories/service/meals.go`, `EntriesService` in `entries.go`). A service holds only the repositories it uses, which may include another collection's, such as the Entries one for the quick list's counts. A service may also ask another service of its area, as Core's Weigh-ins service asks its Settings service for Today. Depends only on `model/` interfaces; never touches the database. Core's services that serve no single collection are named for what they do: `IdentityService` (`identity.go`) signs the Owner in under `/auth/` with the Enrollment Codes, Ceremonies and Passkeys repositories, then has the `SessionsService` start a session; `BodyService` (`body.go`) answers the Body Home Card from Weigh-ins and settings; `NotificationsService` (`notifications.go`) runs the tick, reading Push Subscriptions and recording which Notifications were sent. Calories' `DaysService` (`days.go`) answers the Day view, the preview, the history and the Calories Home Card from Entries and Daily Limits, asking the `EntriesService` which Days can be logged on. Calories' `NotificationsService` (`notifications.go`) is the Module's Notification provider, which `main` adds to Core's tick: it holds no repository, asking the `SettingsService` when the no-Entries reminder and the evening summary are due and the `DaysService` how today went.
- `repository/`: all database code, one repository per collection, over the area's own `*mongo.Database`. Each file holds one collection's repository and all the code for it, including its constructor and indexes (e.g. `calories/repository/meals.go` → `meals`, `entries.go` → `entries`). Turning MongoDB's "nothing found" into `ErrNotFound` is shared through Core's `core/utils/errors.go` (`utils.NotFound`, `utils.Matched`, `utils.Deleted`); a Module checks for Core's `utils.ErrNotFound` rather than declaring its own. A method that reads more than one collection belongs to the repository of the collection it is mainly about, as with a SQL join: counting Entries per Meal is the Entries repository's.

No layer has a file for what its files share (no `model.go`, `service.go` or `repository.go`): that goes in `utils/`. Core and a Module may have one: helpers their own layers share. Core's are also for Modules to use. Examples: Core's `ErrNotFound` and MongoDB error helpers (`core/utils/errors.go`), the response helpers every part of the API answers with (`core/utils/responses.go`), the expiry index and filter for documents that expire (`core/utils/expiry.go`), the half-hour clock times Notifications can be set for, since the tick runs every half hour, and whether a tick is at one (`core/utils/clock_times.go`), Calories' checks on what the Owner sends and their limits (`calories/utils/checks.go`), the request and JSON helpers Calories' services share, such as reading the Day in a path and writing Entries (`calories/utils/requests.go`, `calories/utils/json.go`), and the Editable Window rule (`calories/utils/editable_window.go`). A `utils/` package runs no queries and handles no requests, though it may build a query's parts or write a response.

Core's and every Module's tests live in their `tests/` folder (`core/tests/weigh_ins_test.go`, `calories/tests/meals_test.go`). They use only what Core and the Module export, through the HTTP API, so they don't need to sit next to the code.

A file about something there can be many of is named in the plural in every layer and in tests, matching its collection: `model/meals.go`, `service/meals.go`, `repository/meals.go`, `meals_test.go`. Names with no plural form, such as `settings.go`, `identity.go` or `body.go`, stay as they are.

The one difference: Core also hands every Module's service a `model.CoreService`, answered by Core's Settings service (`OwnerTimeZone`, `Today`) and Weigh-ins service (`LatestWeighIn`) through a small type in `core.go`. When a Module needs Core's data (such as Weigh-ins), its service asks it and then uses that data itself or passes it to its own repository. A Module never receives Core's repository, nor another Module's service, repository or database. Reaching another database from a repository (for example via `db.Client()`) is possible with the driver, so this rule is kept by convention.

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
| `DOMAIN`          | Parent domain of `web.` and `api.`, such as `nanders.app` (required): the passkeys' relying-party ID and the session cookie's domain. Changing it means enrolling again. |
| `ALLOWED_ORIGINS` | Comma-separated web app origins (required), each exactly `scheme://host[:port]` (no trailing slash, no `*`). They may call the API with credentials and use passkeys. |
| `PORT`            | Port to listen on (default `8080`; Cloud Run sets it)            |
| `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY` | The key pair that signs push messages (required to serve), base64url. Generate one with `webpush.GenerateVAPIDKeys`. Changing it invalidates every Push Subscription. In production the private key comes from Secret Manager. |
| `PUSH_CONTACT`    | A `mailto:` or `https:` URL where a push service can reach Nando's operator (required to serve; Apple rejects push messages without one). |
| `TICK_AUDIENCE`   | The audience the Cloud Scheduler job's OIDC token is for (required to serve), exactly as the job sets it: `https://api.nanders.app` in production. |
| `SCHEDULER_SERVICE_ACCOUNT` | The email of the service account Cloud Scheduler signs its OIDC token as (required to serve). `/internal/tick` accepts no other caller. |

## Enroll a passkey

`nando-server enroll` prints an Enrollment Code that registers one passkey within 15 minutes ([ADR 0002](docs/adr/0002-passkey-only-identity.md)). It's also the recovery path. It needs the same `MONGO_URI`, `DOMAIN` and `ALLOWED_ORIGINS` as the server (not the Notification ones), so for production:

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

Every pull request runs `go vet ./...` and the same `go test` on GitHub Actions (`.github/workflows/pr.yml`); the runner's Docker hosts the test containers. It also builds the container image.

## Container image

```sh
docker build -t nando-server .
```

## Deploy

Every push to `main` deploys to Cloud Run (`.github/workflows/deploy.yml`; also runnable by hand from the Actions tab). It signs in through Workload Identity Federation, pushes the image to Artifact Registry tagged with the commit, deploys it with its env vars and Secret Manager secrets, and ends by checking that the API's `/health` answers as nando-server. That workflow is the source of truth for the service's env vars and secrets.

If the new revision doesn't start (for example, MongoDB is unreachable), the deploy fails and traffic stays on the previous revision.

## Backups

Every Sunday at 03:17 Tokyo time, `.github/workflows/backup.yml` (also runnable by hand from the Actions tab) dumps the `core` and `calories` databases with `scripts/backup.sh` and uploads them to the private bucket `gs://nanders-backups` as `nando-<UTC date and time>.tar`: one gzipped BSON file per collection. It signs in through Workload Identity Federation as the backup service account (`GCP_BACKUP_SA`), which can read the `nandoproject-mongo` secret and create objects in the bucket, but not read, overwrite or delete them. The bucket's lifecycle rule deletes backups after 56 days, so the last 8 are kept.

The backup service account needs access to the Mongo secret. Grant it once:

```sh
gcloud secrets add-iam-policy-binding nandoproject-mongo --project <project> \
  --member "serviceAccount:<GCP_BACKUP_SA>" --role roles/secretmanager.secretAccessor
```

GitHub turns off scheduled workflows in a public repository after 60 days without activity; if it does, re-enable Backup in the Actions tab.

### Restore

Signed in to `gcloud` as yourself (the backup account can't read the bucket), pick a backup and unpack it:

```sh
gcloud storage ls gs://nanders-backups/
gcloud storage cp gs://nanders-backups/nando-<when>.tar .
mkdir nando && tar -xf nando-<when>.tar -C nando
```

Then restore it with `mongorestore` ([MongoDB Database Tools](https://www.mongodb.com/docs/database-tools/installation/), or run it in `docker run --rm -it -v "$PWD:/work" -w /work mongo:8 bash`). `--drop` replaces each backed-up collection with its contents in the backup; collections that aren't in the backup are left alone. To check a backup first, restore it into the local MongoDB (`MONGO_URI=mongodb://localhost:27017`).

```sh
MONGO_URI="$(gcloud secrets versions access latest --secret nandoproject-mongo --project <project>)" \
mongorestore --uri "$MONGO_URI" --gzip --drop nando
```
