# Verification

## Ordinary suites (no Docker, working env files or external IdP)

From Mock repository:

```bash
go test ./...
go test -race ./...
make contract-check
GOWORK=off go build ./...
make fuzz
```

`make fuzz` runs four targets for 60 seconds each: decoding/schema preflight, bounded regex parser, relation selectors/plan application and active session command state machine. Saved failing inputs live under package `testdata/fuzz`; ordinary suite replays them. `testdata/default-v1.json` fixes generated values. A compatible profile must retain them; updating the golden is an explicit profile decision (`UPDATE_MOCK_GOLDEN=1`).

Tests exercise all listed JSON types, formats, supported patterns, refs, nullability, choices, properties, numeric constraints and multiple seed/value combinations; negative constraints never yield partial results. Session tests use a fake clock for immediate expiry boundaries and real contexts for cancel/races. TCP gRPC tests use a local RSA JWKS double, signature/issuer/audience/caller/expiry rejection, key rotation/outage, exact large JSON numbers, duplicate/foreign subscriptions and a stalled reader. Additional isolated doubles exercise client-credentials refresh/outage and audience isolation, and real OTLP export failure while generation continues. Literal const/enum/examples/default values are checked against the published structural limits before compilation.

From backend repository:

```bash
go test ./...
go test -race ./internal/usecase/mock_data ./internal/adapter/mockgenerator ./internal/api/http/v1/mock_data ./internal/api/http/openapi ./test/architecture
go test -tags=e2e ./test/e2e -run '^TestMockUnavailableHTTPAndViewerPolicy$' -count=1
```

The tagged E2E uses only the existing guarded `test/support` disposable PostgreSQL container, generated test OIDC keys and fake Workbench. It verifies real HTTP middleware, Viewer/outsider/anonymous policies, unavailable Mock and retained Workbench access. It cannot take an external database DSN.

## Real local infrastructure

`./infra/dev.sh up mock` starts the generator and idempotently configures the local Keycloak audience. A full local backend uses `MOCK_GENERATOR_GRPC_TARGET=service-mock-generator:50052` from Compose.

With an existing Configurator local OIDC session, open `/src/test/features/backend-connections/mock-live-smoke.html` on the local Vite server and run the loopback-only check. This exercises actual cookie/user identity resolution, workspace access, backend service credentials, gRPC, JSON/SSE determinism, pause/resume, KeepAlive and stop. It creates only an ephemeral Mock session and synthetic data.

## Isolated adversarial harness

From Mock repository, with local Keycloak running:

```bash
make harness
```

This builds `test/harness` and backend `test/mock-harness`, chooses loopback ports, launches only owned processes and uses the real local `client_credentials` provider. Client actors/workspaces are explicit synthetic fixtures in the backend harness: it reuses the production access usecase/gateway/SSE handler but does not emulate the production database identity middleware. The separate local browser smoke and tagged backend E2E cover that boundary.

The harness performs 1000 concurrent lifecycle cycles, real TTL 180 s while data continues, foreign actor/workspace checks, quota saturation, strict patch/schema errors, abruptly disconnected/stopped readers, process restarts and at least 900 s mixed traffic. The client reuses a fixed per-thread HTTP connection pool; SSE connections remain separate. After process restarts it warms both servers before measuring the goroutine baseline, and preserves initial/final stacks. It samples RSS, heap, goroutines, sessions, jobs and pending bytes, verifies generated shape/value ranges and batch sequence, and checks cleanup to zero. It never restarts working application containers or accesses a database. Default output is `tmp/adversarial-report.json`; process logs contain no request payloads or tokens.

For the separate fault pass (without repeating the 180-second lease/15-minute interval):

```bash
python3 test/harness/adversarial.py --mock-bin ./tmp/mock-harness --backend-bin ../egorkozelskij-endge-service-backend/tmp/mock-backend-harness --cycles 1000 --duration 0 --skip-lease --output ./tmp/faults-report.json
```

The timed report and fault report establish bounded behavior for the tested workloads, not universal crash immunity. Terminal SSE delivery is best effort; cleanup is checked independently. A low-volume paused disconnect is detected by heartbeat/write within the configured bounds. Unary Fiber requests are bounded by RPC/generation deadlines; Fiber does not itself provide a per-request peer-disconnect context.

## Frontend

From Configurator repository:

```bash
pnpm exec vitest run src/test/features/backend-connections/service-versions-dialog.spec.ts
pnpm exec eslint src/features/backend-connections/ui/ServiceVersions_Dialog.vue src/test/features/backend-connections/service-versions-dialog.spec.ts src/i18n/locales/en.json src/i18n/locales/ru.json
pnpm exec vue-tsc --noEmit -p tsconfig.app.json
```

SSR contract tests cover a row per environment, old/missing metadata, unavailable environment, available version and loading state. Browser verification uses Help → Service versions against local and configured primary environments. It does not deploy a service to remote environments.

## Delivery boundary

Local service startup uses the prepared `service-kit-go` optional-PostgreSQL config change via `go.work`. Standalone compilation is checked with GOWORK=off; standalone startup/container delivery requires publication of that kit change and the corresponding dependency update. No release/push/deploy is implicit in these checks.
