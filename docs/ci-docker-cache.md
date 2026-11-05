# Кеш Docker-сборок в GitHub Actions

Job `containers` использует официальный Buildx и `type=gha,version=2`:
[cache backend Docker](https://docs.docker.com/build/cache/backends/gha/),
[пример для Actions](https://docs.docker.com/build/ci/github-actions/cache/).

Каждый образ имеет независимый scope: `ci-intel-v1`, `ci-backend-v1`,
`ci-frontend-v1`. `mode=max` сохраняет и промежуточные стадии, включая установку
зависимостей. Содержимое и инструкции Dockerfile по-прежнему определяют
пригодность слоя; кеш не подменяет проверку исходников и версий.

`load: true` загружает результат в Docker Engine runner. Теги backend/frontend
совпадают с именами Compose-проекта `project-creator-ci-smoke`, поэтому
`docker compose up --no-build --wait` запускает именно эти образы. Intel-образ
по-прежнему проверяется отдельным контейнером без GPU. Затем выполняются прежние
HTTP smoke/restart проверки и удаление одноразового стека с volumes.
Образы в registry не публикуются, дополнительные секреты не нужны.

Большой Intel/Ollama-образ содержит CUDA-библиотеки даже при проверке без GPU.
Отдельный builder и Docker Engine временно требуют места для своих копий слоёв.
Перед сборкой в одноразовом GitHub-hosted runner удаляются неиспользуемые .NET и
Android SDK (`/usr/share/dotnet`, `/usr/local/lib/android`). После проверки Intel
его локальный образ и данные созданного этим job builder очищаются до сборки
backend/frontend. Экспортированный удалённый кеш сохраняется. Эти операции не
запускаются локальными тестами, не затрагивают другие runner jobs или хост
разработчика; перенос job на self-hosted runner требует пересмотра этой очистки.

Экспорт каждого кеша ограничен 60 секундами и `ignore-error=true`: сбой записи
кеша не превращает успешную сборку в ошибку проверки. При отсутствии кеша
выполняется обычная полная сборка. При обнаружении проблемы с кешем удалите
соответствующие записи через страницу Actions/Caches или измените суффикс `v1`;
это не удаляет данные приложения.

## Границы и измерения

- Кеширование не означает нулевой сетевой трафик: слои кеша нужно получить,
  а metadata базовых образов проверяется сборщиком. Первый прогон холодный.
- Встроенный PostgreSQL service job `checks` стартует до steps; этот кеш слоёв
  не предотвращает загрузку его образа. PostgreSQL и Node fixture в Compose
  также могут загружаться Docker Engine отдельно.
- Cache ограничен квотой, сроком хранения и правилами доступа GitHub. PR может
  читать доступный кеш своей/base/default ветки; общий тёплый baseline появляется
  после успешного запуска на `main`. Кеш PR не является общим кешем всех веток.
- Кеш включает build layers, а не БД, volumes, веса моделей или NPU runtime.
  `.dockerignore` исключает `.env`, `.npu`, `.project-creator`, logs и backups.
- Кеш не отключает smoke и не заменяет чистые временные данные тестирования.

Исходный прогон PR #65, CI №136 (5 ноября 2026): Intel build/verification —
100 с, Compose build/start — 182 с, весь containers job — 324 с.
Это исходное измерение, а не обещание ускорения. Для оценки выполните холодный
и следующий повторный CI-прогон с тем же содержимым, сравните времена шагов,
строки `CACHED`/импорт кеша и время его экспорта. Конкретный выигрыш и расходы
кеша можно подтвердить только в GitHub Actions; локальные проверки ниже
подтверждают конфигурацию и согласованность тегов.

## Инструменты сборки

Используются официальные проекты Docker/Moby с Apache-2.0:

| Инструмент | Закрепление |
| --- | --- |
| [setup-buildx-action](https://github.com/docker/setup-buildx-action) v4.4.1 | `f87e5991a6d7451dcb8d9637bfbc97413f497069` |
| [build-push-action](https://github.com/docker/build-push-action) v7.4.0 | `c3c9e263c25d99ce0380d002d59b67737d91b0dc` |
| [Buildx](https://github.com/docker/buildx) | v0.38.2 |
| [BuildKit](https://github.com/moby/buildkit) | v0.34.2, manifest digest `sha256:c5f8862f52499773af85bc07695b2379ef08e8002352b0df2c4864c91d78dabe` |

Actions выполняются на Node 24. Существенные runtime-зависимости их готовых
bundles — `@actions/core`, `@docker/actions-toolkit`, Handlebars и js-yaml;
SHA фиксирует поставляемый bundle и lockfile, а не плавающий major tag.
Зависимости приложения не изменяются. Проекты сопровождаются Docker; на момент
проверки 5 ноября 2026 API опубликованных security advisories обоих action-репозиториев
вернул пустые списки. Это не полный аудит транзитивных зависимостей или гарантия
отсутствия уязвимостей; обновление закреплений требует повторной проверки.

Проверки из корня:

```bash
node --test scripts/ci-cache.test.mjs
node --test scripts/*.test.mjs
docker compose --env-file .env.smoke.example -f compose.yml -f compose.smoke.yml -p project-creator-ci-smoke config --quiet
git diff --check
```
