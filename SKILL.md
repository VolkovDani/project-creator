---
name: gh
description: >
  Работа с GitHub через GitHub CLI (gh): репозитории, Issues, Pull Requests,
  ревью, GitHub Actions, релизы и REST/GraphQL API. Используй при выполнении
  команд gh и выборе команд для операций с GitHub. Не относится к GitLab
  или локальным операциям Git без обращения к GitHub.
---

# GitHub CLI (gh)

Используй `gh` для запрошенных CLI-операций с GitHub. Предпочитай предметную
команду (`issue`, `pr`, `run`), а `gh api` — для недостающих возможностей.
При явном выборе пользователем CLI используй его; в остальных случаях учитывай
инструкции среды о выборе между CLI и подключённым GitHub-инструментом.

## Контекст и справка

- Проверь `gh --version` и справку нужной команды: `gh <command> <subcommand> --help`.
  Не переноси флаги из `glab` и не считай авторизацию заранее настроенной.
- Определи нужный remote и репозиторий. `gh repo view --json nameWithOwner,url,defaultBranchRef`
  показывает выбранный CLI репозиторий; при fork или нескольких remotes сверяй
  его с целью пользователя. Для предметных команд указывай `-R [HOST/]OWNER/REPO`.
- У `gh api` нет `-R`: используй явный путь `repos/OWNER/REPO/...` либо
  `GH_REPO=OWNER/REPO` и заключённые в кавычки `{owner}` / `{repo}`.
  Для GitHub Enterprise у API указывай `--hostname HOST`.
- При проблемах доступа проверь `gh auth status --hostname HOST`. Не выводи
  токены через `gh auth token`, `--show-token`, дамп окружения или отладку запросов.
  Не меняй аккаунт и scopes без необходимости для согласованной задачи.
- Запускай команды без редактора и диалогов: явные номера/ID, заголовки,
  `--body-file`, `--base` и `--head`. Не меняй глобальную конфигурацию ради этого.

## Чтение Issues, PR и CI

Примеры используют условные `OWNER/REPO`, номера и ID: подставляй проверенные значения.
Выбирай только нужные JSON-поля и ограничивай списки; полный обход делай,
когда полнота действительно нужна. Доступные поля уточняй в справке команды.

```bash
gh issue list -R OWNER/REPO --state open --limit 50 --json number,title,labels,url
gh issue view 123 -R OWNER/REPO --json number,title,body,state,comments,url
gh pr list -R OWNER/REPO --state open --limit 30 --json number,title,headRefName,url
gh pr view 456 -R OWNER/REPO --json number,body,state,baseRefName,headRefName,headRefOid,reviewDecision,statusCheckRollup,url
gh pr diff 456 -R OWNER/REPO
gh pr checks 456 -R OWNER/REPO --json name,state,bucket,link
gh run list -R OWNER/REPO --commit FULL_SHA --limit 10 --json databaseId,headSha,status,conclusion,url
gh run view RUN_ID -R OWNER/REPO --json headSha,status,conclusion,jobs,url
gh run view RUN_ID -R OWNER/REPO --log-failed
```

- `gh pr checks` возвращает код **8**, если проверки ещё ожидаются.
  Отсутствие checks и пустой список required checks не доказывают успех CI.
- Различай `status` и `conclusion`: незавершённый run с пустым conclusion
  ещё не прошёл. `gh run view --exit-status` сам по себе не доказывает завершение.
- Проверяй SHA и попытку запуска: зелёный run старого коммита не подтверждает новый.
  Для конкретной попытки доступен `gh run view RUN_ID --attempt N`.
- Наблюдай с разумным интервалом, например 60 секунд. Перед повторным прогоном
  выясни причину сбоя; выбирай нужный job или `gh run rerun RUN_ID --failed`,
  если это соответствует задаче. Учитывай расход квоты Actions.

## Создание и изменение

Skill описывает команды, но не расширяет разрешённую задачу. Создавай Issues,
комментарии, ревью, PR и релизы только в согласованной области. Уже полученное
разрешение сохраняется; повторное подтверждение для каждого шага не нужно.
Сначала прочитай инструкции и подходящий шаблон в `.github/`.

Многострочный Markdown сохраняй в файл через инструмент записи или quoted heredoc
(`<<'EOF'`), затем передавай `--body-file PATH`. Не вставляй текст с обратными
кавычками и `$()` в исполняемую shell-строку; сохраняй настоящие переводы строк.
Создавай собственные временные файлы без перезаписи чужих и удаляй их после работы.

```bash
gh issue create -R OWNER/REPO --title 'Заголовок' --body-file ISSUE_BODY_PATH
gh issue edit 123 -R OWNER/REPO --body-file ISSUE_BODY_PATH
gh issue comment 123 -R OWNER/REPO --body-file COMMENT_BODY_PATH
gh pr create -R OWNER/REPO --base main --head feature-branch --title 'Заголовок' --body-file PR_BODY_PATH --draft
gh pr edit 456 -R OWNER/REPO --body-file PR_BODY_PATH
gh pr comment 456 -R OWNER/REPO --body-file COMMENT_BODY_PATH
gh pr review 456 -R OWNER/REPO --comment --body-file REVIEW_BODY_PATH
```

- Перед созданием PR проверь и отправь согласованную ветку. `--head` отключает
  автоматический push/fork при создании PR; локальных коммитов недостаточно.
  `gh pr create --dry-run` может отправить Git-изменения и не является чистым чтением.
- Выбирай draft по готовности и задаче пользователя. Тело PR описывает конечное
  поведение и выполненные проверки; `Closes #123` добавляй, если PR решает Issue.
- При сетевой ошибке или ненулевом коде создания сначала проверь, появился ли
  объект. Не повторяй запись вслепую: возможны дубликаты и частичный успех.
- После записи проверь итоговое состояние и сохрани URL. Закрытая Issue или PR
  со статусом closed не означают, что изменения слиты; проверяй `mergedAt`.

## API: отличия, которые легко пропустить

```bash
# Параметры переключают метод на POST — для чтения явно нужен GET.
GH_REPO=OWNER/REPO gh api --method GET 'repos/{owner}/{repo}/issues' -f state=open -F per_page=100

# Полный список inline review-комментариев PR.
gh api --paginate --slurp repos/OWNER/REPO/pulls/456/comments --jq 'add'

# Обычные комментарии в Conversation PR.
gh api --paginate --slurp repos/OWNER/REPO/issues/456/comments --jq 'add'
```

- `-f key=value` передаёт строку; `-F` преобразует bool/null/целые, подставляет
  placeholders и читает `@PATH`. Для буквального `@user` используй `-f`,
  для Markdown из файла — `-F body=@PATH`.
- Для массивов используй повторные `-f 'labels[]=bug'`, для вложенных полей —
  `key[subkey]`. `-F 'labels=["bug"]'` не разбирает JSON-массив как в `glab`.
  Сложное тело передавай JSON-файлом через `--input PATH` с явным методом.
- При `--input` дополнительные `-f`/`-F` становятся query-параметрами,
  а не дополнениями JSON-тела.
- `--paginate` возвращает отдельные документы страниц; `--slurp` собирает их
  во внешний массив. Для GraphQL нужны `$endCursor` и `pageInfo { hasNextPage endCursor }`.
  Учитывай пагинацию вложенных connections отдельно.
- PR Conversation comments идут через `/issues/NUMBER/comments`, inline review
  comments — через `/pulls/NUMBER/comments`, reviews — через `/pulls/NUMBER/reviews`.
  `gh pr view --comments` недостаточно для полного ревью. Для ответа в inline
  thread используй API review comments; для resolve/unresolve — GraphQL
  `resolveReviewThread` / `unresolveReviewThread` с ID thread. Сверяй актуальную
  схему и endpoint перед записью; номер PR, ID комментария и ID thread различаются.
- REST `/issues` включает также PR: при поиске только Issues исключай объекты
  с полем `pull_request` либо используй `gh issue list`.

## Слияние

Когда слияние входит в поручение, проверь актуальные base/head, diff, замечания
ревью, конфликты и необходимые проверки. Не повторяй законченное ревью
неизменившегося кода без конкретной причины. Перед merge заново получи `headRefOid`
и сопоставь с проверенным SHA; при изменении проверь новые изменения и их CI.

Выполняй выбранную для репозитория стратегию с защитой от гонки, например:

```bash
gh pr merge 456 -R OWNER/REPO --squash --match-head-commit VERIFIED_FULL_SHA
gh pr view 456 -R OWNER/REPO --json state,mergedAt,mergeCommit,url
```

Не обходи правила через `--admin`. `--auto` и merge queue могут оставить PR
в ожидании: сообщай это состояние, не объявляй слияние завершённым.
Удаление ветки (`--delete-branch`) выполняй только если оно поручено.

## Источники

- [Справочник команд](https://cli.github.com/manual/)
- [API: поля, методы и пагинация](https://cli.github.com/manual/gh_api)
- [Создание PR: push, fork и dry-run](https://cli.github.com/manual/gh_pr_create)
- [Слияние PR: SHA, auto-merge и очередь](https://cli.github.com/manual/gh_pr_merge)

Локальная `--help` уточняет возможности установленной версии; документация
GitHub уточняет схему API. При расхождении проверь поддержку нужного флага,
не обновляй CLI и не устанавливай расширения без необходимости для задачи.
