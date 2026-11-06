# Запуск на Intel Arc / Core Ultra

Вариант для ноутбука Intel Core Ultra 7 268V со встроенной Intel Arc:
нативный Linux Docker Engine + Compose v2, Ollama 0.40.11 через Vulkan.
Docker Desktop VM/WSL2 здесь не проверены; проброс `/dev/dri` предназначен
для нативного Linux. NPU процессора этот вариант не использует.

## Подготовка Ubuntu

Используйте поддерживаемую Ubuntu с актуальными firmware, ядром и Intel
Vulkan-драйвером. Для Lunar Lake на Ubuntu 24.04 Intel указывает HWE-ядро;
порядок установки для вашей версии ОС смотрите в
[официальной инструкции Intel](https://dgpu-docs.intel.com/installation-guides/installing-packages-from-the-intel-ppa.html).
После обновления ядра перезагрузите ноутбук. Для диагностики на Ubuntu:

```bash
sudo apt update
sudo apt install mesa-vulkan-drivers vulkan-tools
uname -r
ls -l /dev/dri
vulkaninfo --summary
```

В `/dev/dri` должен быть render-узел, а в Vulkan — Intel Arc.
`llvmpipe`/`lavapipe` — программный рендерер, он не подтверждает доступ к Arc.
Если Intel не обнаруживается, сначала исправьте драйвер/ядро на хосте.
Не требуется NVIDIA Container Toolkit, `/dev/kfd` или `privileged`.

В контейнере используется собственный `Dockerfile.ollama-intel`: закреплённый
образ Ollama + явная установка/обновление `mesa-vulkan-drivers` из его
Ubuntu-репозиториев. Vulkan loader/backend и Intel ICD — разные части;
Mesa уже может присутствовать в базовом образе. Пакеты Mesa
получают обновления Ubuntu при пересборке. С конфигурацией ниже сначала выполните
`docker compose build --pull --no-cache ollama`, затем примените новый образ:
`docker compose up -d --no-deps --force-recreate --wait ollama`.

## Первый запуск

Все команды выполняйте из корня репозитория в одном терминале. Сначала
скопируйте `.env.example` в `.env` и замените пароль/токен по корневому README.
`COMPOSE_FILE` обеспечивает применение Intel override ко всем следующим командам:

```bash
export COMPOSE_FILE=compose.yml:compose.intel.yml
docker compose config --quiet
docker compose up -d --build --wait postgres ollama
docker compose exec ollama ollama pull bge-m3:567m
docker compose exec ollama ollama pull qwen2.5:7b
docker compose --profile tools run --rm model-info
```

Перенесите полные digests из `model-info` в `.env` вместе с именами моделей:

```dotenv
PROJECT_CREATOR_EMBEDDING_MODEL=bge-m3:567m
PROJECT_CREATOR_EMBEDDING_DIGEST=sha256:полный-digest-embeddings
PROJECT_CREATOR_GENERATOR_MODEL=qwen2.5:7b
PROJECT_CREATOR_GENERATOR_DIGEST=sha256:полный-digest-генератора
```

```bash
docker compose up -d --build --wait
docker compose ps
```

Откройте http://localhost:3000 (или настроенный `APP_PORT`). В новом терминале
снова задайте `COMPOSE_FILE`; эквивалент каждой команды —
`docker compose -f compose.yml -f compose.intel.yml ...`.
Не объединяйте этот override с `compose.gpu.yml`, предназначенным для NVIDIA.
Модели, API, healthchecks и volumes общие с обычным стеком; Ollama не публикует
порт на хост. Облачный fallback не включается.

## Проверка GPU на ноутбуке

Создайте проект и требования, запустите индексацию и анализ через интерфейс
(сценарий проверки описан в README). Пока запрос выполняется, в другом терминале:

```bash
export COMPOSE_FILE=compose.yml:compose.intel.yml
docker compose exec ollama ollama ps
docker compose logs --tail=200 ollama
docker stats --no-stream
```

В логах ищите Vulkan с именем Intel Arc и сведения о выгруженных на GPU слоях;
в `ollama ps` — колонку `PROCESSOR` с GPU или CPU/GPU. Проверяйте оба типа
запросов: embeddings и генерацию. Готовность контейнера сама по себе не
подтверждает ускорение. Если модель уже выгружена, повторите запрос и сразу
проверьте `ps`. Программный Vulkan renderer не считается аппаратным offload.

Intel override включает `OLLAMA_IGPU_ENABLE=1` для встроенной Arc и ограничивает
Vulkan Intel ICD. Если `GGML_VK_VISIBLE_DEVICES` не задан или пуст, выбирается
Vulkan ID `0` — первый обнаруженный Intel Vulkan device. Для нескольких Intel
GPU найдите ID нужного адаптера в логах Ollama, задайте его в `.env` и выполните
`docker compose up -d --force-recreate ollama`. Номер Vulkan не обязательно
совпадает с номером `renderD*`. Успешный `docker compose config`, доступность
API и выбор ID подтверждают только конфигурацию; offload проверяется отдельно
по запросу в логах и `ollama ps`.

У встроенной Arc общая память с системой. Размер весов не равен расходу памяти:
у генератора сейчас контекст 32768 токенов, память также нужна ОС, БД и сборке.
Конфигурация оставляет одну загруженную модель и один параллельный запрос.
На 268V производительность и доступный GPU-бюджет нужно измерить фактически;
полная загрузка на GPU и конкретная скорость не гарантируются. При нехватке
памяти возможны частичный offload, CPU или ошибка. Начните с одного проекта
и коротких требований, следите за RAM/swap и логами.

## Диагностика и возврат к CPU

- Нет `/dev/dri`: проверьте ядро, firmware и драйвер Intel на хосте.
- Permission denied: проверьте права render-узла и настройки rootless Docker;
  не исправляйте это глобальным `chmod 777` или `privileged`.
- Intel виден хосту, но не контейнеру: пересоберите Intel-образ с обновлением
  пакетов, проверьте `docker compose config` и логи Ollama.
- Только CPU: убедитесь, что выбран Intel Vulkan ID, а не software renderer;
  проверьте память и сообщения offload. Healthcheck проверяет только API.

Для возврата к обычному стеку без удаления данных:

```bash
unset COMPOSE_FILE
docker compose -f compose.yml up -d --force-recreate ollama
docker compose -f compose.yml up -d --wait
```

Сохраняйте каталог/имя Compose-проекта и `.env`; не используйте `down --volumes`.
CI проверяет слияние конфигурации и собирает Intel-образ с проверкой ICD.
Проверка настоящего ускорения требует Intel Arc и выполняется отдельно выше.

Основания: [Ollama Docker / Vulkan](https://docs.ollama.com/docker),
[поддержка GPU Ollama](https://docs.ollama.com/gpu),
[драйверы Intel](https://dgpu-docs.intel.com/driver/client/overview.html).
