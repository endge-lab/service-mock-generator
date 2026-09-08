# Implementation map

Canonical architecture is in root governance (`backend/applications/service-mock-generator`). This document describes the implementation.

- `internal/domain/valueobjects`: strict request DTOs, options, relations, resource limits and owner identity.
- `internal/domain/entities`: stream snapshot and event envelope.
- `internal/usecase/generate`: preflight, bounded schema graph, deterministic profile, relations, reusable plan and cursor; one admission gate for compile and generation.
- `internal/usecase/stream`: synchronized session map, prepared/active leases, scheduler, parameter updates and idempotent cleanup. No global lock spans compilation, generation or network operations.
- `internal/usecase/ports`: schema compiler/validator contract using JSON/domain values only.
- `internal/platform/schema`: jsonschema/v6 Draft 2020-12 adapter; external loading disabled and supported formats asserted.
- `internal/api/grpc/v1`: canonical RPC DTO conversion, owner validation, error mapping and bounded streaming writes.
- `internal/api/http/v1`: technical health/version/docs only.
- `internal/bootstrap`: fx wiring, OIDC verification, gRPC/TLS, technical HTTP, telemetry and shutdown.
- `test/harness`: isolated loopback test executable with introspection; never wired into production.

Backend owns public stream IDs, workspace access and SSE relay. It cannot generate values or restore a generator cursor. A new schema requires a new stream. Both processes forget sessions on restart.
