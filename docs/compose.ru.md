# Docker Compose и эксплуатация

[Project Creator](../README.md) · [English version](compose.md)

## Запуск всего MVP в Docker Compose

Требуются Docker Engine с Compose v2 (команды `docker compose`) либо Docker
Desktop с Linux-контейнерами. Go и Node.js на хосте для запуска приложения не нужны.
Запускайте команды из корня репозитория. По умолчанию запускаются PostgreSQL, бэкенд и фронтенд. Для встроенного Ollama добавьте Compose-файл выбранного устройства.

В конфигурации закреплены Go 1.26.10, Node.js 26.13.1 на Debian Bookworm,
PostgreSQL 16 с pgvector 0.8.7 и Ollama 0.40.16. Модели загружаются отдельно:

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

Для работы с проектами и требованиями выполните `docker compose up -d --build --wait`, оставив переменные моделей из примера незаданными.

Для запуска встроенного Ollama на CPU добавьте `COMPOSE_FILE=compose.yml:compose.cpu.yml` в `.env` перед выполнением следующих команд. При обновлении ручной CPU-установки также добавьте эту строку. Для возврата с ручной GPU-установки экспортируйте такое же значение `COMPOSE_FILE` в shell и выполните `docker compose up -d --force-recreate ollama`, затем `docker compose up -d --wait` с прежним именем проекта. CLI восстанавливает выбранное устройство автоматически. Override-файлы GPU, NPU и smoke сами включают свой runtime.

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
к CPU описаны в [инструкции Intel Arc](intel-arc.md).

### Экспериментальный Intel NPU

Для проверки Core Ultra 7 268V подготовлен явно включаемый host runtime
OpenVINO GenAI и `compose.npu.yml`. Генерация выполняется только на NPU,
BGE-M3 остаётся в локальном Ollama. Вариант требует подготовки драйвера и
IR-весов. После исправления CLI и нормализации ссылок 30.10.2026 прошли
фиксированные аппаратные примеры, включая недостаток данных и перестановку
источников. Полная приёмка с сопоставлением CPU/GPU остаётся отдельной задачей.
Проверены CLI `setup --device npu`, остановка, повторный `up` и `doctor`
с сохранением БД и готовых весов.
Команды, результаты и возврат к Ollama — в [инструкции NPU](intel-npu.md).

Каталог `.npu/` создаётся отдельно командами подготовки NPU, исключён из Git
и Docker-сборки и не появляется автоматически при обычном `docker compose up`.
При CPU/Arc-варианте Ollama он не нужен; для NPU другой пользователь готовит
свои локальные окружения, веса и профиль по инструкции. Автоматическая установка описана в [руководстве CLI](setup-cli.md).

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

CLI поддерживает нативный Linux x86_64, Python 3 и Docker Engine/Compose 2.20+;
профиль NPU дополнительно использует Python 3.12 и установленный uv.
Наличие профиля означает поддержку конфигурации; совместимость конкретного
оборудования подтверждается отдельным реальным запуском `setup`/`up`.

| Режим CLI | Устройство и runtime | Подготовка / статус |
| --- | --- | --- |
| `cpu` | Ollama 0.40.16 на CPU | Базовый вариант, GPU не нужен. |
| `arc` | Intel Arc dGPU/iGPU, в том числе Core Ultra 7 268V; Ollama 0.40.16/Vulkan | Intel Linux graphics, доступ к `/dev/dri`; [инструкция Arc](intel-arc.md). Intel ICD изолирован от AMD/NVIDIA. Новая логика выбора проверена тестами без GPU. |
| `nvidia` | Поддерживаемая NVIDIA GPU; Ollama 0.40.16/CUDA | NVIDIA driver и Container Toolkit, `nvidia-smi`; аппаратная проверка новой CLI-интеграции ещё не выполнена. |
| `amd` (ROCm) | Только AMD dGPU/iGPU из матрицы Ollama 0.40.16; `ollama/ollama:0.40.16-rocm` | ROCm 7, `rocminfo`, `/dev/kfd` и `/dev/dri`. Строгая проверка GFX target; аппаратная приёмка не проведена. |
| `amd-vulkan` (RADV) | AMD dGPU/iGPU с работоспособным Vulkan; Ollama 0.40.16 + Mesa RADV | `amdgpu`, доступный `/dev/dri/renderD*` и поддержка Vulkan в драйвере. ROCm и `/dev/kfd` не требуются. Более широкий, но **не универсальный** вариант; аппаратная приёмка не проведена. |
| `npu` | Intel NPU; OpenVINO GenAI, embeddings на CPU | Фиксированные аппаратные примеры на Core Ultra 7 268V проходят; результаты и ограничения — в [инструкции NPU](intel-npu.md). |

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
[инструкции CLI](setup-cli.md#gpu-и-выбор-адаптера).

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
