# Project Creator

[English version](README.en.md)

## Установка одной командой

На нативном Linux x86_64 с Python 3.12 и Docker Engine/Compose:

```bash
./project-creator setup --device cpu  # либо arc, nvidia, amd, amd-vulkan, npu
./project-creator up
./project-creator doctor
./project-creator down
```

`setup` явно скачивает локальные модели, подготавливает NPU при выборе этого
устройства, запускает приложение и проверяет настоящий AI-сценарий. Существующие
`.env`, имя Compose-проекта, данные и веса сохраняются. Системные prerequisites,
диагностика, повтор после прерывания и аппаратные ограничения описаны в
[инструкции CLI](docs/setup-cli.md). Установка драйверов выполняется отдельно.

Система знаний о продукте и проверки бизнес-требований на возможные коллизии.
Цели — [GOAL.md](GOAL.md), архитектурные контракты — [ARCHITECTURE.md](ARCHITECTURE.md).

## Состояние MVP

Реализованы карточки проектов и бизнес-требования, GraphQL API с PostgreSQL
и интерфейс Next.js. Доступны фоновая индексация локальными embeddings и явный поиск
по смыслу в пределах проекта. Явный анализ локальной LLM проверяет выбранные
пары на возможные коллизии и сохраняет версии источников.

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

Предварительная классификация Laya и конвейер двух моделей включаются явно;
настройка и ограничения длины — в [инструкции Laya](docs/laya-pipeline.md).
Аппаратная приёмка #48 остаётся открытой.

## Запуск всего MVP в Docker Compose

Требуются Docker Engine с Compose v2 (команды `docker compose`) либо Docker
Desktop с Linux-контейнерами. Go и Node.js на хосте для запуска приложения не нужны.
Запускайте команды из корня репозитория. По умолчанию используется CPU; GPU
не обязателен, но на медленном CPU анализ может превысить timeout.

В конфигурации закреплены Go 1.26.10, Node.js 24.22.1 на Debian Bookworm,
PostgreSQL 16 с pgvector 0.8.0 и Ollama 0.40.16. Модели загружаются отдельно:

| Назначение | Модель | Размер загрузки весов |
| --- | --- | --- |
| Многоязычные embeddings, 1024 измерения | `bge-m3:567m` | около 1.2 GB |
| Анализ пар требований, Q4_K_M | `qwen2.5:7b` | около 4.7 GB |

Размеры опубликованы в официальных карточках [BGE-M3](https://ollama.com/library/bge-m3)
и [Qwen2.5 7B](https://ollama.com/library/qwen2.5:7b). Теги моделей могут меняться;
приложение проверяет полный digest установленной модели. Карточка BGE-M3 указывает MIT, Qwen2.5 7B — Apache 2.0; другие размеры
Qwen могут иметь другую лицензию.

Для планирования выделите 24–32 GiB RAM на CPU-вариант всего стека и не менее
20 GiB свободного диска для образов, весов и сборки, дополнительно место под БД
и резервные копии. Для необязательного GPU-варианта ориентир — 12–16 GiB VRAM
плюс RAM для приложения/БД. Это приблизительный бюджет, а не измеренный минимум:
генератор запрашивает контекст 32768 токенов, и память зависит от KV-кеша,
параллельной загрузки моделей, драйвера и runtime. Размер файла весов не равен
потреблению RAM/VRAM. Проверяйте фактическую загрузку через `docker stats` и
`docker compose exec ollama ollama ps`.

### Первый запуск и загрузка моделей

```bash
cp .env.example .env
chmod 600 .env
```

Отредактируйте `.env`: замените `POSTGRES_PASSWORD` и `PROJECT_CREATOR_TOKEN`
на разные случайные значения; токен — минимум 32 символа. Например, получите
каждое значение отдельным вызовом `openssl rand -hex 24`. Для пароля в Compose
используйте hex без специальных символов URL. `.env` не коммитьте. Переменные
`PROJECT_CREATOR_DATABASE_URL` и `POSTGRES_PORT` нужны для запуска API на хосте,
а полный Compose сам задаёт внутренний адрес БД.

```bash
docker compose up -d --wait postgres ollama
docker compose exec ollama ollama pull bge-m3:567m
docker compose exec ollama ollama pull qwen2.5:7b
docker compose --profile tools run --rm model-info
```

Последняя команда печатает JSON локального `/api/tags`. Найдите записи с нужным
`name` и скопируйте их **полные** `digest` вида `sha256:` и 64 hex-символа;
короткий ID из `ollama list` не подходит. Добавьте в `.env`:

```dotenv
PROJECT_CREATOR_EMBEDDING_MODEL=bge-m3:567m
PROJECT_CREATOR_EMBEDDING_DIGEST=sha256:полный-digest-установленной-embedding-модели
PROJECT_CREATOR_GENERATOR_MODEL=qwen2.5:7b
PROJECT_CREATOR_GENERATOR_DIGEST=sha256:полный-digest-установленного-генератора
```

```bash
docker compose up -d --build --wait
docker compose ps
```

Откройте [http://127.0.0.1:3000](http://127.0.0.1:3000). При занятом порте задайте
`APP_PORT`, например `APP_PORT=3100`, в `.env` и используйте новый порт. На хост
публикуется только `127.0.0.1:${APP_PORT:-3000}`. PostgreSQL, backend и Ollama
доступны внутри сети Compose и не публикуют порты. Веса находятся в volume
`ollama_data`, проекты, требования, индексы и анализ — в `postgres_data`.
Имя Compose-проекта входит в реальные имена volumes: сохраняйте его при обновлении.

Веса не скачиваются при старте автоматически. Можно запустить полный стек без AI,
оставив обе пары `MODEL`/`DIGEST` незаданными: CRUD работает, индексация и анализ
сообщают о ненастроенном провайдере. Одной переменной в паре недостаточно;
неверная конфигурация останавливает backend. Переменную
`PROJECT_CREATOR_OLLAMA_URL=http://ollama:11434` задаёт полный Compose.
После изменения модели/digest пересоздайте backend командой
`docker compose up -d --wait backend`. Смена embedding-профиля требует
переиндексации требований для семантического поиска; полный анализ пар от
embeddings не зависит. Смена генератора требует нового анализа, существующие
векторы остаются пригодны.

Healthchecks проверяют PostgreSQL, доступность процесса Ollama и реальный
авторизованный запрос через backend/Next.js. Backend применяет миграции до
приёма запросов. Статус `healthy` не подтверждает загрузку весов или качество LLM.
Для диагностики используйте `docker compose logs --tail=100 backend frontend ollama`
и результат запроса (поле `errors` должно отсутствовать):

```bash
curl -fsS http://127.0.0.1:3000/api/graphql \
  -H 'Content-Type: application/json' \
  --data '{"query":"{projects(limit:1,offset:0){id}}"}'
```

Локальные логи и диагностические отчёты сохраняйте в корневой `logs/`:
NPU runtime и отчёты установки CLI используют `logs/npu/`.
Каталог исключён из Git и Docker build context. Логи контейнеров доступны через
`docker compose logs`; для сохранения в файл используйте
`mkdir -p logs` и `docker compose logs --no-color > logs/compose.log`.

Ручная приёмка: создайте проект с целью, добавьте требования «Оплата разрешена
только картой» и «Оплата разрешена только наличными». Явно проиндексируйте оба,
выполните поиск, запустите анализ. Проверьте ссылки, версии и снимки источников;
возможную коллизию оценивает человек. При недостатке данных или ошибке проверьте
модель и индекс, затем используйте повтор. После `docker compose restart postgres
backend` дождитесь `healthy` и убедитесь, что проект, требования, индекс и результат
остались. Отредактируйте одно требование: прежний анализ должен стать устаревшим.
Отсутствие найденной коллизии не является доказательством согласованности.

### Intel Arc / Core Ultra на ноутбуке

Для встроенной Intel Arc, в том числе Core Ultra 7 268V, добавлен
`compose.intel.yml`: Ollama с Vulkan и Intel Mesa-драйвером внутри контейнера.
Подготовка Ubuntu, первый запуск, проверка фактического GPU offload и возврат
к CPU описаны в [инструкции Intel Arc](docs/intel-arc.md).

### Экспериментальный Intel NPU (отдельная ветка)

Для проверки Core Ultra 7 268V подготовлен явно включаемый host runtime
OpenVINO GenAI и `compose.npu.yml`. Генерация выполняется только на NPU,
BGE-M3 остаётся в локальном Ollama. Вариант требует подготовки драйвера и
IR-весов. После исправления CLI и нормализации ссылок 30.10.2026 прошли
фиксированные аппаратные примеры, включая недостаток данных и перестановку
источников. Полная приёмка с сопоставлением CPU/GPU остаётся отдельной задачей.
Проверены CLI `setup --device npu`, остановка, повторный `up` и `doctor`
с сохранением БД и готовых весов.
Команды, результаты и возврат к Ollama — в [инструкции NPU](docs/intel-npu.md).

Каталог `.npu/` создаётся отдельно командами подготовки NPU, исключён из Git
и Docker-сборки и не появляется автоматически при обычном `docker compose up`.
При CPU/Arc-варианте Ollama он не нужен; для NPU другой пользователь готовит
свои локальные окружения, веса и профиль по инструкции. CLI для автоматизации
этих шагов пока не реализован и описан в [задаче #25](https://github.com/VolkovDani/project-creator/issues/25).

### Необязательный NVIDIA GPU

Через CLI: `./project-creator setup --device nvidia`. При нескольких NVIDIA GPU
задайте `--gpu-id` с UUID из `nvidia-smi -L` (или индексом; CLI сохраняет UUID).
Выбор сохраняется для `up`, `down` и `doctor`.

На Linux предварительно установите совместимый NVIDIA-драйвер и
[NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html),
настройте его для Docker и проверьте доступность GPU контейнерам. Затем:

```bash
docker compose -f compose.yml -f compose.gpu.yml up -d --wait
```

Override запрашивает GPU для Ollama; остальные сервисы не требуют GPU. Пользуйтесь
той же парой `-f` при дальнейших изменениях GPU-конфигурации. Docker Desktop на
macOS не предоставляет Metal GPU этому Linux-контейнеру; используйте CPU-вариант
или отдельный осознанно настроенный локальный runtime. Официальные варианты
описаны в [документации Ollama Docker](https://docs.ollama.com/docker).

### GPU и встроенная графика: матрица запуска

CLI поддерживает нативный Linux x86_64, Python 3.12 и Docker Engine/Compose 2.20+.
Наличие профиля означает поддержку конфигурации; совместимость конкретного
оборудования подтверждается отдельным реальным запуском `setup`/`up`.

| Режим CLI | Устройство и runtime | Подготовка / статус |
| --- | --- | --- |
| `cpu` | Ollama 0.40.16 на CPU | Базовый вариант, GPU не нужен. |
| `arc` | Intel Arc dGPU/iGPU, в том числе Core Ultra 7 268V; Ollama 0.40.16/Vulkan | Intel Linux graphics, доступ к `/dev/dri`; [инструкция Arc](docs/intel-arc.md). Intel ICD изолирован от AMD/NVIDIA. Новая логика выбора проверена тестами без GPU. |
| `nvidia` | Поддерживаемая NVIDIA GPU; Ollama 0.40.16/CUDA | NVIDIA driver и Container Toolkit, `nvidia-smi`; аппаратная проверка новой CLI-интеграции ещё не выполнена. |
| `amd` (ROCm) | Только AMD dGPU/iGPU из матрицы Ollama 0.40.16; `ollama/ollama:0.40.16-rocm` | ROCm 7, `rocminfo`, `/dev/kfd` и `/dev/dri`. Строгая проверка GFX target; аппаратная приёмка не проведена. |
| `amd-vulkan` (RADV) | AMD dGPU/iGPU с работоспособным Vulkan; Ollama 0.40.16 + Mesa RADV | `amdgpu`, доступный `/dev/dri/renderD*` и поддержка Vulkan в драйвере. ROCm и `/dev/kfd` не требуются. Более широкий, но **не универсальный** вариант; аппаратная приёмка не проведена. |
| `npu` | Intel NPU; OpenVINO GenAI, embeddings на CPU | Фиксированные аппаратные примеры на Core Ultra 7 268V проходят; результаты и ограничения — в [инструкции NPU](docs/intel-npu.md). |

**Нельзя гарантировать работу на любой AMD GPU или любом Ryzen APU.** Для ROCm
проверьте модель, ОС и драйвер по [матрице AMD](https://rocm.docs.amd.com/en/latest/compatibility/compatibility-matrix.html)
и [матрице Ollama 0.40.16](https://github.com/ollama/ollama/blob/v0.40.16/docs/gpu.mdx).
Даже если новая версия ROCm поддерживает iGPU, это не расширяет возможности
закреплённого контейнера Ollama автоматически. Пример ROCm:
`./project-creator setup --device amd --gpu-id GPU-...`, где UUID берётся из
`rocminfo`. CLI принимает targets
`gfx908`, `gfx90a`, `gfx942`, `gfx950`, `gfx1030`, `gfx1100`, `gfx1101`, `gfx1102`,
`gfx1150`, `gfx1151`, `gfx1200`, `gfx1201` из
[матрицы Ollama v0.40.16](https://github.com/ollama/ollama/blob/v0.40.16/docs/gpu.mdx).
Это upstream-матрица, не список проверенных проектом видеокарт. Неизвестный target
отклоняется; подмена `HSA_OVERRIDE_GFX_VERSION` не применяется.

Для другой AMD GPU/iGPU можно **попробовать Vulkan**:
`./project-creator setup --device amd-vulkan`. Драйвер RADV из Mesa запускается
без ROCm, а Vulkan ICD изолирован от Intel/NVIDIA. При нескольких AMD render
devices укажите `--gpu-id 0` (или иной **индекс Vulkan из логов Ollama**, не UUID
из `rocminfo` и не номер `renderD*`). Если Vulkan/модель/объём доступной памяти
несовместимы, установка завершится ошибкой; автоматического переключения на
ROCm или CPU нет. Встроенная графика использует общую системную память,
поэтому даже доступный Vulkan не гарантирует загрузку обеих моделей.

Для смены устройства или адаптера повторите `setup` с `--switch-device`.
Данные и закреплённые модели сохраняются. `setup`/`up` проверяют GPU offload
отдельно для embeddings и генератора; нулевой offload даёт ошибку вместо
успешного CPU fallback. Показанные байты в GPU-памяти допускают частичный offload
и не являются доказательством полного выполнения на GPU. `doctor` не запускает
inference и сообщает об отсутствии измерения для незагруженной модели.
Подробности выбора, ручного запуска и аппаратной проверки — в
[инструкции CLI](docs/setup-cli.md#gpu-и-выбор-адаптера).

### Остановка, обновление и резервные копии

`docker compose stop` останавливает процессы; `docker compose down` дополнительно
удаляет контейнеры и сеть. Обе команды сохраняют named volumes. Для продолжения
используйте `docker compose up -d --wait`. Не меняйте имя проекта или путь к
Compose-файлу без переноса volumes: иначе можно получить отдельную пустую БД.

Перед обновлением остановите запись и создайте dump:

```bash
mkdir -p backups
chmod 700 backups
docker compose stop frontend backend
umask 077
docker compose exec -T postgres pg_dump -U project_creator -d project_creator -Fc > backups/project-creator.dump
docker compose up -d --wait
```

Храните dump вне машины приложения и проверяйте восстановление в отдельном
Compose-проекте. Он включает проекты, требования, векторы, версии источников
и историю анализа, но не веса Ollama. Сохраните защищённую копию `.env` вместе
с зафиксированными digests отдельно от git. Для сохранения именно скачанных весов
остановите загрузку/работу Ollama и скопируйте его каталог:

```bash
docker compose stop ollama
docker compose cp ollama:/root/.ollama backups/ollama
docker compose up -d --wait
```

При восстановлении весов остановите Ollama и скопируйте содержимое сохранённого
каталога в его `/root/.ollama`, затем запустите сервис и снова проверьте полные
digests через `model-info`. Альтернатива — загрузить веса заново и проверить digest;
обновлённый тег не гарантирует прежний результат.

Восстановление БД заменяет данные выбранного Compose-проекта. Сначала сделайте
backup его текущей БД; используйте совместимую версию PostgreSQL/pgvector и
приложения. Контейнер PostgreSQL должен работать, приложение — быть остановлено:

```bash
docker compose up -d --wait postgres
docker compose stop frontend backend
docker compose exec -T postgres dropdb -U project_creator --if-exists project_creator
docker compose exec -T postgres createdb -U project_creator project_creator
docker compose exec -T postgres pg_restore -U project_creator -d project_creator --exit-on-error < backups/project-creator.dump
docker compose up -d --wait
```

После восстановления проверьте готовность, список проектов, требования и состояния
индексов. Ошибка пароля после изменения `.env` означает, что пароль уже созданного
PostgreSQL volume остался прежним; меняйте пароль ролью PostgreSQL, а не удалением
volume. Для обновления сначала сохраните backup, изучите миграции и изменения
версий, получите изменения репозитория, затем `docker compose up -d --build --wait`.
Миграции применятся при старте; downgrade не предусмотрен. Major-обновление
PostgreSQL требует `pg_upgrade` или dump/restore, а не смены тега на старом volume.
Обновляйте веса отдельным решением: после смены embedding-профиля нужна
переиндексация для поиска; смена генератора требует нового анализа пар.

Для намеренного **безвозвратного удаления всех локальных данных и весов** выполните
`docker compose down --volumes`. Эта команда удаляет оба named volumes выбранного
проекта; backup за пределами volumes сохраняется. Для обычной остановки она не нужна.

### Container-smoke и CI

Обычный smoke запускает одноразовый Compose-проект с тестовой Ollama-заглушкой.
Он проверяет сборку обоих образов, миграции и полный путь HTTP через Next.js:
проект → требования → индексация → коллизия с версиями источников, затем сохранность
после перезапуска PostgreSQL/backend. Веса и GPU не нужны. Тестовая заглушка
подключается только явным `compose.smoke.yml`; в рабочем Compose её нет.

Для запуска smoke-скрипта на хосте дополнительно требуется Node.js 24. Команды
ниже выполняются в subshell, чтобы тестовые переменные не меняли обычный запуск.
Тест требует пустую БД и отказывается добавлять данные в непустую:

```bash
(
  set -e
  export COMPOSE_FILE=compose.yml:compose.smoke.yml
  export COMPOSE_PROJECT_NAME=project-creator-smoke
  export COMPOSE_ENV_FILE=.env.smoke.example
  export APP_PORT=3200
  docker compose --env-file "$COMPOSE_ENV_FILE" up -d --build --wait --wait-timeout 180
  node scripts/compose-smoke.mjs
)
```

Скрипт сам не создаёт и не удаляет volumes: он проверяет уже запущенный стек и
перезапускает его PostgreSQL/backend. После проверки удалите только тестовый стек:

```bash
docker compose --env-file .env.smoke.example -p project-creator-smoke -f compose.yml -f compose.smoke.yml down --volumes
```

Не подставляйте имя рабочего Compose-проекта. При повторе smoke сначала удалите
именно его тестовые volumes для пустой БД. При ошибке до удаления сохраните вывод
`docker compose ... logs` для диагностики.

Реальный smoke требует отдельного проекта и своего volume с весами. Скопируйте
`.env.example` в `.env.real-smoke`, задайте отдельные случайные пароль и токен,
сначала оставив настройки моделей выключенными. Подготовьте runtime и веса:

```bash
(
  set -e
  export COMPOSE_FILE=compose.yml
  export COMPOSE_PROJECT_NAME=project-creator-real-smoke
  export COMPOSE_ENV_FILE=.env.real-smoke
  docker compose --env-file "$COMPOSE_ENV_FILE" up -d --wait postgres ollama
  docker compose --env-file "$COMPOSE_ENV_FILE" exec ollama ollama pull bge-m3:567m
  docker compose --env-file "$COMPOSE_ENV_FILE" exec ollama ollama pull qwen2.5:7b
  docker compose --env-file "$COMPOSE_ENV_FILE" --profile tools run --rm model-info
)
```

Сохраните полные digests в `.env.real-smoke` по инструкции первого запуска,
затем выполните явную проверку настоящей модели:

```bash
(
  set -e
  export COMPOSE_FILE=compose.yml
  export COMPOSE_PROJECT_NAME=project-creator-real-smoke
  export COMPOSE_ENV_FILE=.env.real-smoke
  export REAL_MODEL_SMOKE=1
  export APP_PORT=3201
  docker compose --env-file "$COMPOSE_ENV_FILE" up -d --build --wait --wait-timeout 180
  node scripts/compose-smoke.mjs
)
```

После проверки команда ниже удаляет только данные и веса этого тестового проекта:

```bash
docker compose --env-file .env.real-smoke -p project-creator-real-smoke -f compose.yml down --volumes
```

Этот запуск не оценивает статистическое качество LLM и может завершиться ошибкой
на медленном CPU; он не включён в обычный CI. Детали standalone-проверок настоящих
адаптеров и требования к тестовой БД приведены ниже. GitHub Actions отдельно
выполняет unit/integration, валидацию SDL/операций и codegen, frontend-тесты,
проверку TypeScript, сборку Next.js, Playwright и container-smoke. Успех обычного
CI подтверждает проводку со стабами, а не точность настоящей локальной модели.

Проверка границ готового технического MVP:

| Сценарий | Проверка и граница |
| --- | --- |
| Создание проекта и работа с требованиями без чата | GraphQL/Playwright; общая валидация и optimistic locking |
| Изоляция данных | Проверка workspace/project до retrieval и вызова моделей; локальный токен вместо полной модели ролей |
| Актуальность двух клиентов | SSE, reconnect/reset, повторные события и гонки чтения/инвалидации |
| Поиск и известная коллизия с источниками | Детерминированные integration/container-тесты; настоящие веса проверяются отдельно |
| Происхождение выводов | ID, версии и снимки источников; устаревание при изменении данных/профиля |
| Локальность и контроль изменений | Нет облачных ключей/fallback; модель не изменяет требования сама |
| Перезапуск и восстановление | Container-smoke проверяет restart; backup/restore проверяйте на своих резервных копиях |

Полноценные роли/участники, функциональные блоки, граф знаний, чат и история всех
редакций остаются частью более широкой карты развития в GOAL, а не завершённого
технического среза. Успех тестового smoke не означает приёмку качества LLM на
реальных требованиях или готовность публичного многопользовательского сервиса.

## Локальная проверка

Требуются Go 1.26.10, C-компилятор для импорта/тестирования SQLite, Node.js 24
и PostgreSQL 16 с pgvector 0.8.0. Обычные тесты не требуют LLM или облачных API.
Интеграционные тесты используют отдельные схемы в базе из `TEST_DATABASE_URL`.
Без этой переменной они пропускаются; такой запуск не проверяет хранилище.

```bash
export TEST_DATABASE_URL='postgresql://project_creator:ваш-пароль@127.0.0.1:5432/project_creator_test?sslmode=disable'
go test -p 1 ./...
go vet ./...
gofmt -l cmd contracts graph internal
```

Тестовая роль должна иметь права создавать и удалять схемы и устанавливать
расширение `vector`. Используйте отдельную тестовую базу, не рабочую.
Пакеты запускаются последовательно (`-p 1`): отдельные схемы тестов
не изолируют общую блокировку модели в PostgreSQL. Параллельные пакеты могут
блокировать запуск заглушек друг друга и вызывать таймауты.
Разработка по TDD: падающий тест поведения → реализация → рефакторинг.

## Карточка проекта

`internal/project.Input`: `name`, `goal`, `description`.
`project.Validate` возвращает нормализованный ввод и список ошибок.
Ошибки содержат `path`, `code`, `messageKey`, `params`; успешная проверка
возвращает пустой список. Валидация не создаёт ID и не сохраняет данные.
Серверный ID, workspace и timestamps добавляются при сохранении.

Источник ограничений — `contracts/project.json`, встроенный в Go-бинарник.
Фронтенд читает тот же файл; общий набор примеров —
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
  TS-адаптер также отклоняет непарные суррогаты до отправки.
- Все ошибки полей собираются за один вызов в порядке name, goal, description.

Общие словари — `contracts/i18n/ru.json` и `en.json`. Они содержат тексты
интерфейса и ошибки валидации; `params.max` передаётся отдельно от текста.
Сервер и UI используют ключ, не анализируют переведённую строку.

## PostgreSQL и запуск API

Решение о хранилище — [ADR-001](docs/adr/001-postgresql-pgvector.md).
В `compose.postgres.yml` закреплены PostgreSQL 16 и pgvector 0.8.0. Это окружение
только БД для разработки с процессами на хосте. Для запуска всего приложения
используйте основной `compose.yml` по инструкции выше.
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
отдельная база. Обычные проверки используют детерминированные заглушки моделей.
Отдельный job собирает контейнеры и проверяет полный HTTP-сценарий со стабом
Ollama; реальные модели запускаются только в opt-in smoke по инструкции выше.
Docker-сборки Intel runtime, backend и frontend используют отдельные кеши слоёв
GitHub Actions; Compose запускает уже загруженные образы без повторной сборки.
Первый запуск остаётся холодным, кеш не исключает загрузку слоёв и сервисных
образов. Версии инструментов, ограничения и проверка эффекта описаны в
[инструкции кеширования CI](docs/ci-docker-cache.md).

Матрица релизных сценариев анализа пар, результаты изолированной приёмки и
оставшиеся ограничения: [отчёт #85](docs/pair-analysis-acceptance.md).

### Автоматическое слияние после CI

`main` остаётся default branch и содержит только принятые изменения. Обычные
изменения направляются PR в `develop`; CI запускается для PR и отменяет
устаревшие прогоны. `develop` создан от `main` на момент подготовки релизного
процесса. Push CI на `main` сохраняется для проверки фактически интегрированного
состояния; отдельный push workflow для `develop` отключён во избежание второго
прогона после PR.

Метка `automerge` действует только для PR этого репозитория с базой `develop`.
Подготовку выпуска выполняйте отдельным PR `develop` → `main` без этой метки;
его слияние вручную, после проверки diff, состояния CI и готовности выпуска.
Release PR не публикует версию.

Изменения только файлов `.md` и `.txt` не запускают GitHub Actions ни для PR,
ни для push в `main`: фильтр `paths-ignore` применяется ко всему workflow.
Если diff содержит хотя бы один другой файл, CI работает как обычно.
Документационный PR после проверки diff и `git diff --check` сливается обычным
merge commit через `gh pr merge --merge --match-head-commit SHA`, без метки
`automerge`: job слияния в этом случае тоже не запускается.
Пропущенный workflow нельзя делать обязательным условием для таких PR:
GitHub может оставить required checks в состоянии Pending. Защиту ветки
не обходить через `--admin`; при добавлении обязательных checks согласовать
их совместимость с этим правилом. См. [фильтры путей GitHub Actions](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#onpushpull_requestpull_request_targetpathspaths-ignore).

Для готового к слиянию PR из ветки этого репозитория в `develop` поставьте
метку `automerge`. Завершающий job `automerge` ждёт успешных `checks` и
`containers`, затем создаёт merge commit с двумя родителями: история сохраняет
коммиты ветки и её соединение с `develop`. Скрипт повторно проверяет базу и
отклоняет PR в `main`. Это отдельный job обычного CI, без платного штатного
GitHub auto-merge, PAT и дополнительных зависимостей.

В настройках репозитория включено **Automatically delete head branches**:
GitHub удаляет удалённую head-ветку после успешного merge PR (включая автомерж).
Закрытие PR без слияния не запускает эту очистку. Локальные ветки сохраняются
до очистки в клоне: `git fetch --prune` удаляет только устаревшие remote-tracking
refs. Агент после merge удаляет подтверждённо интегрированную локальную ветку
по правилам `AGENTS.md`, сохраняя новые коммиты, постоянные `main`/`develop` и
занятые worktree. Если GitHub удалил `develop` после release PR, восстановите её
от актуального `main`.

Метка — явное разрешение на слияние: ставьте её после ревью. Job читает актуальное
состояние PR, поэтому снятие метки до его проверки отменяет слияние. Draft, закрытые
PR, форки, конфликты, изменившийся head и изменение `main` относительно проверенного
merge-коммита приводят к пропуску. GitHub получает ожидаемый head SHA и отклоняет
merge, если head успел измениться. Проверка базы выполняется перед запросом;
атомарной блокировки `develop` между этой проверкой и merge нет. Это не merge queue
и не замена защищённой ветке.

Добавление/снятие метки само по себе не перезапускает CI. Можно поставить метку,
пока проверки идут. Если CI уже завершился, повторите **только job `automerge`**
в Actions (или `gh run rerun RUN_ID --job JOB_ID`), сохранив результаты проверок.
Если изменился head или `develop`, обновите рабочую ветку относительно `develop` и
дождитесь нового полного CI. Сбой API завершает job ошибкой, без автоматического
повтора merge; после проверки причины можно повторить этот job.

Только `automerge` получает `contents: write`; тесты и сборки имеют
`contents: read`. Используется краткоживущий `GITHUB_TOKEN`.
[События от этого токена не запускают push-workflow](https://docs.github.com/en/actions/concepts/security/github_token#when-github_token-triggers-workflow-runs),
поэтому после автоматического merge повторного CI на `main` и обновления кешей
из этого push не будет. Проверки PR остаются основанием слияния; кеши по-прежнему
обновляются на обычных push/ручных merge в `main`.

### Подготовка Source + Compose v0.1.0

Ручной workflow `Draft prerelease` запускается только с `main`. Он проверяет
semantic version tag и полный source SHA, что тег указывает ровно на этот SHA,
что SHA входит в `main`, что успешный CI run на `main` (push или ручной запуск)
завершился на том
же SHA и что закрытая аппаратная issue содержит точные поля tag/SHA и метку
`hardware-accepted`. Затем workflow повторяет проверки и сборки на source SHA и
создаёт только черновик prerelease. Публикация остаётся отдельным ручным
решением maintainer после проверки отчётов и release notes.

Подробная инструкция доступна [на русском](docs/release-acceptance.ru.md) и
[на английском](docs/release-acceptance.md); заметки выпуска доступны
[на русском](docs/release-notes-v0.1.0.ru.md) и
[на английском](docs/release-notes-v0.1.0.md). Фактические проверки и оставшиеся
условия перечислены [в отчёте от 17 ноября 2026 года](docs/release-verification-2026-11-17.md).

## Git hooks и форматирование

Hook хранится в отслеживаемой папке `.githooks/pre-commit`, а его реализация —
в `scripts/biome-pre-commit.mjs`. Git не включает hooks из репозитория автоматически:
на каждом новом компьютере/в каждом новом клоне выполните с Node.js 24:

```bash
git clone https://github.com/VolkovDani/project-creator.git
cd project-creator/frontend
npm ci
npm run hooks:install
```

Для уже существующего клона достаточно последних двух команд из `frontend/`.
Установщик задаёт `git config --local core.hooksPath .githooks`; проверить настройку
можно командой `git config --get core.hooksPath`. Повторная установка допустима.
Если уже настроен другой `core.hooksPath` или есть `.git/hooks/pre-commit`,
установщик сохраняет его и сообщает о конфликте. В этом случае подключите вызов
`.githooks/pre-commit` в существующий механизм hooks после его проверки.

Перед каждым коммитом форматируйте изменённые файлы через закреплённый Biome:

```bash
cd frontend
./node_modules/.bin/biome check --write 'app/projects/[id]/IndexProgress.tsx'
npm run format:check
```

Pre-commit автоматически обрабатывает подготовленные `.ts`, `.tsx`, `.json`,
`.jsonc` внутри `frontend/` с конфигурацией и исключениями `frontend/biome.jsonc`.
Инструмент берётся из локальных `node_modules`; hook ничего не загружает из сети.
Он проверяет временную копию содержимого Git index, включая конфигурацию и
ignore-файлы. Для полностью подготовленных файлов безопасные исправления Biome
переносятся в рабочие файлы и index. Небезопасные исправления не включаются.
Ошибки и оставшиеся предупреждения блокируют коммит.

При частичном staging проверяется только подготовленное содержимое; незастейдженные
изменения не добавляются в коммит и не переписываются. Если подготовленная часть
требует форматирования, hook остановит коммит: примените Biome к нужному файлу,
проверьте diff, заново подготовьте нужные изменения через `git add -p` и повторите
коммит. Сгенерированные файлы остаются исключёнными по конфигурации Biome.
Hook не следует по симлинкам поддерживаемых файлов и отказывается их обрабатывать.

Hook не заменяет `typecheck`, тесты, сборку или CI. Изменения Go и документации
без поддерживаемых frontend-файлов не требуют установленного Biome.
Тесты hooks: после `npm ci` в `frontend/` выполните из корня
`node --test scripts/git-hooks.test.mjs`.
В исходном checkout `npm test` также запускает эту интеграционную проверку после
установки зависимостей. Ранний запуск Node-тестов в CI без `npm ci` проверяет
установщик, а сценарии с настоящим Biome выполняются позже через frontend-тесты.
В Docker-сборке приложения Git tooling отсутствует, поэтому эта проверка hooks
пропускается; остальные frontend-тесты выполняются как обычно.

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

Версии codegen, причина точечного override `minimatch` и порядок его удаления
описаны в [оценке зависимостей генератора](docs/codegen-dependencies.md).

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
версии. Полного журнала всех редакций пока нет, но индекс и попытки анализа
сохраняют версии и снимки использованных источников. После правки требуется
повторная индексация, прежний анализ становится устаревшим. Эти механизмы
реализованы и описаны в разделах RAG и анализа ниже.



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

## Локальные embeddings и поиск требований

RAG использует PostgreSQL + pgvector и локальный Ollama. Без настройки провайдера
проекты и требования продолжают работать; попытка индексации сообщает, что модель
не настроена. Данные не отправляются внешнему API при ошибке локального runtime.

Команды этого раздела рассчитаны на Ollama и backend, работающие на хосте.
Для полного Docker Compose используйте раздел первого запуска: модели
загружаются через `docker compose exec ollama`, настройки задаются в `.env`,
затем пересоздаётся backend. Экспорт переменных в терминале не меняет окружение
уже работающего контейнера; порт Ollama в полном Compose не опубликован на хост.

Установите Ollama по официальной инструкции на https://ollama.com/download и
загрузите модель явно (это отдельная загрузка весов, обычные тесты её не делают):

```bash
ollama pull bge-m3:567m
curl http://127.0.0.1:11434/api/tags
```

Найдите запись выбранной модели и сохраните полный `digest`, включая `sha256:`.
Задайте в окружении backend и перезапустите его:

```bash
export PROJECT_CREATOR_OLLAMA_URL='http://127.0.0.1:11434'
export PROJECT_CREATOR_EMBEDDING_MODEL='bge-m3:567m'
export PROJECT_CREATOR_EMBEDDING_DIGEST='sha256:полный-digest-из-api-tags'
go run ./cmd/server
```

Ollama должен слушать только доверенный локальный интерфейс. Не открывайте его в
интернет. Поддерживаются loopback HTTP-адрес и явное имя локального Docker-сервиса
`ollama`; произвольные удалённые адреса отклоняются. Переключения в облако нет. Не сохраняйте чувствительные адреса или секреты в git.

При настроенном локальном embedding-провайдере новые и изменённые требования
после commit автоматически попадают в фоновую очередь. Сохранение не ждёт модель,
закрытие вкладки не останавливает обработку. Состояния индекса: ожидает,
индексируется, готов или ошибка. Раздел индексации показывает общий прогресс
всего проекта и статусы отдельных требований; ошибки можно повторить вручную.
Без настроенного embedding-провайдера требования сохраняются без вызова модели. Поиск вызывается только кнопкой «Найти требования» и
использует готовые фрагменты этого проекта. Пустая выдача не доказывает отсутствие
дубликатов или противоречий.

Результат показывает найденный фрагмент, версию и исходный текст этой версии;
ссылка ведёт к требованию. После изменения требований результаты помечаются
неактуальными, поиск нужно повторить. Значение similarity не является вероятностью
корректности и не используется для таких утверждений.

Обычные Go/CI-тесты используют детерминированный fake embedding-провайдер;
Playwright проверяет состояния и явные действия с заглушками API. Проверка реальной
локальной модели запускается отдельно и не требует облачного ключа.


Поиск использует точное cosine-сравнение pgvector без HNSW. Профиль связывает
имя модели, полный digest, размерность 1024 и версию разбиения. BGE-M3 выбран
как многоязычный локальный вариант; качество на ваших требованиях необходимо
проверять отдельно. Смена закреплённого профиля исключает старые векторы из поиска; при запуске
backend актуальные требования автоматически ставятся в очередь нового профиля. При перезапуске backend открытые клиенты получают
инвалидацию статусов и результатов, даже если курсор SSE ещё действителен.

Разбиение детерминировано: до 1000 Unicode code points, перекрытие 100, предпочтение
границам абзацев/предложений. Фрагмент хранит точные смещения и текст исходной
версии; требования не объединяются. Это эвристика, а не гарантия качества retrieval.
Индексация имеет короткую транзакцию захвата попытки и lease на две минуты;
embeddings вычисляются вне транзакции с timeout 90 секунд. Завершение сохраняется
только для актуальной версии, действующего профиля и владельца попытки. Очередь
восстанавливается после рестарта и истечения lease. Для временного сбоя модели
предусмотрены до трёх попыток с паузами 5 и 30 секунд; затем нужен явный повтор.
Некорректные embeddings и пустой источник не повторяются автоматически.
Облачного fallback и автоматической загрузки моделей нет. Поисковый запрос ограничен 2000 code points и 20 результатами сервером.

Opt-in smoke реального локального провайдера:

```bash
OLLAMA_SMOKE=1 \
OLLAMA_URL="$PROJECT_CREATOR_OLLAMA_URL" \
OLLAMA_MODEL="$PROJECT_CREATOR_EMBEDDING_MODEL" \
OLLAMA_DIGEST="$PROJECT_CREATOR_EMBEDDING_DIGEST" \
go test ./internal/rag -run TestRealOllama -v -count=1
```

Для полного smoke поиска задайте также `TEST_DATABASE_URL` отдельной тестовой БД
с pgvector. Тесты создают изолированную схему и проверяют ожидаемый источник.
Это короткая проверка проводки; она не заменяет оценку recall на реальном наборе
дубликатов и противоречий. Обычные CI-тесты реальную модель не загружают.


## Анализ возможных коллизий (MVP 7)

После завершения API возвращает `reasonCode` и сохранённую диагностику попытки.
Режим «Все пары» не требует индекса и не возвращает `INDEX_INCOMPLETE`.
Основной режим «Связанные пары» требует актуальные embeddings: при
`INDEX_INCOMPLETE` проверьте общий прогресс, дождитесь очереди либо повторите
ошибочные индексации, затем явно повторите анализ. Анализ не запускается автоматически. Прежние лимиты `REQUIREMENT_LIMIT`/`ANALYSIS_LIMIT` не применяются.
Старые попытки сохраняют свои исходные причины. `TOO_FEW_REQUIREMENTS` означает,
что сравнивать нечего; `MODEL_INSUFFICIENT` — недостаток условий для вывода модели.
Техническая ошибка отдельно отражается в статусе и покрытии.

Анализ запускается только кнопкой в карточке проекта. Индексация нужна для семантического поиска и отбора связанных пар;
для явного режима «Все пары» она не обязательна. Ниже приведена настройка генератора
Ollama для процессов на хосте; в полном Compose задайте те же MODEL/DIGEST в `.env`
и пересоздайте backend. Для OpenVINO NPU используйте [отдельную инструкцию](docs/intel-npu.md).

```bash
ollama pull qwen2.5:7b
curl http://127.0.0.1:11434/api/tags
export PROJECT_CREATOR_GENERATOR_MODEL='qwen2.5:7b'
export PROJECT_CREATOR_GENERATOR_DIGEST='sha256:полный-digest-генератора-из-api-tags'
go run ./cmd/server
```

Генератор использует тот же `PROJECT_CREATOR_OLLAMA_URL`, что и embeddings, но
имеет отдельные модель и digest. Пример — Qwen2.5 7B; веса загружаются только
явной командой пользователя. Адаптер принимает loopback HTTP или имя локального
Docker-сервиса `ollama`, проверяет установленную модель и digest до/после вызова,
отклоняет удалённые модели и не переключается в облако. Обычные операции проектов
и требований работают без генератора. Возможности и качество локальной модели
зависят от выбранных весов и оборудования; успешные fake-тесты не оценивают её
точность на ваших требованиях.

Результат имеет состояние running, failed или completed. На экране различаются
возможная коллизия, недостаток данных и отсутствие найденных коллизий в проверенных
парах. Последнее **не доказывает согласованность проекта**: модель может ошибаться,
а её вывод требует проверки человеком. Полный перебор пар не гарантирует выявление
всех смысловых противоречий. Проценты достоверности не вычисляются. Вывод модели
никогда не изменяет требования.

Анализ запускается явно кнопкой и быстро возвращает ID попытки. Работа продолжается
на сервере после закрытия вкладки. Основной режим «Связанные пары» выбирает
объединение точных совпадений нормализованного текста, пар с cosine similarity
не ниже порога. Совпадение общих слов не обходит порог. Используется
точный поиск по всем актуальным фрагментам, без approximate index и top-5/top-6.
Новые структурные связи не создаются:
в текущей модели данных для требований их пока нет.

Порог настраивается в интерфейсе от 0 до 1; начальное значение0.65 — эвристика
отбора, **не вероятность связанности, коллизии или достоверности**. Порог
и версия правил сохраняются для каждого запуска; причины отбора и cosine score,
где применимо, — для каждой пары. Фиксированный RU/EN набор в тестах проверяет
более пяти подходящих соседей и исключение заведомо посторонних требований.
Эти тесты используют заданные векторы: они проверяют правила отбора, но не
измеряют качество реальной embedding-модели. Порог не откалиброван на клиентских
данных. Embeddings могут пропускать связи или выбирать лишние пары;
непроверенные пары не считаются совместимыми. Правила `related-v2` применяются
к новым запускам; исторические очереди и результаты не пересчитываются.
Для смены правил активного запуска используйте замену анализа.

Отдельный явный режим «Все пары» формирует для N актуальных требований все
N × (N − 1) / 2 уникальных неупорядоченных пар: 4950 для100 требований и19900
для200. Ограничения20 пар/100 требований и top-6 к нему не применяются.
Embeddings и вызов embedding-провайдера этому режиму не нужны.
Недопустимые источники и технические ошибки не считаются успешной проверкой.

Точные дубли определяются без LLM после схлопывания пробелов, табуляции и
переносов строк. Регистр, отрицания, числа и пунктуация сохраняются. В группе
«Дубли требований» показываются оба исходных текста, объяснение, время и
происхождение: точное совпадение алгоритма `exact_text_v1` либо предложение модели.
Модель может вернуть `duplicate` для одинаковых обязательств с совпадающими
условиями, областью и ограничениями. Частичное пересечение или уточнение
не считается дублем автоматически. Ничто не объединяется и не удаляется.

Подготовка сохраняет снимки источников и формирует очередь ограниченными пакетами.
Пока подготовка не завершена, общее число пар неизвестно (`totalPairs=null`),
а не равно нулю. Интерфейс различает все возможные пары проекта, выбранные и
невыбранные, новые и переиспользованные результаты, ожидающие, выполняемые,
ошибочные и завершённые пары. Неотобранные пары не считаются проверенными.
Сворачиваемые группы результатов сохранены; строки загружаются страницами,
а счётчики относятся ко всему запуску, не к текущей странице.
Готовый результат сразу перемещается в свою группу через SSE в открытых вкладках;
reconnect/reset перечитывает сохранённые данные. «Противоречий не обнаружено»
относится к проверенной паре; отдельной классификации «требования не связаны» нет.

Миграция007 переносит очередь и снимки из растущих JSONB-массивов `analysis_runs`
в отдельные строки `analysis_pairs` и `analysis_sources`. Сохраняются ID, версии
и тексты источников, вердикт, объяснение, время проверки, профиль генератора и
ссылка на исходную проверку при переиспользовании. История запусков доступна
страницами через GraphQL; старый запуск можно прочитать по ID.
Кнопка «Отменить анализ всех пар» отменяет весь выбранный выполняющийся запуск:
текущую проверку, все ожидающие пары и незавершённую подготовку очереди.
Готовые результаты сохраняются. GraphQL-мутация
`cancelProjectAnalysis(projectId, analysisId)` идемпотентна; отмена хранится
как `status=failed`, `errorCode=CANCELLED`, а интерфейс показывает «Анализ отменён».
Незавершённые пары получают `failed/CANCELLED` и не считаются готовыми.
Поздний ответ модели не публикуется. Адаптер получает отмену контекста;
на другом экземпляре backend это происходит при следующем heartbeat (до 5 секунд).
Освобождение ресурсов модели зависит от адаптера; следующий вызов ждёт завершения
текущего. NPU runtime завершает отменённый worker и восстанавливает его с тем же
закреплённым профилем при следующем явном запуске анализа; отменённый запрос
автоматически не повторяется. Можно явно повторить незавершённые пары актуального запуска или начать
новый анализ с переиспользованием готовых результатов.

Повтор неудавшихся пар доступен в представлении «Последний запуск»:
выбор сохранённого запуска в истории открывает его для просмотра.
Пользовательские тексты не заменяются текущими версиями в историческом результате.
Старые результаты мигрируются без приписывания им неизвестного происхождения.

Успешные вердикты `collision`, `compatible`, `insufficient` и `duplicate` переиспользуются
между запусками только в том же workspace/project, для тех же канонически
упорядоченных снимков и полного неизменяемого профиля генератора. Профиль включает
адаптер, модель/digest либо NPU manifest, инструкцию, схему ответа и параметры
генерации. Совпадения одного имени модели недостаточно. Ошибки и timeout
не являются готовыми результатами. Повтор неизменённого проекта тем же профилем
не вызывает генератор. В полном режиме изменение одного из N требований требует
N−1 новых сравнений, добавление — N; точные дубли дополнительно исключают inference. Удалённые требования исключаются из новой очереди,
а история остаётся. Устаревший общий запуск не уничтожает пригодные результаты
его неизменённых пар. Изменение embeddings, порога или правил пересобирает
кандидатов, но само по себе не меняет пригодность сравнения тех же снимков
тем же генератором.
Все чтения истории/кеша и передача контекста модели ограничены авторизованными
workspace/project; ответ, полученный после изменения источников или потери lease,
не публикуется как актуальный успешный результат.

Один проект имеет одну активную попытку; повторный запрос возвращает её статус.
Worker последовательно обрабатывает пары, включая разные проекты; PostgreSQL
advisory lock исключает конкурирующие вызовы из других экземпляров.
На один пакет подготовки выделено 180 секунд, на отдельное сравнение —120 секунд;
общей короткой отсечки на весь анализ нет. Предел ответа адаптера —2MiB.

Результат пары, общий прогресс и инвалидация сохраняются после успешного commit.
При первой ошибке модельных вызовов пакет останавливается: текущая пара получает
ошибку, остальные непроверенные пары — `NOT_PROCESSED`, готовые результаты остаются.
Счётчик обработанных пар включает завершённые сравнения и начатые сравнения
с ошибкой; `NOT_PROCESSED` в него не входит. После восстановления модели кнопка
«Повторить неудавшиеся пары» повторяет только ошибочные и непроверенные пары
последней актуальной попытки, сохраняя ID и готовые результаты. Изменённые источники
или профиль генератора/отбора требуют нового анализа отдельной попыткой.
Бесконечных автоматических повторов и облачного fallback нет.

Очередь переживает перезапуск backend. Захваченная работа имеет lease30 секунд
с heartbeat каждые5 секунд; после аварии незавершённые вызовы получают статус ошибки
и требуют явного повтора. При штатной остановке это происходит сразу.
Ожидающая модель задача не теряет lease во время очереди. Поздний результат
предыдущего worker не заменяет результат повтора. Уже сохранённые успешные пары
не вычисляются повторно; inference, оборванный до commit, при явном повторе может
выполниться снова. Exactly-once inference не обещается. Решение и ограничения
описаны в [ADR-003](docs/adr/003-progressive-analysis.md).

Документы в промпте передаются как недоверенные JSON-данные отдельно от системных
инструкций; инструменты модели не предоставляются. Ответ ограничен JSON-схемой,
проверяются ID/версии именно переданных источников, допустимый вердикт и длина
объяснения. Это снижает риск ошибочного ответа, но не превращает вывод LLM в факт.

Обычный CI проверяет retrieval, workflow, доступ и известные коллизии/совместимые
требования с детерминированными провайдерами. Реальная локальная проверка генератора
запускается отдельно:

```bash
ANALYSIS_SMOKE=1 \
OLLAMA_URL="$PROJECT_CREATOR_OLLAMA_URL" \
OLLAMA_MODEL="$PROJECT_CREATOR_GENERATOR_MODEL" \
OLLAMA_DIGEST="$PROJECT_CREATOR_GENERATOR_DIGEST" \
go test ./internal/analysis -run TestRealAnalysisSmoke -v -count=1
```

Она требует заранее загруженной модели и проверяет небольшой набор явных примеров,
не статистическое качество или полноту. Для unit/integration/GraphQL/E2E используйте
команды выше; интеграционные тесты требуют отдельную тестовую PostgreSQL-базу.




### Страницы и обновление интерфейса

| URL | Назначение |
| --- | --- |
| `/` | Список проектов |
| `/projects/new` | Создание проекта |
| `/projects/:id` | Обзор проекта |
| `/projects/:id/requirements` | Создание, изменение и удаление требований |
| `/projects/:id/indexing` | Общий прогресс фоновой очереди, статусы и повтор индексации |
| `/projects/:id/search` | Семантический поиск |
| `/projects/:id/analysis` | Потоковый анализ пар и результаты |

Навигация проекта и выбранный язык RU/EN общие для всех страниц. Язык сохраняется
между открытиями приложения. Ссылки из поиска и анализа открывают страницу
требований, находят нужную страницу списка и выделяют исходную карточку.
Если исходник удалён, сохранённый текст версии остаётся в результате анализа.
В группах анализа объяснение пары видно сразу; тексты источников и технические
сведения пары загружаются отдельно при раскрытии соответствующих блоков.

При SSE-инвалидации и повторном запросе последнее успешное содержимое остаётся
на месте. Ошибка показывает локальное сообщение об устаревших данных и кнопку
повтора. Отказ доступа очищает данные; SSE reset после reconnect только обновляет
их и сохраняет черновик. Счётчики требований и индексации в режиме страниц относятся к текущей
странице из 20 записей. Состояние свёрнутых групп сохраняется в текущей вкладке.
Общий прогресс индексации относится ко всему проекту, отдельно от счётчиков
загруженных карточек. `queued` означает ожидание фоновой обработки, `running` —
выполняемую индексацию, `ready` — готовый актуальный индекс, `failed` — ошибку.
Старое `pending` поддерживается для совместимости. SSE reconnect/reset перечитывает
сохранённое состояние очереди. Причины устройства описаны в [ADR-005](docs/adr/005-index-queue.md).

Детерминированные браузерные регрессии без PostgreSQL и моделей:

```sh
cd frontend
npm run build
npx playwright install chromium
npm run test:ux
```

Сценарии проверяют desktop 1920×1080 и узкий экран 390×844, медленные запросы,
SSE/reconnect/reset, ошибки и повтор, отзыв доступа, язык и навигацию,
пагинацию источников и сворачиваемые группы со счётчиками. Контракт и SSE
используют те же GraphQL-операции и механизм инвалидации, что и раньше.

Длинные слова, URL и идентификаторы переносятся внутри страницы, включая
описание проекта и происхождение модели. Переносы строк в цели и описании
сохраняются; JSON происхождения не обрезается. Списки выбора ограничены шириной
контейнера, полное происхождение выбранного запуска доступно при раскрытии.
Группы и карточки используют существующие общие стили и компонент `Group`.

Регрессии `responsive content` и `responsive analysis` проверяют отсутствие
горизонтального переполнения страницы, текстовых контейнеров и элементов форм
на ширинах 320/390/768/960/1920 px в RU/EN. Проверяются длинные выжимки,
раскрытые оригиналы, поиск, объяснения и профили анализа, большие счётчики
и раскрытие с клавиатуры. Дополнительно проверяется CSS-масштабирование 200%;
960 px проверяет ширину reflow для десктопа 1920 px при browser zoom 200%.
Это автоматическая эмуляция, а не ручная проверка меню масштаба браузера.
Существующие UX-сценарии и сценарии анализа также проверяют переполнение в
своём конечном состоянии, включая ошибки, загрузку и смену параметров анализа.
Скриншоты обзора сохраняются в каталоге результатов Playwright; для локальных
диагностических прогонов задавайте `--output ../logs/issue-44/ux-results`.

### Просмотр длинных списков

Проекты, требования и индексации по умолчанию загружаются при прокрутке порциями по 40
записей. Карточки остаются на экране при фоновых SSE-обновлениях. Внизу есть
кнопка «Загрузить ещё» для клавиатуры; ошибка загрузки имеет локальную кнопку
«Повторить». Длинные списки виртуализированы с учётом высоты карточек.

Переключатель «Режим списка» включает обычные страницы по 20 записей. Режим
и страница сохраняются в адресе и поддерживают Back/Forward. Возврат из
карточки клиентской навигацией сохраняет загруженные порции и позицию
бесконечного списка; перезагрузка вкладки начинает новую загрузку.
Ссылка на источник требования открывает соответствующую обычную страницу.

На странице индексаций статусы загружаются по ID требований каждой порции,
а счётчики групп показывают количество среди загруженных требований.
Над списком отображается число загруженных требований и общее количество.
Переключатель режима также позволяет открыть страницы по 20 требований;
выбранная страница сохраняется в адресе и поддерживает Back/Forward браузера.

У каждой сворачиваемой группы анализа собственная загрузка и курсор; закрытые
группы не запрашивают пары. Счётчики описывают весь запуск, а не загруженные
карточки. Готовые пары перемещаются между прежними группами после SSE-обновления.
Контракт и ограничения курсоров описаны в [ADR-004](docs/adr/004-cursor-lists.md).

### Экономные live-обновления анализа (#47)

Прогресс приходит компактными SSE-снимками с ревизией запуска, без повторного
запроса всей сводки и карточки проекта на каждую пару. Группы по умолчанию закрыты;
раскрытие загружает компактную страницу, раскрытие карточки — только её сохранённые
тексты и объяснение. Неизменяемое происхождение модели запрашивается отдельно при
раскрытии блока и переиспользуется по ID запуска. Фоновая синхронизация сохраняет
показанные карточки, формы и раскрытые группы. Reset/reconnect восстанавливает
состояние; отмена сохраняет готовые результаты.

Контракт и ограничения: [ADR-006](docs/adr/006-analysis-live-snapshots.md).
Замеры до/после, методика и команды: [20 пар](docs/analysis-live-performance.md).
Сценарий: `scripts/analysis-live-benchmark.mjs`.
Обычные тесты не вызывают реальные модели.

### Замена параметров выполняющегося анализа

Изменение полей формы не останавливает анализ. Кнопка «Заменить параметры анализа»
явно применяет выбранный режим и порог с одним из двух вариантов:

- «Дождаться текущей пары»: сохранить пригодный результат текущего сравнения,
  остановить выдачу остальных старых пар и начать новую очередь. Готовые сравнения
  неизменных снимков тем же генератором переиспользуются независимо от порога.
- «Сразу отменить текущий вызов модели»: завершить старый запуск с `REPLACED`,
  отменить контекст inference и начать новую очередь после освобождения генератора.
  Поздний ответ не сохраняется. Для другого экземпляра backend отмена обнаруживается
  heartbeat в пределах 5 секунд; NPU проверяет отключение клиента каждые 100 мс,
  затем ждёт до 5 секунд terminate и до 5 секунд kill. Загрузка закреплённой модели
  при восстановлении имеет отдельный предел 600 секунд; пара ограничена 120 секундами.

GraphQL: `replaceProjectAnalysis(projectId, analysisId, mode, threshold, strategy)`;
`strategy` — `wait` или `cancel`. Запрос привязан к ожидаемому запуску и сохраняется
в PostgreSQL. Повтор идентичной операции возвращает тот же запуск либо его преемника.
До создания преемника новый выбор заменяет ожидающий запрос; после создания менять
нужно уже новый `analysisId`. Устаревший запрос с другими параметрами отклоняется.
Состояние ожидания — `running/waiting_current_pair`. SSE/reset и обновление страницы
читают сохранённое состояние; закрытие вкладки не отменяет замену. Кнопка отмены всего
анализа также удаляет ожидающую замену. Заменённый запуск нельзя возобновить через retry.

После намеренной отмены NPU worker восстанавливается перед следующим вызовом,
с повторной проверкой закреплённых весов и профиля. Неудача восстановления не вызывает
автоматических повторов. `NPU_RECOVERY_FAILED`, `NPU_WORKER_EXIT`, `NPU_UNAVAILABLE`,
`TIMEOUT`, `PROFILE_CHANGED`, `INVALID_OUTPUT` различаются от пользовательской отмены.
Timeout, сбой устройства или изменение файлов требуют операторского восстановления
по [инструкции NPU](docs/intel-npu.md); автоматического облачного fallback нет.
Технические события без исходных требований и ответов сохраняются в `logs/npu/`.

Детерминированная приёмка замены (отдельная тестовая БД обязательна):

```bash
go test ./internal/analysis -run 'TestReplacement|TestCancelWaiting' -count=1
go test ./internal/api -run TestGraphQLReplacement -count=1
python3 -m unittest discover -s runtime/npu -v
cd frontend
E2E_CHROMIUM_PATH=/path/to/chrome npm run test:ux -- --grep 'replacing an analysis'
```

Эти проверки используют заглушки и не доказывают аппаратную готовность NPU.
Аппаратный сценарий отмены и повторной генерации запускается отдельно и явно.

Аппаратная проверка выполняется на **свободном** runtime, запущенном из этой версии
кода. Она намеренно прерывает один inference и ожидает новое сравнение с тем же
закреплённым профилем:

```bash
NPU_REAL_CANCEL_SMOKE=1 \
PROJECT_CREATOR_NPU_SOCKET=/absolute/path/runtime.sock \
PROJECT_CREATOR_NPU_PROFILE=<manifest-sha256> \
go test ./internal/analysis -run '^TestOpenVINORealNPUCancelThenGenerate$' -v -count=1
```

Перед отменой тест ждёт отправку запроса worker (`generation_started`),
после — `worker_stopped` для своего
`call_id` в `logs/npu/runtime-lifecycle.jsonl`. Это отличает остановку native worker
от одного лишь закрытия HTTP-запроса. Само событие отправки не доказывает
выполнение вычислений на устройстве. Новый результат проверяется по схеме и
источникам; качество вердиктов оценивается отдельными аппаратными примерами.
`backend-lifecycle.jsonl` связывает `callId` с ID запуска и пары; заголовок
`X-NPU-Call-ID` связывает его с журналом runtime. Ни один из журналов не содержит
текстов требований, промптов или ответов.

### Автоматические выжимки требований (#72 в составе #48)

Языки выжимок задаются в `.env`: `PROJECT_CREATOR_SUMMARY_LANGUAGES=ru,en`
(также значение по умолчанию при отсутствии настройки). Поддерживаемые коды:
`ru`, `en`, `de`, `fr`, `es`, `uk`, `zh`, `ja`. Например, `ru,en,de` добавляет
немецкий. Пустые элементы и неизвестные коды считаются ошибкой конфигурации.
После изменения примените конфигурацию через `./project-creator up` с пересборкой
backend, если изменялся код. Список учитывается в профиле: старые выжимки
сохраняются в истории, а для нового профиля автоматически создаются новые.

Модель сохраняет язык оригинала, если он разрешён; иначе переводит выжимку
в первый язык канонического списка (коды сортируются и повторы удаляются).
Сервер проверяет алфавит, допускает цифры, пробелы, пунктуацию, символы и emoji.
Остальные буквы не вырезаются: модель получает одну попытку исправления;
при повторной ошибке выжимка не публикуется, а итоговая LLM может проверить
оригиналы. Составные буквы с отдельными комбинируемыми диакритиками отклоняются;
модель должна вернуть обычную цельную букву разрешённого алфавита. Проверка
алфавита не отличает языки с одинаковыми буквами, поэтому языковое ограничение
дополнительно задано в инструкции модели. Оригинальные требования новым правилом
не ограничиваются.


При явной настройке `PROJECT_CREATOR_SUMMARY_MODEL` и полного
`PROJECT_CREATOR_SUMMARY_DIGEST` требования после сохранения автоматически
поступают в локальную фоновую очередь. `PROJECT_CREATOR_SUMMARY_URL` позволяет
выбрать отдельный Ollama runtime. CRUD и поиск не ждут генерации; выжимки
привязаны к версии источника и профилю. 350 UTF-8 байт — мягкая цель для
генерации и условие допуска в Laya, но длинные корректные выжимки сохраняются.
В карточке выжимка служит превью, полный оригинал доступен по раскрытию.
Laya сравнивает выжимки, итоговая LLM — полные оригиналы. Если выжимка отсутствует,
ошибочна или длиннее 350 байт, либо Laya возвращает ошибку, пара пропускает
предварительную классификацию и всё равно поступает финальной LLM с полными
оригиналами. Причина пропуска сохраняется; выжимка остаётся предложением модели.

Настройка, отмена/retry и ограничения: [конвейер Laya](docs/laya-pipeline.md),
[ADR-009](docs/adr/009-requirement-summaries.md) и
[ADR-010](docs/adr/010-optional-laya.md).
Аппаратные результаты и ограничения реальной модели:
[отчёт проверок](docs/requirement-summary-verification.md).
