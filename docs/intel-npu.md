# Эксперимент: Intel NPU на Core Ultra 7 268V

Ветка `feat/22-intel-npu`, отдельный PR к #22; **не сливать до завершения аппаратной приёмки**.
Вариант рассчитан на нативный Linux, Python 3.12 и Docker Engine/Compose v2.
Первичная аппаратная проверка 29.10.2026 подтвердила генерацию Qwen на NPU,
но один из трёх примеров не прошёл проверку качества. После исправления CLI
30.10.2026 фиксированные аппаратные примеры проходят; сравнение с CPU/GPU
для полной приёмки остаётся открытым.
Результаты приведены ниже. Решение и зависимости — [ADR-002](adr/002-openvino-npu.md).

Генерация: OpenVINO GenAI, строго `NPU`. Embeddings: прежний BGE-M3/1024
в локальном Ollama на CPU либо Intel Arc (`compose.intel.yml`). NPU-совместимость
BGE-M3 не подтверждена; модель и размерность не заменяем. Токенизация работает
на CPU. NPU runtime — процесс пользователя на хосте; backend подключается
по Unix socket, без открытия сетевого порта или выдачи контейнеру NPU-устройства.

Локальный каталог `.npu/` подготавливается командами этой инструкции, а не
обычным `docker compose up`. Он исключён из Git и Docker build context;
другой пользователь создаёт свои артефакты, а не копирует окружения с чужого ноутбука.
Исходный код runtime и закреплённые requirements находятся в `runtime/npu/`
и входят в репозиторий. При запуске только через Ollama на CPU/Arc `.npu/` не нужен.

| Содержимое `.npu/` | Назначение |
| --- | --- |
| `python/`, `export-env/`, `runtime-env/` | Локальный Python при необходимости и отдельные окружения подготовки/запуска |
| `huggingface/`, `source-path.txt`, `source-revision.txt` | Кеш исходных весов и закреплённый snapshot |
| `models/qwen2.5-7b-int4/` | Экспортированные веса, токенизаторы, лицензия и freeze exporter |
| `profile.json`, `profile.sha256` | Манифест целостности/режима генерации и его профиль |
| `run/`, `compiled-cache/` | Сокет работающего runtime и кеш компиляции |
| `export-tmp/`, `*-freeze.txt` | Временные файлы экспорта и зафиксированные зависимости окружений |

Логи runtime, отчёты установки и результаты диагностических прогонов хранятся
в корневом `logs/npu/`, исключённом из Git и Docker build context. Веса,
профили, source revision и `pip freeze` остаются в `.npu/` как сведения
о происхождении модели и окружений.

Дополнительные скрипты диагностики и логи ниже — артефакты конкретной проверки,
они не поставляются с репозиторием и не требуются для выполнения команд инструкции.
Подготовку и запуск автоматизирует `./project-creator setup --device npu`;
системные prerequisites и сохранение текущих артефактов описаны в
[инструкции CLI](setup-cli.md). Ручной сценарий ниже остаётся доступен для диагностики.

## 1. Ветка, драйвер и диагностика

```bash
git fetch origin
git switch --track origin/feat/22-intel-npu
uname -a
ls -l /dev/accel/accel*
lspci -nnk | sed -n '/Processing accelerators/,+4p'
lsmod | rg 'intel_vpu'
dpkg-query -W 'intel-driver-compiler-npu' 'intel-fw-npu' 'intel-level-zero-npu' 'libze1'
```

Для Lunar Lake нужен работающий kernel driver, firmware, userspace NPU driver
и права на `/dev/accel/accel0`. Наличие устройства ещё не доказывает inference.
Установку/обновление выполняйте по [Intel Linux NPU driver](https://github.com/intel/linux-npu-driver),
выбрав пакеты для своей Ubuntu. В проверке 29.10.2026 использованы v1.38.0
и Level Zero libze1 1.32.0 с OpenVINO 2026.4.1; release драйвера указывал
OpenVINO 2026.3.1. Наши результаты относятся к указанному в разделе 5 ноутбуку
и окружению. Не переносите номера Windows-драйверов в Ubuntu. При отсутствии
прав проверьте группу устройства и членство пользователя; после добавления
в группу нужен новый login. Не используйте `chmod 777` и privileged.

## 2. Явная подготовка окружений и весов

Выполняйте из корня репозитория. Потребуется дополнительное место для исходных
весов Safetensors, экспортированного INT4 и кешей: ориентир 40–50 GB свободного диска.
На 32 GB RAM экспорт и компиляция могут быть тяжёлыми; закройте лишние процессы,
контролируйте RAM/swap. Здесь ничего не скачивается при обычном запуске приложения.
Следующие команды — отдельное явное скачивание зависимостей и модели.

На Ubuntu 26.04 системным Python может быть 3.14. Закреплённый CPU wheel
PyTorch рассчитан на CPython 3.12: не заменяйте системный Python и не ставьте
экспортные пакеты глобально. Если `python3.12` отсутствует, установите его
локально с помощью уже установленного [uv](https://docs.astral.sh/uv/getting-started/installation/):

```bash
uv python install 3.12.14 --install-dir .npu/python --no-bin --cache-dir .npu/uv-cache
export NPU_PYTHON="$PWD/.npu/python/cpython-3.12.14-linux-x86_64-gnu/bin/python3.12"
"$NPU_PYTHON" --version
```

Этот путь проверен с uv 0.12.13 на Linux x86_64; Python и кеш остаются в
игнорируемом Git каталоге `.npu`. Если Python 3.12 уже установлен, задайте
`NPU_PYTHON=python3.12`. Команды ниже используют выбранный интерпретатор.

```bash
mkdir -p .npu logs/npu
"${NPU_PYTHON:-python3.12}" -m venv .npu/export-env
.npu/export-env/bin/python -m pip install --report logs/npu/export-env-install.json -r runtime/npu/requirements-export.txt
.npu/export-env/bin/opt_in_out --opt_out
.npu/export-env/bin/python -m pip freeze > .npu/export-freeze.txt

"${NPU_PYTHON:-python3.12}" -m venv .npu/runtime-env
.npu/runtime-env/bin/python -m pip install --report logs/npu/runtime-env-install.json -r runtime/npu/requirements.txt
.npu/runtime-env/bin/opt_in_out --opt_out
.npu/runtime-env/bin/python -m pip freeze > .npu/runtime-freeze.txt
.npu/runtime-env/bin/python -c 'import openvino as ov; c=ov.Core(); print(c.available_devices); print(c.get_property("NPU", "FULL_DEVICE_NAME"))'
```

Последняя команда должна показать NPU. Ошибку исправьте до загрузки pipeline.
Python 3.12 нужен прежде всего для закреплённого exporter wheel PyTorch.
Для отдельного runtime с теми же pins разрешение зависимостей под Python 3.14
прошло; это ещё не проверка импорта и NPU inference. Если проверяете runtime
на 3.14, создавайте отдельный venv и выполняйте ту же проверку `ov.Core()`,
не меняя окружение экспорта и системные пакеты.

Для источника используется `Qwen/Qwen2.5-7B-Instruct` (Apache-2.0),
не GGUF из Ollama. Команда ниже разрешает текущий source commit один раз,
сохраняет его и явно скачивает этот immutable snapshot. Для воспроизведения
задайте ранее сохранённый `QWEN_REVISION` вместо получения нового commit.
Никакого `trust_remote_code` не требуется.

```bash
export HF_HOME="$PWD/.npu/huggingface"
export HF_HUB_DISABLE_TELEMETRY=1
export HF_HUB_DISABLE_IMPLICIT_TOKEN=1
.npu/export-env/bin/python - <<'PY'
import os
from pathlib import Path
from huggingface_hub import HfApi, snapshot_download
model = 'Qwen/Qwen2.5-7B-Instruct'
revision_file = Path('.npu/source-revision.txt')
revision = os.environ.get('QWEN_REVISION') or (revision_file.read_text().strip()
    if revision_file.exists() else HfApi().model_info(model).sha)
Path('.npu/source-revision.txt').write_text(revision+'\n')
path = snapshot_download(model, revision=revision,
    allow_patterns=['*.json', '*.safetensors', '*.txt', '*.model', '*.tiktoken', '*.jinja', 'LICENSE*', 'README*'])
Path('.npu/source-path.txt').write_text(path+'\n')
print('Pinned source:', revision, path)
PY
```

Исходные Safetensors Qwen2.5 7B занимают около **15.23 GB (14.19 GiB)**,
четыре shard-файла; GGUF из Ollama не заменяет их. Загрузку можно выполнять
в отдельном терминале без запущенного Codex. Дождитесь успешного завершения
перед экспортом; после прерывания повторите загрузку с тем же revision и
`HF_HOME`. SDK использует уже завершённые файлы кеша; не очищайте `.npu`.
Скрипт загрузки не должен сразу включать NPU-провайдер в работающем приложении:
сначала нужны экспорт, проверка устройства и успешный аппаратный smoke.

```bash
export HF_HUB_OFFLINE=1 TRANSFORMERS_OFFLINE=1
mkdir -p .npu/export-tmp
export TMPDIR="$PWD/.npu/export-tmp"
.npu/export-env/bin/optimum-cli export openvino \
  --model "$(cat .npu/source-path.txt)" \
  --task text-generation-with-past \
  --weight-format int4 --sym --ratio 1.0 --group-size -1 \
  .npu/models/qwen2.5-7b-int4
cp .npu/export-freeze.txt .npu/models/qwen2.5-7b-int4/export-freeze.txt
cp "$(cat .npu/source-path.txt)/LICENSE" .npu/models/qwen2.5-7b-int4/SOURCE-LICENSE
```

Для локального snapshot задача задаётся явно: автоматическое определение
`optimum-intel==2.1.0` завершается ошибкой. Временный каталог должен находиться
на диске: exporter сначала сохраняет промежуточные веса около 15.23 GB,
затем сжимает их. На проверяемом ноутбуке `/tmp` — tmpfs на 16 GiB;
экспорт туда завершался `basic_ios::clear: iostream error`. После переноса
`TMPDIR` на диск тот же экспорт прошёл за 47.11 секунды, пик RSS — 17.77 GiB;
итоговый каталог INT4 с токенизаторами занимает около 4.1 GiB.

Официальный [NPU export guide](https://docs.openvino.ai/2026/openvino-workflow-generative/inference-with-genai/inference-with-genai-on-npu.html)
требует симметричную 4-битную компрессию с ratio 1.0. Результаты проверки этой
экспортированной модели на NPU приведены в разделе 5. Сохраните отчёты установки,
freeze, лицензию и source revision. Для экспорта закреплён официальный CPU wheel PyTorch под Python 3.12/Linux x86_64;
CUDA-пакеты не требуются. Прямые версии закреплены, полное разрешённое
дерево экспортных зависимостей фиксирует freeze на вашем ноутбуке.

## 3. Манифест и локальный runtime

Начальный бюджет — **2048 входных + 512 выходных токенов**, не контекст Ollama
32768. Входной бюджет включает system prompt, схему JSON, chat template и оба
источника. Короткие требования предпочтительны; длинный запрос вернёт ошибку
без усечения. Изменение бюджета требует нового манифеста и профиля.

Манифест v2 также закрепляет `output_format=json-schema-v1`: runtime передаёт
JSON Schema в нативную генерацию и затем строго проверяет ответ и источники.
Если уже подготовлен манифест v1, повторите команду `manifest` ниже на имеющихся
весах, перезапустите host runtime и пересоздайте backend с новым профилем.
Скачивать и экспортировать веса повторно не требуется. Старые результаты анализа
сохраняются со старым профилем и становятся устаревшими.

```bash
.npu/runtime-env/bin/python runtime/npu/npu_runtime.py manifest \
  --model .npu/models/qwen2.5-7b-int4 \
  --model-id Qwen/Qwen2.5-7B-Instruct \
  --revision "$(cat .npu/source-revision.txt)" \
  --license Apache-2.0 \
  --export-command 'optimum-intel==2.1.0; TMPDIR=.npu/export-tmp optimum-cli export openvino --task text-generation-with-past --weight-format int4 --sym --ratio 1.0 --group-size -1 (see export-freeze.txt)' \
  --max-prompt-tokens 2048 --max-new-tokens 512 \
  --output .npu/profile.json > .npu/profile.sha256

mkdir -p .npu/run logs/npu
chmod 750 .npu/run
set -o pipefail
.npu/runtime-env/bin/python runtime/npu/npu_runtime.py serve \
  --model .npu/models/qwen2.5-7b-int4 --manifest .npu/profile.json \
  --profile "$(cat .npu/profile.sha256)" \
  --socket "$PWD/.npu/run/runtime.sock" --cache .npu/compiled-cache \
  2>&1 | tee -a logs/npu/runtime.log
```

Первый запуск компилирует модель; бюджет старта 10 минут. Процесс остаётся
в этом терминале. Он выводит имя NPU, время загрузки и профиль. Затем каждый
успешный запрос пишет prompt tokens и задержку, без текста требований.
Команда сохраняет stdout и stderr в `logs/npu/runtime.log`, продолжая показывать
их в терминале; `pipefail` сохраняет ошибку runtime в коде завершения pipeline.
Абсолютный путь Unix-сокета ограничен 100 байтами UTF-8. Если путь репозитория
слишком длинный, используйте более короткий каталог сокета и согласуйте его
в `--socket`, командах curl и `NPU_SOCKET_DIR` при подключении приложения.
Кеш компиляции хранится отдельно от весов. Не используйте чужие кеши.
Веса и токенизатор проверяются SHA256 при старте; изменения файлов во время
работы проверяются по inode/size/mtime/ctime и составу каталога до и после
запроса. Не изменяйте экспорт во время работы; это проверка целостности локальных
артефактов, а не защита от владельца хоста, способного подменить сам runtime.

В другом терминале можно проверить API, но это ещё не тест генерации:

```bash
curl --unix-socket "$PWD/.npu/run/runtime.sock" http://localhost/health
```

Сокет имеет права 660, каталог — 750 и группу пользователя, создавшего каталог.
При group ownership, отличающемся от primary group, укажите фактический GID
каталога/сокета в следующем шаге. При остановке Ctrl+C сокет удаляется.
После аварии сначала убедитесь, что старый процесс остановлен, и лишь затем
удалите оставшийся `.npu/run/runtime.sock`; сервис не перезаписывает существующий.

## 4. Подключение приложения

Используйте прежние `.env`, имя Compose-проекта и volumes по корневому README.
Embeddings BGE-M3 должны быть явно загружены и закреплены digest по инструкции
первого запуска в README (CPU) либо Intel Arc. Для NPU-генерации скачивание
Qwen через Ollama не требуется: используются экспортированные выше IR-веса.
Для NPU не меняйте `PROJECT_CREATOR_GENERATOR_DIGEST`: это digest
Ollama, он игнорируется выбранным NPU-адаптером.

```bash
export PROJECT_CREATOR_NPU_PROFILE="$(cat .npu/profile.sha256)"
export NPU_SOCKET_DIR="$PWD/.npu/run"
export NPU_SOCKET_GID="$(stat -c %g .npu/run)"
# Intel Arc здесь используется только для BGE-M3:
export COMPOSE_FILE=compose.yml:compose.intel.yml:compose.npu.yml
docker compose config --quiet
docker compose up -d --build --wait
```

Для CPU embeddings уберите `compose.intel.yml` из COMPOSE_FILE. Backend остаётся
пользователем 65532, получает дополнительную группу сокета и read-only bind
существующего каталога. UID mapping/rootless Docker может потребовать отдельной
настройки прав; не обходите её запуском privileged. Для backend на хосте задайте
`PROJECT_CREATOR_GENERATOR_RUNTIME=openvino-npu`, `PROJECT_CREATOR_NPU_SOCKET`
с абсолютным путём и `PROJECT_CREATOR_NPU_PROFILE` вместо Compose override.

Откройте http://localhost:3000. Индексация использует текущий embedding-профиль;
новый generator profile делает старые результаты анализа устаревшими, данные
и векторы сохраняются. Невалидный JSON от NPU не исправляется второй моделью.
Ошибка/занятость требует явного повтора в интерфейсе. Один запрос выполняется
за раз. Лимит генерации 110 секунд. Отмена завершает worker процесс, освобождая
текущий native inference. При следующем явном запуске или повторе анализа
проверка health поднимает новый worker с теми же закреплёнными моделью и профилем;
отменённый запрос автоматически не повторяется. Повторная компиляция может занять
время. Технические события запуска, готовности, отмены, teardown и ошибки
записываются относительно корня репозитория в `logs/npu/runtime-lifecycle.jsonl`;
журнал содержит профиль, код этапа и допустимый `X-NPU-Call-ID`, но не тексты
исходных пар или ответы модели. HTTP-ошибки NPU возвращают стабильный код из
allowlist, а неизвестные ошибки остаются общими и не содержат исходный текст.
Timeout, неожиданное
завершение worker или изменение файлов модели по-прежнему требуют явного
перезапуска runtime оператором. Анализ выполняется
асинхронно с сохранением результата каждой пары. На подготовку выделено180 секунд,
на отдельное сравнение —120 секунд; общего180-секундного лимита пакета нет.
При ошибке NPU текущая пара и непроверенный остаток помечаются ошибкой,
готовые результаты сохраняются. После явного восстановления runtime повторите
неудавшиеся пары; готовые пары повторно не вызывают модель.

## 5. Проверка на ноутбуке и запись результатов

Отдельный прямой smoke вызывает **тот же Go Generator**, минуя retrieval,
на трёх фиксированных парах и дополнительной регрессионной паре с ID приложения;
проверяются ожидаемые ID/версии/вердикты:

```bash
mkdir -p logs/npu
set -o pipefail
NPU_REAL_SMOKE=1 \
PROJECT_CREATOR_NPU_SOCKET="$PWD/.npu/run/runtime.sock" \
PROJECT_CREATOR_NPU_PROFILE="$(cat .npu/profile.sha256)" \
go test ./internal/analysis -run '^TestOpenVINORealNPU' -v -count=1 -timeout=8m 2>&1 | tee logs/npu/smoke-results.txt
```

Этот opt-in smoke требует Go версии из go.mod на хосте; дополнительно проверьте полный
сценарий через интерфейс: проект → два коротких требования → индексация → анализ.
Для взаимоисключающих способов оплаты ожидается предложение о коллизии со
ссылками на оба требования и их версии. После правки источника старый результат
должен стать устаревшим. Совместимая пара и недостаток условий проверяются
отдельными проектами. Отсутствие коллизии не доказывает согласованность проекта.

Запишите для первого и повторного старта: OS/kernel, версии driver/firmware,
`pip freeze`, профиль, `device_name`, `load_seconds`, полное время анализа,
prompt tokens, RAM/swap (например `/usr/bin/time -v` и `free -h`). Подтвердите
NPU-выполнение успешным запросом pipeline, явно созданной с `NPU`, и доступными
средствами драйвера/мониторинга, а не только `accel0`/health. В runtime запрещены
AUTO/HETERO/CPU/GPU fallback. Сравните CPU/GPU на тех же текстах, отдельно указав
разные веса/квантование и контекст; одинаковые названия Qwen не означают
идентичные артефакты. До этих запусков аппаратная приёмка #22 не завершена.

Диагностика без раскрытия исходных данных: `RUNTIME_VERSION_OR_IMPORT`,
`NPU_DEVICE_UNAVAILABLE`, `NPU_COMPILE_OR_MODEL`, `PROMPT_LIMIT_OR_TEMPLATE`,
`NPU_GENERATION`, `OUTPUT_LIMIT_OR_FINISH`, `INVALID_JSON_OR_SOURCES` в stderr.
При лимите увеличьте явный бюджет с новым профилем или сократите тестовые данные;
усечение, починка JSON и смена устройства автоматически не выполняются.

### Результаты 29.10.2026 до включения JSON Schema

Ubuntu 26.04, kernel `7.0.0-42-generic`, Intel AI Boost / Core Ultra 7 268V,
32 GB RAM, swap 8 GiB. Driver/compiler/firmware `1.38.0.20260910-34487311128~ubuntu26.04`,
`libze1=1.32.0-1~26.04~ppa1`. Runtime: CPython 3.12.14 и закреплённые зависимости
из `runtime/npu/requirements.txt`. Источник Qwen:
`a09a35458c702b33eeacc393d103063234e8bc28`; INT4 symmetric channel-wise,
бюджет 2048 + 512. SHA256 манифеста:
`faadc4dfd75451445f0475a2be366fea19f19640a6a92933e28399b766261557`.

Проверен существующий `TestOpenVINORealNPUSmoke` через Go-адаптер; тестовый
бинарник собран Go 1.27.3. Загрузка pipeline: 42.25 s без compiled cache,
4.59 s с кешем (583 MiB). Пик RSS по `/usr/bin/time -v`: 5.81 / 6.54 GiB;
это не полное потребление системы. Swap оставался занят после экспорта,
параллельная нагрузка не изолировалась; сравнение скоростей устройств не проводилось.

| Пример | Ожидание | Оба запуска | Первый запрос, s | После перезапуска, s | Prompt tokens |
| --- | --- | --- | --- | --- | --- |
| Взаимоисключающие способы оплаты | `collision` | `collision`, PASS | 5.87 | 8.59 | 477 |
| Альтернативные способы оплаты | `compatible` | `compatible`, PASS | 5.47 | 7.95 | 453 |
| Неопределённое время обработки | `insufficient` | `compatible`, FAIL | 7.92 | 9.92 | 447 |

Все шесть ответов содержали валидный JSON и точные ID/версии источников;
тексты ответов совпали между запусками. Общий smoke завершился ошибкой качества
в обоих случаях: ожидание третьего примера не меняли. В его объяснении также
заметны дефекты русского текста. Это ограничение текущего сочетания модели,
квантования и prompt; причина отдельно не установлена.

Pipeline создавалась явно с `NPU`. Счётчик драйвера `npu_busy_time_us` вырос
с 0 до 18503709 на первом прогоне и до 43846004 на повторном, подтверждая
аппаратное выполнение. 16 детерминированных Python-тестов прошли на хосте.
Песочница скрывала `/dev/accel` и запрещала сетевые сокеты, поэтому аппаратные
проверки и транспортные тесты запускались вне неё. Локальные артефакты:
`logs/npu/export-disk-tmp.log`, `logs/npu/runtime-first.log`, `logs/npu/runtime-repeat.log`,
`logs/npu/smoke-first.txt`, `logs/npu/smoke-repeat.txt`, `logs/npu/repeat-metrics.json`.
Веса, окружения и кеш не коммитятся. Полная проверка качества и сравнение
CPU/GPU остаются открытыми; наличие генерации на NPU не завершает приёмку #22.

Полный путь через Next.js `/api/graphql` также проверен на отдельном
демонстрационном проекте «NPU: проверка оплаты»: создание проекта и требований,
индексация обоих источников в Ollama BGE-M3 прошли (`ready`). Backend пересобран
и явно переключён на `openvino-npu`; прежние БД и Ollama сохранены. Однако анализ
с выданными приложением ID источников завершился `GENERATION_FAILED` за 9.83 s,
диагностика runtime — `INVALID_JSON_OR_SOURCES`. При дальнейшем воспроизведении
на синтетическом запросе найден повтор ключа `explanation`; ID и версии были
правильными. Успешный smoke с короткими ID `left`/`right` не гарантирует успех
полного сценария.
Подробности синтетического прогона — `logs/npu/app-smoke.json` и `logs/npu/runtime-app.log`.
Этот результат относится к прежнему runtime с манифестом v1; исправление и
повторная проверка описаны ниже.

### Исправление JSON Schema и повторная проверка 29.10.2026

Для каждого запроса теперь передаётся `StructuredOutputConfig(json_schema=...)`;
строгая проверка дубликатов, ID/версий, завершения и лимитов сохранена.
При ошибке ограниченной генерации нет повтора без схемы или починки JSON.
Манифест v2, `output_format=json-schema-v1`, SHA256:
`f36b13cbad8752f05d141eba6f817045986a615d13ffb5688fe417faeebca271`.
Профиль Go-генератора:
`2fc2eff3f491352f6575fc89d86689601297f243d1cb66a40e74ada39d9f865d`.
Веса и кеш не менялись, загрузка нового runtime заняла 2.71 s.

Новый `TestOpenVINORealNPUApplicationIDs` воспроизвёл ошибку на прежнем runtime
(RED, 9.50 s), а после исправления прошёл (GREEN, 8.06 s). Тот же проект
«NPU: проверка оплаты» повторно проанализирован через Next.js API: `completed`,
`possible_collision`, оба исходных ID и версии, `stale=false`, 9.73 s.
На момент проверки host runtime оставлен работающим для ручного запуска;
после перезагрузки его нужно запустить вновь по разделу 3. Логи нового прогона —
`logs/npu/runtime-structured.log`, `logs/npu/smoke-structured.txt`,
`logs/npu/app-smoke-structured.json`.

21 детерминированный Python-тест прошёл на хосте; выполнены `gofmt`,
`go test ./...`, `go vet ./...` (Go 1.27.3), `git diff --check`.
Аппаратные примеры: коллизия — PASS (8.49 s), совместимость — PASS (5.23 s),
новый регрессионный пример с ID приложения — PASS (8.06 s).
Недостаток данных по-прежнему получает `compatible` вместо `insufficient`
(6.43 s), поэтому полный аппаратный smoke остаётся FAIL по качеству.
Ограничение схемой устраняет структурную ошибку, а не гарантирует правильность
выводов; полная приёмка #22 остаётся открытой.

### Исправление CLI и нормализация ссылок 30.10.2026

CLI принимает полную SHA256 Ollama с префиксом `sha256:` и без него,
одинаково сравнивая pin при установке, проверке исполнения и `doctor`.
Существующие Python-окружения без `pip` дополняются локальным `ensurepip`.
Готовые веса и манифест используются повторно; скрытого перехода на другое
устройство или облачный API нет.

Идентификаторы, версии и порядок требований больше не передаются модели
как случайные технические данные: адаптер нормализует пару и проверяет
обратное отображение ссылок (см. [ADR-002](adr/002-openvino-npu.md)).
Инструкция требует объяснить условия до выбора вердикта и содержит независимые
примеры трёх классов. Манифест весов остался
`f36b13cbad8752f05d141eba6f817045986a615d13ffb5688fe417faeebca271`,
профиль генератора стал
`87a1d28c4652402ecef8a85aeab135d95980338b6f1b72905261e35419990501`.
Прежние анализы становятся устаревшими и требуют повторного запуска.

`TestOpenVINORealNPU*` прошли 14 сравнений: три базовые пары, пара с ID приложения,
три установочные пары и две независимые пары в обоих порядках.
Лог — `logs/npu/cli-final-hardware.txt`. Вердикт, исходные ID/версии и непустое
объяснение проверяются в каждом случае. Это проверка фиксированных примеров,
а не оценка общей точности модели. Сгенерированные объяснения всё ещё требуют
сверки с обоими источниками, в том числе при употреблении слов «слева/справа».
Сравнение производительности/качества CPU/GPU и полная приёмка #22 этим не закрыты.

Финальная цепочка `setup --device npu` → `down` → `up` → `doctor` завершилась
успешно на том же Core Ultra 7 268V. После повторного запуска все три анализа
имеют `completed`, `stale=false`, ожидаемые вердикты и точные снимки источников;
state содержит `verified=true`, управляемый процесс подтверждён, health сообщает
`device=NPU` и закреплённый профиль. Приложение оставлено запущенным.
Веса и БД сохранились; `up` не вызывал загрузку моделей, exporter или `pip`.
Проверка использовала готовый INT4-экспорт; полный экспорт с нуля не повторялся.

Отчёт — `logs/npu/cli-verification.json`, логи —
`logs/npu/cli-{setup,down,up,doctor}-verified.txt`.
73 теста CLI и 21 тест runtime прошли; выполнены `gofmt`, `go test ./...`,
`go vet ./...`, проверка NPU Compose и `git diff --check`.

## Возврат к Ollama

Остановите host runtime Ctrl+C. В другом терминале:

```bash
export COMPOSE_FILE=compose.yml:compose.intel.yml
# либо только compose.yml для CPU
docker compose up -d --build --force-recreate backend
docker compose up -d --wait
```

Сохраните `.env`, имя Compose-проекта, БД и volumes. Не используйте
`down --volumes`. При прямом запуске backend явно выберите
`PROJECT_CREATOR_GENERATOR_RUNTIME=ollama` и прежние модель/digest.
Прежние анализы сохранятся с прежними профилями; актуальность пересчитывается.

Обычные проверки без зависимостей OpenVINO, весов и NPU:

```bash
python3 -m unittest discover -s runtime/npu -v
go test ./...
go vet ./...
```

### Приёмка отмены и восстановления (#45), 5 ноября 2026

`TestOpenVINORealNPUCancelThenGenerate` прошёл на Intel AI Boost с локальным
Qwen2.5-7B INT4 и manifest-профилем
`f36b13cbad8752f05d141eba6f817045986a615d13ffb5688fe417faeebca271`.
Для проверки запущен отдельный runtime/socket; runtime действующего приложения
не перезапускался. Отправленный worker запрос отменён, в журнале с тем же
`call_id` подтверждена остановка процесса; следующий запрос восстановил worker
с прежним профилем и вернул результат с допустимыми источниками и схемой.

От обнаружения отключения клиента до `worker_stopped` прошло 0,349 с;
от отмены до нового результата — 14,71 с, включая восстановление и генерацию.
Сама повторная генерация заняла 9,508 с. Это один сценарий на свободном runtime,
не нагрузочный benchmark и не сравнение CPU/GPU. Проверка не подтверждает качество
вердиктов на произвольных требованиях. Отдельный runtime остановлен, временный
socket удалён; веса и общий compiled cache сохранены.

Локальные технические артефакты: `logs/npu/issue45-hardware-acceptance.json`,
`logs/npu/issue45-cancel-then-generate.log`,
`logs/npu/issue45-hardware-runtime.log` и журналы lifecycle. Логи исключены из Git
и Docker build context. Воспроизводимая команда — в разделе замены анализа README.
