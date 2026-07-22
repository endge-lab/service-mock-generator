# Test Strategy

Template содержит только инфраструктурный smoke-level набор тестов.

## Что проверяется в template

- package naming;
- базовые architecture boundaries;
- отсутствие reference business logic;
- общий error contract;
- Redpanda client wrapper в disabled/enabled режимах;
- наличие `docs/openapi3.yaml`;
- компиляция всех packages.

## Что должен добавить конкретный сервис

- unit tests для domain/usecase;
- repository integration tests;
- HTTP contract tests;
- e2e tests для основных user flows;
- migration tests для реальных бизнес-таблиц.
