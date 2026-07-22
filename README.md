# Endge Service Template Go

`service-template-go` - эталонный минимальный Go microservice template для Endge-сервисов.

Шаблон содержит только инфраструктурный скелет:

- HTTP на Fiber;
- DI через `fx`;
- config через `service-kit-go`;
- logger;
- OpenTelemetry middleware/providers;
- optional JWT/JWKS auth middleware;
- optional Redpanda/Kafka client;
- `/health` и `/version`;
- Swagger/Scalar в non-production окружениях;
- единый JSON-формат ошибок;
- architecture tests для защиты базовой структуры.

В шаблоне намеренно нет бизнес-usecase, repository layer и бизнес-миграций. Новый сервис должен добавлять их самостоятельно под свою предметную область.

## Как использовать

1. Скопируйте репозиторий или создайте новый сервис на его основе.
2. Замените module path в `go.mod`:

   ```go
   module github.com/your-org/your-service
   ```

3. Замените импорты `github.com/endge-lab/service-template-go/internal/...` на module path нового сервиса.
4. Скопируйте env-файл:

   ```bash
   cp .env.development.example .env.development
   ```

5. Настройте `APP_NAME`, `PUBLIC_URL`, `CORS_ALLOWED_ORIGINS` и `POSTGRES_*`, если сервису нужна БД.
6. Запустите тесты:

   ```bash
   go test ./...
   ```

7. Запустите сервис:

   ```bash
   make run
   ```

## API

Технические endpoints:

```text
GET /health
GET /version
GET /swagger
GET /swagger/openapi3.yaml
```

Бизнесовые endpoints нового сервиса должны добавляться под:

```text
/api/v1
```

## Config

`service-kit-go` загружает `.env.*`, затем читает YAML-конфиг из `configs/<APP_ENV>.yaml`.

В шаблоне есть безопасные дефолты:

```text
configs/development.yaml
configs/production.yaml
```

Env-переменные должны переопределять значения YAML.

## Auth

Auth опционален. По умолчанию:

```env
AUTH_ENABLED=false
```

Чтобы включить JWT/JWKS auth:

```env
AUTH_ENABLED=true
AUTH_SERVICE_URL=https://auth.example.com
AUTH_ISSUER=https://auth.example.com
AUTH_ALLOWED_AUDIENCES=your-audience
```

## Redpanda/Kafka

Redpanda опциональна. По умолчанию:

```env
REDPANDA_ENABLED=false
```

Включайте ее только в event-driven сервисах:

```env
REDPANDA_ENABLED=true
REDPANDA_BROKERS=redpanda:9092
REDPANDA_CLIENT_ID=your-service
```

## Telemetry

Telemetry опциональна. По умолчанию:

```env
TELEMETRY_ENABLED=false
OTEL_EXPORTER_OTLP_ENDPOINT=
```

Если нужен OpenTelemetry export:

```env
TELEMETRY_ENABLED=true
OTEL_EXPORTER_OTLP_ENDPOINT=otel-collector:4317
OTEL_EXPORTER_OTLP_INSECURE=true
```

## Добавление бизнес-логики

Рекомендуемый порядок:

1. Добавить domain entities/valueobjects/errors.
2. Добавить usecase ports в application layer.
3. Добавить repository implementation в infrastructure layer.
4. Добавить HTTP transport в `internal/api/http/v1`.
5. Зарегистрировать зависимости в `internal/bootstrap`.
6. Добавить миграции только для реальных бизнес-таблиц сервиса.

Usecase слой не должен импортировать postgres или HTTP packages.

## Проверки

```bash
go test ./...
docker compose --env-file .env.development config
GOWORK=off go test ./...
```
