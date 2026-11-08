# nando-server

Go API for Nando. Domain language: [`GLOSSARY.md`](GLOSSARY.md). Decisions: [`docs/adr/`](docs/adr/).

## Layout

- `cmd/nando-server`: the program: reads config, connects to MongoDB, wires Core and Modules, serves HTTP.
- `internal/server`: the HTTP API (routes, CORS) and `server.RegisterDatabases`, which opens the MongoDB connection and binds it as singletons in a [golobby container](https://github.com/golobby/container): the `*mongo.Client` and one named `*mongo.Database` each for Core and every Module (`core`, `calories`). `main` fills a `server.Databases` from the container and hands each area its own database.
- `internal/core`: Core, wired in `core.go`. Its HTTP API is documented in [`internal/core/API.md`](internal/core/API.md).
- `internal/calories`: the Calories Module, wired in `calories.go`.
- `internal/testmongo`: test helper that starts a real MongoDB container.

Core and every Module are split into the same three layers:

- `model/`: the area's data and its repository interface (`CoreRepository`, `CaloriesRepository`), shaped for that one area. Core's also declares `CoreService`.
- `service/`: request handling and validation. Depends only on `model/` interfaces; never touches the database.
- `repository/`: all database code. Implements the `model/` interface over the area's own `*mongo.Database`. `repository.go` holds the constructor and the shared Mongo type; every other file corresponds to exactly one collection of that database and holds all the code for it (e.g. `calories/repository/meals.go` → `meals`, `entries.go` → `entries`).

The one difference: Core's service (as `model.CoreService`) is also handed to every Module's service. When a Module needs Core's data (such as Weigh-ins), its service asks Core's service and then uses that data itself or passes it to its own repository. A Module never receives Core's repository, nor another Module's service, repository or database. Reaching another database from a repository (for example via `db.Client()`) is possible with the driver, so this rule is kept by convention.

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
