# Эксперимент: Intel NPU на Core Ultra 7 268V

Ветка `feat/22-intel-npu`, отдельный PR к #22; **не сливать до проверки на ноутбуке**.
Вариант рассчитан на нативный Linux, Python 3.12 и Docker Engine/Compose v2.
Здесь нет обещания ускорения: NPU, качество JSON и работа Qwen на конкретном
ноутбуке ещё требуют аппаратной проверки. Решение и зависимости — [ADR-002](adr/002-openvino-npu.md).

Генерация: OpenVINO GenAI, строго `NPU`. Embeddings: прежний BGE-M3/1024
в локальном Ollama на CPU либо Intel Arc (`compose.intel.yml`). NPU-совместимость
BGE-M3 не подтверждена; модель и размерность не заменяем. Токенизация работает
на CPU. NPU runtime — процесс пользователя на хосте; backend подключается
по Unix socket, без открытия сетевого порта или выдачи контейнеру NPU-устройства.

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
выбрав пакеты для своей Ubuntu. Кандидат: v1.38.0 + Level Zero libze1 1.32.0;
его release проверял OpenVINO 2026.3.1, совместимость с нашей 2026.4.1 проверяем
на ноутбуке. Не переносите номера Windows-драйверов в Ubuntu. При отсутствии
прав проверьте группу устройства и членство пользователя; после добавления
в группу нужен новый login. Не используйте `chmod 777` и privileged.

## 2. Явная подготовка окружений и весов

Выполняйте из корня репозитория. Потребуется дополнительное место для исходных
FP16 весов, экспортированного INT4 и кешей: ориентир 40–50 GB свободного диска.
На 32 GB RAM экспорт и компиляция могут быть тяжёлыми; закройте лишние процессы,
контролируйте RAM/swap. Здесь ничего не скачивается при обычном запуске приложения.
Следующие команды — отдельное явное скачивание зависимостей и модели.

```bash
mkdir -p .npu
python3.12 -m venv .npu/export-env
.npu/export-env/bin/python -m pip install --report .npu/export-install.json -r runtime/npu/requirements-export.txt
.npu/export-env/bin/opt_in_out --opt_out
.npu/export-env/bin/python -m pip freeze > .npu/export-freeze.txt

python3.12 -m venv .npu/runtime-env
.npu/runtime-env/bin/python -m pip install --report .npu/runtime-install.json -r runtime/npu/requirements.txt
.npu/runtime-env/bin/opt_in_out --opt_out
.npu/runtime-env/bin/python -m pip freeze > .npu/runtime-freeze.txt
.npu/runtime-env/bin/python -c 'import openvino as ov; c=ov.Core(); print(c.available_devices); print(c.get_property("NPU", "FULL_DEVICE_NAME"))'
```

Последняя команда должна показать NPU. Ошибку исправьте до загрузки pipeline.
Для источника используется `Qwen/Qwen2.5-7B-Instruct` (Apache-2.0),
не GGUF из Ollama. Команда ниже разрешает текущий source commit один раз,
сохраняет его и явно скачивает этот immutable snapshot. Для воспроизведения
задайте ранее сохранённый `QWEN_REVISION` вместо получения нового commit.
Никакого `trust_remote_code` не требуется.

```bash
.npu/export-env/bin/python - <<'PY'
import os
from pathlib import Path
from huggingface_hub import HfApi, snapshot_download
model = 'Qwen/Qwen2.5-7B-Instruct'
revision = os.environ.get('QWEN_REVISION') or HfApi().model_info(model).sha
Path('.npu/source-revision.txt').write_text(revision+'\n')
path = snapshot_download(model, revision=revision,
    allow_patterns=['*.json', '*.safetensors', '*.txt', '*.model', '*.tiktoken', '*.jinja', 'LICENSE*', 'README*'])
Path('.npu/source-path.txt').write_text(path+'\n')
print('Pinned source:', revision, path)
PY

export HF_HUB_OFFLINE=1 TRANSFORMERS_OFFLINE=1
.npu/export-env/bin/optimum-cli export openvino \
  --model "$(cat .npu/source-path.txt)" \
  --weight-format int4 --sym --ratio 1.0 --group-size -1 \
  .npu/models/qwen2.5-7b-int4
cp .npu/export-freeze.txt .npu/models/qwen2.5-7b-int4/export-freeze.txt
cp "$(cat .npu/source-path.txt)/LICENSE" .npu/models/qwen2.5-7b-int4/SOURCE-LICENSE
```

Официальный [NPU export guide](https://docs.openvino.ai/2026/openvino-workflow-generative/inference-with-genai/inference-with-genai-on-npu.html)
требует симметричную 4-битную компрессию с ratio 1.0. Совместимость этой
экспортированной модели с нашим NPU ещё не измерена. Сохраните отчёты установки,
freeze, лицензию и source revision. Для экспорта закреплён официальный CPU wheel PyTorch под Python 3.12/Linux x86_64;
CUDA-пакеты не требуются. Прямые версии закреплены, полное разрешённое
дерево экспортных зависимостей фиксирует freeze на вашем ноутбуке.

## 3. Манифест и локальный runtime

Начальный бюджет — **2048 входных + 512 выходных токенов**, не контекст Ollama
32768. Входной бюджет включает system prompt, схему JSON, chat template и оба
источника. Короткие требования предпочтительны; длинный запрос вернёт ошибку
без усечения. Изменение бюджета требует нового манифеста и профиля.

```bash
.npu/runtime-env/bin/python runtime/npu/npu_runtime.py manifest \
  --model .npu/models/qwen2.5-7b-int4 \
  --model-id Qwen/Qwen2.5-7B-Instruct \
  --revision "$(cat .npu/source-revision.txt)" \
  --license Apache-2.0 \
  --export-command 'optimum-intel==2.1.0; optimum-cli export openvino --weight-format int4 --sym --ratio 1.0 --group-size -1 (see export-freeze.txt)' \
  --max-prompt-tokens 2048 --max-new-tokens 512 \
  --output .npu/profile.json > .npu/profile.sha256

mkdir -p .npu/run
chmod 750 .npu/run
.npu/runtime-env/bin/python runtime/npu/npu_runtime.py serve \
  --model .npu/models/qwen2.5-7b-int4 --manifest .npu/profile.json \
  --profile "$(cat .npu/profile.sha256)" \
  --socket "$PWD/.npu/run/runtime.sock" --cache .npu/compiled-cache
```

Первый запуск компилирует модель; бюджет старта 10 минут. Процесс остаётся
в этом терминале. Он выводит имя NPU, время загрузки и профиль. Затем каждый
успешный запрос пишет prompt tokens и задержку, без текста требований.
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
Intel Arc. Для NPU не меняйте `PROJECT_CREATOR_GENERATOR_DIGEST`: это digest
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
за раз. Лимит генерации 110 секунд; отмена/timeout завершает worker процесс,
после чего требуется явно перезапустить runtime. Это освобождает зависший native
inference; повторная компиляция может занять время. Общий timeout анализа
приложения остаётся 180 секунд и может остановить анализ нескольких пар.

## 5. Проверка на ноутбуке и запись результатов

Отдельный прямой smoke вызывает **тот же Go Generator**, минуя retrieval,
на трёх фиксированных парах и проверяет ожидаемые ID/версии/вердикты:

```bash
set -o pipefail
NPU_REAL_SMOKE=1 \
PROJECT_CREATOR_NPU_SOCKET="$PWD/.npu/run/runtime.sock" \
PROJECT_CREATOR_NPU_PROFILE="$(cat .npu/profile.sha256)" \
go test ./internal/analysis -run '^TestOpenVINORealNPUSmoke$' -v -count=1 -timeout=7m 2>&1 | tee .npu/smoke-results.txt
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
