# Техническое задание 2: потоковая генерация JSON-данных по WebSocket

## 1. Назначение документа

Этот документ описывает вторую задачу развития Mock Generator Service: постоянную потоковую генерацию JSON-значений по WebSocket.

Задача выполняется после реализации синхронного generator из [todo-1.md](./todo-1.md) и обязана переиспользовать его schema, generation profile, regex и relations semantics. WebSocket transport не должен реализовывать второй независимый generator.

Документ рассчитан на разработчика, не знакомого с каким-либо бизнес-приложением. Протокол универсален: клиент передаёт самостоятельную JSON Schema, generation options и скорость потока, после чего получает последовательность сгенерированных JSON batches.

## 2. Цель

Клиент должен иметь возможность:

1. Открыть одно постоянное WebSocket-соединение.
2. Передать schema произвольного корневого JSON-значения.
3. Передать generation options и relations из первого задания.
4. Указать скорость генерации.
5. Получать сгенерированные batches без повторных HTTP-запросов.
6. Изменять скорость активного потока.
7. Приостанавливать и возобновлять поток.
8. Атомарно заменять schema и generation configuration.
9. Останавливать поток, не обязательно закрывая WebSocket.

Тип одного корневого значения остаётся произвольным:

- object;
- array;
- string;
- number;
- integer;
- boolean;
- `null`.

Обычно batch содержит один root object либо небольшое количество root objects, но protocol не должен предполагать конкретную business structure.

## 3. Почему WebSocket, а не SSE

Server-Sent Events передаёт сообщения только от сервера к клиенту. Для команд клиента потребовались бы дополнительные REST endpoints и отдельная идентификация stream session.

В этой задаче клиент должен по одному соединению отправлять:

- `start`;
- `set-rate`;
- `pause`;
- `resume`;
- `replace`;
- `stop`.

Поэтому V1 streaming protocol реализуется через WebSocket.

SSE fallback, long polling и отдельный session REST API не входят в эту задачу.

## 4. Связь с первым заданием

WebSocket задача не меняет meaning JSON Schema и generation profile.

Необходимо переиспользовать из `todo-1.md`:

- JSON Schema Draft 2020-12;
- `$defs` и локальные `$ref`;
- primitive types;
- object, array и nullable generation;
- `const`, `enum`, `examples`, `default`;
- numeric constraints;
- formats;
- ограниченный regex subset;
- generation profile `default-v1`;
- deterministic seed;
- relations и dependency graph;
- capability validation;
- resource limits;
- post-generation validation;
- public diagnostics.

Различается только transport и lifecycle:

```text
REST:
request - generate N items - response - завершение

WebSocket:
connect - start - batch - batch - control commands - ... - stop/close
```

## 5. Границы задачи

Streaming service остаётся stateless относительно постоянного хранения:

- session существует только в памяти процесса;
- session уничтожается при закрытии WebSocket;
- reconnect создаёт новую session;
- сервер не сохраняет schema, PRNG state или sequence для последующего reconnect;
- сервер не предоставляет replay;
- сервер не обращается к backend или database;
- server restart завершает все активные streams.

В рамках одного активного соединения допустимо хранить ephemeral state:

- compiled schema;
- generation plan;
- relation plan;
- resolved seed;
- PRNG state;
- current sequence;
- current stream configuration;
- connection/session identity.

## 6. WebSocket endpoint

### 6.1. URL

```http
GET /api/v1/mock-data/stream
Connection: Upgrade
Upgrade: websocket
Sec-WebSocket-Protocol: mock-data.v1
```

Production URL обязан использовать TLS:

```text
wss://<host>/api/v1/mock-data/stream
```

Plain `ws://` разрешён только в локальной development environment.

### 6.2. Subprotocol

Клиент должен запросить subprotocol:

```text
mock-data.v1
```

Сервер должен:

- принять только поддерживаемый subprotocol;
- вернуть выбранный subprotocol в handshake;
- отклонить неизвестную версию protocol;
- не менять message contract без новой версии subprotocol.

### 6.3. Frame type

V1 использует только UTF-8 text frames с JSON object внутри.

Binary frames не поддерживаются. Получение binary frame является protocol error.

## 7. Общий envelope сообщений

Каждое application message является JSON object и содержит `type`.

Client command:

```json
{
  "type": "start",
  "requestId": "req-1",
  "payload": {}
}
```

Server message:

```json
{
  "type": "ack",
  "requestId": "req-1",
  "streamId": "stream-id",
  "payload": {}
}
```

Правила:

- `type` обязателен;
- `requestId` обязателен для client commands;
- `requestId` создаётся клиентом и используется для correlation;
- `requestId` должен быть непустой строкой не длиннее 128 символов;
- сервер возвращает тот же `requestId` в `ack` или command-level `error`;
- `streamId` отсутствует в ошибке первого `start`, если active stream ещё не создан;
- после успешного `start` server messages текущего stream содержат `streamId`;
- unknown fields envelope отклоняются;
- unknown message type возвращает protocol diagnostic;
- один client command получает ровно один terminal `ack` или `error`.

## 8. Stream state machine

Одна WebSocket connection поддерживает не более одного активного stream.

States:

```text
idle
configuring
running
paused
error
closed
```

Переходы:

```text
connect              - idle
idle + start         - configuring - running
running + pause      - paused
paused + resume      - running
running + replace    - configuring - running
paused + replace     - configuring - paused
running + stop       - idle
paused + stop        - idle
error + replace      - configuring - running
error + stop         - idle
any + socket close   - closed
fatal error          - closed
```

Недопустимая команда для текущего state возвращает `stream.invalid_state`, не изменяя state.

## 9. Команда `start`

### 9.1. Назначение

Создаёт configuration первого stream в state `idle`.

### 9.2. Формат

```json
{
  "type": "start",
  "requestId": "req-start-1",
  "payload": {
    "schema": {
      "$schema": "https://json-schema.org/draft/2020-12/schema",
      "type": "object",
      "properties": {
        "sequenceValue": {
          "type": "string",
          "pattern": "^SU[0-9]{3,4}$"
        }
      },
      "required": ["sequenceValue"],
      "additionalProperties": false
    },
    "generation": {
      "seed": "stream-preview-42",
      "profile": "default-v1",
      "optionalPropertyProbability": 0.7,
      "defaultArrayLength": 3,
      "relations": []
    },
    "stream": {
      "intervalMs": 1000,
      "itemsPerMessage": 1,
      "emitImmediately": true
    }
  }
}
```

### 9.3. Отличие от REST request

Поле `generation.count` отсутствует. Его streaming equivalent:

```text
stream.itemsPerMessage
```

На каждом tick generator создаёт `itemsPerMessage` независимых root values.

### 9.4. Stream configuration

| Путь | Тип | Обязательное | Default | Ограничение |
| --- | --- | --- | --- | --- |
| `stream.intervalMs` | integer | да | — | От `100` до `60000` ms |
| `stream.itemsPerMessage` | integer | да | — | От `1` до `100` |
| `stream.emitImmediately` | boolean | нет | `true` | Отправить первый batch сразу после `ack` |

Фактическая верхняя скорость равна:

```text
itemsPerMessage / intervalMs
```

Server limits могут быть строже конфигурации protocol.

### 9.5. Обработка `start`

До `ack` сервер обязан:

1. Проверить envelope.
2. Проверить schema по правилам первого задания.
3. Проверить regex subset.
4. Проверить relations.
5. Скомпилировать schema.
6. Построить reusable generation plan.
7. Разрешить или создать seed.
8. Создать stream session state.

Если validation не прошла, сервер отправляет `error`, остаётся в `idle` и не запускает scheduler.

## 10. Ответ `ack`

```json
{
  "type": "ack",
  "requestId": "req-start-1",
  "streamId": "b63b871c-2255-4fe6-95ed-b24575316b89",
  "payload": {
    "command": "start",
    "state": "running",
    "configVersion": 1,
    "seed": "stream-preview-42",
    "profile": "default-v1",
    "stream": {
      "intervalMs": 1000,
      "itemsPerMessage": 1,
      "emitImmediately": true
    }
  }
}
```

`streamId` создаётся сервером для каждого успешного `start` и остаётся неизменным при `set-rate`, `pause`, `resume` и `replace`. После `stop` active stream уничтожается. Новый `start` на той же connection получает новый `streamId` и начинает с `configVersion: 1`.

## 11. Сообщение `data`

### 11.1. Формат

```json
{
  "type": "data",
  "streamId": "b63b871c-2255-4fe6-95ed-b24575316b89",
  "payload": {
    "configVersion": 1,
    "sequence": 1,
    "generatedAt": "2026-07-22T17:50:00.123Z",
    "items": [
      {
        "sequenceValue": "SU1042"
      }
    ],
    "meta": {
      "count": 1,
      "seed": "stream-preview-42",
      "profile": "default-v1"
    }
  }
}
```

### 11.2. Поля

| Путь | Тип | Описание |
| --- | --- | --- |
| `payload.configVersion` | integer | Версия активной configuration |
| `payload.sequence` | integer | Номер data message внутри текущей configVersion, начиная с `1` |
| `payload.generatedAt` | string | RFC 3339 timestamp формирования batch |
| `payload.items` | array | Batch сгенерированных root JSON values |
| `payload.meta.count` | integer | Равно `itemsPerMessage` |
| `payload.meta.seed` | string | Resolved seed активной configuration |
| `payload.meta.profile` | string | Generation profile |

### 11.3. Произвольный root type

`payload.items` является transport batch. Каждый элемент внутри может иметь любой JSON type.

Object root:

```json
{
  "items": [
    { "id": 1 }
  ]
}
```

Array root:

```json
{
  "items": [
    [1, 2, 3]
  ]
}
```

Primitive root:

```json
{
  "items": [
    "SU100"
  ]
}
```

Наличие transport array `items` не означает, что schema должна описывать array.

## 12. Скорость и scheduling

### 12.1. Базовая модель

V1 использует sequential fixed-delay scheduling:

1. Сгенерировать batch.
2. Провалидировать весь batch поэлементно.
3. Отправить одно `data` message.
4. Дождаться `intervalMs`.
5. Повторить.

Нельзя запускать несколько ticks одной session параллельно.

### 12.2. Медленная генерация

Если generation и отправка занимают дольше `intervalMs`:

- ticks не накапливаются;
- пропущенные intervals не догоняются burst-отправкой;
- следующий delay начинается после завершения предыдущего send cycle;
- фактическая скорость становится ниже запрошенной;
- session остаётся последовательной и bounded.

### 12.3. Immediate first batch

При `emitImmediately: true`:

- сначала отправляется успешный `ack`;
- затем без initial delay формируется первый `data`;
- последующие batches используют `intervalMs`.

При `false` первый batch отправляется после первого interval.

## 13. Команда `set-rate`

Изменяет скорость без замены schema, seed, PRNG state, configVersion или sequence.

```json
{
  "type": "set-rate",
  "requestId": "req-rate-1",
  "payload": {
    "intervalMs": 500,
    "itemsPerMessage": 2
  }
}
```

Оба поля обязательны. Новая скорость применяется после завершения текущего send cycle.

Ответ:

```json
{
  "type": "ack",
  "requestId": "req-rate-1",
  "streamId": "b63b871c-2255-4fe6-95ed-b24575316b89",
  "payload": {
    "command": "set-rate",
    "state": "running",
    "configVersion": 1,
    "stream": {
      "intervalMs": 500,
      "itemsPerMessage": 2,
      "emitImmediately": true
    }
  }
}
```

## 14. Команды `pause` и `resume`

### 14.1. Pause

```json
{
  "type": "pause",
  "requestId": "req-pause-1",
  "payload": {}
}
```

После `ack` новые batches не создаются. Если batch уже генерируется, server должен либо корректно завершить его перед `ack`, либо отменить через context; выбранное поведение должно быть единообразным. Рекомендуется завершить текущий atomic batch, отправить его, затем подтвердить pause.

### 14.2. Resume

```json
{
  "type": "resume",
  "requestId": "req-resume-1",
  "payload": {}
}
```

Resume продолжает:

- тот же configVersion;
- тот же PRNG state;
- sequence со следующего номера;
- relations semantics без изменений.

Pause time не влияет на generated values.

## 15. Команда `replace`

### 15.1. Назначение

Атомарно заменяет schema, generation options и stream configuration без закрытия WebSocket.

```json
{
  "type": "replace",
  "requestId": "req-replace-1",
  "payload": {
    "schema": {
      "$schema": "https://json-schema.org/draft/2020-12/schema",
      "type": "array",
      "items": {
        "type": "integer",
        "minimum": 1,
        "maximum": 10
      },
      "minItems": 2,
      "maxItems": 2
    },
    "generation": {
      "seed": "replacement-seed",
      "profile": "default-v1",
      "relations": []
    },
    "stream": {
      "intervalMs": 2000,
      "itemsPerMessage": 1,
      "emitImmediately": true
    }
  }
}
```

### 15.2. Atomicity

Server должен:

1. Оставить текущую configuration активной во время validation новой.
2. Полностью скомпилировать schema и plans во временное состояние.
3. При ошибке отправить command-level `error` и продолжить старый stream.
4. При успехе остановить old scheduler в safe point.
5. Атомарно заменить active configuration.
6. Увеличить `configVersion`.
7. Сбросить sequence на `1` для новой configVersion.
8. Создать новый PRNG state из replacement seed.
9. Продолжить в прежнем running/paused state.

Нельзя сначала разрушить рабочую configuration, а затем обнаружить invalid replacement.

## 16. Команда `stop`

```json
{
  "type": "stop",
  "requestId": "req-stop-1",
  "payload": {}
}
```

После `ack`:

- scheduler остановлен;
- compiled schema и generation plan освобождены;
- state становится `idle`;
- WebSocket остаётся открытым;
- клиент может отправить новый `start`.

`stop` не закрывает connection. Для полного завершения клиент закрывает WebSocket normal close frame.

## 17. Determinism

### 17.1. PRNG lifecycle

Seed разрешается один раз при успешном `start` или `replace`.

PRNG state продолжается между `data` messages. Каждый следующий batch является продолжением одной детерминированной последовательности.

### 17.2. Что не должно влиять на данные

При неизменной configVersion на generated sequence не должны влиять:

- wall-clock time;
- network latency;
- pause duration;
- изменение `intervalMs`;
- скорость чтения клиента.

Изменение `itemsPerMessage` меняет grouping, но не должно менять flattened sequence сгенерированных root items.

### 17.3. Relations

Relations применяются независимо внутри каждого root item.

В V1 relation не может ссылаться:

- на root item из предыдущего batch;
- на root item из следующего batch;
- на другой item того же transport batch;
- на state предыдущей connection.

Поток является последовательностью независимых generated snapshots, а не evolving state model.

## 18. Error messages

### 18.1. Command-level error

```json
{
  "type": "error",
  "requestId": "req-start-1",
  "streamId": "b63b871c-2255-4fe6-95ed-b24575316b89",
  "payload": {
    "fatal": false,
    "code": "schema.pattern_unsupported",
    "message": "Pattern содержит неподдерживаемую конструкцию",
    "details": {
      "path": "/schema/properties/code/pattern"
    }
  }
}
```

Command-level validation error не закрывает connection.

### 18.2. Runtime generation error

Если active configuration неожиданно перестала генерировать валидные данные:

1. Остановить scheduler.
2. Перевести session в `error`.
3. Отправить `error` с `fatal: false`, если connection остаётся пригодной для нового `replace` или `stop`.
4. Не отправлять partial batch.

### 18.3. Fatal error

При нарушении frame protocol, повреждении connection или внутренней ошибке, после которой безопасная работа невозможна:

1. По возможности отправить `error` с `fatal: true`.
2. Закрыть WebSocket соответствующим close code.
3. Отменить все session goroutines и contexts.

### 18.4. Error codes

Дополнительно к codes первого задания:

```text
stream.protocol_unsupported
stream.message_invalid
stream.command_unknown
stream.invalid_state
stream.rate_invalid
stream.configuration_invalid
stream.backpressure
stream.generation_failed
stream.internal
```

## 19. WebSocket close codes

| Code | Использование |
| --- | --- |
| `1000` | Нормальное закрытие клиентом или сервером |
| `1002` | Нарушение WebSocket/application frame protocol |
| `1008` | Policy violation, invalid origin или authorization failure после upgrade невозможен |
| `1009` | Message превышает limit |
| `1011` | Неожиданная server error |
| `1013` | Client слишком медленно читает или server перегружен |

Перед upgrade обычные HTTP authentication/validation ошибки возвращаются HTTP status, а не WebSocket close code.

## 20. Backpressure

Нельзя создавать неограниченную очередь `data` messages.

V1 policy:

- один scheduler генерирует последовательно;
- outbound queue bounded;
- write deadline обязателен;
- если клиент систематически не успевает читать и queue/write deadline превышены, stream завершается;
- server отправляет `stream.backpressure`, если connection ещё writable;
- connection закрывается code `1013`;
- batches не накапливаются бесконечно;
- server не должен silently drop messages, потому что sequence должен оставаться однозначным.

Размер bounded queue задаётся config и должен быть малым. Значение по умолчанию, например `4`, необходимо подтвердить нагрузочными tests.

## 21. Keepalive и connection lifecycle

Server должен использовать WebSocket ping/pong:

- периодический ping;
- read deadline;
- продление deadline при pong;
- закрытие dead connection;
- отмена session context при disconnect.

Рекомендуемые configurable defaults:

| Параметр | Default |
| --- | --- |
| Ожидание первого `start` | 15 секунд |
| Ping interval | 30 секунд |
| Pong timeout | 10 секунд |
| Write timeout | 10 секунд |
| Максимальная session duration | 1 час либо platform policy |

WebSocket handler не должен оставлять goroutines после disconnect, failed start, stop или replace.

## 22. Limits

Дополнительно к limits первого задания:

| Limit | Recommended default |
| --- | --- |
| Connections на identity/IP | Configurable |
| Одновременных streams на connection | `1` |
| Client message | 2 MiB |
| Server data message | 20 MiB |
| `intervalMs` minimum | `100` |
| `intervalMs` maximum | `60000` |
| `itemsPerMessage` maximum | `100` |
| Outbound queue | Небольшой bounded buffer |
| Commands в секунду | Configurable rate limit |
| Schema replacements в минуту | Configurable rate limit |

Server имеет право отклонить configuration, если ожидаемый максимальный response превышает message limit.

## 23. Authentication, Origin и browser client

### 23.1. Origin

WebSocket server обязан проверять HTTP `Origin` во время handshake по configurable allowlist.

Нельзя использовать permissive origin policy в production.

### 23.2. Authentication

Browser WebSocket API не позволяет произвольно установить `Authorization` header. Deployment должен выбрать один явный вариант:

1. Same-origin secure HttpOnly cookie.
2. API gateway, который authenticates handshake.
3. Короткоживущий одноразовый stream ticket, полученный отдельным authenticated HTTP request.
4. Auth disabled только в контролируемой development environment.

Не рекомендуется передавать долгоживущий access token в query string: URL может попасть в access logs и telemetry.

Выбранная auth схема является infrastructure concern. Generator и streaming application layer не должны обращаться к business backend.

## 24. Clean Architecture

### 24.1. Переиспользование generator

REST и WebSocket должны вызывать один application generation boundary.

Запрещено:

- копировать recursion/schema traversal в WebSocket handler;
- иметь отдельный regex generator для stream;
- иметь отдельную relations implementation для stream;
- ослаблять post-generation validation для производительности без отдельного решения.

### 24.2. Ответственность слоёв

#### Domain/application generation

Остаётся реализованным в первом задании и не знает о WebSocket.

#### Streaming application service

Отвечает за:

- state machine;
- active configuration;
- sequencing;
- pause/resume/stop;
- atomic replace;
- scheduling policy;
- session context cancellation;
- вызов batch generation use case.

Не импортирует Fiber WebSocket types.

#### WebSocket transport adapter

Отвечает за:

- upgrade;
- subprotocol;
- frame reading/writing;
- JSON DTO;
- command correlation;
- ping/pong;
- close codes;
- transport deadlines;
- mapping public errors.

#### Infrastructure

Предоставляет конкретный WebSocket adapter, clock/ticker и connection-level telemetry.

### 24.3. Рекомендуемая структура

```text
internal/
  api/websocket/v1/mockdata/
    dto.go
    handler.go
    reader.go
    writer.go
    mapper.go

  application/stream/
    command.go
    event.go
    session.go
    state.go
    scheduler.go
    errors.go

  application/generate/
    ... reused from todo-1 ...

  infrastructure/websocket/
    adapter.go
```

Не создавать persistence repository для ephemeral sessions.

## 25. Concurrency model

Одна connection должна иметь понятную ownership model.

Рекомендуется:

- один reader loop;
- один writer loop;
- один session/scheduler loop;
- bounded channels между ними;
- только writer loop пишет frames;
- commands сериализуются session loop;
- replace и stop выполняются через cancellation/safe point;
- закрытие connection выполняется один раз через единый coordinator.

Нельзя допускать конкурентную запись WebSocket frames из нескольких goroutines.

## 26. Наблюдаемость

Metrics:

```text
mock_stream_connections_active
mock_stream_connections_total
mock_stream_started_total
mock_stream_stopped_total
mock_stream_batches_total
mock_stream_items_total
mock_stream_errors_total
mock_stream_backpressure_total
mock_stream_generation_duration_seconds
mock_stream_message_size_bytes
```

Span/log attributes:

```text
stream.id
stream.state
stream.config_version
stream.interval_ms
stream.items_per_message
generation.profile
relation.count
```

Не логировать:

- schema body;
- generated items;
- enum/examples/const values;
- auth token/ticket;
- полный WebSocket URL с sensitive query parameters;
- seed, если он может содержать пользовательский identifier.

## 27. Документация protocol

Добавить отдельный protocol document рядом с OpenAPI, содержащий:

- WebSocket URL;
- subprotocol;
- handshake requirements;
- все client commands;
- все server messages;
- state machine;
- close codes;
- полный start/data/replace flow;
- limits;
- authentication deployment notes.

OpenAPI может документировать handshake endpoint только частично. Message protocol должен иметь самостоятельную machine-readable или строго структурированную документацию; при выборе AsyncAPI она должна соответствовать реальному protocol и не заменять examples из этого задания.

## 28. Тестирование

### 28.1. Unit tests

- state transitions;
- invalid command/state combinations;
- scheduler fixed-delay behavior;
- immediate/delayed first batch;
- set-rate без reset PRNG;
- pause/resume без потребления generated values;
- stop cleanup;
- replace success;
- replace validation failure с сохранением old config;
- sequence/configVersion rules;
- deterministic flattened sequence при изменении batch size;
- relations внутри каждого streamed root item;
- root object, root array и primitive root;
- context cancellation;
- backpressure policy.

### 28.2. Protocol tests

- successful upgrade;
- missing/unsupported subprotocol;
- text JSON frame;
- malformed JSON;
- binary frame rejection;
- unknown message type;
- requestId correlation;
- start/ack/data;
- command-level error;
- stop без закрытия socket;
- повторный start после stop;
- ping/pong timeout;
- close codes;
- message size limit;
- Origin validation;
- authentication handshake.

### 28.3. Integration tests

- реальное WebSocket connection;
- несколько data messages;
- rate change;
- pause/resume;
- schema replace;
- client disconnect во время generation;
- slow reader/backpressure;
- отсутствие goroutine leak;
- shutdown сервиса при активных connections.

### 28.4. Invariants

Для каждого `data` message:

- `items` содержит ровно `itemsPerMessage` values;
- каждый item проходит active schema;
- relations выполнены;
- `sequence` монотонен внутри configVersion;
- partial batch отсутствует;
- message принадлежит current configVersion;
- после успешного replace не появляются data старой configVersion.

## 29. Этапы реализации

1. Убедиться, что REST generator из `todo-1.md` имеет reusable application boundary.
2. Зафиксировать WebSocket subprotocol и DTO.
3. Реализовать stream state machine без transport dependency.
4. Реализовать scheduler и deterministic sequencing.
5. Реализовать start/stop.
6. Реализовать data messages.
7. Реализовать set-rate.
8. Реализовать pause/resume.
9. Реализовать atomic replace.
10. Реализовать WebSocket transport adapter.
11. Добавить ping/pong, deadlines и close codes.
12. Добавить bounded writer/backpressure.
13. Добавить Origin/auth handshake policy.
14. Добавить telemetry.
15. Добавить protocol/unit/integration tests.
16. Обновить README и protocol documentation.
17. Выполнить formatting, static analysis и `go test ./...`.

## 30. Git workflow и Conventional Commits

Все изменения выполняются в ветке:

```text
develop
```

Не коммитить реализацию напрямую в `main`.

Commits должны быть атомарными и соответствовать Conventional Commits.

Примеры:

```text
feat(stream): define websocket protocol
feat(stream): add connection state machine
feat(stream): emit generated data batches
feat(stream): support runtime rate changes
feat(stream): replace schema atomically
fix(stream): stop scheduler on disconnect
test(stream): cover backpressure and cancellation
docs(stream): document websocket messages
```

## 31. Критерии приёмки

Задача считается выполненной, когда:

1. Реализован WebSocket endpoint `/api/v1/mock-data/stream`.
2. Поддержан subprotocol `mock-data.v1`.
3. Streaming использует тот же generator, schema subset, regex и relations, что REST.
4. Поддержаны arbitrary root object, array и primitives.
5. Реализованы `start`, `set-rate`, `pause`, `resume`, `replace`, `stop`.
6. `start` полностью валидирует configuration до запуска scheduler.
7. `replace` атомарен и не разрушает old stream при invalid replacement.
8. `data` batches соответствуют active schema.
9. Relations выполняются независимо внутри каждого root item.
10. Seed и PRNG продолжаются между messages детерминированно.
11. Pause и rate changes не меняют flattened generated sequence.
12. ConfigVersion и sequence соответствуют contract.
13. Один stream не запускает parallel ticks.
14. Backpressure bounded и не создаёт memory growth.
15. Disconnect отменяет session и не оставляет goroutines.
16. Origin/auth handshake обрабатывается безопасно.
17. Errors и close codes соответствуют protocol.
18. Protocol documentation соответствует реализации.
19. Tests покрывают state machine, lifecycle, timing, replacement и backpressure.
20. `go test ./...` проходит.
21. Изменения находятся в `develop` и commits соответствуют Conventional Commits.

## 32. Что явно не входит в эту задачу

- SSE implementation;
- long polling;
- несколько одновременных streams в одной connection;
- replay;
- resume после reconnect;
- persisted sessions;
- delivery guarantee между reconnects;
- cross-message relations;
- evolving mutable dataset;
- client acknowledgements каждого data message;
- binary protocol;
- compression-specific contract;
- external schema loading;
- database или business backend integration;
- distributed coordination streams между несколькими service instances.

Если позднее потребуется stateful evolving dataset или resume/replay, это должно стать отдельным protocol version и отдельной архитектурной задачей.

## 33. Внешние спецификации и документация

- WebSocket RFC 6455: <https://www.rfc-editor.org/rfc/rfc6455>
- JSON Schema Draft 2020-12: <https://json-schema.org/draft/2020-12>
- JSON Pointer RFC 6901: <https://www.rfc-editor.org/rfc/rfc6901>
- Conventional Commits: <https://www.conventionalcommits.org/en/v1.0.0/>
