# CHANGELOG

Краткий changelog для `service-template-go`.

## 0.1.0

- Подготовлен публичный Go module `github.com/endge-lab/service-template-go`.
- Шаблон переведен на зависимость `github.com/endge-lab/service-kit-go v0.1.0`.
- Удален `package.json`: версия шаблона задается git tag-ами и `CHANGELOG.md`.
- Добавлена инструкция на русском по публикации, локальному `go.work` и подключению kit из локальной папки.
- Auth сделан опциональным через `AUTH_ENABLED=false` по умолчанию.
- Telemetry сделана опциональной через `TELEMETRY_ENABLED=false` по умолчанию.
- Redpanda/Kafka оставлены optional и выключены по умолчанию.
- Реальные `.env.*` заменены на безопасные `.env.*.example`.
- CI заменен на GitHub Actions.
- Dockerfile и docker-compose очищены от приватных module credentials.
