# Experiment: Intel NPU on Core Ultra 7 268V

[Russian version](intel-npu.md)

This configuration targets native Linux, Python 3.12 and Docker Engine/Compose v2.
The implementation shipped through PR #24. The dated sections below preserve the initial acceptance results and subsequent fixes. The [device verification report](intel-device-verification.md) covers CPU/GPU/NPU generation and BGE-M3 static windows. The design and dependencies are recorded in [ADR-002](adr/002-openvino-npu.md).

Generation uses OpenVINO GenAI strictly on `NPU`. Embeddings continue to use BGE-M3/1024 in local Ollama on CPU or Intel Arc (`compose.intel.yml`). Standalone BGE-M3 encoder inference was verified on NPU at 128 and 512 tokens. Tokenization runs on CPU. The NPU runtime is a host process owned by the user; the backend connects through a Unix socket, without opening a network port or passing the NPU device to a container.

The local `.npu/` directory is prepared by this guide, not by ordinary `docker compose up`. It is excluded from Git and the Docker build context. Each user creates their own artifacts rather than copying environments from another laptop. Runtime source and pinned requirements live in the repository under `runtime/npu/`. `.npu/` is unnecessary when running only Ollama on CPU/Arc.

| Contents of `.npu/` | Purpose |
| --- | --- |
| `python/`, `export-env/`, `runtime-env/` | Local Python if needed and separate export/runtime environments |
| `huggingface/`, `source-path.txt`, `source-revision.txt` | Source-weight cache and pinned snapshot |
| `models/qwen2.5-7b-int4/` | Exported weights, tokenizers, license, and exporter freeze |
| `profile.json`, `profile.sha256` | Integrity/generation-mode manifest and its profile |
| `run/`, `compiled-cache/` | Runtime socket and compilation cache |
| `export-tmp/`, `*-freeze.txt` | Temporary export files and recorded environment dependencies |
| `uv-cache/` | uv cache for local Python and NPU dependencies |

Runtime logs, installation diagnostics, and diagnostic-run results are stored in the repository-root `logs/npu/`, excluded from Git and Docker build context. Weights, profiles, source revision, and `pip freeze` remain in `.npu/` as provenance for the model and environments.

Additional diagnostic scripts and logs mentioned below are artifacts of particular checks; they are not included in the repository and are not needed to follow this guide. `./project-creator setup --device npu` automates preparation and startup. System prerequisites and preserving existing artifacts are described in the [CLI guide](setup-cli.en.md). The manual procedure below remains available for diagnostics.

## 1. Branch, driver, and diagnostics

```bash
git fetch origin
git switch --track origin/feat/22-intel-npu
uname -a
ls -l /dev/accel/accel*
lspci -nnk | sed -n '/Processing accelerators/,+4p'
lsmod | rg 'intel_vpu'
dpkg-query -W 'intel-driver-compiler-npu' 'intel-fw-npu' 'intel-level-zero-npu' 'libze1'
```

Lunar Lake requires a working kernel driver, firmware, userspace NPU driver, and access to `/dev/accel/accel0`. Device presence alone does not prove inference. Install/update using the [Intel Linux NPU driver](https://github.com/intel/linux-npu-driver), selecting packages for your Ubuntu release. The 2026-10-29 check used v1.38.0 and Level Zero libze1 1.32.0 with OpenVINO 2026.4.1; the driver release listed OpenVINO 2026.3.1. Our results apply to the laptop and environment in section 5. Do not apply Windows driver numbers to Ubuntu. If access is denied, check the device group and the user's membership; logging in again is required after adding a group. Do not use `chmod 777` or privileged mode.

## 2. Explicitly prepare environments and weights

Run from the repository root. Allow additional disk space for source Safetensors, exported INT4, and caches: budget 40–50 GB free. Export and compilation can be demanding with 32 GB RAM; close unneeded processes and monitor RAM/swap. Nothing here is downloaded during ordinary application startup. The commands below explicitly download dependencies and the model.

On Ubuntu 26.04 the system Python may be 3.14. The pinned CPU PyTorch wheel targets CPython 3.12: do not replace system Python or install export packages globally. Environment setup requires uv **0.12.13 or newer**. This minimum matches the installed and tested version; `uv venv`, `uv pip install --python`, and `uv pip freeze --python` were checked with it. Upstream released 0.12.13 on 2026-09-10. uv is distributed under either MIT or Apache-2.0; [install uv using its official instructions](https://docs.astral.sh/uv/getting-started/installation/) and check its version:

```bash
uv --version
```

If `python3.12` is absent, install it locally through uv:

```bash
uv python install 3.12.14 --install-dir .npu/python --no-bin --cache-dir .npu/uv-cache
export NPU_PYTHON="$PWD/.npu/python/cpython-3.12.14-linux-x86_64-gnu/bin/python3.12"
"$NPU_PYTHON" --version
```

This path was checked with uv 0.12.13 on Linux x86_64; Python and its cache remain in Git-ignored `.npu/`. If Python 3.12 is already installed, set `NPU_PYTHON=python3.12`. The following commands use the selected interpreter.

```bash
mkdir -p .npu logs/npu
uv venv --no-project --no-config --python "${NPU_PYTHON:-python3.12}" --no-python-downloads .npu/export-env
uv pip install --python .npu/export-env/bin/python --no-python-downloads --no-config --cache-dir .npu/uv-cache --requirements runtime/npu/requirements-export.txt
.npu/export-env/bin/opt_in_out --opt_out
uv pip freeze --python .npu/export-env/bin/python --no-python-downloads > .npu/export-freeze.txt

uv venv --no-project --no-config --python "${NPU_PYTHON:-python3.12}" --no-python-downloads .npu/runtime-env
uv pip install --python .npu/runtime-env/bin/python --no-python-downloads --no-config --cache-dir .npu/uv-cache --requirements runtime/npu/requirements.txt
.npu/runtime-env/bin/opt_in_out --opt_out
uv pip freeze --python .npu/runtime-env/bin/python --no-python-downloads > .npu/runtime-freeze.txt
.npu/runtime-env/bin/python -c 'import openvino as ov; c=ov.Core(); print(c.available_devices); print(c.get_property("NPU", "FULL_DEVICE_NAME"))'
```

`uv pip freeze` records the installed environment; it is not an installation-event report. The CLI writes honest JSON diagnostics under `logs/npu/*-install.json` with the uv version, input requirements hash, command, result status, and freeze snapshot path/hash. It does not imitate pip's `--report` format. For a manual package inventory, use `uv pip list --python .npu/runtime-env/bin/python --format json > logs/npu/runtime-env-packages.json` (and likewise for the exporter). Both environments use `.npu/uv-cache`.

The last command should list the NPU. Resolve errors before downloading the pipeline. Python 3.12 is needed primarily for the pinned exporter PyTorch wheel. Dependency resolution for a separate runtime environment with the same pins succeeded on Python 3.14; this does not verify imports or NPU inference. If testing the runtime on 3.14, create a separate venv and perform the same `ov.Core()` check without changing the export environment or system packages.

The source is `Qwen/Qwen2.5-7B-Instruct` (Apache-2.0), not Ollama's GGUF. The command below resolves the current source commit once, saves it, and explicitly downloads that immutable snapshot. For reproducibility, set the previously saved `QWEN_REVISION` instead of resolving a new commit. `trust_remote_code` is not required.

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

The Qwen2.5 7B source Safetensors occupy about **15.23 GB (14.19 GiB)** across four shards; Ollama GGUF is not a substitute. The download can run in a separate terminal without Codex. Wait for it to complete before export. After interruption, retry with the same revision and `HF_HOME`; the SDK reuses completed cache files. Do not clear `.npu/`. The download script must not immediately enable the NPU provider in a running application: export, device validation, and a successful hardware smoke are required first.

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

For a local snapshot, specify the task explicitly: automatic detection for `optimum-intel==2.1.0` fails. The temporary directory must be on disk: the exporter first saves about 15.23 GB of intermediate weights and then compresses them. On the tested laptop `/tmp` is a 16 GiB tmpfs; export there failed with `basic_ios::clear: iostream error`. With `TMPDIR` moved to disk, the same export completed in 47.11 seconds, with 17.77 GiB peak RSS; the final INT4 directory including tokenizers is about 4.1 GiB.

The official [NPU export guide](https://docs.openvino.ai/2026/openvino-workflow-generative/inference-with-genai/inference-with-genai-on-npu.html) requires symmetric 4-bit compression with ratio 1.0. Results for this exported model on NPU appear in section 5. Keep installation diagnostics, freeze, license, and source revision. Export uses the pinned official CPU PyTorch wheel for Python 3.12/Linux x86_64; CUDA packages are unnecessary. Direct versions are pinned; the freeze records the full resolved export dependency tree on your laptop.

## 3. Manifest and local runtime

The initial budget is **2048 input + 512 output tokens**, not Ollama's 32768 context. The input budget includes the system prompt, JSON schema, chat template, and both sources. Short requirements are preferred; an overlong request returns an error without truncation. Changing the budget requires a new manifest and profile.

Manifest v2 also pins `output_format=json-schema-v1`: the runtime passes JSON Schema to native generation and then strictly validates the response and sources. If you already prepared a v1 manifest, rerun the `manifest` command below on the existing weights, restart the host runtime, and recreate the backend with the new profile. Weights do not need to be downloaded or exported again. Existing analysis results remain associated with the old profile and become stale.

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

The first startup compiles the model; allow up to 10 minutes. The process remains in this terminal. It prints the NPU name, load time, and profile. Each successful request then logs prompt tokens and latency, without requirement text. The command saves stdout and stderr to `logs/npu/runtime.log` while displaying them in the terminal; `pipefail` preserves a runtime error in the pipeline exit status. A Unix socket's absolute path is limited to 100 UTF-8 bytes. If the repository path is too long, use a shorter socket directory and set it consistently in `--socket`, curl commands, and `NPU_SOCKET_DIR` when connecting the application. The compilation cache is separate from weights; do not use another user's cache. Weights and tokenizer are checked with SHA256 at startup. File changes during operation are checked by inode/size/mtime/ctime and directory contents before and after a request. Do not modify the export while running. This checks local artifact integrity; it does not protect against a host owner who can replace the runtime itself.

In another terminal, you can check the API; this does not test generation:

```bash
curl --unix-socket "$PWD/.npu/run/runtime.sock" http://localhost/health
```

The socket has mode 660 and the directory mode 750, with the group of the user who created the directory. If group ownership differs from the primary group, use the actual directory/socket GID in the next step. Ctrl+C removes the socket. After a crash, first confirm the old process has stopped, and only then remove a leftover `.npu/run/runtime.sock`; the service will not overwrite an existing socket.

## 4. Connect the application

Use the existing `.env`, Compose project name, and volumes as described in the [Compose guide](compose.md). BGE-M3 embeddings must be explicitly downloaded and pinned by digest according to the [first-run instructions](compose.md#first-run-and-model-download) (CPU) or Intel Arc. NPU generation does not require downloading Qwen through Ollama: it uses the IR weights exported above. Do not change `PROJECT_CREATOR_GENERATOR_DIGEST` for NPU; that Ollama digest is ignored by the selected NPU adapter.

```bash
export PROJECT_CREATOR_NPU_PROFILE="$(cat .npu/profile.sha256)"
export NPU_SOCKET_DIR="$PWD/.npu/run"
export NPU_SOCKET_GID="$(stat -c %g .npu/run)"
# Intel Arc is used only for BGE-M3 embeddings here:
export COMPOSE_FILE=compose.yml:compose.intel.yml:compose.npu.yml
docker compose config --quiet
docker compose up -d --build --wait
```

For CPU embeddings, remove `compose.intel.yml` from `COMPOSE_FILE`. The backend remains user 65532, receives the socket's supplementary group and a read-only bind of the existing directory. UID mapping/rootless Docker may need separate permission configuration; do not work around it with privileged mode. For a backend running on the host, set `PROJECT_CREATOR_GENERATOR_RUNTIME=openvino-npu`, `PROJECT_CREATOR_NPU_SOCKET` to the absolute path, and `PROJECT_CREATOR_NPU_PROFILE` instead of using a Compose override.

Open http://localhost:3000. Indexing uses the current embedding profile; a new generator profile makes old analysis results stale while preserving data and vectors. Invalid NPU JSON is not repaired by a second model. Errors/busy status require an explicit retry in the UI. Only one request runs at a time. Generation timeout is 110 seconds. Cancellation terminates the worker process and releases the current native inference. On the next explicit launch or analysis retry, a health check starts a new worker with the same pinned model and profile; the cancelled request is not retried automatically. Recompilation may take time. Startup, readiness, cancellation, teardown, and error events are recorded relative to the repository root in `logs/npu/runtime-lifecycle.jsonl`; the journal contains the profile, stage code, and permitted `X-NPU-Call-ID`, but no source-pair text or model responses. NPU HTTP errors return a stable allowlisted code; unknown errors remain generic and contain no original text. A timeout, unexpected worker exit, or model-file change still requires an operator to explicitly restart the runtime. Analysis runs asynchronously and saves each pair's result. Preparation has 180 seconds and each separate comparison 120 seconds; there is no shared 180-second package deadline. On NPU error, the current pair and remaining unchecked pairs are marked as failed, while completed results are preserved. After explicitly restoring the runtime, retry failed pairs; completed pairs do not call the model again.

## 5. Laptop verification and recording results

A separate direct smoke invokes **the same Go Generator**, bypassing retrieval, on three fixed pairs and an additional regression pair with application IDs; expected IDs/versions/verdicts are checked:

```bash
mkdir -p logs/npu
set -o pipefail
NPU_REAL_SMOKE=1 \
PROJECT_CREATOR_NPU_SOCKET="$PWD/.npu/run/runtime.sock" \
PROJECT_CREATOR_NPU_PROFILE="$(cat .npu/profile.sha256)" \
go test ./internal/analysis -run '^TestOpenVINORealNPU' -v -count=1 -timeout=8m 2>&1 | tee logs/npu/smoke-results.txt
```

This opt-in smoke requires the Go version in go.mod on the host. Also check the full UI flow: project → two short requirements → indexing → analysis. Mutually exclusive payment methods should yield a collision proposal citing both requirements and versions. After editing a source, the old result should become stale. A compatible pair and insufficient conditions are checked in separate projects. Absence of a collision does not prove project consistency.

Record for first and repeated startup: OS/kernel, driver/firmware versions, `pip freeze`, profile, `device_name`, `load_seconds`, total analysis time, prompt tokens, RAM/swap (for example `/usr/bin/time -v` and `free -h`). Confirm NPU execution with a successful request on a pipeline explicitly created with `NPU`, and available driver/monitoring tools—not just `accel0` or health. AUTO/HETERO/CPU/GPU fallback is forbidden in the runtime. Compare CPU/GPU on the same texts, noting different weights/quantization and context separately; identical Qwen names do not mean identical artifacts. Hardware acceptance #22 is not complete until these runs are done.

Diagnostics that avoid exposing source data include `RUNTIME_VERSION_OR_IMPORT`, `NPU_DEVICE_UNAVAILABLE`, `NPU_COMPILE_OR_MODEL`, `PROMPT_LIMIT_OR_TEMPLATE`, `NPU_GENERATION`, `OUTPUT_LIMIT_OR_FINISH`, and `INVALID_JSON_OR_SOURCES` on stderr. At a limit, increase the explicit budget with a new profile or shorten the test data; truncation, JSON repair, and device switching are never automatic.

### Results on 2026-10-29 before enabling JSON Schema

Ubuntu 26.04, kernel `7.0.0-42-generic`, Intel AI Boost / Core Ultra 7 268V, 32 GB RAM, 8 GiB swap. Driver/compiler/firmware `1.38.0.20260910-34487311128~ubuntu26.04`, `libze1=1.32.0-1~26.04~ppa1`. Runtime: CPython 3.12.14 and pinned dependencies from `runtime/npu/requirements.txt`. Qwen source: `a09a35458c702b33eeacc393d103063234e8bc28`; symmetric channel-wise INT4, budget 2048 + 512. Manifest SHA256: `faadc4dfd75451445f0475a2be366fea19f19640a6a92933e28399b766261557`.

The existing `TestOpenVINORealNPUSmoke` was checked through the Go adapter; the test binary was built with Go 1.27.3. Pipeline load: 42.25 s without compiled cache, 4.59 s with cache (583 MiB). Peak RSS per `/usr/bin/time -v`: 5.81 / 6.54 GiB; this is not total system use. Swap remained occupied after export, and concurrent load was not isolated; device speed comparison was not performed.

| Example | Expected | Both runs | First request, s | After restart, s | Prompt tokens |
| --- | --- | --- | --- | --- | --- |
| Mutually exclusive payment methods | `collision` | `collision`, PASS | 5.87 | 8.59 | 477 |
| Alternative payment methods | `compatible` | `compatible`, PASS | 5.47 | 7.95 | 453 |
| Unspecified processing time | `insufficient` | `compatible`, FAIL | 7.92 | 9.92 | 447 |

All six responses contained valid JSON and exact source IDs/versions; response text matched across runs. The overall smoke failed its quality check on both runs: the expected result for the third example was not changed. Its explanation also showed Russian-language defects. This is a limitation of the current model, quantization, and prompt combination; the cause has not been determined.

The pipeline was explicitly created with `NPU`. Driver counter `npu_busy_time_us` increased from 0 to 18503709 on the first run and to 43846004 on the repeat, confirming hardware execution. 16 deterministic Python tests passed on the host. The sandbox hid `/dev/accel` and prohibited network sockets, so hardware and transport checks ran outside it. Local artifacts: `logs/npu/export-disk-tmp.log`, `logs/npu/runtime-first.log`, `logs/npu/runtime-repeat.log`, `logs/npu/smoke-first.txt`, `logs/npu/smoke-repeat.txt`, `logs/npu/repeat-metrics.json`. Weights, environments, and caches are not committed. Full quality checks and CPU/GPU comparison remain open; generation on NPU alone does not complete acceptance #22.

The full flow through Next.js `/api/graphql` was also checked on a separate demo project, “NPU: payment check”: project and requirements creation, and indexing both sources with Ollama BGE-M3 completed (`ready`). The backend was rebuilt and explicitly switched to `openvino-npu`; the existing database and Ollama were preserved. Analysis with application-issued source IDs failed with `GENERATION_FAILED` in 9.83 s; runtime diagnostics reported `INVALID_JSON_OR_SOURCES`. Further reproduction on a synthetic request found a duplicate `explanation` key; IDs and versions were correct. A successful smoke using short IDs `left`/`right` does not guarantee the full flow will work. Details: `logs/npu/app-smoke.json` and `logs/npu/runtime-app.log`. This result used the earlier runtime and manifest v1; the fix and repeat check are described below.

### JSON Schema fix and repeat check on 2026-10-29

Each request now passes `StructuredOutputConfig(json_schema=...)`; strict checking of duplicate keys, IDs/versions, completion, and limits remains. If constrained generation fails, there is no unconstrained retry or JSON repair. Manifest v2, `output_format=json-schema-v1`, SHA256: `f36b13cbad8752f05d141eba6f817045986a615d13ffb5688fe417faeebca271`. Go generator profile: `2fc2eff3f491352f6575fc89d86689601297f243d1cb66a40e74ada39d9f865d`. Weights and cache were unchanged; starting the new runtime took 2.71 s.

The new `TestOpenVINORealNPUApplicationIDs` reproduced the error on the previous runtime (RED, 9.50 s), then passed after the fix (GREEN, 8.06 s). The same “NPU: payment check” project was analyzed again through the Next.js API: `completed`, `possible_collision`, both source IDs and versions, `stale=false`, 9.73 s. The host runtime was left running for manual use at the time of the check; restart it after reboot using section 3. New run logs: `logs/npu/runtime-structured.log`, `logs/npu/smoke-structured.txt`, `logs/npu/app-smoke-structured.json`.

21 deterministic Python tests passed on the host; `gofmt`, `go test ./...`, `go vet ./...` (Go 1.27.3), and `git diff --check` were run. Hardware examples: collision PASS (8.49 s), compatibility PASS (5.23 s), new regression example with application IDs PASS (8.06 s). Insufficient data still returns `compatible` instead of `insufficient` (6.43 s), so the full hardware smoke still FAILS on quality. Schema constraints address structural errors; they do not guarantee correct conclusions. Full acceptance #22 remains open.

### CLI fix and reference normalization on 2026-10-30

The CLI accepts a complete Ollama SHA256 with or without the `sha256:` prefix and compares pins identically during setup, execution checks, and `doctor`. Existing Python environments without `pip` are supplemented with local `ensurepip`. Ready weights and manifests are reused; there is no hidden switch to another device or cloud API.

Requirement IDs, versions, and order are no longer passed to the model as arbitrary technical details: the adapter normalizes each pair and verifies the reverse mapping of references (see [ADR-002](adr/002-openvino-npu.md)). The instruction asks the model to explain conditions before choosing a verdict and includes independent examples for all three classes. The weight manifest remained `f36b13cbad8752f05d141eba6f817045986a615d13ffb5688fe417faeebca271`; the generator profile became `87a1d28c4652402ecef8a85aeab135d95980338b6f1b72905261e35419990501`. Previous analyses become stale and must be rerun.

`TestOpenVINORealNPU*` passed 14 comparisons: three base pairs, the application-ID pair, three installation pairs, and two independent pairs in both orders. Log: `logs/npu/cli-final-hardware.txt`. Verdict, original IDs/versions, and non-empty explanation are checked in every case. This is a fixed-example check, not an estimate of general model accuracy. Generated explanations still need to be checked against both sources, including when using “left/right”. CPU/GPU performance and quality comparison and full acceptance #22 remain open.

The final chain `setup --device npu` → `down` → `up` → `doctor` completed successfully on the same Core Ultra 7 268V. After restart, all three analyses were `completed`, `stale=false`, with expected verdicts and exact source snapshots; state contained `verified=true`, the managed process was confirmed, and health reported `device=NPU` and the pinned profile. The application was left running. Weights and database were preserved; `up` did not download models, run the exporter, or invoke `pip`. The check used the existing INT4 export; a full export from scratch was not repeated.

Report: `logs/npu/cli-verification.json`; logs: `logs/npu/cli-{setup,down,up,doctor}-verified.txt`. 73 CLI tests and 21 runtime tests passed; `gofmt`, `go test ./...`, `go vet ./...`, NPU Compose validation, and `git diff --check` were run.

## Return to Ollama

Stop the host runtime with Ctrl+C. In another terminal:

```bash
export COMPOSE_FILE=compose.yml:compose.intel.yml
# or compose.yml only for CPU
docker compose up -d --build --force-recreate backend
docker compose up -d --wait
```

Preserve `.env`, Compose project name, database, and volumes. Do not use `down --volumes`. When running the backend directly, explicitly select `PROJECT_CREATOR_GENERATOR_RUNTIME=ollama` and the previous model/digest. Existing analyses remain with their original profiles; staleness is recalculated.

Ordinary checks that do not require OpenVINO dependencies, weights, or NPU:

```bash
python3 -m unittest discover -s runtime/npu -v
go test ./...
go vet ./...
```

### Cancellation and recovery acceptance (#45), 2026-11-05

`TestOpenVINORealNPUCancelThenGenerate` passed on Intel AI Boost with local Qwen2.5-7B INT4 and manifest profile `f36b13cbad8752f05d141eba6f817045986a615d13ffb5688fe417faeebca271`. A separate runtime/socket was started for the check; the runtime serving the application was not restarted. A worker request was cancelled, and the journal confirmed process termination with the same `call_id`; the next request recovered the worker with the previous profile and returned a result with allowed sources and schema.

From detecting client disconnect to `worker_stopped` took 0.349 s; from cancellation to the next result took 14.71 s, including recovery and generation. The repeated generation itself took 9.508 s. This is one scenario on an otherwise idle runtime, not a load benchmark or CPU/GPU comparison. It does not confirm verdict quality on arbitrary requirements. The separate runtime was stopped and its temporary socket removed; weights and shared compiled cache were preserved.

Local technical artifacts: `logs/npu/issue45-hardware-acceptance.json`, `logs/npu/issue45-cancel-then-generate.log`, `logs/npu/issue45-hardware-runtime.log`, and lifecycle journals. Logs are excluded from Git and Docker build context. The reproducible command is in the [analysis replacement guide](requirement-analysis.md#replacing-parameters-of-a-running-analysis).
