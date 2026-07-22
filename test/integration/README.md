# Integration Tests

Здесь хранятся integration-тесты конкретного сервиса, которые поднимают реальные зависимости:

- PostgreSQL adapters, если сервис использует БД
- migrations, если сервис содержит бизнес-таблицы
- transaction boundaries
- взаимодействие infrastructure adapters <-> external systems
