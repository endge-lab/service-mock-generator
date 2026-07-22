# Template Architecture Checklist

## Цель

Зафиксировать, что template остается инфраструктурным скелетом без reference business feature.

## Проверки

| Проверка | Приоритет | Файл |
| --- | --- | --- |
| `docs/openapi3.yaml` существует | P0 | `test/architecture/architecture_test.go` |
| HTTP слой живет в `internal/api/http/v1` | P0 | `test/architecture/architecture_test.go` |
| Bootstrap собирает приложение через `fx` | P0 | `test/architecture/architecture_test.go` |
| Template не содержит reference business feature, usecase, repo, services и ports | P0 | `test/architecture/architecture_test.go` |
| Domain не зависит от HTTP/Postgres/Fiber | P0 | `test/architecture/architecture_test.go` |
