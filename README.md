# Project Creator

Система знаний о продукте и проверки бизнес-требований на возможные коллизии.
Цели — [GOAL.md](GOAL.md), архитектурные контракты — [ARCHITECTURE.md](ARCHITECTURE.md).

## Состояние MVP

Реализованы доменная модель карточки проекта, общий контракт валидации и
GraphQL API с SQLite-хранением. Интерфейс пока не реализован.

Порядок реализации:

1. [Модель карточки и валидация](https://github.com/VolkovDani/project-creator/issues/2).
2. [GraphQL API и хранение](https://github.com/VolkovDani/project-creator/issues/3).
3. [Next.js и карточки проектов](https://github.com/VolkovDani/project-creator/issues/4).
4. [Бизнес-требования](https://github.com/VolkovDani/project-creator/issues/5).
5. [SSE-инвалидация](https://github.com/VolkovDani/project-creator/issues/6).
6. [Локальный RAG](https://github.com/VolkovDani/project-creator/issues/7).
7. [Анализ коллизий](https://github.com/VolkovDani/project-creator/issues/8).
8. [Docker-развёртывание и CI](https://github.com/VolkovDani/project-creator/issues/9).

## Локальная проверка

Требуется Go 1.23 или новее. Внешних Go-зависимостей, БД, API-ключей и LLM для
этой задачи нет. Из корня репозитория:

```bash
go test ./...
go vet ./...
gofmt -l contracts/*.go internal/project/*.go
```

Последняя команда должна ничего не вывести. Для форматирования:

```bash
gofmt -w contracts/*.go internal/project/*.go
```

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

## Docker и GitHub Actions

Docker Compose и образы пока не реализованы. Полная инструкция будет добавлена
**в этот README** в задаче #9: первый запуск, загрузка локальных моделей,
миграции, healthchecks, сохранность данных, backup/restore и обновление.
До появления запускаемого приложения команды развёртывания не публикуются.
GitHub Actions для проверок и сборки контейнеров также входит в #9.

## GraphQL API (MVP 2)

Project Creator project — контейнер знаний о продукте; сам описываемый продукт
и роли внутри него являются отдельными понятиями. Для хранения выбран SQLite
(драйвер go-sqlite3 с CGO); для GraphQL — gqlgen. SDL в `graph/schema.graphqls`
является источником серверных привязок. SQLite хранит данные в указанном файле;
пространство будущего векторного поиска остаётся предметом следующей задачи.

Требуются Go 1.23, C compiler и SQLite-совместимая файловая система. Запуск:

```bash
export PROJECT_CREATOR_TOKEN='замените-на-длинный-случайный-секрет-не-менее-32-символов'
export PROJECT_CREATOR_DB='./project-creator.db'
go run ./cmd/server
```

Отправляйте POST на `http://127.0.0.1:8080/graphql` с заголовком
`Authorization: Bearer <PROJECT_CREATOR_TOKEN>`. Один настроенный токен даёт
доступ к единственному workspace; это минимальная локальная сессия, не система
управления пользователями. Не открывайте сервис наружу без полноценной защиты.
Миграция схемы применяется при старте; файл БД нужно сохранять между запусками.
Тело запроса ограничено 1 МиБ, сложность GraphQL — 200, список — 100 записей.

```bash
go run github.com/99designs/gqlgen@v0.17.70 generate
git diff --exit-code -- graph/generated.go graph/model/models_gen.go
go test ./...
go vet ./...
```
