# Project Creator

Система знаний о продукте и проверки бизнес-требований на возможные коллизии.
Цели — [GOAL.md](GOAL.md), архитектурные контракты — [ARCHITECTURE.md](ARCHITECTURE.md).

## Состояние MVP

Реализованы карточки проектов и бизнес-требования, GraphQL API с PostgreSQL
и интерфейс Next.js. pgvector подготовлен для будущего поиска; embeddings, RAG и анализ коллизий пока не реализованы.

Порядок реализации:

1. [Модель карточки и валидация](https://github.com/VolkovDani/project-creator/issues/2).
2. [GraphQL API и хранение](https://github.com/VolkovDani/project-creator/issues/3).
3. [Next.js и карточки проектов](https://github.com/VolkovDani/project-creator/issues/4).
4. [Бизнес-требования](https://github.com/VolkovDani/project-creator/issues/5).
5. [PostgreSQL + pgvector](https://github.com/VolkovDani/project-creator/issues/14).
6. [SSE-инвалидация](https://github.com/VolkovDani/project-creator/issues/6).
7. [Локальный RAG](https://github.com/VolkovDani/project-creator/issues/7).
8. [Анализ коллизий](https://github.com/VolkovDani/project-creator/issues/8).
9. [Docker-развёртывание и CI](https://github.com/VolkovDani/project-creator/issues/9).

## Локальная проверка

Требуются Go 1.26.10, C-компилятор для импорта/тестирования SQLite, Node.js 24
и PostgreSQL 16 с pgvector 0.8.0. Обычные тесты не требуют LLM или облачных API.
Интеграционные тесты используют отдельные схемы в базе из `TEST_DATABASE_URL`.
Без этой переменной они пропускаются; такой запуск не проверяет хранилище.

```bash
export TEST_DATABASE_URL='postgresql://project_creator:ваш-пароль@127.0.0.1:5432/project_creator_test?sslmode=disable'
go test ./...
go vet ./...
gofmt -l cmd contracts graph internal
```

Тестовая роль должна иметь права создавать и удалять схемы и устанавливать
расширение `vector`. Используйте отдельную тестовую базу, не рабочую.
Разработка по TDD: падающий тест поведения → реализация → рефакторинг.

## Карточка проекта

`internal/project.Input`: `name`, `goal`, `description`.
`project.Validate` возвращает нормализованный ввод и список ошибок.
Ошибки содержат `path`, `code`, `messageKey`, `params`; успешная проверка
возвращает пустой список. Валидация не создаёт ID и не сохраняет данные.
Серверный ID, workspace и timestamps добавляются при сохранении.

Источник ограничений — `contracts/project.json`, встроенный в Go-бинарник.
Будущий фронтенд читает тот же файл; общий набор примеров —
`contracts/project.fixtures.json`. Ограничения не надо копировать в TS.
Неизвестная версия, поле или режим контракта приводит к ошибке загрузки.

- Название: обязательно, максимум 120 Unicode code points.
- Цель: обязательно, максимум 2000 code points.
- Описание: необязательно, максимум 10000 code points.
- Перед проверкой удаляются только ASCII SPACE, TAB, LF, CR с краёв каждого поля.
  NBSP, zero-width, внутренние пробелы и переводы строк сохраняются.
- Длина считается после trim, по Unicode code points (не байтам, UTF-16 units
  или графемам). На TypeScript эквивалентный счётчик — `Array.from(value).length`.
  Unicode NFC/NFKC-нормализация не выполняется. Некорректный UTF-8 отклоняется;
  будущий TS-адаптер также должен отклонять непарные суррогаты до отправки.
- Все ошибки полей собираются за один вызов в порядке name, goal, description.

Общие словари — `contracts/i18n/ru.json` и `en.json`. Для этой версии они
содержат ключи ошибок валидации; `params.max` передаётся отдельно от текста.
Сервер и будущий UI используют ключ, не анализируют переведённую строку.

## PostgreSQL и запуск API

Решение о хранилище — [ADR-001](docs/adr/001-postgresql-pgvector.md).
В `compose.postgres.yml` закреплены PostgreSQL 16 и pgvector 0.8.0. Это окружение
только БД; полная контейнеризация приложения и локальной модели относится к #9.
SQLite больше не является рабочим хранилищем сервера.

Первый запуск:

```bash
cp .env.example .env
# Отредактируйте .env: задайте случайные пароль и токен, согласуйте пароль в URL.
docker compose --env-file .env -f compose.postgres.yml up -d --wait
set -a
. ./.env
set +a
go run ./cmd/server
```

В пароле URL специальные символы должны быть percent-encoded. `.env` не должен
попадать в git. Compose публикует PostgreSQL только на `127.0.0.1`; API по
умолчанию слушает `127.0.0.1:8080`. `sslmode=disable` предназначен для локального
loopback; для удалённой БД используйте проверяемое TLS-соединение.

`PROJECT_CREATOR_DATABASE_URL` обязателен и принимает PostgreSQL URL;
`PROJECT_CREATOR_DB` больше не используется. `PROJECT_CREATOR_TOKEN` должен
содержать не менее 32 символов. POST `/graphql` требует
`Authorization: Bearer <PROJECT_CREATOR_TOKEN>`. Один токен даёт доступ к одному
workspace — это локальная сессия, не полноценная многопользовательская авторизация.
Не публикуйте интерфейс или API в интернет без отдельной защиты.

Версионируемые SQL-миграции применяются автоматически при старте. Сначала
проверяются соединение и доступность pgvector; при ошибке сервер не запускается.
Для первоначального `CREATE EXTENSION vector` нужны права установки расширения
(обычно DBA/superuser). Администратор может установить его заранее в целевой БД;
прикладной роли нужны права на таблицы и применяемые изменения схемы. Сделайте
backup перед обновлением приложения и миграциями. Новая неизвестная версия схемы
отклоняется, автоматического downgrade нет.

Проверить готовность:

```bash
docker compose --env-file .env -f compose.postgres.yml ps
docker compose --env-file .env -f compose.postgres.yml exec -T postgres psql -U project_creator -d project_creator -c "SELECT extversion FROM pg_extension WHERE extname='vector'"
```

До первого старта API расширение может ещё не быть установлено. Ошибка
`connection refused` обычно означает неготовую БД или неправильный порт;
`password authentication failed` — несогласованный пароль в URL и volume.
Изменение `.env` не меняет пароль уже созданной БД: меняйте его через PostgreSQL.
Ошибка доступности `vector` требует образа с pgvector и прав установки расширения.

Остановка сохраняет volume:

```bash
docker compose --env-file .env -f compose.postgres.yml down
```

Для backup остановите запись приложением и выполните:

```bash
docker compose --env-file .env -f compose.postgres.yml exec -T postgres pg_dump -U project_creator -d project_creator -Fc > project-creator.dump
```

Для восстановления используйте пустую базу с pgvector той же совместимой версии;
остановите приложение, затем:

```bash
docker compose --env-file .env -f compose.postgres.yml exec -T postgres pg_restore -U project_creator -d project_creator --exit-on-error < project-creator.dump
```

Перед обновлением сохраняйте dump, проверяйте восстановление отдельно и изучайте
изменения миграций. Обновление major PostgreSQL требует `pg_upgrade` или
backup/restore, а не простой смены тега образа. Команда
`docker compose --env-file .env -f compose.postgres.yml down -v` безвозвратно
удаляет локальный volume; используйте её только для намеренного удаления данных.

## Перенос прежних данных SQLite

Остановите старое и новое приложение. Сохраните исходную SQLite-базу вместе с
файлами WAL/SHM, если они есть; не копируйте только основной файл работающей БД.
Импортёр создаёт отдельную согласованную резервную копию и не удаляет источник.
Укажите несуществующий путь backup и пустую целевую PostgreSQL-базу:

```bash
export PROJECT_CREATOR_DATABASE_URL='postgresql://project_creator:ваш-пароль@127.0.0.1:5432/project_creator?sslmode=disable'
go run ./cmd/import-sqlite --source ./project-creator.db --backup ./project-creator-before-postgres.db
```

Поддерживается SQLite-схема версии 2 (бизнес-требования, MVP 4). Для более старой
базы сначала используйте предыдущую версию приложения для её миграции, сохранив
backup. Импорт проверяет целостность, принадлежность требований проектам, версии
и количество записей; сохраняет ID, workspace, текст и timestamps. Вся запись в
PostgreSQL транзакционна: при ошибке она откатывается. Непустая целевая база,
неизвестная версия SQLite или нарушенные связи отклоняются. Повторный импорт в
заполненную БД не выполняется. После успешного импорта сравните выведенные
количества и откройте проекты/требования в UI; исходник и backup сохраняйте до
проверки. Не удаляйте их автоматически.

## GraphQL и проверки CI

SDL в `graph/schema.graphqls` остаётся источником серверных привязок.
Project Creator project — контейнер знаний, а не описываемый продукт.
Тело запроса ограничено 1 МиБ, глубина — 8, одновременных резолверов — 10,
развёрнутых полей и фрагментов — 100. Пагинация требует limit (1–100) и offset ≥ 0.

```bash
go run ./cmd/codegen
git diff --exit-code -- graph/models_gen.go
```

GitHub Actions поднимает PostgreSQL + pgvector, запускает Go unit/integration,
vet, codegen, TypeScript-проверки, сборку и Playwright E2E. Для E2E создаётся
отдельная база. Проверки не обращаются к моделям. Сборка контейнеров всего
приложения и реальные модели будут добавлены в #9.

## Интерфейс Next.js

Требуются Node.js 24 и Go 1.26.10. Запустите GraphQL-сервер в одном терминале,
задав `PROJECT_CREATOR_TOKEN` и `PROJECT_CREATOR_DATABASE_URL` как описано выше. В другом:

```bash
cd frontend
npm ci
export PROJECT_CREATOR_TOKEN='тот-же-секрет-что-у-сервера'
npm run dev -- -H 127.0.0.1
```

Откройте `http://127.0.0.1:3000`. Сервер Next.js пересылает GraphQL-запросы
на `http://127.0.0.1:8080/graphql`; при другом адресе задайте серверную переменную
`PROJECT_CREATOR_GRAPHQL_URL`. Токен не передаётся в браузер. Это локальный
однопользовательский режим: не публикуйте интерфейс в интернет без отдельной
аутентификации и контроля доступа.

Проверка после изменения GraphQL-схемы и операций:

```bash
cd frontend
npm run codegen
npm test
npm run typecheck
npm run build
npx playwright install chromium
npm run e2e
```

Для E2E создайте отдельную пустую одноразовую базу с pgvector и задайте
`E2E_DATABASE_URL` перед `npm run e2e`. Playwright запускает отдельные Go и
Next.js серверы. Тесты создают данные: для повторного чистого прогона пересоздайте
именно E2E-базу; никогда не указывайте URL рабочей базы. Пример подготовки:

```bash
createdb -h 127.0.0.1 -U project_creator project_creator_e2e
export E2E_DATABASE_URL='postgresql://project_creator:ваш-пароль@127.0.0.1:5432/project_creator_e2e?sslmode=disable'
cd frontend
npm run e2e
```

## Бизнес-требования

В карточке проекта можно добавлять, редактировать и удалять требования. Текст
проверяется по общему контракту `contracts/requirement.json`, одинаково на Go
и TypeScript; общий набор примеров — `contracts/requirement.fixtures.json`.
Требования принадлежат workspace и проекту, список использует пагинацию.

Каждое изменение повышает версию. Изменение или удаление проверяет ожидаемую
версию и не перезаписывает более новую редакцию. При конфликте интерфейс
перечитывает список и позволяет применить сохранённый черновик поверх текущей
версии. Старые версии текста пока не сохраняются. Индексация RAG и пометка
результатов анализа устаревшими будут реализованы в задачах #7–#8.



## Обновление двух открытых клиентов (SSE, MVP 5)

Go-сервер предоставляет авторизованный `GET /events`. Next.js пересылает поток
через same-origin `/api/events`, сохраняя токен только на сервере. Интерфейс
остаётся локальным однопользовательским приложением; это не замена полноценной
аутентификации для публикации в интернете. В URL нет токенов.

Миграция PostgreSQL 002 создаёт журнал `cache_events` и триггеры на проекты и
требования. Запись события и изменение данных находятся в одной транзакции:
rollback не публикуется. Журнал является transactional outbox: каждый экземпляр
сервера читает общий PostgreSQL, поэтому сбой процесса между commit и отправкой
не теряет инвалидацию. Публикация не зависит от памяти процесса или LISTEN/NOTIFY.
Транзакционный advisory lock сериализует выделение номеров событий до commit,
чтобы polling не пропускал событие из более ранней, ещё не завершённой транзакции.
Это осознанное ограничение пропускной способности локального MVP.

Ответ GraphQL содержит `extensions.cacheTags`; события `cache.invalidate`
передают `{schemaVersion:1,keys:[...]}`. Теги: `projects`, `project:<id>` и
`project:<id>:requirements`. Тег `projects` пока является консервативной общей
зависимостью в пределах workspace. У разных workspace отдельная выдача событий.
Клиентский кеш учитывает поколение инвалидации и повторяет чтение, если событие
пришло во время запроса; повторные event ID безопасны.

История ограничена последними 1000 событиями. `Last-Event-ID` — непрозрачный
идентификатор; неизвестный, чужой или удалённый курсор приводит к `cache.reset`.
Первое подключение также вызывает сверку. Клиент перечитывает данные, сохраняя
несохранённый ввод. Поток проверяет новые записи каждые 300 мс и отправляет
heartbeat каждые 15 секунд; после обрыва клиент повторяет подключение с
экспоненциальной задержкой до 30 секунд. Offline/online восстанавливается тем же
механизмом. `resetSession()` очищает кеш и курсор при будущей смене сессии;
ответы 401/403 очищают недоступные данные. SSE и GraphQL не используют долгоживущий
серверный кеш Next.js. При настройке reverse proxy отключите buffering потока и
задайте idle timeout больше интервала heartbeat.

Проверка: откройте список проектов или одну карточку в двух вкладках. Создание
проекта и создание/изменение/удаление требований в первой вкладке должны
отображаться во второй без reload. В CI это проверяет Playwright; Go-тесты
проверяют workspace, replay/reset, сохранность журнала, commit/rollback и отмену
соединения. Команды тестирования остаются приведёнными выше.
