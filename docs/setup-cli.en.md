# Install and run with the CLI

[Russian version](setup-cli.md)

From the root of the cloned repository:

On a fresh installation, `./project-creator setup` starts PostgreSQL, the backend and the frontend and checks the UI/API. It downloads no model weights. On an existing CLI installation, the same command reuses the saved device. Use `setup --device none --switch-device` to switch back to CRUD while preserving data and model pins.

To prepare local models on a fresh installation, choose the device explicitly.

```bash
./project-creator setup --device cpu
# or Intel Arc:
./project-creator setup --device arc
# or NVIDIA GPU / AMD GPU with ROCm support:
./project-creator setup --device nvidia
./project-creator setup --device amd
# or AMD GPU/iGPU with Vulkan (without ROCm):
./project-creator setup --device amd-vulkan
# or Intel NPU for generation + CPU for BGE-M3:
./project-creator setup --device npu
# or Intel NPU for generation + Intel Arc for Ollama (embeddings):
./project-creator setup --device npu --ollama-device arc --gpu-id 0
```

For local inference, `setup` explicitly authorizes downloading local weights, containers, and required NPU dependencies. It prepares configuration, downloads and pins models, exports Qwen if needed, starts the runtime and Compose, and checks the UI/API, indexing, and real analysis of three synthetic pairs. On success it prints the application URL. You do not need to create `.env`, copy digests by hand, export weights manually, or start a second terminal.

```bash
./project-creator up       # restart and verify the configured roles
./project-creator doctor   # diagnose without downloading models
./project-creator down     # stop while preserving all volumes/weights
./project-creator --lang en doctor
```

`up` uses prepared weights; it does not run `ollama pull`, snapshot downloads, `uv pip install`, or the exporter. Container builds may access registries and dependency repositories. `doctor` checks the environment, UI/API availability, and installed model pins; it does not run inference or claim model quality. The diagnostic-tool container starts with `--pull never`.

## Remote generation and embeddings

Use `--device remote` to run the application on a machine without Ollama, an NPU, or local model weights. Use Docker Compose 2.20.2 or later for the remote configuration, including NPU generation with remote embeddings. PostgreSQL runs in Compose. Configure the endpoints in the private repository-root `.env` before setup. On a fresh installation the CLI generates its own database password and application token.

Set the URL and model ID for each role you want to use. Leave all variables for an unused role unset. Generation alone supports all-pairs comparisons and summaries. Embeddings alone support indexing and semantic search. Setup verifies only the selected roles using synthetic records.

For a manager on a separate Docker network, set `PROJECT_CREATOR_MODELS_NETWORK=models-configurator-clients` (or the existing network name) in the same private `.env`. When an active role uses the compatible API, the CLI adds `compose.models-configurator.yml`, connecting the backend to that network. Use the manager address reachable from the backend and its immutable model ID for each role.

**Managed Intel NPU with local Arc embeddings.** Start the separate runtime first. Set these values in Project Creator’s private `.env` and copy the full NPU ID from the manager's `GET /v1/models`. Keep the existing local embedding model, tag and digest. The application supplies the same NPU comparison prompts, aliases and validation as the native socket route.

```dotenv
PROJECT_CREATOR_MODELS_NETWORK=models-configurator-clients
PROJECT_CREATOR_GENERATOR_RUNTIME=openai-compatible
PROJECT_CREATOR_GENERATOR_URL=http://models-configurator:8317/v1
PROJECT_CREATOR_GENERATOR_MODEL=<full-openvino-npu-profile-id>
PROJECT_CREATOR_EMBEDDING_RUNTIME=ollama
```

To switch an installation from direct NPU to this combination, run `./project-creator setup --device arc --switch-device`. Subsequent starts use `./project-creator up`. The socket route remains available. [Manager setup](https://github.com/VolkovDani/models-configurator/blob/develop/docs/npu.md)

**Self-hosted vLLM.** Substitute the addresses reachable from the backend container and the model IDs served by your deployments. The chat model must support Chat Completions with strict JSON Schema output. The embedding model in this example returns 1024 dimensions natively.

```dotenv
PROJECT_CREATOR_GENERATOR_URL=http://inference.internal:8000/v1
PROJECT_CREATOR_GENERATOR_MODEL=team-chat-v1
PROJECT_CREATOR_EMBEDDING_URL=http://inference.internal:8001/v1
PROJECT_CREATOR_EMBEDDING_MODEL=team-embed-v1
```

**Hosted API.** For example, configure these values for an OpenAI API deployment and replace the key placeholders privately. Either endpoint can use its own optional bearer key. Use HTTPS when sending credentials or content across untrusted networks.

```dotenv
PROJECT_CREATOR_GENERATOR_URL=https://api.openai.com/v1
PROJECT_CREATOR_GENERATOR_MODEL=gpt-4.1-mini-2025-04-14
PROJECT_CREATOR_GENERATOR_API_KEY=your-generation-key
PROJECT_CREATOR_EMBEDDING_URL=https://api.openai.com/v1
PROJECT_CREATOR_EMBEDDING_MODEL=text-embedding-3-small
PROJECT_CREATOR_EMBEDDING_API_KEY=your-embedding-key
PROJECT_CREATOR_EMBEDDING_DIMENSIONS=1024
```

```bash
./project-creator setup --device remote
./project-creator doctor
```

Use `--switch-device` when changing an existing installation. For mixed operation select the local device and set only the external role's runtime to `openai-compatible` using `PROJECT_CREATOR_GENERATOR_RUNTIME` or `PROJECT_CREATOR_EMBEDDING_RUNTIME`. Configure that role's URL and model explicitly. A direct backend launch also requires explicit runtime selection. Connection and authorization errors stay with the selected provider.

Generation sends the two versioned requirement texts for comparison. Automatic summaries use the generation endpoint and send the versioned source and applicable confirmed glossary hints. The embeddings endpoint receives document chunks during automatic indexing and query text during search. Enabling a provider on an existing installation can send existing requirements as background work begins. Setup and up also run synthetic diagnostics.

Use versioned model IDs. Changing the embedding endpoint, model ID, or dimension mode creates a separate index profile. Restart the backend with the new settings and let automatic indexing finish before relying on search. Retry a failed requirement using the indexing action. If weights change behind a model alias, assign a new model ID before restarting. The backend requires 1024 components per vector.

## System prerequisites

The first supported configuration is **native Linux x86_64, Python 3 for the CLI, Docker Engine, and Docker Compose v2.20+**. NPU also requires Python 3.12 and uv. Docker Desktop, Windows, macOS, ARM, and automatic rootless NPU socket mapping are outside this support matrix. CLI hints, stages, and explanations for stable error codes are available in Russian and English (`--lang en`). Raw technical diagnostics from Docker, Python, Ollama, and other tools remain in their original language (usually English) and are clearly separated from the localized explanation and next action.

The CLI is shipped as a native executable in the repository, with no third-party Python dependencies for CPU/GPU use. Run it as a regular user with access to Docker Engine; do not run the installer through `sudo`.

Install Docker using its official instructions, start Engine, and verify that `docker info` and `docker compose version` work for the current user. Driver installation, changing groups/permissions, and rebooting are separate steps and are not hidden inside `setup`.

| Mode | Checks and preparation |
| --- | --- |
| CPU | At least 8 GiB RAM, at least 12 GiB RAM+swap, and 24 GiB free disk before the first install. 16–32 GB RAM is recommended for Qwen 7B. |
| Arc | Same resources, an Intel render device in `/dev/dri` with read/write permission; Linux graphics/Vulkan as described in the [Arc guide](intel-arc.md). |
| NVIDIA | Same resources, a compatible GPU and NVIDIA driver, NVIDIA Container Toolkit configured for Docker, and `nvidia-smi` available on the host. |
| AMD ROCm (`amd`) | Same resources, a compatible ROCm 7 driver, `rocminfo` on the host, and access to `/dev/kfd` and `/dev/dri`. Only GFX targets in the pinned upstream Ollama matrix are supported. |
| AMD Vulkan (`amd-vulkan`) | Same resources and enough memory for both models; `amdgpu` kernel driver, an AMD `/dev/dri/renderD*` device, and a Vulkan-compatible GPU/iGPU. Mesa RADV is included in a separate container; ROCm and `rocminfo` are not needed. Not all adapters or kernel/driver combinations work. |
| NPU | At least 20 GiB RAM, 28 GiB RAM+swap, and 50 GiB free disk for source Safetensors, intermediate exports, INT4, and caches. Loaded `intel_vpu`, an accessible `/dev/accel/accel*`, and firmware/userspace driver as described in the [NPU guide](intel-npu.md). |

After preparation, at least 6 GiB free disk is required for CPU/Arc, or 8 GiB for NPU. This is a startup budget, not a maximum-usage guarantee. Actual usage depends on Docker build cache, weights, and compilation. The CLI checks device detection separately from actual inference.

NPU requires CPython 3.12 and `uv` version **0.12.13** or later, installed in advance. The CLI checks that uv is installed; the installer does not download or install uv itself. uv 0.12.13 is the installed and tested baseline and includes the `uv venv` and `uv pip` commands used here; upstream released it on September 10, 2026. uv is available under either the MIT or Apache-2.0 license. Install and update it using the [official uv instructions](https://docs.astral.sh/uv/getting-started/installation/). If `python3.12` is not available, the CLI explicitly downloads a local CPython **3.12.14** through uv into `.npu/python`, without replacing system Python. Runtime and exporter use separate environments created with `uv venv`; packages from `runtime/npu/requirements*.txt` are installed with `uv pip install` into the explicitly selected Python. Existing environments, including those created without `pip`, are reused without recreation and without `ensurepip`. A legacy marker from a pip installation triggers one reapplication of the requirements with uv; when the marker is unchanged, uv install commands are skipped. The `logs/npu/export-env-install.json` and `logs/npu/runtime-env-install.json` reports are CLI diagnostics containing the uv version, status, requirements file hash, and environment snapshot reference; they do not claim to follow the `pip install --report` format. `uv pip freeze` is stored in `.npu/` with model provenance. The uv cache is stored in `.npu/uv-cache`; OpenVINO telemetry and implicit Hugging Face authentication are disabled. The environment checks `ov.Core().available_devices` before downloading source weights.

## Existing installations and data preservation

Before the first `setup`, run the command from the same repository directory, with the existing `.env` and Compose project name. If you previously used `-p`, specify that name **on the first CLI run**:

```bash
./project-creator setup --device arc --project-name existing-project-name
```

Project-name precedence: explicit argument → `COMPOSE_PROJECT_NAME` environment variable → value in `.env` → directory name. The name is written to `.env` and `.project-creator/state.json`; afterward the CLI rejects accidental changes. Using a different name creates a different Docker volume namespace, so do not guess the old installation's name. Arbitrary Compose override files from the `COMPOSE_FILE` environment variable are not inherited. The CLI selects Compose files for the saved device and the explicitly configured model network.

Existing secrets and settings are preserved; missing database passwords and tokens are generated independently (`secrets.token_hex(24)`). `.env` and state are written atomically with mode 600. Placeholder secrets are rejected rather than replaced automatically, so an existing database does not lose its password. The password must be URL-safe because the standard Compose setup interpolates it into a connection URL. A basic literal `.env` is supported; complex shell interpolation, multiline values, and duplicate keys must be simplified manually first. Secrets are not printed; do not publish `.env`, runtime logs, or install reports.

Logs and diagnostic reports are stored in the root `logs/`, which is excluded from Git and the Docker build context. For NPU runtime and installation reports, the CLI creates `logs/npu/`; runtime stdout/stderr are appended to `logs/npu/runtime.log`. Weights, profiles, and environment freeze files remain in `.npu/`. Container logs are stored in Docker. Save them separately if needed:

```bash
mkdir -p logs
docker compose logs --no-color > logs/compose.log
```

For manual startup, use the same Compose files and project name as for running the application.

No CLI command runs `down --volumes`, deletes the database, or automatically migrates between different project names. `down` stops Compose and only a CLI NPU runtime that has been confirmed as managed. Weights, source cache, compiled cache, manifest, analysis history, and synthetic diagnostic projects remain. Concurrent `setup`/`up`/`down` commands are blocked by a local lock.

Explicitly switch devices with:

```bash
./project-creator setup --device cpu --switch-device
```

When switching away from NPU, the managed runtime is stopped; NPU weights are not deleted. An NPU failure does not trigger a switch to CPU/Arc/cloud. By default, the NPU profile uses CPU for Ollama and embeddings. To use Intel Arc for Ollama while generating with Intel NPU, set `--ollama-device arc`; `--gpu-id` selects the Arc Vulkan ID from Ollama logs and is used as in a regular Arc profile. The CLI starts `compose.intel.yml` before `compose.npu.yml`, retaining the NPU generation runtime. The installation stores the two devices separately in private `.project-creator/state.json`; repeated `up` and `setup --device npu` preserve the selected Ollama profile. Changing it requires explicit `--switch-device`. Older state files without `ollama_device` continue to mean NPU generation with CPU Ollama. The embeddings check confirms Ollama offload; the generator check still confirms the NPU profile. Pinned models, their digests, and installation data are preserved when switching.

## Clear analysis history for one project

With Project Creator running, copy the project ID from `/projects/<id>` in the browser URL. Replace `PROJECT_ID` below with that ID. The command uses the local frontend proxy on `APP_PORT` (default `3000`).

```bash
./project-creator clear-analysis-history --project-id PROJECT_ID
```

The preview shows how many finished runs are eligible and how many running analyses will be skipped. To permanently delete the finished runs found by the command, including their saved pair results and source snapshots, run it with `--yes`.

```bash
./project-creator clear-analysis-history --project-id PROJECT_ID --yes
```

An interruption can leave a run scheduled for deletion. Repeat the preview to see what remains, then repeat the command with `--yes` to finish.

## GPUs and adapter selection

`arc` means a GPU, including integrated Intel Arc; `npu` is a separate OpenVINO generator. NVIDIA uses CUDA, `amd` uses ROCm, and `amd-vulkan` uses Mesa RADV/Vulkan for AMD dGPU/iGPU without ROCm. Each CLI profile attaches exactly one override; do not combine different GPU overrides.

```bash
nvidia-smi -L
./project-creator setup --device nvidia --gpu-id GPU-full-uuid
rocminfo
./project-creator setup --device amd --gpu-id GPU-full-uuid
# Vulkan: numeric ID from Ollama logs, not a renderD* number or ROCm UUID:
./project-creator setup --device arc --gpu-id 0
./project-creator setup --device amd-vulkan --gpu-id 0
./project-creator setup --device cpu --switch-device
```

These are examples: replace the UUID with the actual value reported by the utility. With one NVIDIA or AMD ROCm GPU, the CLI selects it automatically; if there are several, `--gpu-id` is required. NVIDIA accepts an `nvidia-smi` index or UUID, then stores the UUID. For AMD, use `Uuid: GPU-...` from `rocminfo`; the CLI's numeric index is the zero-based order of GPUs, excluding CPU agents. It is **not** the `Agent N` value in the output. UUID is preferred. The selected AMD ROCm GPU must be in the [GFX matrix](compose.md#gpu-and-integrated-graphics-startup-matrix); an unrelated or unsupported adapter is rejected before models are downloaded.

Configure NVIDIA Container Toolkit in advance using the [official guide](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html). The CLI checks that the NVIDIA runtime is available in Docker and that the driver is version 550 or later; some older cards require 570 or later. Check card compatibility against the [pinned Ollama version's documentation](https://github.com/ollama/ollama/blob/v0.40.16/docs/gpu.mdx). `amd` requires the [ROCm 7 Linux driver](https://rocm.docs.amd.com/projects/install-on-linux/en/latest/) and `rocminfo`; it uses the official `ollama/ollama:0.40.16-rocm` image. Check the **specific device and operating system** against the [AMD matrix](https://rocm.docs.amd.com/en/latest/compatibility/compatibility-matrix.html) and the GFX target against [Ollama v0.40.16](https://github.com/ollama/ollama/blob/v0.40.16/docs/gpu.mdx). An iGPU supported by a newer ROCm release may still be absent from the pinned Ollama version. The CLI does not install system drivers.

`amd-vulkan` requires working `amdgpu` and Vulkan, but **not** ROCm. Check `ls -l /dev/dri/renderD*` and, if `vulkaninfo` is available, run `vulkaninfo --summary` on the host. The container is built from `Dockerfile.ollama-amd-vulkan` with Mesa RADV and a single AMD Vulkan ICD. The ROCm GFX target list does not apply. With one available AMD GPU, the index can be omitted; with multiple AMD render devices, `--gpu-id` must be the numeric index **from Ollama logs inside this container**. Vulkan indices can change after driver, container, or hardware updates. For manual startup, inspect logs with `docker compose -f compose.yml -f compose.amd-vulkan.yml logs ollama`; when starting with the CLI, keep the original Compose project name. If the selected Vulkan backend does not support the model or available GPU memory, `setup` fails; operation is not guaranteed on every Radeon/Ryzen APU. See [Vulkan in Ollama v0.40.16](https://github.com/ollama/ollama/blob/v0.40.16/docs/gpu.mdx) and [Mesa RADV](https://docs.mesa3d.org/drivers/radv.html). The project has not performed AMD/NVIDIA hardware acceptance for this CLI version.

The Intel profile loads only the Intel Mesa ICD; AMD Vulkan loads only the AMD RADV ICD. Other vendors are excluded from device discovery. If there are multiple Intel render devices, explicitly set the ID from logs of **this** container; ordering may change after driver or hardware updates. A wrong index must cause an offload error. To obtain the initial logs, use the manual Arc startup in `docs/intel-arc.md`. `--gpu-id` accepts one adapter, not a list. This option is forbidden for CPU and NPU with CPU embeddings; for NPU + Arc it selects the Intel Ollama adapter.

The `amd-vulkan` profile stores the numeric adapter selection for future `up` and `doctor` runs. When explicitly switching from AMD Vulkan to NPU + Arc, the old AMD index is not reused for Intel; set a new `--gpu-id` if needed.

The UUID/index is stored in `.project-creator/state.json`; changing it requires `setup --switch-device`. For an existing Arc state without the new field, the old `GGML_VK_VISIBLE_DEVICES` value from `.env` is preserved (one ID or automatic selection). For `amd-vulkan`, the new selection is stored separately from the ROCm UUID. The stored selection takes priority over `.env` and shell variables, including for `up`, `down`, and `doctor`. CUDA/ROCr/HIP/HSA overrides from the shell are not inherited. Changing GPUs does not replace secrets, Compose project name, database, or weights.

`setup`/`up` run the same three synthetic AI scenarios. After the embedding probe and generation, loaded model/digest and positive `size_vram` are checked. Diagnostics list the roles separately, with the requested adapter and model/VRAM bytes. Positive `size_vram` confirms offload but does not measure the share of operations and does not prove that every operation ran on the GPU; partial CPU offload is possible. Device identity is constrained by runtime isolation and selection; it is not inferred from VRAM. Automatic CPU fallback is not accepted as a successful installation. Driver/model errors do not change the selected profile.

`doctor` reads inventory, pins, and `/api/ps`; it does not run inference. For an already loaded model it reports a measurement; for an unloaded model it clearly says a fresh check requires `up`. `doctor --device` cannot relabel the device of an existing installation; switch it with `setup`.

Manual AMD startup after configuring `.env` and downloading/pinning models:

```bash
PROJECT_CREATOR_GPU_ID=GPU-full-uuid docker compose -f compose.yml -f compose.amd.yml up -d --wait
# or Vulkan (without ROCm); the index can be omitted with one AMD GPU:
GGML_VK_VISIBLE_DEVICES=0 docker compose -f compose.yml -f compose.amd-vulkan.yml up -d --build --wait
```

For manual startup, keep the same `-f` files and the corresponding UUID/Vulkan index for later commands; never combine the two AMD overrides. NVIDIA continues to use `compose.gpu.yml` (without a UUID, index 0 is selected).

Hardware acceptance is performed separately. Record the GPU/iGPU, OS, kernel, driver, runtime version, UUID/index, model digests, `setup`/`up` results, measurements for both roles, and all three analysis results with their sources. Check a repeated `up` and explicit CPU↔GPU switching while preserving projects and models. Tests without a GPU check logic and Compose configuration, not these measurements.

## Pin models and resume after interruption

Ollama models: BGE-M3 `bge-m3:567m` (1024 dimensions) and Qwen `qwen2.5:7b`. If the corresponding MODEL/DIGEST values are already set in `.env`, the CLI keeps them. It checks the local cache first; if a model is missing, it explicitly downloads it with `ollama pull` and Ollama's normal progress output. The full `sha256:` digest, provenance, metadata, and returned license are stored in state. A completed model is not downloaded again. If a tag's content changes while a pin exists, the CLI reports `MODEL_DRIFT`; it never silently rewrites the pin. Ollama verifies content-addressed blobs on load, and the backend also checks the model/digest before and after requests. The CLI accepts a full SHA256 from the API with or without the `sha256:` prefix, stores the pin with the prefix, and compares both forms equivalently. Short IDs from `ollama list` are not valid pins.

NPU model: the official `Qwen/Qwen2.5-7B-Instruct` (Apache-2.0), without `trust_remote_code`. The source revision is resolved once to an immutable commit **before** download and saved in `.npu/source-revision.txt`. The Hugging Face SDK reuses completed/partially downloaded files from its cache and shows progress. Safetensors files are checked against SHA256 values from the pinned commit's LFS metadata; SHA256 values for all allowed files are written to `.npu/source.json` and checked again when using a ready source snapshot. The license is copied to the export. A revision from a manual setup is preserved.

Export runs offline with explicit `--task text-generation-with-past`, symmetric INT4, ratio 1.0, and group-size -1. `TMPDIR` points to `.npu/export-tmp` on disk, not tmpfs `/tmp`. An unfinished export is stored in a separate `.partial` directory; on retry, only that incomplete output is removed and the source weights are preserved. A completed export with the CLI completion marker can recover an interrupted manifest creation without exporting again.

Ready `.npu/models/qwen2.5-7b-int4`, `profile.json`, and `profile.sha256` from a manual installation are used without downloading sources or preparing the exporter. Runtime checks all file hashes against the existing manifest. If an older runtime was started manually, stop it first: the CLI will not take over a foreign socket or stop a process without a confirmed managed PID.

If interrupted, rerun the same `setup --device ...`. Do not delete `.npu`, `.project-creator`, `.env`, or volumes. If an export predates the CLI but has no manifest or CLI completion marker, preserve it and create its manifest as described in `docs/intel-npu.md`; the CLI will not replace such weights automatically. For a damaged cache/manifest, first examine the specific error; the saved pin is not updated automatically after corruption. Automatic model updates to newer versions are not included in this first version.

## Verify the result

Success is reported only after a UI/API response and the complete path through the frontend proxy: three synthetic projects → two requirements each → indexing → analysis. The checks cover a known collision, compatible requirements, and missing conditions, including exact IDs/versions/source text, explanation, and expected verdict. The generator instruction requires explaining obligations and missing criteria first, then choosing a verdict; uncertain conditions are not evidence of compatibility. The instruction text is part of the Ollama/NPU generator profile: after it changes, a previous analysis becomes stale and must be run again. The NPU weight profile does not change.

The NPU adapter sorts a pair by text (string comparison), then sends the model the texts with short `left`/`right` references and fixed technical versions. After strict response validation it restores the original IDs/versions from the validated pair and checks the result again. UUIDs and version counters must not affect the model's semantic conclusion; unknown references and incorrect versions are rejected before restoration. Reversing the two requirements gives the model the same request; the result is returned with IDs/versions in the original order. Positional words in generated explanations may refer to the model's order; verify their meaning against both source texts. Diagnostic projects are named `Installation diagnostic: ... [marker]`, are created explicitly by this command, and remain in the application. A retry uses its own projects, does not change existing customer requirement text, and does not remove manual edits in a diagnostic project. If diagnostic requirements change, the check stops with `FIXTURE_CHANGED`.

Analysis runs asynchronously. The CLI and Compose smoke wait for the same attempt through `projectAnalysis` instead of creating a new one on each poll, and check IDs and source snapshots. The wait is limited to 45 minutes; if it expires, saved progress remains in the project. `setup`/`up` still require a completed analysis with expected results, not just a `running` state.

A healthy container and a detected `accel0` alone do not count as success. For Ollama, a separate embedding probe (1024 finite, nonzero values) runs, and the loaded digest and `size_vram` are checked after inference: CPU is 0; Arc/NVIDIA/AMD must be greater than 0. This confirms GPU offload but does not guarantee that all operations run on the GPU. For NPU, `device=NPU` and the exact native runtime profile are confirmed; the runtime selects NPU only, without AUTO. Invalid JSON, sources, verdict, or device cause a nonzero exit and save `verified=false`.

The installer does not fix model quality. If NPU returns valid JSON but gets one example wrong, `MODEL_QUALITY` remains an error; the application may still be available for manual diagnosis. This is not hidden by claiming installation success. The three examples check wiring and provide a short model acceptance check; they are not a statistical measure of RAG quality.

| Code | What to check |
| --- | --- |
| `PLATFORM`, `DOCKER`, `RESOURCES` | Supported OS, Engine/Compose, RAM/swap/disk. |
| `DEVICE_ACCESS`, `DEVICE_DRIVER`, `DEVICE_EXECUTION` | Driver, groups, actual offload, and selected device; fallback is not accepted. |
| `GPU_SELECTION`, `GPU_UNSUPPORTED` | Explicit adapter UUID/index and supported GFX target; check device inventory and the GPU matrix above. |
| `CONFIG_*`, `PROJECT_CHANGE`, `DEVICE_CHANGE` | Private `.env`, previous project name, explicit device switch. |
| `MODEL_DOWNLOAD`, `SOURCE_DOWNLOAD`, `PYTHON_*`, `TOOLS_DOWNLOAD` | Network and cache; retry the same setup command after resolving the cause. |
| `MODEL_DRIFT`, `MODEL_DIGEST`, `SOURCE_INTEGRITY`, `MODEL_MANIFEST` | Saved pin, weight integrity, and source snapshot. |
| `MODEL_EXPORT` | Python 3.12 exporter, disk-backed TMPDIR, free space, and partial output. |
| `NPU_SOCKET`, `NPU_RUNTIME`, `NPU_PROFILE`, `NPU_PID` | `logs/npu/runtime.log`, socket access, PID, and profile. |
| `UI`, `API`, `INDEX` | Compose logs for frontend/backend/Ollama, database readiness, and index. |
| `MODEL_RESPONSE`, `MODEL_SOURCES`, `MODEL_QUALITY`, `MODEL_EMBEDDING` | Real model response, synthetic sources, and quality; errors are not masked. |

Regular CLI tests are deterministic: APIs, downloads, and models are replaced with stubs. The uv installation path is tested in temporary environments, including one without `pip`, and with a separate local wheel smoke test that does not access the network. Tests do not install NPU dependencies or weights and do not prove laptop hardware compatibility. Run:

```bash
python3 -m unittest discover -s runtime/setup -v
python3 -m compileall -q runtime/setup project-creator
```
