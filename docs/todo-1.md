> Актуальный transport: этот генератор вызывается внутренним gRPC API `mockdata.v1`; HTTP endpoint из примеров предоставляется service-backend. Нормативная архитектура — root governance/backend. Сессии и SSE описаны в todo-2.md.

# Техническое задание 1: синхронная генерация JSON-данных по schema

## 1. Назначение документа

Этот документ является полным техническим заданием на разработку с нуля отдельного HTTP-сервиса, который генерирует JSON-данные по переданной клиентом JSON Schema.

Документ не предполагает знания какого-либо бизнес-приложения, его предметной области, внутренних сущностей или других репозиториев. Сервис должен оставаться универсальным: любой HTTP-клиент может передать поддерживаемую JSON Schema и получить заданное количество соответствующих ей JSON-значений.

## 2. Цель сервиса

Сервис должен:

1. Принять по HTTP:
   - самостоятельную JSON Schema;
   - количество требуемых значений;
   - необязательные параметры детерминированной генерации.
2. Проверить корректность HTTP-запроса.
3. Скомпилировать и провалидировать JSON Schema.
4. Убедиться, что schema использует только поддерживаемое в V1 подмножество JSON Schema.
5. Сгенерировать указанное количество независимых JSON-значений.
6. Провалидировать каждое сгенерированное значение исходной schema.
7. Вернуть значения и метаданные генерации клиенту.

Сервис должен быть stateless: один запрос содержит всё необходимое для генерации и не зависит от предыдущих запросов.

## 3. Основные архитектурные ограничения

Сервис не должен:

- обращаться к backend-приложению за типами или данными;
- обращаться к базе данных;
- хранить schemas между запросами;
- создавать собственный постоянный реестр типов;
- загружать schemas по внешним URL;
- исполнять JavaScript, Go-код, шаблоны или любые выражения из входной schema;
- знать бизнесовые названия, идентификаторы или сущности клиента;
- зависеть от исходного формата, из которого клиент получил JSON Schema.

Сервису разрешены только:

- входящий HTTP-запрос;
- локальная обработка в памяти;
- возврат HTTP-ответа;
- технические логи, metrics и traces без сохранения полного содержимого пользовательских schemas и сгенерированных данных.

## 4. Термины

### 4.1. JSON Schema

Стандартное декларативное описание допустимой структуры JSON-значения. В V1 сервис принимает dialect:

```text
https://json-schema.org/draft/2020-12/schema
```

### 4.2. Именованный тип

Переиспользуемая subschema, объявленная внутри `$defs` корневого schema document.

### 4.3. Локальная ссылка

Ссылка `$ref`, которая указывает на элемент того же schema document:

```json
{
  "$ref": "#/$defs/Airport"
}
```

### 4.4. Корневая schema

Schema, описывающая одно итоговое JSON-значение. Именно это значение сервис генерирует `count` раз.

Корневая schema может:

- сама описывать `object`, `array` или primitive;
- ссылаться на один тип из `$defs`;
- собирать новый объект из нескольких переиспользуемых типов.

Сервис не предполагает, что корневое значение является массивом объектов. Допустимы, например:

- один primitive;
- один object;
- один array;
- object с несколькими вложенными objects и arrays;
- array primitives;
- object, чьи вложенные collections и одиночные objects связаны между собой relations.

`generation.count` определяет количество независимых корневых значений, а не размер вложенных массивов. Размер каждого вложенного массива определяется его собственной schema.

### 4.5. Generation profile

Версионированный набор правил, определяющий, как из множества допустимых schema значений выбрать конкретное значение. Первый profile имеет identity:

```text
default-v1
```

Версия profile нужна потому, что JSON Schema определяет ограничения и валидацию, но не задаёт алгоритм генерации данных.

### 4.6. Relation

Декларативное правило согласования уже генерируемых частей одного корневого JSON-значения. Relation выбирает source object или один object из source collection и копирует указанные значения в один или несколько target objects.

Relations не заменяют JSON Schema. Schema продолжает определять форму и допустимые значения, а relations добавляют cross-object constraints, которые стандартная JSON Schema не выражает.

## 5. Примитивные JSON-типы

Сервис должен поддерживать все примитивные категории JSON Schema, соответствующие JSON data model.

| JSON Schema type | Значение | Примеры |
| --- | --- | --- |
| `null` | Пустое JSON-значение | `null` |
| `boolean` | Логическое значение | `true`, `false` |
| `integer` | Целое число | `0`, `-10`, `42` |
| `number` | Любое конечное JSON-число | `0`, `1.5`, `-3.14` |
| `string` | Unicode-строка | `"text"`, `"Москва"` |
| `array` | Упорядоченный массив JSON-значений | `[1, 2, 3]` |
| `object` | JSON-объект с именованными properties | `{"id": 1}` |

Особые правила:

- `integer` является более узким числовым типом, чем `number`;
- `NaN`, `Infinity` и `-Infinity` не являются допустимыми JSON-числами;
- отдельного primitive `ID`, `UUID`, `Date` или `DateTime` в JSON Schema нет;
- такие значения описываются через `type: "string"` и `format`;
- nullable-значение в V1 задаётся через `type` с `null`, например `{"type": ["string", "null"]}`;
- в V1 массив `type` поддерживается только для комбинации одного основного типа с `null`.

Примеры:

```json
{
  "type": "string",
  "format": "uuid"
}
```

```json
{
  "type": ["integer", "null"]
}
```

## 6. HTTP API

### 6.1. Endpoint

```http
POST /api/v1/mock-data/generate
Content-Type: application/json
Accept: application/json
```

Endpoint синхронный. Job queue, polling и асинхронные задания в V1 не требуются.

### 6.2. Формат запроса

```json
{
  "schema": {},
  "generation": {
    "count": 1,
    "seed": "optional-seed",
    "profile": "default-v1",
    "optionalPropertyProbability": 0.7,
    "defaultArrayLength": 3,
    "relations": []
  }
}
```

### 6.3. Поля верхнего уровня

| Путь | Тип | Обязательное | Правило |
| --- | --- | --- | --- |
| `schema` | object | да | JSON Schema Draft 2020-12 |
| `generation` | object | да | Параметры одного запуска генерации |
| `generation.count` | integer | да | От `1` до `1000` включительно |
| `generation.seed` | string | нет | От `1` до `256` Unicode-символов |
| `generation.profile` | string | нет | В V1 допускается только `default-v1`; это значение используется по умолчанию |
| `generation.optionalPropertyProbability` | number | нет | От `0` до `1`; по умолчанию `0.7` |
| `generation.defaultArrayLength` | integer | нет | От `0` до `100`; по умолчанию `3` |
| `generation.relations` | array | нет | Правила связей внутри каждого корневого результата; по умолчанию пустой массив |

Неизвестные поля request envelope должны отклоняться. Это позволяет обнаруживать опечатки и несовместимые версии клиента.

### 6.4. Seed

Если `seed` передан, одинаковая комбинация из:

- canonical schema;
- generation options;
- seed;
- profile;

должна приводить к одинаковому результату.

Если `seed` отсутствует, сервис должен:

1. создать криптографически случайный seed;
2. использовать его для текущей генерации;
3. вернуть его в response metadata.

Алгоритм PRNG и преобразование строкового seed должны быть зафиксированы реализацией profile `default-v1`. Изменение алгоритма, нарушающее воспроизводимость, требует нового profile, например `default-v2`.

### 6.5. Полный пример запроса

```json
{
  "schema": {
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    "$defs": {
      "Airport": {
        "type": "object",
        "properties": {
          "code": {
            "type": "string",
            "examples": ["SVO", "LED", "DME"]
          },
          "name": {
            "type": "string",
            "minLength": 3,
            "maxLength": 80
          }
        },
        "required": ["code", "name"],
        "additionalProperties": false
      },
      "Flight": {
        "type": "object",
        "properties": {
          "id": {
            "type": "string",
            "format": "uuid"
          },
          "number": {
            "type": "string",
            "examples": ["SU100", "SU101"]
          },
          "departureAirport": {
            "$ref": "#/$defs/Airport"
          },
          "arrivalAirport": {
            "$ref": "#/$defs/Airport"
          },
          "scheduledAt": {
            "type": "string",
            "format": "date-time"
          },
          "delayMinutes": {
            "type": ["integer", "null"],
            "minimum": 0,
            "maximum": 240
          }
        },
        "required": [
          "id",
          "number",
          "departureAirport",
          "arrivalAirport",
          "scheduledAt"
        ],
        "additionalProperties": false
      }
    },
    "type": "object",
    "properties": {
      "mainFlight": {
        "$ref": "#/$defs/Flight"
      },
      "relatedFlights": {
        "type": "array",
        "items": {
          "$ref": "#/$defs/Flight"
        },
        "minItems": 2,
        "maxItems": 5
      },
      "baseAirport": {
        "$ref": "#/$defs/Airport"
      }
    },
    "required": ["mainFlight", "relatedFlights", "baseAirport"],
    "additionalProperties": false
  },
  "generation": {
    "count": 1,
    "seed": "preview-42",
    "profile": "default-v1",
    "optionalPropertyProbability": 0.7,
    "defaultArrayLength": 3
  }
}
```

## 7. Переиспользование типов

Переиспользуемые types объявляются один раз внутри `$defs`:

```json
{
  "$defs": {
    "Address": {
      "type": "object",
      "properties": {
        "city": { "type": "string" },
        "street": { "type": "string" }
      },
      "required": ["city", "street"]
    }
  }
}
```

После этого type можно использовать в нескольких местах:

```json
{
  "billingAddress": {
    "$ref": "#/$defs/Address"
  },
  "deliveryAddress": {
    "$ref": "#/$defs/Address"
  }
}
```

`$defs` обеспечивает переиспользование внутри одного schema document и одного HTTP-запроса.

V1 не предоставляет API вида «зарегистрировать type один раз и использовать его в будущих запросах». Такое API потребовало бы stateful type registry, хранения, versioning и lifecycle управления schemas, что не входит в задачу.

## 8. Поддерживаемое подмножество JSON Schema Draft 2020-12

Сервис не обязан генерировать данные по всем возможностям JSON Schema Draft 2020-12. Внешний формат остаётся стандартным JSON Schema, но V1 поддерживает только явно перечисленные keywords.

### 8.1. Общие keywords

| Keyword | Поддержка | Правило V1 |
| --- | --- | --- |
| `$schema` | обязательно | Только URI Draft 2020-12 |
| `$defs` | да | Object с именованными subschemas |
| `$ref` | да | Только локальные ссылки `#/$defs/...` |
| `$id` | нет | Не требуется для локального self-contained document |
| `$anchor` | нет | Отклонять как неподдерживаемый keyword |
| `$dynamicRef` | нет | Отклонять |
| `$dynamicAnchor` | нет | Отклонять |
| `title` | да | Annotation, не влияет на генерацию |
| `description` | да | Annotation, не влияет на генерацию |
| `default` | да | Использовать как fallback перед type-based generation, если значение валидно |
| `examples` | да | Использовать как источник значений, если examples валидны |
| `deprecated` | да | Annotation, не влияет на генерацию |
| `readOnly` | да | Annotation, не влияет на генерацию |
| `writeOnly` | да | Annotation, не влияет на генерацию |

В V1 schema должна быть JSON object. Boolean schemas `true` и `false` необходимо отклонять как неподдерживаемые для генерации.

### 8.2. Universal value keywords

| Keyword | Поддержка | Правило |
| --- | --- | --- |
| `const` | да | Всегда вернуть это значение |
| `enum` | да | Выбрать один из элементов |
| `type` | да | Один type либо основной type вместе с `null` |
| `oneOf` | да | Выбрать одну ветку и проверить, что результат соответствует ровно одной ветке |
| `anyOf` | нет | Отклонять в V1 |
| `allOf` | нет | Отклонять в V1 |
| `not` | нет | Отклонять в V1 |
| `if` / `then` / `else` | нет | Отклонять в V1 |

### 8.3. Object keywords

| Keyword | Поддержка | Правило |
| --- | --- | --- |
| `properties` | да | Генерировать объявленные properties |
| `required` | да | Все required properties присутствуют всегда |
| `additionalProperties` | да | `false`, `true` или поддерживаемая schema значений словаря; при `true` дополнительные поля не генерируются |
| `minProperties` | да | Добирать optional properties до минимума |
| `maxProperties` | да | Не превышать максимум; required должны помещаться в максимум |
| `patternProperties` | нет | Отклонять |
| `propertyNames` | нет | Отклонять |
| `dependentRequired` | нет | Отклонять |
| `dependentSchemas` | нет | Отклонять |
| `unevaluatedProperties` | нет | Отклонять |

Правила:

- required property, отсутствующий в `properties`, является ошибкой generation contract;
- если количество required properties превышает `maxProperties`, schema негенерируема;
- optional properties обрабатываются в стабильном лексикографическом порядке;
- каждое optional property включается с вероятностью `optionalPropertyProbability`;
- если после случайного выбора не выполнен `minProperties`, сервис добавляет optional properties в стабильном порядке;
- `additionalProperties: true` не требует придумывать неизвестные свойства.
- Schema в `additionalProperties` компилируется с теми же лимитами, `$ref` и проверкой циклов, что обычные поля. Обязательные ключи вне `properties` получают эту schema.
- Для типизированного словаря без именованных полей генерируется до трёх ключей `key_1`, `key_2`, `key_3`; `minProperties` и `maxProperties` имеют приоритет. Для смешанного объекта дополнительные ключи заполняют недостающий минимум, не заменяя объявленные поля. Построение ключей и значений учитывает общий бюджет, а результат проходит полную schema validation.

### 8.4. Array keywords

| Keyword | Поддержка | Правило |
| --- | --- | --- |
| `items` | да | Одна schema для всех элементов массива |
| `minItems` | да | Минимальный размер |
| `maxItems` | да | Максимальный размер |
| `uniqueItems` | да | Повторять генерацию до получения уникальных значений с ограничением попыток |
| `prefixItems` | нет | Tuple generation не входит в V1 |
| `contains` | нет | Отклонять |
| `minContains` / `maxContains` | нет | Отклонять |
| `unevaluatedItems` | нет | Отклонять |

Определение длины массива:

1. Если заданы `minItems` и `maxItems`, выбрать детерминированное значение в диапазоне.
2. Если задан только `minItems`, использовать максимум из `minItems` и `defaultArrayLength`.
3. Если задан только `maxItems`, использовать минимум из `maxItems` и `defaultArrayLength`.
4. Если ограничения отсутствуют, использовать `defaultArrayLength`.
5. Итоговая длина не должна превышать глобальный server limit.

Для `uniqueItems: true` число попыток должно быть ограничено. Если schema не позволяет получить необходимое число уникальных элементов, вернуть контролируемую ошибку, а не бесконечный цикл.

### 8.5. String keywords

| Keyword | Поддержка | Правило |
| --- | --- | --- |
| `minLength` | да | Минимальное количество Unicode code points |
| `maxLength` | да | Максимальное количество Unicode code points |
| `format` | да | Только formats из таблицы ниже |
| `pattern` | ограниченно | Генерировать по явно описанному безопасному regex subset V1 |
| `contentEncoding` | нет | Отклонять |
| `contentMediaType` | нет | Отклонять |
| `contentSchema` | нет | Отклонять |

Поддерживаемые string formats:

| Format | Формат результата |
| --- | --- |
| `date` | RFC 3339 full-date, например `2026-07-22` |
| `time` | RFC 3339 full-time |
| `date-time` | RFC 3339 date-time с timezone |
| `duration` | ISO 8601 duration |
| `uuid` | UUID canonical textual representation |
| `email` | Синтаксически корректный email для reserved example domain |
| `hostname` | Синтаксически корректный hostname |
| `ipv4` | IPv4 address |
| `ipv6` | IPv6 address |
| `uri` | Абсолютный URI с безопасным example domain |

Неизвестный `format` должен приводить к ошибке unsupported schema keyword/value, а не молча игнорироваться.

#### 8.5.1. Поддерживаемый `pattern` subset V1

V1 должен уметь генерировать строки по ограниченному и заранее валидируемому подмножеству regular expressions.

Поддерживаются:

| Конструкция | Пример | Семантика |
| --- | --- | --- |
| Anchors | `^...$` | Полное соответствие строки pattern |
| Literal characters | `SU`, `-`, `_` | Фиксированная часть строки |
| Escaped literal | `\.`, `\-`, `\(` | Экранированный символ |
| Digit shorthand | `\d` | Одна ASCII-цифра `0-9` |
| Character class | `[ABC]` | Один символ из набора |
| Character range | `[A-Z]`, `[a-z]`, `[0-9]` | Один символ из диапазона |
| Fixed quantifier | `{3}` | Ровно три повторения предыдущего atom |
| Bounded quantifier | `{3,5}` | От трёх до пяти повторений |
| Optional atom | `?` | Ноль или одно повторение |
| Literal alternatives | `(SU|FV)` | Один из перечисленных literal вариантов |

Обязательные примеры поддерживаемых patterns:

```text
^SU[0-9]{3,4}$
^[A-Z]{3}$
^(SU|FV)[0-9]{4}$
^[A-Z]{2}-[0-9]{2}$
^ITEM-\d{6}$
```

Не поддерживаются в V1:

- unbounded quantifiers `*` и `+`;
- wildcard `.`;
- lookahead и lookbehind;
- backreferences;
- named/capturing groups, кроме простой группы literal alternatives;
- nested alternatives;
- Unicode categories и Unicode property escapes;
- flags;
- сложные zero-width assertions;
- patterns, которые не могут быть разобраны ограниченным parser.

Generator не должен передавать неподдерживаемый pattern в эвристический или потенциально неограниченный random-regex generator. Он должен вернуть `schema.pattern_unsupported` с path и причиной.

Сгенерированная по pattern строка обязана дополнительно удовлетворять `minLength`, `maxLength`, `enum`, `const` и другим keywords той же subschema. Если constraints противоречат друг другу, вернуть `schema.not_generatable`.

Если вместе с pattern переданы валидные `const`, `enum`, `examples` или `default`, общий приоритет generation profile сохраняется. Pattern служит обязательной финальной проверкой выбранного значения.

### 8.6. Numeric keywords

| Keyword | Поддержка | Правило |
| --- | --- | --- |
| `minimum` | да | Нижняя включительная граница |
| `maximum` | да | Верхняя включительная граница |
| `exclusiveMinimum` | да | Нижняя исключительная граница |
| `exclusiveMaximum` | да | Верхняя исключительная граница |
| `multipleOf` | да | Результат обязан быть кратен значению |

Правила:

- `minimum` не может быть больше `maximum`;
- exclusive constraints должны оставлять хотя бы одно генерируемое значение;
- для `integer` результат всегда должен быть целым;
- для `number` нельзя возвращать `NaN` или infinity;
- если границы отсутствуют, profile использует документированный конечный диапазон по умолчанию;
- arithmetic для `multipleOf` не должна создавать очевидные floating-point validation errors;
- после генерации число обязательно валидируется исходной schema.

## 9. Приоритет правил генерации

Для каждой subschema profile `default-v1` использует следующий приоритет:

1. `$ref` — разрешить ссылку и продолжить по целевой schema.
2. `const` — вернуть значение `const`.
3. `enum` — детерминированно выбрать один элемент.
4. `examples` — выбрать один пример, который проходит текущую schema.
5. `default` — использовать значение, если оно проходит текущую schema.
6. `oneOf` — выбрать одну поддерживаемую ветку.
7. `format` — использовать format-specific generator.
8. `type` и type-specific constraints — сгенерировать значение по правилам primitive/object/array.

Если `const`, `enum`, `examples` или `default` содержат значение, не соответствующее остальным ограничениям schema, невалидное значение нельзя возвращать. Для `examples` и `default` допускается переход к следующему правилу. Невалидный `const` или полностью невалидный `enum` означает, что schema негенерируема.

## 10. Разрешение `$ref`

Разрешены только ссылки следующего вида:

```text
#/$defs/<JSON-Pointer-token>
```

Нужно корректно поддержать JSON Pointer escaping:

- `~0` означает `~`;
- `~1` означает `/`.

Запрещены:

- `http://...`;
- `https://...`;
- `file://...`;
- относительные файловые пути;
- ссылки на другие HTTP resources;
- любые попытки сетевой загрузки schema.

До генерации необходимо:

1. разрешить все reachable local references;
2. обнаружить отсутствующие targets;
3. обнаружить reference cycles;
4. построить или проверить замкнутый граф reachable schemas.

В V1 циклические `$ref` отклоняются, даже если теоретически schema допускает рекурсивное завершение. Поддержка рекурсивной генерации может быть добавлена отдельно с явной семантикой ограничения глубины.

Неиспользуемые definitions в `$defs` допускаются и не должны мешать генерации.

## 11. Связанные objects и collections

### 11.1. Зачем нужны relations

JSON Schema умеет описать, что `departureAirportId` является string, но не умеет стандартным `$ref` потребовать, чтобы его значение было равно `id` одного из уже сгенерированных объектов `airports`.

Такая структура должна описываться как обычная JSON Schema плюс отдельный generation relation:

```json
{
  "airports": [
    {
      "id": "airport-1",
      "code": "SVO"
    }
  ],
  "flights": [
    {
      "departureAirportId": "airport-1",
      "departureAirportCode": "SVO"
    }
  ]
}
```

`$ref` переиспользует schema типа. Relation переиспользует конкретное значение из одного сгенерированного object.

### 11.2. Произвольная форма корневого результата

Relations работают внутри каждого отдельно сгенерированного корневого значения независимо от его формы.

Поддерживаемые cardinalities V1:

| Source selector | Target selector | Пример |
| --- | --- | --- |
| Один object | Один object | `settings` заполняет `summary` |
| Один object | Несколько objects | `tenant` копируется во все `records[*]` |
| Несколько objects | Один object | Один `users[*]` выбирается для `owner` |
| Несколько objects | Несколько objects | Для каждого `orders[*]` выбирается один `customers[*]` |

Корневое значение может быть:

- object с несколькими связанными arrays;
- object с одиночными связанными objects;
- object со смешанными одиночными objects и arrays;
- root array, элементы которого выбираются restricted path `$[*]`;
- primitive или несвязанный array — в этом случае `relations` должны быть пустыми.

Relations одного root item не могут ссылаться на другой root item из response `items`. Каждый из `generation.count` результатов имеет собственный независимый relation graph.

### 11.3. Формат relation

```json
{
  "id": "flight-departure-airport",
  "source": {
    "path": "$.airports[*]",
    "uniqueBy": "/id"
  },
  "target": {
    "path": "$.flights[*]"
  },
  "selection": "random",
  "mappings": [
    {
      "from": "/id",
      "to": "/departureAirportId"
    },
    {
      "from": "/code",
      "to": "/departureAirportCode"
    }
  ]
}
```

| Путь | Тип | Обязательное | Правило |
| --- | --- | --- | --- |
| `id` | string | да | Уникальная identity relation внутри request |
| `source.path` | string | да | Restricted selector source candidates |
| `source.uniqueBy` | string | нет | JSON Pointer относительно source object; выбранные значения должны быть уникальны |
| `target.path` | string | да | Restricted selector target objects |
| `selection` | string | да | `single`, `random` или `round-robin` |
| `mappings` | array | да | Непустой список копируемых значений |
| `mappings[].from` | string | да | JSON Pointer относительно выбранного source object |
| `mappings[].to` | string | да | JSON Pointer относительно target object |

Пустая строка `""` как `from` означает весь выбранный source value. Пустой `to` в V1 запрещён: relation изменяет поля target object, но не заменяет target node целиком.

### 11.4. Restricted selectors

`source.path` и `target.path` используют ограниченное path-подмножество, похожее на JSONPath:

| Конструкция | Значение |
| --- | --- |
| `$` | Корень одного генерируемого JSON-значения |
| `.property` | Object property с identifier-safe именем |
| `['property-name']` | Object property с произвольным именем |
| `[0]` | Конкретный array index |
| `[*]` | Все элементы array |

Примеры:

```text
$.airports[*]
$.schedule.departures[*]
$.configuration.owner
$[*]
$.groups[0].members[*]
$['property-with-dash'][*]
```

Не поддерживаются:

- filters;
- recursive descent;
- script expressions;
- functions;
- unions и slices;
- wildcards object properties;
- обращение за пределы текущего root item.

`mappings[].from`, `mappings[].to` и `source.uniqueBy` используют JSON Pointer относительно выбранного node, а не selector syntax. Это намеренно разделённые contracts: selector выбирает nodes, JSON Pointer читает или записывает значение внутри одного node.

### 11.5. Selection strategies

#### `single`

Source selector обязан разрешиться ровно в один node. Этот source используется для каждого target.

#### `random`

Для каждого target детерминированно выбирается один source candidate с использованием stream PRNG текущего seed. Выбор выполняется с возвращением: один source может использоваться несколькими targets.

#### `round-robin`

Targets последовательно получают sources в стабильном порядке. После последнего source выбор продолжается с первого.

Порядок source и target nodes определяется порядком массивов в JSON. Для одиночного object существует ровно один node.

### 11.6. Несколько mappings из одного source

Все mappings одной relation используют один и тот же выбранный source object для конкретного target. Это обеспечивает согласованность денормализованных копий:

```json
{
  "departureAirportId": "airport-1",
  "departureAirportCode": "SVO",
  "departureAirportName": "Airport One"
}
```

Нельзя выбирать новый source отдельно для каждого mapping одной relation.

### 11.7. Пример object с несколькими связанными arrays

```json
{
  "schema": {
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    "$defs": {
      "Airport": {
        "type": "object",
        "properties": {
          "id": { "type": "string", "format": "uuid" },
          "code": { "type": "string", "pattern": "^[A-Z]{3}$" }
        },
        "required": ["id", "code"],
        "additionalProperties": false
      },
      "Flight": {
        "type": "object",
        "properties": {
          "number": { "type": "string", "pattern": "^SU[0-9]{3,4}$" },
          "departureAirportId": { "type": "string", "format": "uuid" },
          "departureAirportCode": { "type": "string", "pattern": "^[A-Z]{3}$" }
        },
        "required": ["number", "departureAirportId", "departureAirportCode"],
        "additionalProperties": false
      }
    },
    "type": "object",
    "properties": {
      "airports": {
        "type": "array",
        "items": { "$ref": "#/$defs/Airport" },
        "minItems": 5,
        "maxItems": 5
      },
      "flights": {
        "type": "array",
        "items": { "$ref": "#/$defs/Flight" },
        "minItems": 20,
        "maxItems": 20
      }
    },
    "required": ["airports", "flights"],
    "additionalProperties": false
  },
  "generation": {
    "count": 1,
    "seed": "linked-preview-1",
    "profile": "default-v1",
    "relations": [
      {
        "id": "flight-departure-airport",
        "source": {
          "path": "$.airports[*]",
          "uniqueBy": "/id"
        },
        "target": {
          "path": "$.flights[*]"
        },
        "selection": "random",
        "mappings": [
          { "from": "/id", "to": "/departureAirportId" },
          { "from": "/code", "to": "/departureAirportCode" }
        ]
      }
    ]
  }
}
```

### 11.8. Пример связи между одиночными objects

```json
{
  "generation": {
    "count": 1,
    "relations": [
      {
        "id": "owner-to-summary",
        "source": {
          "path": "$.owner",
          "uniqueBy": "/id"
        },
        "target": {
          "path": "$.summary"
        },
        "selection": "single",
        "mappings": [
          { "from": "/id", "to": "/ownerId" },
          { "from": "/name", "to": "/ownerName" }
        ]
      }
    ]
  }
}
```

Полная JSON Schema в сокращённом примере опущена только для читаемости. В реальном request `schema` остаётся обязательной.

### 11.9. Validation relations

До генерации необходимо проверить:

- уникальность relation identities;
- синтаксис selectors и JSON Pointers;
- существование source и target paths в schema;
- совместимость source/target cardinality со strategy;
- существование `from` и `to` относительно source/target schemas;
- совместимость типов каждого mapping;
- возможность записи в target property;
- уникальность `source.uniqueBy`, если он задан;
- отсутствие конфликтующих relations, записывающих разные значения в один target path;
- отсутствие cycles в relation dependency graph;
- достижимость source до начала заполнения зависимого target.

Target properties, заполняемые relations, не должны независимо получать финальное случайное значение. Generator должен рассматривать их как deferred relation-bound fields либо безопасно перезаписывать до финальной validation. Предпочтителен deferred подход, чтобы не расходовать PRNG и не скрывать ошибки generation plan.

Если source selector во время генерации возвращает пустой набор, вернуть `relation.source_empty`. Partial result не возвращать.

### 11.10. Порядок генерации

Для каждого root item:

1. Построить structural generation plan.
2. Выделить relation-bound target fields.
3. Построить dependency graph выбранных root sections.
4. Сгенерировать независимые nodes и source sections.
5. Сгенерировать target containers и несвязанные поля.
6. Применить relations в topological order.
7. Провалидировать весь root item исходной JSON Schema.

Порядок объявления properties в JSON object не должен определять корректность relations. Dependency graph является источником порядка.

### 11.11. Границы relations V1

V1 не поддерживает:

- произвольный исполняемый код;
- `where` filters и predicates;
- joins нескольких source collections в одной relation;
- arithmetic, aggregates и computed expressions;
- cross-root relations между элементами response `items`;
- cross-request state;
- автоматическое условие «source A не равен source B»;
- запись одного relation mapping сразу в несколько target schemas с различной структурой.

Эти возможности должны добавляться отдельными версиями relation contract, а не неявным расширением строковых expressions.

## 12. Проверка поддерживаемого подмножества

Успешная компиляция стандартной библиотекой JSON Schema ещё не означает, что сервис умеет сгенерировать значение по этой schema.

Поэтому нужны две независимые проверки:

1. Standard validation — schema корректна относительно Draft 2020-12.
2. Generator capability validation — все reachable keywords и их комбинации поддержаны profile `default-v1`.

Неизвестный или неподдерживаемый keyword нельзя молча игнорировать. Сервис должен вернуть диагностическую ошибку с точным JSON Pointer path.

Пример diagnostic:

```json
{
  "code": "schema.keyword_unsupported",
  "path": "/schema/$defs/Flight/allOf",
  "keyword": "allOf",
  "message": "Keyword allOf не поддерживается generation profile default-v1"
}
```

## 13. Алгоритм обработки запроса

Обязательная последовательность:

1. Принять JSON request на backend HTTP gateway.
2. Применить transport body limit и проверить ресурсный лимит Mock перед forwarding.
3. Декодировать request envelope как JSON.
4. Запретить неизвестные поля envelope.
5. Провалидировать generation options.
6. Проверить `$schema` и dialect.
7. Проверить структурные бюджеты, включая literal const/enum/examples/default.
8. Выполнить capability validation поддерживаемого V1 subset, разрешить reachable local `$ref` и проверить отсутствие cycles.
9. Скомпилировать schema библиотекой JSON Schema.
10. Провалидировать relation selectors, mappings и dependency graph.
11. Создать deterministic random source из seed и profile.
12. Для каждого root item построить structural и relation generation plan.
13. Сгенерировать `count` независимых корневых значений в dependency order.
14. Применить relations внутри каждого root item.
15. Каждое полное корневое значение проверить скомпилированной исходной schema.
16. При любой ошибке не возвращать частичный список данных.
17. Вернуть полный response.

## 14. Формат успешного ответа

### 14.1. HTTP status

```http
200 OK
Content-Type: application/json
```

### 14.2. Response envelope

```json
{
  "items": [],
  "meta": {
    "count": 0,
    "seed": "resolved-seed",
    "profile": "default-v1"
  }
}
```

| Путь | Тип | Описание |
| --- | --- | --- |
| `items` | array | Сгенерированные корневые JSON-значения |
| `meta.count` | integer | Фактическое количество элементов; равно request `generation.count` |
| `meta.seed` | string | Переданный либо созданный сервисом seed |
| `meta.profile` | string | Фактически использованный generation profile |

Порядок элементов `items` является частью воспроизводимого результата.

`items` является transport envelope и не ограничивает форму корневой schema. Каждый элемент `items` может быть object, array, string, number, integer, boolean или `null`. Если сама корневая schema описывает array, response содержит array корневых arrays.

### 14.3. Пример ответа

```json
{
  "items": [
    {
      "mainFlight": {
        "id": "03d6f0c2-28b1-4c65-a774-c1ce7f2d9e9d",
        "number": "SU100",
        "departureAirport": {
          "code": "SVO",
          "name": "sample-string"
        },
        "arrivalAirport": {
          "code": "LED",
          "name": "sample-string"
        },
        "scheduledAt": "2026-07-22T14:00:00Z"
      },
      "relatedFlights": [
        {
          "id": "12876135-b9d0-41e7-a707-6c4518108466",
          "number": "SU101",
          "departureAirport": {
            "code": "DME",
            "name": "sample-string"
          },
          "arrivalAirport": {
            "code": "SVO",
            "name": "sample-string"
          },
          "scheduledAt": "2026-07-22T15:00:00Z"
        },
        {
          "id": "f69d78ba-88e6-44b6-879b-1a4d48848a60",
          "number": "SU100",
          "departureAirport": {
            "code": "LED",
            "name": "sample-string"
          },
          "arrivalAirport": {
            "code": "DME",
            "name": "sample-string"
          },
          "scheduledAt": "2026-07-22T16:00:00Z"
        }
      ],
      "baseAirport": {
        "code": "DME",
        "name": "sample-string"
      }
    }
  ],
  "meta": {
    "count": 1,
    "seed": "preview-42",
    "profile": "default-v1"
  }
}
```

Пример является иллюстрацией envelope. Конкретные значения должны определяться seed и profile.

## 15. Формат ошибок

Все контролируемые ошибки возвращаются в едином envelope:

```json
{"code":"schema.ref_missing","message":"Reference target does not exist","details":{"path":"/schema/properties/airport/$ref"}}
```

### 15.1. HTTP statuses

| Status | Когда используется |
| --- | --- |
| `400 Bad Request` | Невалидный JSON, envelope или generation options |
| `401 Unauthorized` | Endpoint защищён auth middleware и token отсутствует/невалиден |
| `413 Content Too Large` | Превышен limit request body |
| `400 Bad Request` | JSON Schema невалидна, не поддержана или негенерируема; сохранён backend error envelope |
| `429 Too Many Requests` | Превышен rate limit |
| `500 Internal Server Error` | Неожиданная внутренняя ошибка |

### 15.2. Стабильные error codes

Минимальный набор:

```text
request.invalid
request.too_large
generation.count_invalid
generation.profile_unsupported
generation.options_invalid
generation.seed_invalid
generation.busy
generation.limit_exceeded
schema.dialect_unsupported
schema.invalid
schema.keyword_unsupported
schema.ref_unsupported
schema.ref_missing
schema.ref_cycle
schema.not_generatable
schema.limit_exceeded
schema.pattern_unsupported
relation.invalid
relation.path_invalid
relation.path_missing
relation.type_mismatch
relation.source_empty
relation.target_conflict
relation.cycle
```

`message` предназначен для человека. Клиентская логика должна опираться на стабильный `code` и структурированные `details`.

В diagnostics нельзя возвращать stack traces, пути файловой системы или внутренние ошибки библиотек без безопасного mapping.

## 16. Resource limits и защита от abuse

Все limits должны находиться в конфигурации сервиса и иметь безопасные defaults.

Рекомендуемые defaults для V1:

| Limit | Default |
| --- | --- |
| Request body | 2 MiB |
| `generation.count` | максимум `1000` |
| Definitions в `$defs` | максимум `1000` |
| Reachable schema nodes | максимум `10000` |
| Максимальная глубина schema/generation | `32` |
| Максимальная длина массива | `1000` |
| Максимальная длина сгенерированной строки | `10000` Unicode code points |
| Relations в одном request | максимум `1000` |
| Mappings в одной relation | максимум `100` |
| Попытки для `uniqueItems`/`oneOf` | ограниченный configurable budget |
| Максимальный response body | configurable, например 20 MiB |
| Request timeout | configurable, например 10 секунд |

При превышении limits сервис должен завершать запрос контролируемой ошибкой. Нельзя допускать:

- бесконечную рекурсию;
- бесконечный retry;
- неограниченное выделение памяти;
- goroutine leak после отмены HTTP request;
- продолжение тяжёлой генерации после `context` cancellation.

## 17. Безопасность

Обязательные правила:

- никакой сетевой загрузки `$ref`;
- никакого выполнения кода из schema;
- никакой интерпретации schema annotations как executable templates;
- request и response limits;
- request timeout;
- rate limit на уровне сервиса или gateway;
- configurable CORS allowlist;
- CORS не считается механизмом authentication или authorization;
- не логировать request/response body по умолчанию;
- не помещать generated personal-looking data в logs и traces;
- ошибки библиотек преобразовывать в безопасные публичные diagnostics.

Клиент обращается к authenticated backend gateway. Генератор принимает только OIDC service identity backend; production требует TLS. Полный контракт — в todo-2.

## 18. JSON Schema library для Go

Для standard compilation и validation использовать:

```text
github.com/santhosh-tekuri/jsonschema/v6
```

Требования к adapter:

- явно выбрать Draft 2020-12, не полагаться на изменяемый default библиотеки;
- добавить входную schema как in-memory resource;
- запретить remote resource loading;
- компилировать schema до генерации;
- использовать скомпилированную schema для финальной проверки каждого item;
- преобразовывать library errors в собственные diagnostics;
- не пропускать типы библиотеки в domain и application API.

Библиотека отвечает за стандартную schema compilation/validation. Она не определяет generation profile и не заменяет application generator.

## 19. Реализация, transport и поставка

Архитектурные boundaries, OIDC, session lifecycle и HTTP/SSE описаны в [todo-2](todo-2.md), реализация — в [architecture](architecture.md), запуск и проверки — в [README](../README.md) и [tests](tests/README.md). Нормативный профиль принадлежит root governance. Старый WebSocket proposal заменён; бизнесовый HTTP endpoint `/api/v1/mock-data/generate` принадлежит backend gateway.

`default-v1` использует SHA-256 UTF-8 seed: первые 16 bytes читаются как два big-endian uint64 для Go `math/rand/v2.NewPCG`. Стабильный лексикографический порядок properties; независимый cursor на запрос/сессию. Время генерируется в UTC в диапазоне 2020-01-01 — 2029-12-31; текущее время не участвует. Числа без bounds используют [-1000,1000], шаг number по умолчанию 0.001; exact rational arithmetic используется для multipleOf. Golden fixtures закрепляют фактическую последовательность.

Структурные ограничения длины строк, массивов и вложенности применяются также к значениям const/enum/examples/default до compilation и к полному root после relations.

Входные numeric literals ограничены 128 bytes и exponent [-100,100] до compilation. Regex quantifiers используют обычную десятичную запись без leading zeros. Graph relations имеет дополнительный бюджет сравнений `schemaNodes * attempts`; превышение завершается контролируемым отказом. Это complexity limits, а не silent truncation.

Не входят: сохранение схем между запросами, persistence сессий, replay, schema replacement активного stream, Simulation/Core/Configurator source switching, удалённый deployment.
