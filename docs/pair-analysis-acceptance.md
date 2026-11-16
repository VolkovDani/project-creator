# Детерминированная приёмка анализа пар

Срез для задачи [#85](https://github.com/VolkovDani/project-creator/issues/85),
16 ноября 2026 года. Проверен `main` на
[`d1ccde5ff97ac23d1c46ca99c44af580502247e9`](https://github.com/VolkovDani/project-creator/commit/d1ccde5ff97ac23d1c46ca99c44af580502247e9).
Изменения UI из [PR #83](https://github.com/VolkovDani/project-creator/pull/83)
и инструкции из [PR #84](https://github.com/VolkovDani/project-creator/pull/84)
в этот SHA не входят. Их CI прошёл отдельно; этот отчёт не является проверкой
будущего объединённого состояния.

## Что подтверждает приёмка

Проверяется работа существующего приложения с фиксированными данными и
детерминированными провайдерами. Вердикты заглушек заданы тестами: их успешное
сравнение с ожиданиями не измеряет качество LLM и не подтверждает отсутствие
противоречий в произвольных требованиях. Реальные модели и пользовательские
проекты в этом прогоне не использовались.

Для этой сверки использована отдельная PostgreSQL + pgvector
`pgvector/pgvector:0.8.0-pg16` с публикацией случайного порта только на loopback.
Go integration-тесты создавали и удаляли отдельные схемы. Браузерный backend
использовал отдельную тестовую БД того же временного контейнера. Рабочая БД,
общие контейнеры и веса моделей не изменялись.

## Матрица проверок

Названия ниже позволяют найти точные assertions, данные и границы сценария.
Это существующие тесты, повторно выполненные для среза, а не новое покрытие.

| Условие выпуска | Проверка и ожидаемый результат |
| --- | --- |
| Полное покрытие `all`, отбор `related`, дубли, кеш, история, доступ | [`TestGraphQLQueueSelectionDuplicatesPagesAndHistory`](../internal/api/analysis_queue_test.go): три требования дают три пары в `all`; одна точная копия определяется без LLM, два оставшихся сравнения вызывают заглушку. В `related` выбрана одна пара, две не называют проверенными; выбранная пара переиспользуется без новых вызовов. Проверяются версии источников, `exact_text_v1`, `reusedFrom`, история и запрет чужой области доступа. |
| Глобальные счётчики, границы страниц | [`TestFullQueueCoverage100And200`, `TestFullQueueRunTotalsAreNotCurrentPageSize`](../internal/analysis/full_queue_test.go): подготовка 4 950/19 900 уникальных пар для 100/200 требований, отдельно 66 завершённых пар для 12 требований. Первый тест проверяет подготовку очереди, а не время полного анализа реальными моделями. |
| Семантический отбор и отличие дубликатов | [`TestRelatedQueueFixedRUENCoverageBeyondTopSix`, `TestExactDuplicatesNormalizeWhitespaceWithoutInference`, `TestExactDuplicateDoesNotEraseNegationNumbersCaseOrPunctuation`, `TestSemanticDuplicateIsModelSuggestionAndReusable`](../internal/analysis/related_queue_test.go): фиксированные векторы выделяют 28 из 45 пар; повтор не вызывает embedding/генерацию; значимые различия текста не исчезают при нормализации. |
| Повтор и изменение исходников | [`TestFullQueueCrossRunReuseAndIncrementalChanges`, `TestFullQueueReuseSurvivesStaleWholeRun`](../internal/analysis/full_queue_test.go): четыре требования дают шесть сравнений; повтор использует кеш; изменение одного источника добавляет только три вызова. При добавлении/удалении требования пригодные сравнения сохраняются. |
| Изменение во время inference и профиль модели | [`TestFullQueueRejectsLateResultWhenSourceChanges`, `TestFullQueueProfileChangeDuringInferenceRejectsLateResult`, `TestFullQueueReuseAllVerdictsAndProfileIsolation`](../internal/analysis/full_queue_test.go): поздний/чужой профиль не публикуется как актуальный, непригодный кеш не используется. |
| Отмена, повтор ошибки, рестарт | [`TestCancelFencesLatePairAndIsIdempotent`, `TestPairFailureRetryKeepsCompletedPairsAndRequiresCurrentLatestSnapshot`, `TestShutdownPreservesPartialPairsAndRestartRetriesOnlyInterruptedWork`](../internal/analysis/async_test.go): готовые пары сохраняются, поздние ответы отсекаются, повтор ограничен незавершённой работой. |
| Замена запуска | [`TestReplacementWaitPreservesCurrentPairAndCachesIt`, `TestReplacementCancelFencesLateResponseAndIsIdempotent`, `TestReplacementLatestChoiceSurvivesBackendRestart`](../internal/analysis/replacement_test.go): ожидание и отмена имеют разную семантику, сохранённый выбор переживает рестарт. GraphQL-проводка проверена в [`analysis_replace_test.go`](../internal/api/analysis_replace_test.go). |
| Таймаут и недоступность модели | [`TestAnalysisInvalidOutputRetryAndTimeout`](../internal/analysis/postgres_test.go), [`TestLateSuccessfulGeneratorIsTimeout`](../internal/analysis/service_test.go), [`TestPipelineSkipsUnavailableLayaButRunsEveryFinalPair`](../internal/analysis/pipeline_test.go): ошибки и поздний успех не маскируются успешным результатом, недоступная Laya не блокирует сравнение оригиналов. |
| Нет выжимки / выжимка длиннее лимита | [`TestLongPreviewIsSavedAndSkipsOnlyLaya`, `TestUnconfiguredSummariesNeverSendOriginalsToLaya`](../internal/summary/optional_laya_test.go), [`TestComparisonKeepsFullLongOriginal`](../internal/analysis/provider_test.go): длинная выжимка сохраняется, пропускается только Laya, оригиналы не заменяются выжимками и не обрезаются. |
| Поиск во время анализа | [`TestPipelineSearchDoesNotWaitForEitherModel`](../internal/analysis/pipeline_test.go), [`inference_priority_test.go`](../internal/rag/inference_priority_test.go): детерминированное управление конкурентностью, без оценки аппаратной задержки. |
| SSE replay/reset, область доступа и гонки | [`TestEventReplayResetAndWorkspace`, `TestEventResetSnapshotDoesNotLoseNewEvents`, `TestEventsVisibleOnlyAfterCommit`](../internal/api/events_test.go), [`analysis_live_test.go`](../internal/api/analysis_live_test.go), [`cache.test.ts`](../frontend/tests/cache.test.ts), [`analysisLive.test.ts`](../frontend/tests/analysisLive.test.ts): порядок ревизий, дедупликация, очистка и невозможность старого ответа затереть новые данные. |
| Браузерные состояния анализа | [`analysis.spec.ts`](../frontend/e2e/analysis.spec.ts): режимы, счётчики, источники, история, повтор, отмена, замена, устаревание, два клиента, reload/reconnect-reset и отзыв доступа. [`analysis-live.spec.ts`](../frontend/e2e/analysis-live.spec.ts): пропущенный финальный снимок восстанавливается через reset, поздний снимок не затирает новый, слишком длинная выжимка не блокирует финальный результат. Браузерные ответы анализа/SSE контролируются тестовыми маршрутами. |

Сквозной GraphQL-сценарий уже связывает отбор, результаты, версии, дубли и кеш.
Отдельный тест с теми же assertions не добавлялся. Не выявлено дефекта, для
которого в этой задаче требовалось бы изменение поведения или новый Red/Green.

## Команды и результаты

Для повторения нужны Go из `go.mod`, Node по конфигурации CI, Docker и Chromium
для установленной версии Playwright. `TEST_DATABASE_URL` и `E2E_DATABASE_URL`
должны указывать на **изолированные тестовые БД**, а не на рабочее приложение.
Пример ниже предполагает, что они уже созданы. Порты 8080 и 3100 должны быть свободны.

```bash
# Из корня; -p 1 исключает конкуренцию пакетов за общий model advisory lock.
mkdir -p logs/85-acceptance
CI=1 go test -p 1 -count=1 -json ./... > logs/85-acceptance/go-tests.jsonl
go vet ./...
cd frontend
npm ci
npm run hooks:install
npm run format:check
npm run typecheck
npm run build  # prebuild также запускает npm test
npm run e2e -- --grep-invert 'screenshots:'
```

Go: **225 тестов верхнего уровня прошли**, **10 пропущены** только как явные
opt-in проверки реальных Ollama/NPU/выжимок; вложенные subtests в это число не
добавлены. Пропущенных PostgreSQL integration-тестов нет. `go vet ./...` прошёл.

Frontend: **67 unit-тестов** и **44 E2E-сценария** прошли; `format:check`,
`typecheck` и production build прошли. E2E — Chromium из локальной установки
Playwright; screenshots и отдельный UX-suite в этот прогон не включались.
Адаптивность #44 подтверждается отдельным CI PR #83, а не этим результатом.

Локальное окружение: Go 1.27.3, Node 24.21.0; CI закрепляет Node 24.22.1.
Исходный запуск Go без `/usr/local/go/bin` в PATH не стартовал; после исправления
PATH выполнена команда выше. Первый Node-прогон до установки зависимостей
получил ошибки загрузки `graphql`; окончательные результаты приведены после
успешного `npm ci`. Это ошибки подготовки окружения, не результаты приёмки.

## Что остаётся до релиза

- Принять итоговый UI после слияния #83 и проверить окончательный SHA выпуска.
- Отдельно проверить качество и lifecycle выбранных реальных моделей/NPU.
  Предыдущие аппаратные результаты и ограничение quality smoke **4/5** описаны
  в [отчёте выжимок](requirement-summary-verification.md); этот прогон их
  не обновляет и не снимает таймаут плотного примера.
- Зафиксировать допустимый размер проекта и пороги времени/памяти. Детерминированная
  подготовка 19 900 пар не является нагрузочной приёмкой реальной LLM.
- Браузерные SSE-тесты моделируют события и reset; replay и cursor/reset
  покрыты отдельными серверными/клиентскими тестами. Фактический сетевой обрыв
  с последующим reconnect/replay через браузер в этой приёмке не проверялся.
- Выполнить отдельную приёмку чистой установки и обновления существующей БД,
  закрепить релизный профиль, подготовить release notes и выпуск.

## Состояние зависимостей

`npm ci` завершился успешно, но `npm audit --json` сообщил о **14 затронутых
пакетах** (12 high, 2 moderate). Это не 14 независимых уязвимостей: значительная
часть — распространение предупреждений по цепочке GraphQL Code Generator.
Непосредственные advisory затрагивают `@graphql-tools/utils` (prototype pollution:
[mergeDeep](https://github.com/advisories/GHSA-7mx3-vvmw-hjmv),
[mergeIncrementalResult](https://github.com/advisories/GHSA-c4qx-w738-7hr3))
и `minimatch` ([DoS](https://github.com/advisories/GHSA-g3hv-75wg-m249)).

`npm audit --omit=dev --json` сообщил **0** находок в production-зависимостях.
Это отделяет выявленную цепочку инструментов разработки/генерации от runtime,
но не отменяет её значимости для сборки. Автоматический `audit fix --force` не
применялся: предлагаемые изменения затрагивают major-версии codegen. До выпуска
нужна отдельная оценка и проверенное обновление инструментов; зелёные тесты
не закрывают этот пункт. Lockfile этой задачей не изменён.

## Артефакты и очистка

Диагностика сохранена в корневой игнорируемой `logs/85-acceptance/`: JSONL Go,
вывод vet, npm/build/E2E и результаты audit. Временный контейнер
`project-creator-85-postgres` вместе с созданным для него volume удалён после
проверок. Тестовые web-серверы остановлены Playwright. Рабочие контейнеры,
постоянные volumes, реальные данные и веса моделей оставлены без изменений.
