# Endge Mock Generator

`github.com/endge-lab/service-mock-generator`, version `0.1.2`: bounded, deterministic JSON Schema Draft 2020-12 generation and in-memory stream sessions. No database, Redis or broker is required.

Clients use the authenticated backend `/api/v1/mock-data` HTTP/SSE API. Only backend connects to the canonical `mockdata.v1` gRPC API. The generator's HTTP port exposes `/health`, `/version`, `/swagger` and `/swagger/openapi3.yaml` only.

## Local development

From the monorepo root:

```bash
./infra/dev.sh up mock
./infra/dev.sh status mock
./infra/dev.sh logs mock
./infra/dev.sh down mock
```

The general `./infra/dev.sh up` includes Mock. Compose uses `service-mock-generator:50052` and internal HTTP 8082 without published Mock ports. `up mock` starts Keycloak and idempotently adds the backend service-client audience mapper; it does not create a database or replace the realm/volume/users. Backend remains usable without Mock.

For a process outside Compose, copy `.env.development.example` to `.env.development`, use the local Keycloak issuer/JWKS configuration, then `make run`. `make build`, Air and Docker embed `VERSION`. Production requires service identity and TLS; only explicit `MOCK_ALLOW_INSECURE_DEVELOPMENT=true` permits disabled verification in development.

Stateless-конфигурация использует опубликованный `service-kit-go v0.5.0` с `postgres.enabled: false`; эта версия закреплена в `go.mod`. `GOWORK=off go build ./...` проверяет самостоятельную сборку с опубликованной зависимостью. Локальный `go.work` сохраняет связь с исходниками kit для разработки.

## Contracts

- [Generation rules, schema subset and relations](docs/todo-1.md)
- [gRPC, HTTP/SSE, ownership, lease and examples](docs/todo-2.md)
- [Implementation map](docs/architecture.md)
- [Tests, fuzzing and adversarial harness](docs/tests/README.md)
- Canonical source: `api/proto/mockdata/v1/mockdata.proto`; generated server: `api/mockdata/v1`; backend client: `internal/adapter/mockpb` in the backend repository.

`make proto` regenerates both copies from the canonical source; `make contract-check` verifies the generated copies and contract hash. `protoc`, `protoc-gen-go` and `protoc-gen-go-grpc` must be on PATH.

## Limits and operations

`GetCapabilities` publishes effective limits. Numeric settings use `MOCK_REQUEST_BYTES`, `RESULT_BYTES`, `SCHEMA_NODES`, `DEPTH`, `DEFINITIONS`, `ARRAY_LENGTH`, `STRING_LENGTH`, `RELATIONS`, `MAPPINGS`, `ATTEMPTS`, `COUNT`, `ITEMS_PER_MESSAGE`, `MIN_INTERVAL_MS`, `MAX_INTERVAL_MS`, `SESSIONS`, `SESSIONS_PER_OWNER`, `JOBS`, `BUFFER_BYTES` (each prefixed with `MOCK_`). Durations: `MOCK_GENERATION_TIMEOUT`, `MOCK_IDLE_TIMEOUT`, `MOCK_READY_TIMEOUT`, `MOCK_WRITE_TIMEOUT`, in Go duration syntax.

Defaults: 2 MiB input, 20 MiB output, 4 concurrent generation jobs, 32 sessions total/5 per actor, 64 MiB pending stream bytes, 10 s generation and write timeouts. Ready sessions expire after 30 s; active sessions expire 180 s after the last successful client KeepAlive. Send KeepAlive around every 30 s, also while paused. Data and SSE heartbeats do not renew a lease.

Server metrics report active sessions/jobs, buffered bytes, batch count, errors, lease expiry, backpressure, total generation duration and bytes. They contain no actor/session labels or generated payloads. Changes to defaults require a new load run and documentation update.
