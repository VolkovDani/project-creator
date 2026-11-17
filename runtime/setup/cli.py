"""Explicit local installer. Standard library only; never downloads from up/doctor."""
import argparse
import contextlib
import fcntl
import hashlib
import http.client
import json
import math
import os
from pathlib import Path
import platform
import re
import secrets
import shlex
import shutil
import signal
import stat
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.request

import gpu

UV_MIN_VERSION = (0, 12, 13)
UV_MIN_VERSION_TEXT = '.'.join(map(str, UV_MIN_VERSION))

MESSAGES = {
    'en': {
        'title': 'Install and run Project Creator on native Linux x86_64.',
        'setup': 'Check prerequisites, explicitly download local models and start the application.',
        'up': 'Start an installation already prepared by setup; no model downloads.',
        'down': 'Stop containers and the managed NPU runtime; preserve all data and weights.',
        'doctor': 'Check prerequisites and an existing installation; no downloads or changes.',
        'device': 'Explicit generator device (NPU uses CPU embeddings by default; --ollama-device can select Arc).',
        'ollama_device': 'Ollama device for embeddings; use with --device npu.',
        'project': 'Keep the existing Compose project name; only sets the name on first setup.',
        'switch': 'Explicitly change execution device; keep existing data and pinned weights.',
        'locale': 'Diagnostic language.',
        'step': '[{step}] {detail}',
        'error': '{code}: {detail}\nRun ./project-creator doctor for diagnostics. Details: docs/setup-cli.md',
        'ready': 'Application: {url}\nFull synthetic AI verification passed. Diagnostics: ./project-creator doctor',
        'stopped': 'Stopped. Database volumes, model weights and configuration are preserved.',
        'checks': 'Prerequisites passed for {device}; RAM {ram:.1f} GiB, swap {swap:.1f} GiB, free disk {disk:.1f} GiB.',
        'doctor_ok': 'Installation diagnostics passed. A fresh full AI check runs with ./project-creator up.',
        'prerequisites': 'Native Linux x86_64, Docker Engine + Compose v2, device access and sufficient memory/disk are required.',
        'config': 'Private configuration and stable Compose project.',
        'models': 'Local model download/cache, immutable digests and licenses.',
        'npu': 'Pinned Python environments, source snapshot, INT4 export and manifest.',
        'runtime': 'Managed NPU runtime, socket and profile.',
        'start': 'Build/start containers without deleting volumes.',
        'verify': 'UI/API, index and analysis with fixed synthetic sources and expected findings.',
        'fixture': 'Synthetic diagnostic project retained: {id}',
        'download': 'Model download is explicit; interrupted files are reused on retry.',
        'gpu_id': 'GPU UUID/index (numeric Ollama Vulkan index for Arc/AMD Vulkan); explicit change requires --switch-device.',
        'gpu_detected': 'Detected {device}: {name}; backend {backend}; requested adapter {id}. Inference is checked separately.',
        'execution': '{role}: {device}, requested adapter {id}; GPU offload bytes {vram}, model bytes {size}. Partial offload is possible; adapter identity/full GPU execution is not established by this metric.',
        'execution_unchecked': '{role}: no loaded-model measurement; doctor does not run inference. Run up for a fresh offload check.',
    },
    'ru': {
        'title': 'Установка и запуск Project Creator на нативном Linux x86_64.',
        'setup': 'Проверить окружение, явно скачать локальные модели и запустить приложение.',
        'up': 'Запустить подготовленную установку без скачивания моделей.',
        'down': 'Остановить контейнеры и управляемый NPU runtime, сохранить данные и веса.',
        'doctor': 'Проверить окружение и текущую установку без скачивания и изменений.',
        'device': 'Явное устройство генератора (для NPU embeddings по умолчанию работают на CPU; --ollama-device может выбрать Arc).',
        'ollama_device': 'Устройство Ollama для embeddings; используется вместе с --device npu.',
        'project': 'Сохранить имя существующего Compose-проекта; задаётся только при первой установке.',
        'switch': 'Явно изменить устройство, сохранить данные и закреплённые веса.',
        'locale': 'Язык диагностики.',
        'step': '[{step}] {detail}',
        'error': '{code}: {detail}\nДиагностика: ./project-creator doctor. Подробнее: docs/setup-cli.md',
        'ready': 'Приложение: {url}\nПолная синтетическая AI-проверка пройдена. Диагностика: ./project-creator doctor',
        'stopped': 'Остановлено. Тома БД, веса моделей и конфигурация сохранены.',
        'checks': 'Окружение для {device} проверено: RAM {ram:.1f} GiB, swap {swap:.1f} GiB, свободно {disk:.1f} GiB.',
        'doctor_ok': 'Диагностика установки пройдена. Полная AI-проверка выполняется командой ./project-creator up.',
        'prerequisites': 'Нужны нативный Linux x86_64, Docker Engine + Compose v2, доступ к устройству, память и место на диске.',
        'config': 'Приватная конфигурация и постоянное имя Compose-проекта.',
        'models': 'Скачивание/кеш локальных моделей, закрепление digest и лицензий.',
        'npu': 'Закреплённые Python-окружения, snapshot исходников, экспорт INT4 и манифест.',
        'runtime': 'Управляемый NPU runtime, сокет и профиль.',
        'start': 'Сборка/запуск контейнеров с сохранением томов.',
        'verify': 'UI/API, индексация и анализ с синтетическими источниками и ожидаемыми выводами.',
        'fixture': 'Сохранён синтетический проект диагностики: {id}',
        'download': 'Скачивание явно запрошено; после прерывания готовые файлы используются повторно.',
        'gpu_id': 'UUID/индекс GPU (для Arc/AMD Vulkan числовой индекс Ollama Vulkan); смена требует --switch-device.',
        'gpu_detected': 'Обнаружено {device}: {name}; backend {backend}; выбран адаптер {id}. Inference проверяется отдельно.',
        'execution': '{role}: {device}, выбран адаптер {id}; GPU offload {vram} байт, модель {size} байт. Возможен частичный offload; метрика не подтверждает личность адаптера и полное выполнение на GPU.',
        'execution_unchecked': '{role}: нет измерения загруженной модели; doctor не запускает inference. Для новой проверки offload выполните up.',
    },
}


# User-facing explanations are translated; source tool diagnostics remain verbatim.
ERROR_MESSAGES = {
    'GPU_SELECTION': ('Неоднозначный или отсутствующий адаптер. Укажите --gpu-id из nvidia-smi/rocminfo либо индекс Vulkan из логов Ollama; подробности в docs/setup-cli.md.', 'Ambiguous or missing adapter. Set --gpu-id from nvidia-smi/rocminfo or the Vulkan index from Ollama logs; see docs/setup-cli.md.'),
    'GPU_UNSUPPORTED': ('AMD GPU не входит в матрицу закреплённого ROCm. Проверьте docs/setup-cli.md; подмена GFX target и fallback отключены.', 'AMD GPU is outside the pinned ROCm matrix. See docs/setup-cli.md; target spoofing and fallback are disabled.'),
    'API': ('API недоступен или отклонил запрос. Проверьте frontend/backend и их логи.', 'The API is unavailable or rejected a request. Check frontend/backend and their logs.'),
    'BUSY': ('Другая команда установки уже работает. Дождитесь её завершения.', 'Another installation command is active. Wait for it to finish.'),
    'CONFIG_FORMAT': ('Неподдерживаемый формат .env. Исправьте кавычки, дубли ключей или интерполяцию; файл сохраняется.', 'Unsupported .env format. Fix quoting, duplicate keys or interpolation; the file is preserved.'),
    'CONFIG_PORT': ('Некорректный APP_PORT. Задайте в .env число от 1024 до 65535.', 'Invalid APP_PORT. Set a number between 1024 and 65535 in .env.'),
    'CONFIG_PROJECT': ('Некорректное имя Compose-проекта. Используйте строчные латинские буквы, цифры, дефис или подчёркивание.', 'Invalid Compose project name. Use lowercase letters, digits, hyphens or underscores.'),
    'CONFIG_SECRET': ('Проверьте секреты в приватном .env: замените placeholders, используйте token от 32 символов и URL-safe пароль БД. Существующие значения сохранены.', 'Check secrets in private .env: replace placeholders, use a token of at least 32 characters and a URL-safe database password. Existing values are preserved.'),
    'CONFIG_STATE': ('Состояние установки отсутствует или повреждено. Выполните setup с явным --device либо восстановите state.json.', 'Installation state is missing or invalid. Run setup with an explicit --device or restore state.json.'),
    'DEVICE_ACCESS': ('Нет доступа к выбранному устройству. Проверьте драйвер, группу устройства и новый вход в систему по инструкции.', 'The selected device is inaccessible. Check the driver, device group and a new login using the device guide.'),
    'DEVICE_CHANGE': ('Для смены устройства повторите setup с --device и явным --switch-device.', 'To change devices, repeat setup with --device and explicit --switch-device.'),
    'DEVICE_DRIVER': ('Runtime не видит рабочее устройство. Проверьте kernel/userspace драйвер и firmware по инструкции устройства.', 'The runtime cannot find a working device. Check kernel/userspace drivers and firmware using the device guide.'),
    'DEVICE_EXECUTION': ('Выполнение на выбранном устройстве или профиль модели не подтверждены. Проверьте логи runtime; переход на другое устройство не принят.', 'Execution on the selected device or the model profile was not confirmed. Check runtime logs; device fallback was not accepted.'),
    'DOCKER': ('Docker Engine/Compose недоступен или не подходит. Установите Compose v2.20+, запустите Engine и проверьте права пользователя.', 'Docker Engine/Compose is unavailable or unsupported. Install Compose v2.20+, start Engine and check user permissions.'),
    'FIXTURE': ('Не удалось подготовить синтетический проект проверки. Проверьте API и повторите команду: checkpoint сохранён.', 'Could not prepare a synthetic verification project. Check the API and retry; checkpoints are preserved.'),
    'FIXTURE_CHANGED': ('Диагностический проект изменён вручную или содержит дубли. Сохраните изменения и проверьте проект/state.json перед повтором.', 'A diagnostic project was edited manually or contains duplicates. Preserve edits and inspect the project/state.json before retrying.'),
    'INDEX': ('Индексация синтетического требования не завершилась. Проверьте состояние индекса и логи embeddings/Ollama.', 'Synthetic requirement indexing did not finish. Check index state and embeddings/Ollama logs.'),
    'MODEL_API': ('Локальный Ollama API недоступен или вернул неверный ответ. Проверьте контейнер Ollama.', 'The local Ollama API is unavailable or returned an invalid response. Check the Ollama container.'),
    'MODEL_CONFIG': ('Некорректное имя локальной модели. Проверьте MODEL в .env; удалённые URL не поддерживаются.', 'Invalid local model name. Check MODEL in .env; remote URLs are unsupported.'),
    'MODEL_DIGEST': ('Не получен полный digest модели. Проверьте загрузку и локальную запись модели Ollama.', 'A complete model digest was not returned. Check the download and local Ollama model record.'),
    'MODEL_DRIFT': ('Модель отсутствует или отличается от закреплённого digest. Восстановите прежние веса; pin не изменяется автоматически.', 'The model is missing or differs from its pinned digest. Restore the original weights; the pin is not updated automatically.'),
    'MODEL_EMBEDDING': ('Проверка embeddings не вернула корректный вектор размерности 1024. Проверьте BGE-M3 и её профиль.', 'The embedding probe did not return a valid 1024-dimensional vector. Check BGE-M3 and its profile.'),
    'MODEL_EXPORT': ('Экспорт NPU не завершён или существующие веса требуют проверки. Проверьте exporter Python 3.12, disk TMPDIR и инструкцию восстановления.', 'NPU export is incomplete or existing weights require inspection. Check the Python 3.12 exporter, disk TMPDIR and recovery guide.'),
    'MODEL_LICENSE': ('Лицензия модели не получена. Проверьте происхождение и лицензию весов перед повтором.', 'The model license was not returned. Check weight provenance and license before retrying.'),
    'MODEL_MANIFEST': ('Не удалось создать или проверить манифест/профиль. Проверьте завершённый экспорт и целостность файлов.', 'Could not create or verify the manifest/profile. Check the completed export and file integrity.'),
    'MODEL_QUALITY': ('Модель не прошла проверку ожидаемого вывода. Изучите синтетический проект; установка не помечена успешной.', 'The model failed an expected-finding check. Inspect the synthetic project; installation is not marked successful.'),
    'MODEL_RESPONSE': ('Анализ не завершён или ответ модели отклонён. Проверьте код ошибки анализа и логи runtime.', 'Analysis did not complete or the model response was rejected. Check the analysis error code and runtime logs.'),
    'MODEL_SOURCES': ('Ответ модели ссылается на неверные источники или версии. Изучите диагностический проект и ответ runtime.', 'The model response cites incorrect sources or versions. Inspect the diagnostic project and runtime response.'),
    'NPU_PID': ('Не удалось подтвердить личность управляемого NPU-процесса. Проверьте приватную запись PID; посторонние процессы не завершаются.', 'The managed NPU process identity could not be confirmed. Inspect the private PID record; unrelated processes are not terminated.'),
    'NPU_PROFILE': ('Профиль работающего NPU runtime отличается от настроенного. Выполните down, затем up.', 'The running NPU runtime profile differs from configuration. Run down, then up.'),
    'NPU_RUNTIME': ('NPU runtime не запустился или не прошёл проверку. Изучите logs/npu/runtime.log и состояние драйвера; fallback не включён.', 'The NPU runtime failed to start or pass health checks. Inspect logs/npu/runtime.log and the driver; fallback is disabled.'),
    'NPU_SOCKET': ('NPU-сокет недоступен или его владелец не подтверждён. Проверьте logs/npu/runtime.log, права и инструкцию восстановления сокета.', 'The NPU socket is unavailable or its owner is unverified. Check logs/npu/runtime.log, permissions and the socket recovery guide.'),
    'PLATFORM': ('Эта конфигурация не поддерживается. Нужен нативный Linux x86_64 с Docker Engine; подробная матрица приведена в инструкции.', 'This configuration is unsupported. Native Linux x86_64 with Docker Engine is required; see the support matrix.'),
    'PROJECT_CHANGE': ('Имя Compose-проекта отличается от сохранённого. Используйте прежнее имя, чтобы сохранить доступ к существующим томам.', 'The Compose project name differs from saved state. Use the original name to preserve access to existing volumes.'),
    'PYTHON': ('Для NPU нужны Python 3.12 и uv. Установите их отдельно, не заменяя системный Python.', 'NPU requires Python 3.12 and uv. Install them explicitly without replacing system Python.'),
    'UV_MISSING': ('Для окружений NPU нужен установленный uv. Установите uv отдельно по docs/setup-cli.md и повторите setup.', 'NPU environments require uv. Install uv separately as documented in docs/setup-cli.en.md, then repeat setup.'),
    'UV_VERSION': (f'Для окружений NPU нужен uv версии не ниже {UV_MIN_VERSION_TEXT}. Обновите uv по официальной инструкции и повторите setup.', f'NPU environments require uv {UV_MIN_VERSION_TEXT} or newer. Update uv using its official instructions, then repeat setup.'),
    'RESOURCES': ('Недостаточно RAM, swap или свободного диска. Освободите ресурсы по таблице в инструкции; измерения приведены в технической диагностике.', 'RAM, swap or free disk is insufficient. Free resources using the guide; measured values appear in technical diagnostics.'),
    'SOURCE_MANIFEST': ('Манифест исходных весов отсутствует или повреждён. Проверьте source.json и сохранённый revision перед повтором.', 'The source-weight manifest is missing or invalid. Inspect source.json and the saved revision before retrying.'),
    'UI': ('Интерфейс приложения недоступен. Проверьте Compose-сервисы, APP_PORT и логи frontend/backend.', 'The application UI is unavailable. Check Compose services, APP_PORT and frontend/backend logs.'),
    'COMMAND': ('Внешняя команда завершилась ошибкой. Проверьте текущий шаг и техническую диагностику инструмента.', 'An external command failed. Check the current step and tool diagnostics.'),
    'COMPOSE': ('Команда Docker Compose завершилась ошибкой. Проверьте конфигурацию, доступ к Engine и логи сервисов.', 'Docker Compose failed. Check configuration, Engine access and service logs.'),
    'MODEL_DOWNLOAD': ('Загрузка модели прервана или завершилась ошибкой. Проверьте сеть и повторите setup: готовые файлы кеша сохраняются.', 'The model download was interrupted or failed. Check the network and repeat setup; completed cache files are retained.'),
    'SOURCE_DOWNLOAD': ('Не удалось скачать или проверить исходные веса NPU. Проверьте сеть, source revision и контрольные суммы; повтор использует кеш.', 'Could not download or verify NPU source weights. Check the network, source revision and checksums; retry reuses the cache.'),
    'PYTHON_DOWNLOAD': ('Не удалось подготовить локальный Python 3.12. Проверьте uv, сеть и свободное место, затем повторите setup.', 'Could not prepare local Python 3.12. Check uv, network and free disk, then repeat setup.'),
    'PYTHON_ENV': ('Не удалось подготовить изолированное Python-окружение через uv. Проверьте Python и права на .npu.', 'Could not prepare an isolated Python environment with uv. Check Python and permissions on .npu.'),
    'PYTHON_INSTALL': ('Установка закреплённых NPU-зависимостей через uv завершилась ошибкой. Проверьте сеть, Python и logs/npu/*-install.json.', 'Pinned NPU dependency installation with uv failed. Check network, Python and logs/npu/*-install.json.'),
    'TOOLS_DOWNLOAD': ('Не удалось загрузить контейнер диагностического инструмента. Проверьте Docker registry и сеть, затем повторите setup.', 'Could not download the diagnostic tools container. Check Docker registry access and network, then repeat setup.'),
    'INTERRUPTED': ('Команда прервана. Повторите её: завершённые загрузки, экспорт и checkpoint сохранены.', 'The command was interrupted. Repeat it; completed downloads, exports and checkpoints are preserved.'),
    'LOCAL_STATE': ('Локальная операция завершилась ошибкой. Проверьте текущий шаг, права и приватное состояние установки.', 'A local operation failed. Check the current step, permissions and private installation state.'),
}
for _code, (_ru, _en) in ERROR_MESSAGES.items():
    MESSAGES['ru']['error.'+_code] = _ru
    MESSAGES['en']['error.'+_code] = _en
MESSAGES['ru']['technical'] = 'Техническая диагностика инструмента (исходный текст):'
MESSAGES['en']['technical'] = 'Technical tool diagnostic (original text):'


def describe_failure(error, lang):
    summary = MESSAGES[lang].get('error.'+error.code, MESSAGES[lang]['error.COMMAND'])
    return summary+'\n'+MESSAGES[lang]['technical']+' '+error.detail


class Failure(Exception):
    def __init__(self, code, detail):
        self.code, self.detail = code, detail
        super().__init__(f'{code}: {detail}')


def atomic(path, content, mode=0o600):
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    temporary = path.with_name(path.name + '.tmp-' + secrets.token_hex(5))
    try:
        with open(temporary, 'x', opener=lambda p, flags: os.open(p, flags, mode)) as stream:
            stream.write(content)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
    finally:
        temporary.unlink(missing_ok=True)


def read_env(path):
    return parse_env(path.read_text() if path.exists() else '')


def parse_env(content):
    values = {}
    for line in content.splitlines():
        line = line.strip()
        if not line or line.startswith('#'):
            continue
        match = re.fullmatch(r'(?:export\s+)?([A-Za-z_][A-Za-z_0-9]*)\s*=\s*(.*)', line)
        if not match:
            raise Failure('CONFIG_FORMAT', 'Unsupported .env syntax; use KEY=value or quoted literal values.')
        name, value = match.groups()
        try:
            words = shlex.split(value, comments=True)
        except ValueError:
            raise Failure('CONFIG_FORMAT', f'Invalid quoting for {name}.') from None
        if len(words) > 1 or '$' in value or '`' in value:
            raise Failure('CONFIG_FORMAT', f'Interpolation/multiline syntax is unsupported for {name}; use literal values.')
        if name in values:
            raise Failure('CONFIG_FORMAT', f'Duplicate setting {name}.')
        values[name] = words[0] if words else ''
    return values


class Config:
    def __init__(self, root):
        self.root = root.resolve()
        self.path = self.root / '.project-creator' / 'state.json'
        try:
            self.state = json.loads(self.path.read_text()) if self.path.exists() else {}
        except (ValueError, OSError):
            raise Failure('CONFIG_STATE', 'Cannot read state.json; restore it before retrying.') from None
        if not isinstance(self.state, dict) or self.state.get('version', 1) != 1:
            raise Failure('CONFIG_STATE', 'Unsupported setup state version.')
        self.values = read_env(self.root / '.env')

    def save(self):
        atomic(self.path, json.dumps(self.state, indent=2, ensure_ascii=False)+'\n')

    def update_env(self, updates):
        path = self.root / '.env'
        content = path.read_text() if path.exists() else ''
        for name, value in updates.items():
            if '\n' in value or '\r' in value or "'" in value or '$' in value:
                raise Failure('CONFIG_FORMAT', f'Unsupported literal value for {name}.')
            line = f"{name}='{value}'"
            pattern = rf'(?m)^[ \t]*(?:export[ \t]+)?{re.escape(name)}[ \t]*=.*$'
            if re.search(pattern, content):
                content = re.sub(pattern, lambda _: line, content)
            else:
                content += ('' if not content or content.endswith('\n') else '\n') + line + '\n'
        future = parse_env(content)
        atomic(path, content)
        self.values = future

    def ensure(self, device, project=None, switch=False, gpu_id=None, ollama_device=None):
        if device not in gpu.DEVICES:
            raise Failure('CONFIG_STATE', 'Unknown device; use setup --help.')
        ollama_device = ollama_device or (self.state.get('ollama_device') if device == 'npu' else device) or ('cpu' if device == 'npu' else device)
        if ollama_device not in ('cpu', *gpu.GPU_DEVICES) or (device != 'npu' and ollama_device != device):
            raise Failure('CONFIG_STATE', 'Ollama device must match the generator device except for NPU installations.')
        if gpu_id is not None:
            allowed = r'[0-9]+' if ollama_device in gpu.VULKAN_DEVICES else r'(?:[0-9]+|GPU-[a-fA-F0-9-]+)'
            if ollama_device not in gpu.GPU_DEVICES or not re.fullmatch(allowed, gpu_id):
                raise Failure('GPU_SELECTION', 'Use a numeric Ollama Vulkan index for Arc/AMD Vulkan, or one UUID/index for NVIDIA/AMD ROCm.')
        prior = self.state.get('device')
        if prior and prior != device and not switch:
            raise Failure('DEVICE_CHANGE', 'Use setup --device ... --switch-device to explicitly change devices.')
        desired = project or os.environ.get('COMPOSE_PROJECT_NAME') or self.values.get('COMPOSE_PROJECT_NAME') or self.root.name.lower()
        if not re.fullmatch(r'[a-z0-9][a-z0-9_-]*', desired):
            raise Failure('CONFIG_PROJECT', 'Use a lowercase Compose project name starting with a letter or digit.')
        if self.state.get('project') and self.state['project'] != desired:
            raise Failure('PROJECT_CHANGE', 'Project name differs from the saved installation; preserve the original volumes.')
        previous_ollama = self.state.get('ollama_device', 'cpu' if prior == 'npu' else prior)
        selected = gpu_id if gpu_id is not None else (self.state.get('gpu_id') if prior == device and previous_ollama == ollama_device else None)
        if prior == device and previous_ollama != ollama_device and not switch:
            raise Failure('DEVICE_CHANGE', 'Changing the Ollama device requires --switch-device.')
        if gpu_id is not None and prior == device and previous_ollama == ollama_device and self.state.get('gpu_id') != gpu_id and self.state.get('gpu_id') is not None and not switch:
            raise Failure('DEVICE_CHANGE', 'Changing the selected GPU requires --switch-device.')
        updates = {'PROJECT_CREATOR_GPU_ID': (selected or '') if ollama_device in ('nvidia', 'amd') else ''}
        if ollama_device in gpu.VULKAN_DEVICES and (gpu_id is not None or (previous_ollama != ollama_device and switch) or ollama_device == 'amd-vulkan'):
            updates['GGML_VK_VISIBLE_DEVICES'] = selected or ''
        for key in ('POSTGRES_PASSWORD', 'PROJECT_CREATOR_TOKEN'):
            value = self.values.get(key)
            if value is None:
                updates[key] = secrets.token_hex(24)
            elif not value or value.startswith('replace-') or (key == 'PROJECT_CREATOR_TOKEN' and len(value) < 32):
                raise Failure('CONFIG_SECRET', f'Replace the placeholder or invalid {key} privately in .env; existing values were preserved.')
        password = updates.get('POSTGRES_PASSWORD', self.values.get('POSTGRES_PASSWORD', ''))
        if not re.fullmatch(r'[A-Za-z0-9._~-]+', password):
            raise Failure('CONFIG_SECRET', 'POSTGRES_PASSWORD must be URL-safe for the existing Compose database URL.')
        port = self.values.get('APP_PORT', '3000')
        if not port.isdigit() or not 1024 <= int(port) <= 65535:
            raise Failure('CONFIG_PORT', 'APP_PORT must be between 1024 and 65535.')
        if 'APP_PORT' not in self.values:
            updates['APP_PORT'] = port
        if 'COMPOSE_PROJECT_NAME' not in self.values:
            updates['COMPOSE_PROJECT_NAME'] = desired
        self.update_env(updates)
        self.state.update(version=1, device=device, ollama_device=ollama_device, project=desired, gpu_id=selected)
        self.state.pop('execution', None)
        self.save()
        return dict(self.values)


class Runner:
    def run(self, args, *, capture=False, env=None, timeout=120, code='COMMAND'):
        try:
            result = subprocess.run(list(map(str, args)), check=False, text=True, cwd=None,
                                    env=env, stdout=subprocess.PIPE if capture else None,
                                    stderr=subprocess.PIPE if capture else None, timeout=timeout)
        except FileNotFoundError:
            raise Failure(code, f'Required executable {Path(args[0]).name} is missing.') from None
        except subprocess.TimeoutExpired:
            raise Failure(code, f'{Path(args[0]).name} timed out; repeat setup to reuse completed artifacts.') from None
        if result.returncode:
            # Captured commands can contain secrets in their output; do not include it.
            raise Failure(code, f'{Path(args[0]).name} exited with status {result.returncode}; see the step and local tool diagnostics.')
        return result.stdout if capture else ''


CASES = [
    ('collision', [
        'При любом заказе оплата разрешена только банковской картой. Другие способы запрещены.',
        'При любом заказе оплата разрешена только наличными. Другие способы запрещены.']),
    ('compatible', ['Пользователь может оплатить заказ банковской картой.',
                    'Пользователь может выбрать оплату наличными вместо банковской карты.']),
    ('insufficient', ['Заказ должен быть обработан быстро.',
                      'Для некоторых заказов обработка может занимать больше времени.']),
]


ANALYSIS_FIELDS = 'id status outcome stale errorCode sources{id version text} findings{leftId rightId leftVersion rightVersion verdict explanation}'


def validate_analysis_sources(result, expected):
    actual = result.get('sources', [])
    if len(actual) != len(expected) or {x['id']: x for x in actual} != {x['id']: x for x in expected}:
        raise Failure('MODEL_SOURCES', 'Analysis source snapshots do not match the synthetic requirements.')


def validate_analysis(result, expected, verdict):
    if result.get('status') != 'completed':
        raise Failure('MODEL_RESPONSE', f'Analysis did not complete ({result.get("errorCode", "unknown")}).')
    expected_outcome = {'collision': 'possible_collision', 'compatible': 'no_collisions_found', 'insufficient': 'insufficient'}[verdict]
    if result.get('stale') or result.get('outcome') != expected_outcome:
        raise Failure('MODEL_QUALITY', f'Expected {verdict}; inspect synthetic diagnostic project before relying on analysis.')
    sources = {x['id']: x for x in expected}
    validate_analysis_sources(result, expected)
    findings = result.get('findings', [])
    for finding in findings:
        left, right = sources.get(finding.get('leftId')), sources.get(finding.get('rightId'))
        if not left or not right or left['id'] == right['id'] or finding.get('leftVersion') != left['version'] or finding.get('rightVersion') != right['version']:
            raise Failure('MODEL_SOURCES', 'Analysis cited an unknown source or incorrect version.')
    if len(findings) != 1 or findings[0].get('verdict') != verdict or not findings[0].get('explanation', '').strip():
        raise Failure('MODEL_QUALITY', f'Expected one explained {verdict} finding; inspect the diagnostic project.')


class UnixConnection(http.client.HTTPConnection):
    def __init__(self, path):
        super().__init__('localhost', timeout=5)
        self.path = str(path)

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(self.timeout)
        self.sock.connect(self.path)


def normalized_digest(value):
    """Ollama APIs accept full SHA256 with or without the algorithm prefix."""
    if isinstance(value, str) and re.fullmatch(r'(sha256:)?[a-f0-9]{64}', value):
        return 'sha256:' + value.removeprefix('sha256:')
    return None


class Installer:
    def __init__(self, root, runner=None, output=None, lang='ru'):
        self.root = root.resolve()
        self.config = Config(root)
        self.runner = runner or Runner()
        self.output = output or sys.stdout
        self.lang = lang

    @property
    def npu(self):
        return self.root / '.npu'

    @property
    def logs(self):
        return self.root / 'logs' / 'npu'

    @property
    def url(self):
        return 'http://127.0.0.1:' + self.config.values.get('APP_PORT', '3000')

    def say(self, key, **params):
        print(MESSAGES[self.lang][key].format(**params), file=self.output, flush=True)

    def step(self, name):
        self.say('step', step=name, detail=MESSAGES[self.lang][name])

    def environment(self):
        # Compose shell variables override .env; never inherit an unrelated installation.
        return {key: value for key, value in os.environ.items()
                if not key.startswith(('PROJECT_CREATOR_', 'COMPOSE_', 'OLLAMA_', 'NPU_SOCKET_', 'GGML_', 'CUDA_', 'ROCR_', 'HIP_', 'HSA_', 'NVIDIA_'))
                and key not in ('POSTGRES_PASSWORD', 'APP_PORT', 'POSTGRES_PORT')}

    def compose(self, *args, capture=False, timeout=240, code='COMPOSE'):
        if not self.config.state.get('project'):
            raise Failure('CONFIG_STATE', 'Run setup with an explicit --device first.')
        command = ['docker', 'compose', '--project-directory', str(self.root), '--env-file', str(self.root / '.env'),
                   '--project-name', self.config.state['project'], '-f', str(self.root / 'compose.yml')]
        device = self.config.state.get('device')
        if device not in gpu.DEVICES:
            raise Failure('CONFIG_STATE', 'Unknown saved device; restore installation state.')
        ollama_device = self.config.state.get('ollama_device', 'cpu') if device == 'npu' else device
        if device == 'npu' and ollama_device not in ('cpu', 'arc'):
            raise Failure('CONFIG_STATE', 'Unknown saved Ollama device; restore state or explicitly switch devices.')
        if ollama_device in gpu.OVERRIDES:
            command += ['-f', str(self.root / gpu.OVERRIDES[ollama_device])]
        if device == 'npu':
            command += ['-f', str(self.root / gpu.OVERRIDES['npu'])]
        environment = self.environment()
        # Saved selections are authoritative even if .env or the shell drifted.
        selected = self.gpu_id(ollama_device)
        if ollama_device in ('nvidia', 'amd'):
            if selected is not None and not re.fullmatch(r'(?:[0-9]+|GPU-[a-fA-F0-9-]+)', selected):
                raise Failure('GPU_SELECTION', 'Invalid saved GPU selector; restore state or explicitly switch devices.')
            environment['PROJECT_CREATOR_GPU_ID'] = selected or ('0' if ollama_device == 'nvidia' else '')
        elif ollama_device in gpu.VULKAN_DEVICES:
            if selected is not None and not re.fullmatch(r'[0-9]+', selected):
                raise Failure('GPU_SELECTION', 'Vulkan requires one numeric Ollama GPU index; explicitly switch devices if desired.')
            environment['GGML_VK_VISIBLE_DEVICES'] = selected or ''
        return self.runner.run(command + list(args), capture=capture, timeout=timeout, env=environment, code=code)

    def gpu_id(self, device):
        if self.config.state.get('ollama_device', self.config.state.get('device')) == device and self.config.state.get('gpu_id') is not None:
            return self.config.state['gpu_id']
        if device == 'arc':
            return self.config.values.get('GGML_VK_VISIBLE_DEVICES') or None
        return None

    def inspect_gpu(self, device, identifier=None):
        try:
            if device == 'nvidia':
                inventory = self.runner.run(['nvidia-smi', '--query-gpu=index,uuid,name,driver_version', '--format=csv,noheader'], capture=True, env=self.environment(), code='DEVICE_DRIVER')
                selected = gpu.select(gpu.nvidia_devices(inventory), identifier)
            elif device == 'amd':
                if not os.access('/dev/kfd', os.R_OK | os.W_OK):
                    raise Failure('DEVICE_ACCESS', 'AMD ROCm requires accessible /dev/kfd; install the supported driver and check groups.')
                inventory = self.runner.run(['rocminfo'], capture=True, env=self.environment(), code='DEVICE_DRIVER')
                selected = gpu.select(gpu.amd_devices(inventory), identifier)
            elif device in gpu.VULKAN_DEVICES:
                # Vulkan IDs are Ollama enumeration IDs, not DRM render-node numbers.
                vendor = '0x8086' if device == 'arc' else '0x1002'
                render_nodes = [p for p in Path('/sys/class/drm').glob('renderD*')
                                if (p/'device/vendor').is_file() and (p/'device/vendor').read_text().strip() == vendor]
                if not render_nodes:
                    raise Failure('DEVICE_ACCESS', f'No {device} render device detected.')
                if device == 'amd-vulkan' and not any(os.access('/dev/dri/'+p.name, os.R_OK | os.W_OK) for p in render_nodes):
                    raise Failure('DEVICE_ACCESS', 'AMD Vulkan requires an accessible AMD /dev/dri/renderD* device.')
                if identifier is not None and not re.fullmatch(r'[0-9]+', identifier):
                    raise Failure('GPU_SELECTION', 'Vulkan needs one numeric GPU ID from Ollama logs.')
                if len(render_nodes) > 1 and identifier is None:
                    raise Failure('GPU_SELECTION', 'Multiple vendor render devices; specify --gpu-id using Ollama Vulkan logs.')
                selected = dict(id=identifier, name=f'{device} render device (Vulkan identity requires runtime logs)', backend='Vulkan')
            else:
                raise Failure('CONFIG_STATE', f'Unknown GPU mode: {device}')
        except gpu.GPUError as error:
            raise Failure(error.code, str(error)) from None
        self.say('gpu_detected', device=device, name=selected['name'], backend=selected['backend'], id=selected['id'] or 'auto')
        return selected['id']

    def prerequisites(self, device, gpu_id=None):
        if platform.system() != 'Linux' or platform.machine() not in ('x86_64', 'amd64'):
            raise Failure('PLATFORM', MESSAGES[self.lang]['prerequisites'])
        version = self.runner.run(['docker', 'compose', 'version', '--short'], capture=True, code='DOCKER')
        match = re.match(r'v?(\d+)\.(\d+)', version.strip())
        if not match or tuple(map(int, match.groups())) < (2, 20):
            raise Failure('DOCKER', 'Install Docker Compose v2.20 or newer using the official Docker instructions.')
        try:
            info = json.loads(self.runner.run(['docker', 'info', '--format', '{{json .}}'], capture=True, code='DOCKER'))
        except ValueError:
            raise Failure('DOCKER', 'Invalid Docker Engine diagnostics.') from None
        if info.get('OSType') != 'linux' or 'desktop' in info.get('OperatingSystem', '').lower():
            raise Failure('PLATFORM', 'Use native Linux Docker Engine; Docker Desktop is outside the supported matrix.')
        if device == 'nvidia' and 'nvidia' not in info.get('Runtimes', {}):
            raise Failure('DEVICE_DRIVER', 'Configure NVIDIA Container Toolkit for Docker (nvidia-ctk runtime configure); CLI does not install drivers or restart Docker.')
        memory = {}
        for line in Path('/proc/meminfo').read_text().splitlines():
            key, value = line.split(':', 1)
            memory[key] = int(value.split()[0]) * 1024
        gib = 1024**3
        ram, swap = memory['MemTotal']/gib, memory['SwapTotal']/gib
        disk = shutil.disk_usage(self.root).free/gib
        # Already exported NPU weights need much less temporary space on restart.
        exported = (self.npu / 'profile.json').exists()
        ollama_prepared = bool(self.config.values.get('PROJECT_CREATOR_EMBEDDING_DIGEST') and self.config.values.get('PROJECT_CREATOR_GENERATOR_DIGEST'))
        minimum_disk = (8 if exported else 50) if device == 'npu' else (6 if ollama_prepared else 24)
        if ram < (20 if device == 'npu' else 8) or ram+swap < (28 if device == 'npu' else 12) or disk < minimum_disk:
            raise Failure('RESOURCES', f'Need RAM >= {20 if device == "npu" else 8} GiB, RAM+swap >= {28 if device == "npu" else 12} GiB and disk >= {minimum_disk} GiB; found {ram:.1f}/{swap:.1f}/{disk:.1f}.')
        if device in ('npu', 'arc', 'amd', 'amd-vulkan'):
            devices = list(Path('/dev/accel').glob('accel*')) if device == 'npu' else list(Path('/dev/dri').glob('renderD*'))
            if not devices or not any(os.access(path, os.R_OK | os.W_OK) for path in devices):
                raise Failure('DEVICE_ACCESS', f'No accessible {device} device; install the vendor driver and check device groups. See docs/setup-cli.md.')
            if device == 'npu':
                if not Path('/sys/module/intel_vpu').exists():
                    raise Failure('DEVICE_DRIVER', 'intel_vpu is not loaded. Follow docs/intel-npu.md; CLI does not install drivers or change permissions.')
                if any('rootless' in option.lower() for option in info.get('SecurityOptions', [])):
                    raise Failure('DEVICE_ACCESS', 'NPU socket group mapping with rootless Docker needs manual configuration; use native rootful Engine for this automated path.')
        self.say('checks', device=device, ram=ram, swap=swap, disk=disk)
        if device in gpu.GPU_DEVICES:
            return self.inspect_gpu(device, gpu_id if gpu_id is not None else self.gpu_id(device))

    def ollama(self, path, body=None):
        # Node tools container reaches the private Compose service, not an exposed port.
        program = "const r=await fetch('http://ollama:11434'+process.argv[1],{method:process.argv[2]==='null'?'GET':'POST',headers:{'content-type':'application/json'},body:process.argv[2]==='null'?undefined:process.argv[2],signal:AbortSignal.timeout(30000)});if(!r.ok)process.exit(2);console.log(JSON.stringify(await r.json()));"
        text = self.compose('run', '--rm', '-T', '--no-deps', '--pull', 'never', '--entrypoint', 'node', 'model-info',
                            '--input-type=module', '-e', program, path, json.dumps(body), capture=True, code='MODEL_API')
        try:
            return json.loads(text)
        except ValueError:
            raise Failure('MODEL_API', 'Invalid local Ollama response.') from None

    def pin_model(self, model, role):
        if not re.fullmatch(r'[a-z0-9][a-z0-9._:/-]*', model) or any(x in model for x in ('..', '://')):
            raise Failure('MODEL_CONFIG', 'Use an explicit local Ollama model name.')
        records = self.config.state.setdefault('models', {})
        pin = records.get(model, {}).get('digest') or self.config.values.get('PROJECT_CREATOR_'+role+'_DIGEST')
        def digest():
            return next((item.get('digest') for item in self.ollama('/api/tags').get('models', []) if item.get('name') == model), None)
        actual = digest()
        if not actual:
            self.say('download')
            self.compose('exec', '-T', 'ollama', 'ollama', 'pull', model, timeout=7200, code='MODEL_DOWNLOAD')
            actual = digest()
        actual = normalized_digest(actual)
        if not actual:
            raise Failure('MODEL_DIGEST', 'Downloaded model has no complete immutable digest.')
        if pin and actual != normalized_digest(pin):
            raise Failure('MODEL_DRIFT', f'{model} differs from the saved digest; restore the pinned weights. No profile was changed.')
        details = self.ollama('/api/show', {'model': model})
        license_text = details.get('license', '')
        if not isinstance(license_text, str) or not license_text.strip():
            raise Failure('MODEL_LICENSE', f'No license returned for {model}; inspect model provenance before retrying.')
        records[model] = {'digest': actual, 'license': license_text, 'source': 'https://ollama.com/library/'+model,
                          'details': details.get('details', {})}
        self.config.save()
        self.config.update_env({'PROJECT_CREATOR_'+role+'_MODEL': model, 'PROJECT_CREATOR_'+role+'_DIGEST': actual})

    def uv_cli(self):
        executable = shutil.which('uv')
        if not executable:
            raise Failure('UV_MISSING', 'Install uv >= '+UV_MIN_VERSION_TEXT+' using the official uv instructions; the CLI does not install uv.')
        version = self.runner.run([executable, '--version'], capture=True, env=self.environment(), code='UV_VERSION').strip()
        match = re.match(r'^uv\s+(\d+)\.(\d+)\.(\d+)', version)
        if not match or tuple(map(int, match.groups())) < UV_MIN_VERSION:
            raise Failure('UV_VERSION', f'Requires uv >= {UV_MIN_VERSION_TEXT}; found {version or "an unrecognized version"}.')
        return executable, version

    def python_npu(self, uv=None):
        executable = shutil.which('python3.12')
        if executable:
            return executable
        python = self.npu / 'python/cpython-3.12.14-linux-x86_64-gnu/bin/python3.12'
        if not python.exists():
            uv = uv or self.uv_cli()[0]
            self.runner.run([uv, 'python', 'install', '3.12.14', '--install-dir', self.npu/'python', '--no-bin', '--cache-dir', self.npu/'uv-cache'], timeout=1200, code='PYTHON_DOWNLOAD')
        return str(python)

    def environments_npu(self, prepare_export=True):
        self.logs.mkdir(parents=True, exist_ok=True, mode=0o700)
        environments = [('runtime-env', 'requirements.txt')]
        if prepare_export:
            environments.insert(0, ('export-env', 'requirements-export.txt'))
        plans = []
        for name, requirement in environments:
            folder = self.npu/name
            requirements = self.root/'runtime/npu'/requirement
            digest = hashlib.sha256(requirements.read_bytes()).hexdigest()
            marker = folder/'project-creator-requirements.sha256'
            try:
                marker_data = json.loads(marker.read_text()) if marker.exists() else None
            except (OSError, ValueError):
                marker_data = None
            expected_marker = {'schema_version': 1, 'installer': 'uv', 'requirements_sha256': digest}
            plans.append((name, folder, requirements, digest, marker, (folder/'bin/python').exists(), marker_data == expected_marker))

        if any(not environment_exists or not marker_matches
               for _, _, _, _, _, environment_exists, marker_matches in plans):
            uv, uv_version = self.uv_cli()
            python = self.python_npu(uv=uv) if any(not plan[5] for plan in plans) else None
        else:
            uv = uv_version = python = None

        for name, folder, requirements, digest, marker, environment_exists, marker_matches in plans:
            environment_python = folder/'bin/python'
            if not environment_exists:
                self.runner.run([uv, 'venv', '--no-project', '--no-config', '--allow-existing', '--python', python,
                                 '--no-python-downloads', folder],
                                env=self.npu_environment(), code='PYTHON_ENV')
            expected_marker = {'schema_version': 1, 'installer': 'uv', 'requirements_sha256': digest}
            if not environment_exists or not marker_matches:
                report_path = self.logs/(name+'-install.json')
                freeze_path = self.npu/(name+'-freeze.txt')
                install_command = [uv, 'pip', 'install', '--python', environment_python,
                                   '--no-python-downloads', '--no-config', '--cache-dir', self.npu/'uv-cache',
                                   '--requirements', requirements]
                report = {
                    'schema_version': 1,
                    'report_format': 'project-creator.uv-install-diagnostics.v1',
                    'installer': 'uv',
                    'uv_version': uv_version,
                    'status': 'installing',
                    'python': str(environment_python),
                    'requirements_file': str(requirements.relative_to(self.root)),
                    'requirements_sha256': digest,
                    'install_command': list(map(str, install_command)),
                    'started_at': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()),
                }
                atomic(report_path, json.dumps(report, indent=2)+'\n')
                try:
                    self.runner.run(install_command, timeout=7200, env=self.npu_environment(), code='PYTHON_INSTALL')
                    self.runner.run([folder/'bin/opt_in_out', '--opt_out'], env=self.npu_environment(), code='PYTHON_ENV')
                    freeze = self.runner.run([uv, 'pip', 'freeze', '--python', environment_python,
                                              '--no-python-downloads', '--no-config'], capture=True, env=self.npu_environment(), code='PYTHON_ENV')
                    atomic(freeze_path, freeze)
                except Failure as error:
                    report.update(status='failed', failure_code=error.code,
                                  finished_at=time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()))
                    atomic(report_path, json.dumps(report, indent=2)+'\n')
                    raise
                report.update(status='installed', freeze_file=str(freeze_path.relative_to(self.root)),
                              freeze_sha256=hashlib.sha256(freeze.encode()).hexdigest(),
                              installed_package_count=sum(bool(line.strip()) for line in freeze.splitlines()),
                              finished_at=time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()))
                atomic(report_path, json.dumps(report, indent=2)+'\n')
                atomic(marker, json.dumps(expected_marker, sort_keys=True)+'\n')
        # Check userspace driver/runtime before downloading 15 GB of source weights.
        self.runner.run([self.npu/'runtime-env/bin/python', '-c', "import openvino as ov; c=ov.Core(); assert 'NPU' in c.available_devices, 'NPU unavailable'; print(c.get_property('NPU','FULL_DEVICE_NAME'))"], code='DEVICE_DRIVER', env=self.npu_environment())

    def npu_environment(self):
        result = self.environment()
        result.update(HF_HOME=str(self.npu/'huggingface'), HF_HUB_DISABLE_TELEMETRY='1', HF_HUB_DISABLE_IMPLICIT_TOKEN='1',
                      TMPDIR=str(self.npu/'export-tmp'), HF_HUB_DISABLE_PROGRESS_BARS='0',
                      UV_CACHE_DIR=str(self.npu/'uv-cache'), UV_NO_PROGRESS='1')
        (self.npu/'export-tmp').mkdir(parents=True, exist_ok=True)
        return result

    def download_npu(self):
        # SDK pins a commit once, resumes cache transfers and checks Safetensors SHA256 from LFS metadata.
        self.runner.run([self.npu/'export-env/bin/python', self.root/'runtime/setup/source.py', self.npu],
                        timeout=10800, env=self.npu_environment(), code='SOURCE_DOWNLOAD')
        try:
            source = json.loads((self.npu/'source.json').read_text())
        except (ValueError, OSError):
            raise Failure('SOURCE_MANIFEST', 'Missing or invalid verified source snapshot manifest.') from None
        self.config.state['source'] = source
        self.config.save()

    def manifest(self, model):
        value = self.runner.run([self.npu/'runtime-env/bin/python', self.root/'runtime/npu/npu_runtime.py', 'manifest',
            '--model', model, '--model-id', 'Qwen/Qwen2.5-7B-Instruct', '--revision', self.config.state['source']['revision'],
            '--license', 'Apache-2.0', '--export-command', 'optimum-intel==2.1.0; TMPDIR=.npu/export-tmp optimum-cli export openvino --task text-generation-with-past --weight-format int4 --sym --ratio 1.0 --group-size -1',
            '--max-prompt-tokens', '2048', '--max-new-tokens', '512', '--output', self.npu/'profile.json'], capture=True, env=self.npu_environment(), code='MODEL_MANIFEST').strip()
        if not re.fullmatch(r'[a-f0-9]{64}', value):
            raise Failure('MODEL_MANIFEST', 'Manifest did not return a complete profile.')
        atomic(self.npu/'profile.sha256', value+'\n')
        return value

    def export_npu(self):
        model = self.npu/'models/qwen2.5-7b-int4'
        profile = self.npu/'profile.sha256'
        if model.exists() and profile.exists() and (self.npu/'profile.json').exists():
            # Runtime verifies every hash at startup; do not silently trust marker alone.
            return
        partial = self.npu/'models/qwen2.5-7b-int4.partial'
        if partial.exists():
            shutil.rmtree(partial)  # Only this CLI-owned incomplete export; source cache remains.
        if model.exists():
            marker = model/'project-creator-export.json'
            if marker.exists() and json.loads(marker.read_text()).get('revision') == self.config.state['source']['revision']:
                self.manifest(model)
                return
            raise Failure('MODEL_EXPORT', 'Existing export has no verified CLI completion marker/profile; preserve it and inspect recovery documentation.')
        source = Path(self.config.state['source']['path'])
        env = self.npu_environment()
        env.update(HF_HUB_OFFLINE='1', TRANSFORMERS_OFFLINE='1')
        self.runner.run([self.npu/'export-env/bin/optimum-cli', 'export', 'openvino', '--model', source,
                         '--task', 'text-generation-with-past', '--weight-format', 'int4', '--sym', '--ratio', '1.0', '--group-size', '-1', partial],
                        timeout=7200, env=env, code='MODEL_EXPORT')
        if not (partial/'openvino_model.xml').is_file() or not (partial/'openvino_model.bin').is_file():
            raise Failure('MODEL_EXPORT', 'Export did not produce complete IR files; retry setup.')
        shutil.copyfile(source/'LICENSE', partial/'SOURCE-LICENSE')
        freeze = self.npu/'export-env-freeze.txt'
        if freeze.exists():
            shutil.copyfile(freeze, partial/'export-freeze.txt')
        atomic(partial/'project-creator-export.json', json.dumps({'revision': self.config.state['source']['revision']}))
        partial.rename(model)
        self.manifest(model)

    def npu_health(self):
        connection = UnixConnection(self.npu/'run/runtime.sock')
        try:
            connection.request('GET', '/health')
            response = connection.getresponse()
            data = json.loads(response.read(65536))
            if response.status != 200:
                raise Failure('NPU_RUNTIME', 'Runtime health failed; inspect logs/npu/runtime.log.')
            return data
        except (OSError, ValueError, http.client.HTTPException):
            raise Failure('NPU_SOCKET', 'NPU socket is unavailable; inspect logs/npu/runtime.log and device permissions.') from None
        finally:
            connection.close()

    def process_stamp(self, pid):
        try:
            fields = Path(f'/proc/{pid}/stat').read_text().rsplit(')', 1)[1].split()
            return {'pid': pid, 'start': fields[19], 'boot': Path('/proc/sys/kernel/random/boot_id').read_text().strip()}
        except (OSError, IndexError):
            return None

    def process_identity(self, pid, record=None):
        try:
            raw = Path(f'/proc/{pid}/cmdline').read_bytes().split(b'\0')
            expected = record or {}
            stamp = self.process_stamp(pid)
            return bool(stamp and all(stamp[key] == expected.get(key) for key in ('pid', 'start', 'boot'))
                        and str(self.root/'runtime/npu/npu_runtime.py').encode() in raw and b'serve' in raw)
        except OSError:
            return False

    def socket_stamp(self):
        try:
            value = (self.npu/'run/runtime.sock').lstat()
            if not stat.S_ISSOCK(value.st_mode) or value.st_uid != os.getuid():
                return None
            return {'device': value.st_dev, 'inode': value.st_ino, 'uid': value.st_uid}
        except OSError:
            return None

    def runtime_record(self):
        path = self.npu/'runtime.pid'
        if not path.exists():
            return None
        try:
            record = json.loads(path.read_text())
            if not isinstance(record, dict) or not isinstance(record['pid'], int) or record['pid'] < 2 or not record.get('start') or not record.get('boot'):
                raise ValueError('invalid pid record')
            return record
        except (ValueError, KeyError):
            raise Failure('NPU_PID', 'Invalid managed process record; inspect it without killing unrelated processes.') from None

    def recover_socket(self, record):
        sock = self.npu/'run/runtime.sock'
        if not sock.exists():
            return
        stamp = self.socket_stamp()
        if not record or not stamp or record.get('socket') != stamp or self.process_identity(record['pid'], record):
            raise Failure('NPU_SOCKET', 'Socket ownership is unverified. Confirm the owner stopped it before removing a stale socket.')
        # A different live runtime may have reused the pathname; never unlink a working listener.
        try:
            self.npu_health()
        except Failure as error:
            if error.code != 'NPU_SOCKET':
                raise
        else:
            raise Failure('NPU_SOCKET', 'Socket has a live listener; stop its owner before starting another runtime.')
        if self.socket_stamp() != stamp:
            raise Failure('NPU_SOCKET', 'Socket changed during stale recovery; no path was removed.')
        sock.unlink()

    def start_npu(self):
        run = self.npu/'run'
        run.mkdir(parents=True, exist_ok=True, mode=0o750)
        os.chmod(run, 0o750)
        sock = run/'runtime.sock'
        if len(str(sock).encode()) > 100:
            raise Failure('NPU_SOCKET', 'Repository path is too long for the runtime socket; move the repository to a shorter path.')
        profile = (self.npu/'profile.sha256').read_text().strip()
        pidfile = self.npu/'runtime.pid'
        prior = self.runtime_record()
        if prior and self.process_identity(prior['pid'], prior):
            health = self.npu_health()
            if health != {'device': 'NPU', 'profile': profile}:
                raise Failure('NPU_PROFILE', 'Running runtime differs from the saved profile; run down before up.')
        else:
            self.recover_socket(prior)
            arguments = [str(self.npu/'runtime-env/bin/python'), str(self.root/'runtime/npu/npu_runtime.py'), 'serve',
                '--model', str(self.npu/'models/qwen2.5-7b-int4'), '--manifest', str(self.npu/'profile.json'),
                '--profile', profile, '--socket', str(sock), '--cache', str(self.npu/'compiled-cache')]
            self.logs.mkdir(parents=True, exist_ok=True, mode=0o700)
            with open(self.logs/'runtime.log', 'a', opener=lambda p, flags: os.open(p, flags, 0o600)) as log:
                process = subprocess.Popen(arguments, env=self.npu_environment(), stdin=subprocess.DEVNULL,
                                           stdout=log, stderr=log, start_new_session=True)
            record = self.process_stamp(process.pid)
            if not record:
                raise Failure('NPU_PID', 'Could not record managed runtime identity; inspect logs/npu/runtime.log.')
            atomic(pidfile, json.dumps(record)+'\n')
            deadline = time.monotonic()+600
            while True:
                if process.poll() is not None:
                    raise Failure('NPU_RUNTIME', 'NPU compilation/start failed; inspect logs/npu/runtime.log. No fallback was enabled.')
                try:
                    stamp = self.socket_stamp()
                    if stamp and record.get('socket') != stamp:
                        record['socket'] = stamp
                        atomic(pidfile, json.dumps(record)+'\n')
                    health = self.npu_health()
                    if health == {'device': 'NPU', 'profile': profile}:
                        break
                except Failure:
                    pass
                if time.monotonic() > deadline:
                    self.stop_npu()
                    raise Failure('NPU_RUNTIME', 'NPU compilation exceeded 10 minutes; inspect logs/npu/runtime.log.')
                time.sleep(0.5)
        self.config.update_env({'PROJECT_CREATOR_NPU_PROFILE': profile, 'NPU_SOCKET_DIR': str(run), 'NPU_SOCKET_GID': str(run.stat().st_gid)})

    def stop_npu(self):
        pidfile = self.npu/'runtime.pid'
        if not pidfile.exists():
            return
        record = self.runtime_record()
        pid = record['pid']
        if self.process_identity(pid, record):
            os.killpg(pid, signal.SIGTERM)
            deadline = time.monotonic()+10
            while self.process_identity(pid, record) and time.monotonic() < deadline:
                time.sleep(0.1)
            if self.process_identity(pid, record):
                os.killpg(pid, signal.SIGKILL)
                deadline = time.monotonic()+5
                while self.process_identity(pid, record) and time.monotonic() < deadline:
                    time.sleep(0.1)
            if self.process_identity(pid, record):
                raise Failure('NPU_PID', 'Managed process did not stop; inspect logs/npu/runtime.log before retrying.')
        self.recover_socket(record)
        pidfile.unlink(missing_ok=True)

    def probe_embedding(self):
        model = self.config.values.get('PROJECT_CREATOR_EMBEDDING_MODEL', 'bge-m3:567m')
        response = self.ollama('/api/embed', {'model': model, 'input': ['Synthetic installation verification.'], 'truncate': False})
        embeddings = response.get('embeddings', [])
        if len(embeddings) != 1 or len(embeddings[0]) != 1024 or not all(isinstance(v, (int, float)) and math.isfinite(v) for v in embeddings[0]) or not any(v != 0 for v in embeddings[0]):
            raise Failure('MODEL_EMBEDDING', 'Embedding probe did not return one finite nonzero 1024-dimensional vector.')
        self.check_execution('EMBEDDING')

    def check_execution(self, role):
        device = self.config.state['device']
        if device == 'npu' and role == 'GENERATOR':
            expected = self.config.values.get('PROJECT_CREATOR_NPU_PROFILE')
            if self.npu_health() != {'device': 'NPU', 'profile': expected}:
                raise Failure('DEVICE_EXECUTION', 'Generation did not confirm NPU execution with the configured profile.')
            return
        model = self.config.values['PROJECT_CREATOR_'+role+'_MODEL']
        digest = normalized_digest(self.config.values['PROJECT_CREATOR_'+role+'_DIGEST'])
        loaded = next((item for item in self.ollama('/api/ps').get('models', []) if item.get('name') == model), None)
        if (not digest or not loaded or normalized_digest(loaded.get('digest')) != digest
                or type(loaded.get('size_vram')) is not int or loaded['size_vram'] < 0):
            raise Failure('DEVICE_EXECUTION', f'Loaded {role.lower()} model/digest is not confirmed by Ollama.')
        uses_gpu = loaded['size_vram'] > 0
        execution_device = self.config.state.get('ollama_device', 'cpu') if device == 'npu' else device
        if uses_gpu != (execution_device in gpu.GPU_DEVICES):
            raise Failure('DEVICE_EXECUTION', f'{role} is not executing on the explicitly selected {execution_device} path. Inspect Ollama logs; no fallback accepted.')
        record = dict(device=execution_device, requested_gpu=self.gpu_id(execution_device), size_vram=loaded['size_vram'], size=loaded.get('size'))
        self.config.state.setdefault('execution', {})[role] = record
        if execution_device in gpu.GPU_DEVICES:
            self.say('execution', role=role, device=execution_device, id=record['requested_gpu'] or 'auto', vram=record['size_vram'], size=record['size'])

    def graphql(self, query, variables=None):
        request = urllib.request.Request(self.url+'/api/graphql', data=json.dumps({'query': query, 'variables': variables or {}}).encode(), headers={'Content-Type': 'application/json'})
        try:
            with urllib.request.urlopen(request, timeout=210) as response:
                value = json.load(response)
            if value.get('errors') or not value.get('data'):
                raise Failure('API', 'GraphQL rejected the diagnostic operation; inspect backend logs without publishing secrets.')
            return value['data']
        except (OSError, ValueError, urllib.error.URLError):
            raise Failure('API', 'Frontend GraphQL proxy is unavailable or returned invalid JSON.') from None

    def fixture(self, name, texts):
        records = self.config.state.setdefault('fixtures', {})
        record = records.setdefault(name, {})
        if not record.get('project'):
            # Persist an ownership marker before a potentially interrupted create response.
            record.setdefault('marker', secrets.token_hex(8))
            self.config.save()
            title = 'Installation diagnostic: '+name+' ['+record['marker']+']'
            offset = 0
            while True:
                page = self.graphql('query($offset:Int!){projects(limit:100,offset:$offset){id name}}', {'offset': offset})['projects']
                matches = [project['id'] for project in page if project['name'] == title]
                if matches:
                    if len(matches) != 1:
                        raise Failure('FIXTURE_CHANGED', 'Duplicate installation marker; inspect synthetic projects.')
                    record['project'] = matches[0]
                    break
                if len(page) < 100:
                    break
                offset += len(page)
            if not record.get('project'):
                result = self.graphql('mutation($input:ProjectInput!){createProject(input:$input){project{id} fieldErrors{code}}}', {'input': {'name': title, 'goal': 'Synthetic local AI installation verification.', 'description': 'Generated by project-creator setup. No customer data.'}})['createProject']
                if result.get('fieldErrors') or not result.get('project'):
                    raise Failure('FIXTURE', 'Could not create the synthetic diagnostic project.')
                record['project'] = result['project']['id']
            self.config.save()
            self.say('fixture', id=record['project'])
        project = record['project']
        # Discover requirements after an interrupted request before creating more.
        requirements = self.graphql('query($p:ID!){requirements(projectId:$p,limit:20,offset:0){id version text}}', {'p': project})['requirements']
        if len(requirements) > 2 or any(item['text'] not in texts for item in requirements):
            raise Failure('FIXTURE_CHANGED', 'The diagnostic project contains manual changes; preserve it and inspect state.json before a new diagnostic run.')
        for text in texts:
            if not any(item['text'] == text for item in requirements):
                result = self.graphql('mutation($input:CreateRequirementInput!){createRequirement(input:$input){requirement{id version text} fieldErrors{code}}}', {'input': {'projectId': project, 'text': text}})['createRequirement']
                if result.get('fieldErrors') or not result.get('requirement'):
                    raise Failure('FIXTURE', 'Could not create a synthetic requirement.')
                requirements.append(result['requirement'])
        if len(requirements) != 2 or len({item['text'] for item in requirements}) != 2:
            raise Failure('FIXTURE_CHANGED', 'Duplicate diagnostic requirements; inspect the diagnostic project.')
        for item in requirements:
            result = self.graphql('mutation($p:ID!,$r:ID!){indexRequirement(projectId:$p,requirementId:$r){status version errorCode}}', {'p': project, 'r': item['id']})['indexRequirement']
            if result['status'] != 'ready' or result['version'] != item['version']:
                raise Failure('INDEX', f'Synthetic requirement indexing failed ({result.get("errorCode", "unknown")}).')
        # Already-ready indexes do not reload Ollama after a restart; probe explicitly.
        self.probe_embedding()
        return project, requirements

    def wait_analysis(self, project, result, expected):
        if result.get('status') != 'running':
            return result
        run_id = result.get('id')
        if not isinstance(run_id, str) or not run_id:
            raise Failure('MODEL_RESPONSE', 'Running analysis is missing its ID; cannot safely poll it.')
        # Twenty sequential pairs may each use the full 120-second model budget.
        deadline = time.monotonic() + 45*60
        while result.get('status') == 'running':
            validate_analysis_sources(result, expected)
            if result.get('stale'):
                raise Failure('MODEL_QUALITY', 'Analysis became stale while waiting; inspect the synthetic project.')
            remaining = deadline-time.monotonic()
            if remaining <= 0:
                raise Failure('MODEL_RESPONSE', 'Timed out waiting for the started analysis; progress remains saved in the project.')
            time.sleep(min(1, remaining))
            result = self.graphql('query($p:ID!){projectAnalysis(projectId:$p){'+ANALYSIS_FIELDS+'}}', {'p': project}).get('projectAnalysis')
            if not result or result.get('id') != run_id:
                raise Failure('MODEL_RESPONSE', 'Analysis run changed while waiting; refusing another attempt\'s result.')
        return result

    def verify(self):
        self.config.state['verified'] = False
        self.config.save()
        for name, texts in CASES:
            project, expected = self.fixture(name, texts)
            result = self.graphql('mutation($p:ID!){startProjectAnalysis(projectId:$p,mode:"all"){'+ANALYSIS_FIELDS+'}}', {'p': project})['startProjectAnalysis']
            result = self.wait_analysis(project, result, expected)
            validate_analysis(result, expected, name)
            self.check_execution('GENERATOR')
            self.config.state.setdefault('verification', {})[name] = {'project': project, 'checkedAt': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()), 'device': self.config.state['device']}
            self.config.save()
        self.config.state['verified'] = True
        self.config.save()

    def up(self):
        device = self.config.state.get('device')
        if not device:
            raise Failure('CONFIG_STATE', 'Run setup with an explicit --device first.')
        self.config.state['verified'] = False
        self.config.state.pop('execution', None)
        self.config.save()
        self.prerequisites(device)
        ollama_device = self.config.state.get('ollama_device', 'cpu') if device == 'npu' else device
        if device == 'npu' and ollama_device != 'cpu':
            self.prerequisites(ollama_device)
        if device == 'npu':
            self.step('runtime')
            self.start_npu()
        self.step('start')
        self.compose('up', '-d', '--build', '--wait', '--wait-timeout', '600', timeout=3600)
        self.check_ui()
        self.step('verify')
        self.verify()
        self.say('ready', url=self.url)

    def check_ui(self):
        try:
            with urllib.request.urlopen(self.url, timeout=15) as response:
                if response.status != 200 or b'<html' not in response.read(1000000):
                    raise Failure('UI', 'Frontend did not return an HTML application page.')
        except OSError:
            raise Failure('UI', 'Frontend is unavailable; inspect docker compose logs.') from None
        self.graphql('{projects(limit:1,offset:0){id}}')

    def setup(self, device, project=None, switch=False, gpu_id=None, ollama_device=None):
        ollama_device = ollama_device or (self.config.state.get('ollama_device') if device == 'npu' else device) or ('cpu' if device == 'npu' else device)
        if device != 'npu' and ollama_device != device:
            raise Failure('CONFIG_STATE', '--ollama-device is only available with --device npu.')
        if gpu_id is not None and ollama_device not in gpu.GPU_DEVICES:
            raise Failure('GPU_SELECTION', '--gpu-id requires arc, nvidia, amd or amd-vulkan.')
        selected = self.prerequisites(device, gpu_id=gpu_id)
        if device == 'npu' and ollama_device != 'cpu':
            selected = self.prerequisites(ollama_device, gpu_id=gpu_id)
        self.step('config')
        previous_device = self.config.state.get('device')
        self.config.ensure(device, project=project, switch=switch, gpu_id=selected, ollama_device=ollama_device)
        self.config.state['verified'] = False
        self.config.save()
        if previous_device == 'npu' and device != 'npu':
            self.stop_npu()
        self.step('models')
        # A newly selected NPU profile needs a nonempty profile variable even for parsing
        # the Compose tools service; use a placeholder only until preparation completes.
        if device == 'npu' and not self.config.values.get('PROJECT_CREATOR_NPU_PROFILE'):
            run = self.npu/'run'
            run.mkdir(parents=True, exist_ok=True, mode=0o750)
            self.config.update_env({'PROJECT_CREATOR_NPU_PROFILE': '0'*64, 'NPU_SOCKET_DIR': str(run), 'NPU_SOCKET_GID': str(run.stat().st_gid)})
        self.compose('up', '-d', '--build', '--wait', 'ollama', timeout=1800)
        self.compose('pull', 'model-info', timeout=1800, code='TOOLS_DOWNLOAD')
        self.pin_model(self.config.values.get('PROJECT_CREATOR_EMBEDDING_MODEL') or 'bge-m3:567m', 'EMBEDDING')
        if device != 'npu':
            self.pin_model(self.config.values.get('PROJECT_CREATOR_GENERATOR_MODEL') or 'qwen2.5:7b', 'GENERATOR')
        else:
            self.step('npu')
            exported = (self.npu/'profile.json').exists() and (self.npu/'profile.sha256').exists() and (self.npu/'models/qwen2.5-7b-int4').exists()
            self.environments_npu(prepare_export=not exported)
            if not exported:
                self.download_npu()
                self.export_npu()
        self.up()

    def down(self):
        self.compose('down', timeout=300)
        self.stop_npu()
        self.say('stopped')

    def doctor(self, device=None):
        if device and self.config.state.get('device') and device != self.config.state['device']:
            raise Failure('DEVICE_CHANGE', 'Doctor cannot reinterpret a saved installation as another device; switch with setup.')
        device = device or self.config.state.get('device') or 'cpu'
        self.prerequisites(device)
        ollama_device = self.config.state.get('ollama_device', 'cpu') if device == 'npu' else device
        if device == 'npu' and ollama_device != 'cpu':
            self.prerequisites(ollama_device)
        if self.config.state:
            self.compose('config', '--quiet', capture=True)
            self.check_ui()
            for role in ('EMBEDDING', 'GENERATOR'):
                if device == 'npu' and role == 'GENERATOR':
                    self.check_execution(role)
                    continue
                model = self.config.values.get('PROJECT_CREATOR_'+role+'_MODEL')
                pin = self.config.values.get('PROJECT_CREATOR_'+role+'_DIGEST')
                actual = next((item.get('digest') for item in self.ollama('/api/tags').get('models', []) if item.get('name') == model), None)
                if not normalized_digest(pin) or normalized_digest(pin) != normalized_digest(actual):
                    raise Failure('MODEL_DRIFT', f'{role} model is missing or differs from the pinned digest; repeat setup with the same device.')
                if ollama_device in gpu.GPU_DEVICES:
                    if any(item.get('name') == model for item in self.ollama('/api/ps').get('models', [])):
                        self.check_execution(role)
                    else:
                        self.say('execution_unchecked', role=role)
        self.say('doctor_ok')


@contextlib.contextmanager
def locked(root):
    directory = root/'.project-creator'
    directory.mkdir(exist_ok=True, mode=0o700)
    with open(directory/'setup.lock', 'a', opener=lambda p, flags: os.open(p, flags, 0o600)) as stream:
        try:
            fcntl.flock(stream, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise Failure('BUSY', 'Another setup/up/down command is active; wait for it to complete.') from None
        yield


def main(root=None, argv=None):
    root = Path(root or Path(__file__).resolve().parents[2])
    argv = sys.argv[1:] if argv is None else argv
    lang = 'en' if '--lang=en' in argv or ('--lang' in argv and argv[argv.index('--lang')+1:][:1] == ['en']) else 'ru'
    messages = MESSAGES[lang]
    parser = argparse.ArgumentParser(description=messages['title'])
    parser.add_argument('--lang', choices=['ru', 'en'], default=lang, help=messages['locale'])
    sub = parser.add_subparsers(dest='command', required=True)
    for name in ('setup', 'up', 'down', 'doctor'):
        command = sub.add_parser(name, help=messages[name])
        if name in ('setup', 'doctor'):
            command.add_argument('--device', choices=gpu.DEVICES, required=name == 'setup', help=messages['device'])
        if name == 'setup':
            command.add_argument('--project-name', help=messages['project'])
            command.add_argument('--switch-device', action='store_true', help=messages['switch'])
            command.add_argument('--gpu-id', help=messages['gpu_id'])
            command.add_argument('--ollama-device', choices=('cpu', 'arc'), help=messages['ollama_device'])
    args = parser.parse_args(argv)
    try:
        app = Installer(root, lang=args.lang)
        if args.command == 'doctor':
            app.doctor(args.device)
        else:
            with locked(root):
                if args.command == 'setup':
                    app.setup(args.device, project=args.project_name, switch=args.switch_device, gpu_id=args.gpu_id, ollama_device=args.ollama_device)
                else:
                    getattr(app, args.command)()
        return 0
    except Failure as error:
        print(messages['error'].format(code=error.code, detail=describe_failure(error, args.lang)), file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        print(messages['error'].format(code='INTERRUPTED', detail=describe_failure(Failure('INTERRUPTED', 'Repeat the same command; completed downloads and exports are retained.'), args.lang)), file=sys.stderr)
        return 130
    except (OSError, ValueError, KeyError) as error:
        # Avoid tracebacks containing configuration values or user model responses.
        print(messages['error'].format(code='LOCAL_STATE', detail=describe_failure(Failure('LOCAL_STATE', f'Local operation failed ({type(error).__name__}); inspect the current step and private state.'), args.lang)), file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())

