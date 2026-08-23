# MotionForge

MotionForge is a production-style Go backend for operating cross-robot data-capture programs. It coordinates capture facilities and rig leases, multimodal stream manifests, annotation review, immutable dataset releases, training jobs, durable outbox delivery, audit trails, and restart recovery.

The product theme is inspired by China News Service reporting on a cross-robot data-capture training center. MotionForge is an independent fictional system and does not claim affiliation with organizations named in that report.

## Requirements

- Go 1.26 with `GOTOOLCHAIN=local`
- Docker for the container workflow
- A writable path for the SQLite database

## Run locally

```bash
MOTIONFORGE_DATABASE_PATH=./motionforge.db GOTOOLCHAIN=local go run ./cmd/server
```

The server listens on `:8080` by default. Configuration uses environment variables:

| Variable | Default | Purpose |
|---|---|---|
| `MOTIONFORGE_ADDRESS` | `:8080` | HTTP listen address |
| `MOTIONFORGE_DATABASE_PATH` | `motionforge.db` | SQLite database path |
| `MOTIONFORGE_SHUTDOWN_TIMEOUT` | `10s` | graceful shutdown deadline |
| `MOTIONFORGE_SESSION_TTL` | `12h` | login session lifetime |
| `MOTIONFORGE_LEASE_TTL` | `90s` | rig, annotation, training, and outbox lease lifetime |
| `MOTIONFORGE_WORKER_RETRY_BASE` | `250ms` | exponential retry base |
| `MOTIONFORGE_WORKER_MAX_ATTEMPTS` | `5` | terminal attempt limit |
| `MOTIONFORGE_DATABASE_MAX_OPEN_CONNS` | `8` | SQLite connection pool bound |

`GET /healthz` reports process liveness. `GET /readyz` verifies the database dependency before reporting ready.

## Docker

```bash
docker build -t motionforge:local .
docker run --rm -p 8080:8080 -v motionforge-data:/data motionforge:local
```

The multi-stage image builds the real `./cmd/server` entry with Go 1.26 and runs it as a non-root user. It does not pin a CPU architecture.

## Core workflows

1. Bootstrap a tenant administrator, log in, create operator/reviewer/steward/worker users, then create a facility, capture rig, and scenario.
2. Plan an idempotent capture, reserve a compatible rig, transition to recording, ingest and seal at least two multimodal streams, align timecodes, submit, and validate.
3. Expand validated stream segments into an annotation batch, claim with an expiring lease, complete items, submit, and review.
4. Build a dataset from eligible captures, freeze immutable membership, approve and publish a release, then enqueue and execute training work through leased attempts and checkpoints.

Every mutation propagates the HTTP request context and request ID into services and SQL, and couples its business state with durable audit/outbox effects in one transaction. Conditional updates bind claims to owner, token, version, and expiry to prevent stale workers from changing current ownership.

## Verification

```bash
GOTOOLCHAIN=local go test ./... -count=1
GOTOOLCHAIN=local go test -race ./... -count=1
GOTOOLCHAIN=local go vet ./...
GOTOOLCHAIN=local go build ./...
```

Tests use temporary real SQLite databases and cover migration/restart persistence, state transitions, rollback, tenant isolation, idempotency, deterministic concurrent ownership, context cancellation, error mapping, worker retry/stop, response-body lifecycle, pagination, and resource blockers.
