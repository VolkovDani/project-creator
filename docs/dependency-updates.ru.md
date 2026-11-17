# Автоматические обновления зависимостей

**English version:** [docs/dependency-updates.md](dependency-updates.md).

Репозиторий использует обновления версий Dependabot через
[`.github/dependabot.yml`](../.github/dependabot.yml). Конфигурация должна
находиться в ветке по умолчанию (`main`); каждый блок обновлений версий нацелен
на `develop`, поэтому Dependabot читает манифесты из `develop` и создаёт PR
обновлений версий для `develop`. Ветка `develop` существовала во время подготовки
конфигурации.

## Охват

| Экосистема | Настроенные каталоги | Отслеживаемые файлы |
| --- | --- | --- |
| Go modules (`gomod`) | `/` | `go.mod`, `go.sum` |
| Bun (`bun`) | `/frontend` | `package.json`, `bun.lock` |
| pip | `/runtime/npu` | `requirements.txt`, `requirements-export.txt` |
| Docker | `/`, `/frontend` | Корневой `Dockerfile`, корневые `Dockerfile.*` и `frontend/Dockerfile` |
| Docker Compose | `/` | Корневые файлы `compose*.yml` |
| GitHub Actions | `/` | Workflow-файлы в `.github/workflows` |

GitHub указывает Bun как экосистему обновления версий Dependabot (Bun 1.1.39
или новее) и поддерживает текущий текстовый формат `bun.lock`, но не устаревший
бинарный `bun.lockb`. См. [раздел Bun в официальном списке поддерживаемых
экосистем](https://docs.github.com/en/code-security/reference/supply-chain-security/supported-ecosystems-and-repositories#bun).
Это поддержка обновления версий. Bun сейчас отсутствует в официальном списке
[экосистем графа зависимостей](https://docs.github.com/en/code-security/reference/supply-chain-security/dependency-graph-supported-package-ecosystems),
поэтому данная конфигурация не означает, что для зависимостей из `bun.lock`
создаются alerts графа зависимостей или PR обновления безопасности. npm advisory
может относиться к пакету, который также используется через Bun, но наличие
advisory само по себе не добавляет зависимость в граф GitHub и не подтверждает
покрытие установок Bun alerts-ами.

Обновляющий модуль Python поддерживает файлы зависимостей `.txt`. Модуль Docker
распознаёт имена Dockerfile, содержащие `dockerfile` (включая варианты
`Dockerfile.ollama-*` в этом репозитории); в текущем исходном коде Dependabot
используется регистронезависимый шаблон имён `dockerfile|containerfile`. Этот
охват предполагаемый и не гарантирует обновление каждого закреплённого значения.
В частности, файл export-зависимостей содержит прямую ссылку на wheel, локальные
теги образов в Compose исключены, а разбор Docker-образов имеет ограничения,
описанные ниже.

## Расписание и число PR

Каждая экосистема проверяется еженедельно, по очереди с понедельника по субботу.
Обновления SemVer minor и patch для каждой экосистемы объединяются в один PR;
major обновления остаются отдельными. Для каждого блока экосистемы Dependabot
может держать открытыми не более двух PR обновлений версий. При шести настроенных
блоках это ограничивает общее число открытых PR обновлений версий двенадцатью.
Для PR обновлений безопасности действует отдельное ограничение GitHub; заданный
здесь лимит обновлений версий на них не распространяется.

По умолчанию Dependabot выдерживает трёхдневную паузу перед обновлением версии;
она не задерживает обновления безопасности. Конфигурация не добавляет метку
`automerge` и не настраивает автоматическое слияние. Стандартные метки Dependabot
для зависимостей не запускают автомерж; предложенные обновления по-прежнему
требуют CI и проверки по обычным правилам репозитория.

PR Dependabot запускают существующую проверку pull request в репозитории. В
актуальной документации биллинга GitHub стандартные GitHub-hosted runners для
Dependabot указаны как бесплатные, в том числе для приватных репозиториев.
Репозиторий приватный, но общие настройки биллинга аккаунта и остаток квоты не
проверялись; это не гарантирует отсутствие платы за другие запуски Actions или
хранение данных.

## Ветки и обновления безопасности

Веткой по умолчанию остаётся `main`. Параметр `target-branch: develop` действует
на обновления версий: Dependabot читает манифесты из `develop` и направляет PR на
эту ветку. По документации GitHub, настройки `target-branch` не применяются к
обновлениям безопасности. Такие PR продолжают направляться в ветку по умолчанию
(`main`) и используют отдельные настройки и лимиты. Существующее поведение npm
security alerts и PR обновлений безопасности в `main` не меняется при замене
обновления версий на Bun; это не подтверждает покрытие безопасности для lock-файла
Bun в `develop`.

17 ноября 2026 года read-only запросы к GitHub API вернули HTTP 204 для endpoint
предупреждений об уязвимостях репозитория и `{ "enabled": true, "paused": false }`
для автоматических исправлений безопасности. Эти результаты подтверждают только
состояние двух endpoint на тот момент; они не устанавливают состояние всех
функций безопасности кода.

## Ограничения проверки

Dependabot обновляет манифесты, но не заменяет проверку совместимости и CI. По
документации GitHub, обновление Dockerfile может затронуть только первую
инструкцию `FROM`, а ссылки на образы в инструкциях `ARG` не обновляются. Во
`frontend/Dockerfile` один закреплённый образ Node используется и на этапе сборки,
и на этапе запуска, поэтому нужно проверять, что предложенное обновление сохраняет
одинаковую версию в обоих этапах. Также проверяйте изменения Docker-образов и
Compose. Для локально собранных образов `project-creator-ollama-intel` и
`project-creator-ollama-amd-vulkan` нет внешнего источника версий, поэтому Compose
updater их игнорирует.

Перед слиянием изучайте примечания к выпуску каждого изменения и запускайте
подходящие проверки репозитория. Не принимайте major обновление только потому,
что его предложил Dependabot.

## Источники

- [Обновления версий Dependabot](https://docs.github.com/en/code-security/concepts/supply-chain-security/dependabot-version-updates)
- [Поддерживаемые экосистемы Dependabot](https://docs.github.com/en/code-security/reference/supply-chain-security/supported-ecosystems-and-repositories)
- [Параметры конфигурации Dependabot](https://docs.github.com/en/code-security/reference/supply-chain-security/dependabot-options-reference)
- [Dependabot и права GitHub Actions](https://docs.github.com/en/code-security/reference/supply-chain-security/troubleshoot-dependabot/dependabot-on-actions)
- [Биллинг и использование GitHub Actions](https://docs.github.com/en/actions/concepts/billing-and-usage)
