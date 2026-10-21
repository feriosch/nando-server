# nando-server

Go API for Nando. Domain language: [`GLOSSARY.md`](GLOSSARY.md). Decisions: [`docs/adr/`](docs/adr/).

## Layout

- `cmd/nando-server`: the program: reads config, connects to MongoDB, wires Core and Modules, serves HTTP.
- `internal/server`: the HTTP API (routes, CORS) and `server.RegisterDatabases`, which opens the MongoDB connection and binds it as singletons in a [golobby container](https://github.com/golobby/container): the `*mongo.Client` and one named `*mongo.Database` each for Core and every Module (`core`, `calories`). `main` fills a `server.Databases` from the container and hands each area its own database.
- `internal/core`: Core, wired in `core.go`.
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
ALLOWED_ORIGINS=http://localhost:5173 \
go run ./cmd/nando-server

curl localhost:8080/health
```

| Env var           | Meaning |
| ----------------- | --- |
| `MONGO_URI`       | MongoDB connection string (required)                             |
| `ALLOWED_ORIGINS` | Comma-separated web app origins allowed to call with credentials, each exactly `scheme://host[:port]` (no trailing slash, no `*`) |
| `PORT`            | Port to listen on (default `8080`; Cloud Run sets it)            |

## Test

```sh
go test ./...
```

Tests that touch data run against a real MongoDB through `testmongo.URI(t)`: each one starts a fresh `mongo:8` container with testcontainers-go and terminates it when the test ends. Docker must be running; `docker compose` is not needed for tests.

Every pull request runs `go vet ./...` and `go test -count=1 ./...` on GitHub Actions (`.github/workflows/pr.yml`); the runner's Docker hosts the test containers.

## Container image

```sh
docker build -t nando-server .
```
