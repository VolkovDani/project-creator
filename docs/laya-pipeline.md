# Laya → LLM: настройка и ограничения

Функция #48 включается явно. Без настроенной Laya приложение сохраняет прежний
анализ. Веса автоматически не загружаются; существующие данные и профили сохраняются.

Локальный Ollama 0.40.11 поддерживает `/v1/systemone`. Используется только
`laya:322m-multilingual-bf16`; английский `laya:latest` и aliases не подменяют эту
модель. Установка по явной команде:

```sh
docker compose exec ollama ollama pull laya:322m-multilingual-bf16
docker compose --profile tools run --rm model-info
```

В `.env` задайте полный digest из `/api/tags` (не короткий ID):

```dotenv
PROJECT_CREATOR_LAYA_MODEL=laya:322m-multilingual-bf16
PROJECT_CREATOR_LAYA_DIGEST=sha256:полный-digest
PROJECT_CREATOR_LAYA_URL=http://ollama:11434
```

Обе переменные MODEL/DIGEST обязательны вместе. Backend проверяет модель и
отсутствие remote_host/remote_model до и после классификации; redirects и HTTP
proxy отключены. Изменение digest или инструкций меняет профиль сравнения и
исключает переиспользование несовместимых результатов. Ответ Laya сохраняется
как подсказка, а LLM независимо проверяет полные источники. `unrelated` не
отбрасывается. Оценки модели не являются вероятностью правильности решения.

Три роли поддерживают отдельные URL:

- `PROJECT_CREATOR_EMBEDDING_URL` — embeddings для поиска/индексации;
- `PROJECT_CREATOR_LAYA_URL` — классификация;
- `PROJECT_CREATOR_GENERATOR_URL` — генерация Ollama. При OpenVINO используется
  прежний явно выбранный NPU socket/profile.

Незаданный URL наследует `PROJECT_CREATOR_OLLAMA_URL`; Compose явно задаёт
`http://ollama:11434`. Для отдельных локальных экземпляров допустимы loopback
и имена `ollama-embeddings`, `ollama-laya`, `ollama-generator`, `ollama`.
Самостоятельное указание имени не создаёт сервис или устройство.

При генераторе OpenVINO и двух моделях в общей Ollama явно задайте
`OLLAMA_MAX_LOADED_MODELS=2`; при трёх моделях в Ollama — `3`, только если хватает
памяти. По умолчанию остаётся `1`. Настройка не гарантирует параллельные вычисления
на общем GPU. Для строгого разделения ресурсов нужны отдельные runtime и явно
проверенное размещение; не включается перенос на CPU или облако при ошибке.
После изменения конфигурации пересоздайте backend/Ollama штатной командой
`docker compose up -d --wait backend ollama`. CLI сохраняет существующий `.env`,
но не скачивает и не закрепляет Laya за пользователя; `doctor` не является
проверкой качества или конкурентности классификатора.

## Ограничение длины

Multilingual checkpoint имеет внутренние `max_len=1024`, `head_max_len=256`.
Значение `num_ctx=8192` в карточке Ollama не подтверждает отсутствие усечения
внутри классификатора. Адаптер консервативно ограничивает сериализованный снимок
пары 704 UTF-8 байтами. Это оценка бюджета, не точная токенизация; длинные пары
получают `LAYA_INPUT_TOO_LONG` и не передаются LLM без классификации. Тексты не
усекаются приложением. До расширения этого ограничения нужны проверки токенизатора
и поведения закреплённого runtime; длинные многострочные требования пока не
покрыты аппаратной приёмкой. Автоматическое суммирование или дробление не добавлено.

## Проверки

Детерминированные тесты с локальными заглушками:

```sh
go test ./internal/analysis -run 'TestLaya|TestPipeline' -count=1
go test ./internal/rag -run TestInference -count=1
```

Для очереди нужен `TEST_DATABASE_URL` отдельной тестовой PostgreSQL/pgvector БД.
Проверены commit-before-LLM, перекрытие разных пар, ошибки каждого этапа, retry,
restart, отмена и поздний ответ, ожидание при замене параметров и точные дубли.
Обычный CI не загружает веса и не использует оборудование.

Аппаратная приёмка пока **не выполнена**. На отдельном тестовом проекте следует
запустить 20 пар и во время Laya(B)/LLM(A) выполнить поиск, записать время ожидания,
inference и SQL, p50/p95, загрузку устройств и память (`docker stats`, Ollama `ps`,
NPU diagnostics). Нужны холодный/тёплый запуск и проверка отсутствия thrashing/OOM.
Сохраняйте отчёты в `logs/issue48/`; не используйте реальные клиентские данные.
Без этих замеров ускорение и latency budget не подтверждены.

Источники: [точный тег Ollama](https://ollama.com/library/laya:322m-multilingual-bf16),
[контракт API 0.40.11](https://github.com/ollama/ollama/blob/v0.40.11/docs/openapi.yaml),
[upstream конфигурация Laya](https://github.com/NandhaKishorM/laya/blob/main/laya-rust/README.md).
Решение: [ADR-008](adr/008-laya-pipeline.md).
