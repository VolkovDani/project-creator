# Зависимости генерации GraphQL

Обновление [#88](https://github.com/VolkovDani/project-creator/issues/88),
16 ноября 2026 года. Это инструменты разработки и сборки; runtime-зависимости
приложения, SDL и документы операций не меняются.

## Выбор и совместимость

Сохраняем GraphQL Code Generator от The Guild: существующий инструмент с
документированными миграциями и интеграциями. При выборе зависимостей учитываем
реальное использование, поддержку, происхождение и уязвимости, а не только звёзды.
`@graphql-typed-document-node/core@3.2.0` оставлен по решению владельца:
пакет содержит определения TypeScript без исполняемой реализации, связан
с экосистемой Codegen. Его MIT-лицензия и содержимое проверены локально.

Закреплены CLI и typed-document-node 8.0.0, typescript-operations 7.0.1
(MIT). Минимум Node 22.18 удовлетворяется проектным Node 24;
GraphQL остаётся 16.11.0. См. [миграцию CLI](https://the-guild.dev/graphql/codegen/docs/migration/from-7-0).

По [миграции операций](https://the-guild.dev/graphql/codegen/docs/migration/operations-and-client-preset-from-5-0)
плагин `typescript-operations` самостоятельно генерирует используемые input-типы.
Лишний прямой плагин `typescript` удалён: совместное использование создавало
дубли типов, подтверждённые падающим `typecheck`. Файл `lib/generated.ts`
пересоздан штатной командой. Все 30 документов GraphQL совпадают с исходными;
изменились только определения TypeScript. Неиспользуемые типы всей схемы
больше не генерируются, nullable-поля операций отражают фактический ответ.

## Исправления и ограничение override

Исходный `npm audit` завершался с кодом 1: 14 затронутых пакетов
(12 high, 2 moderate), а не 14 независимых уязвимостей.

- [GHSA-7mx3-vvmw-hjmv](https://github.com/advisories/GHSA-7mx3-vvmw-hjmv)
  и [GHSA-c4qx-w738-7hr3](https://github.com/advisories/GHSA-c4qx-w738-7hr3):
  обновлённая цепочка разрешает `@graphql-tools/utils` 13.2.2 и 12.1.0,
  выше уязвимых диапазонов (второе исправление — начиная с 12.0.2).
- [GHSA-g3hv-75wg-m249](https://github.com/advisories/GHSA-g3hv-75wg-m249):
  `graphql-config@6.0.0` всё ещё требует minimatch `^10.0.0`.
  Поэтому только для него задан override `minimatch@11.0.1` — официальный
  исправленный выпуск, а не форк. Это переход через major: отдельный тест
  `tests/codegenDependencies.test.ts` проверяет загрузку конфигурации и
  соответствие schema/documents/include/exclude, вложенные пути и dotfiles.
  Override следует убрать после перехода upstream на исправленный диапазон,
  повторив тест, codegen и audit.

minimatch 11.0.1 имеет лицензию BlueOak-1.0.0 и требует Node 18/20/22+.
Его цепочка закреплена lockfile, включая brace-expansion 5 и
`@isaacs/nonrecursive-glob-match`; это части реализации glob того же
сопровождающего, не новые средства приложения. Аудит проверяет всю цепочку.
Удаление override без проверки вернёт известную уязвимость.

## Проверки

На Node 24.21.0 прошли чистый `npm ci`, установка Git hook,
`npm run codegen`, `npm run format:check`, `npm run typecheck`,
`npm run build` (включая unit-тесты). Полный и production-only
`npm audit --json` сообщают 0 находок. Логи — `logs/88-codegen/`.
Нулевой audit отражает известные advisory на дату проверки, а не гарантию
отсутствия любых дефектов. Go-код и реальные модели не затронуты.
