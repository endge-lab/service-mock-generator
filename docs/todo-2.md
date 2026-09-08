# Mock Generator V1: gRPC, backend HTTP/SSE and session lifecycle

This replaces the earlier WebSocket proposal. Generation semantics remain in [todo-1](todo-1.md); no business HTTP/WebSocket endpoint exists on the generator.

## Identity and transport ownership

Clients authenticate with the existing backend user JWT and `X-Endge-Workspace`. Viewer is sufficient. Backend derives actor/workspace from validated context and passes it with a separate `client_credentials` token. Request `ownerId` is rejected. Generator validates issuer, JWKS, audience `endge-mock-generator` and allowed caller `endge-service-backend`; production requires TLS. Workbench has an independent audience/provider instance.

Generator owns canonical `api/proto/mockdata/v1/mockdata.proto`. RPCs: `GetServiceInfo`, `GetCapabilities`, `Generate`, `CreateStream`, `GetStream`, `SubscribeStream` (server stream), `UpdateStream`, `KeepAliveStream`, `StopStream`. JSON payloads are bytes, never protobuf double/Struct. Generated server and backend client must match the canonical hash and generated descriptors.

## Backend routes

All paths are under `/api/v1/mock-data`:

| Method/path | Result |
| --- | --- |
| `GET /capabilities` | Availability, canRun, protocol, profiles and effective limits |
| `POST /generate` | `{items,meta:{count,seed,profile}}` |
| `POST /streams` | 201 prepared stream and relative eventsUrl |
| `GET /streams/{id}` | Owned session state |
| `GET /streams/{id}/events` | Sole authorized SSE subscription |
| `PATCH /streams/{id}` | Validated mutable parameters and increased parametersVersion |
| `POST /streams/{id}/keepalive` | Renewed active lease, after upstream acknowledgement |
| `DELETE /streams/{id}` | 204, cancel/cleanup |

An unknown/foreign/expired ID returns 404. Second subscription returns 409. Invalid input/unsupported schemas use 400, body/result limits 413, admission capacity 429, unavailable service 503, deadlines 504. Errors use backend `{code,message,details?}`; schema diagnostics include `details.path`. Capabilities report `available:false,canRun:false` when unconfigured/unavailable. `/version.services` still includes the known unavailable service and HTTP status remains 200.

## Examples

Synchronous request:

```json
{"schema":{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"integer","minimum":1,"maximum":100},"generation":{"count":3,"seed":"demo","profile":"default-v1"}}
```

Create (same generation options except count):

```json
{"schema":{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"integer","minimum":1,"maximum":100},"generation":{"seed":"demo"},"stream":{"intervalMs":1000,"itemsPerMessage":10,"emitImmediately":true}}
```

201 response includes `id`, `status:"ready"`, resolved `seed`, `profile`, `parameters`, `parametersVersion:1`, `sequence:0`, `idleTimeoutMs:180000`, `expiresAt` and `eventsUrl:"/api/v1/mock-data/streams/<public-id>/events"`. The public ID differs from the internal generator ID.

PATCH accepts only:

```json
{"intervalMs":500,"itemsPerMessage":20,"paused":false}
```

No schema, seed, count, relations or emitImmediately changes are accepted. Invalid/empty patch preserves current settings. Pause/resume requires an active subscription. A new schema needs a new session.

## Lease and cleanup

Create compiles but generates nothing. Ready expires after 30 seconds with no subscription; KeepAlive cannot extend ready. First subscription starts running and a 180-second client lease; concurrent subscriptions conflict. Only successful KeepAlive renews that lease, including paused sessions. Recommended client cadence: 30 seconds. PATCH, data and heartbeat never renew it.

Both processes check expiry on every session operation and reap once per second. Backend mirrors only public access/cancellation state and never advertises a lease longer than the generator. Disconnect, explicit stop, deadline, generation error, expiry and shutdown cancel work and remove sessions idempotently. Lost create responses are bounded by ready expiry. Restart drops state; old IDs cannot be replayed or revived.

## SSE framing and pacing

Use the authorized fetch/SSE adapter with headers, not a bare EventSource that cannot attach authorization. Backend sends default SSE `message` events:

```text
data: {"type":"started","streamId":"public-id","parametersVersion":1}

data: {"type":"data","streamId":"public-id","sequence":1,"parametersVersion":1,"items":[1,2,3]}

: heartbeat

```

All events use the same envelope; terminal types are `completed` or `failed` with safe `errorCode`/`errorMessage`. Terminal delivery is best effort. `Last-Event-ID` is rejected; there is no replay, automatic recreation or continuity across restarts.

Every batch is fully generated, related and validated before emission. Sequence increases per batch. Changing batch size changes grouping, not the concatenated value sequence. Scheduler waits intervalMs after transmission; overload can reduce actual frequency. Pause consumes no PRNG values. Parameters apply between complete batches. Heartbeat comments occur every 15 seconds, including pause. Response has no full buffering, has no-cache/no-transform and X-Accel-Buffering:no; writes and pending byte budgets are bounded.

## Limits and deployment

Defaults: 180 s client lease, 30 s ready lease, 10 s unary/batch/write timeout; body 2 MiB; output 20 MiB; count 1–1000; itemsPerMessage 1–100; intervalMs 100–60000; 5 sessions per actor across workspaces; 32 sessions globally; 4 generation jobs; 64 MiB pending bytes. The structural generation limits remain in todo-1. No unbounded job queue exists.

Generator env settings are documented in README. Backend settings: `MOCK_GENERATOR_GRPC_TARGET`, `MOCK_GENERATOR_AUDIENCE`, `MOCK_GENERATOR_REQUEST_TIMEOUT`, `MOCK_GENERATOR_HEALTH_TIMEOUT` (2s), `MOCK_GENERATOR_HEALTH_CACHE_TTL` (5s), `MOCK_GENERATOR_TLS_*`; gateway settings `MOCK_GATEWAY_SESSIONS`, `SESSIONS_PER_OWNER`, `REQUEST_BYTES`, `BUFFER_BYTES`, `IDLE_TIMEOUT`, `READY_TIMEOUT`, `WRITE_TIMEOUT` (each prefixed `MOCK_GATEWAY_`). The service identity client uses the shared `SERVICE_AUTH_*` configuration with an independent provider.

Local Compose exposes no Mock host ports. `infra/dev.sh up mock` has no PostgreSQL preflight. A local Keycloak mapper update only changes the backend client's Mock audience; realm/users/volumes remain intact. Remote environments require separate deployment. Frontend scope is only the service-version dialog.

## Verification

See [tests and harness](tests/README.md). Ordinary tests use local test doubles, never working `.env`, DB or external IdP. Infrastructure E2E is separate. Destructive operations affect only harness-owned processes. The report must distinguish unit/race/fuzz/static evidence, isolated network evidence and real local-stack evidence.
