# Docker Compose and operations

[Project Creator](../README.en.md) · [Русская версия](compose.ru.md)

## Run the complete MVP with Docker Compose

Docker Engine with Compose v2 (the `docker compose` command), or Docker Desktop
with Linux containers, is required. Go and Node.js are not needed on the host to
run the application. Run commands from the repository root. The default services are PostgreSQL, backend and frontend. A device override adds the bundled Ollama runtime.

The configuration pins Go 1.26.10, Node.js 26.13.1 on Debian Bookworm,
PostgreSQL 16 with pgvector 0.8.7, and Ollama 0.40.16. Models are downloaded
separately:

| Purpose | Model | Weight download size |
| --- | --- | --- |
| Multilingual embeddings, 1024 dimensions | `bge-m3:567m` | about 1.2 GB |
| Requirement-pair analysis, Q4_K_M | `qwen2.5:7b` | about 4.7 GB |

The sizes are published on the official [BGE-M3](https://ollama.com/library/bge-m3)
and [Qwen2.5 7B](https://ollama.com/library/qwen2.5:7b) model pages. Model tags
may change; the application checks the full digest of the installed model. The
BGE-M3 page lists MIT, and Qwen2.5 7B lists Apache 2.0; other Qwen sizes may have
a different license.

For planning, allocate 24–32 GiB RAM for the CPU version of the whole stack and
at least 20 GiB of free disk space for images, weights, and builds, plus space
for the database and backups. For the optional GPU version, plan for 12–16 GiB
VRAM plus RAM for the application/database. This is an approximate budget, not a
measured minimum: the generator requests a 32768-token context, and memory use
depends on the KV cache, simultaneous model loading, driver, and runtime. Weight
file size is not the same as RAM/VRAM consumption. Check actual usage with
`docker stats` and `docker compose exec ollama ollama ps`.

### First run and model download

```bash
cp .env.example .env
chmod 600 .env
```

Edit `.env`: replace `POSTGRES_PASSWORD` and `PROJECT_CREATOR_TOKEN` with
different random values; the token must be at least 32 characters. For example,
generate each value with a separate call to `openssl rand -hex 24`. Use hex without
URL-special characters for the Compose password. Do not commit `.env`. The
`PROJECT_CREATOR_DATABASE_URL` and `POSTGRES_PORT` variables are needed to run the
API on the host; the full Compose setup supplies the internal database address
itself.

For project and requirement CRUD, run `docker compose up -d --build --wait` with the example model variables unset.

For bundled CPU inference, add `COMPOSE_FILE=compose.yml:compose.cpu.yml` to `.env` before running the following commands. Apply this setting to existing manually managed CPU installations on upgrade. To return from a manual GPU installation, export this same `COMPOSE_FILE` in the shell and run `docker compose up -d --force-recreate ollama` followed by `docker compose up -d --wait` with the original project name. The CLI restores the saved device automatically. GPU, NPU and smoke override files already enable their runtime.

```bash
docker compose up -d --wait postgres ollama
docker compose exec ollama ollama pull bge-m3:567m
docker compose exec ollama ollama pull qwen2.5:7b
docker compose --profile tools run --rm model-info
```

The last command prints the local `/api/tags` JSON. Find the entries with the
required `name` and copy their **full** `digest`, in the form `sha256:` followed
by 64 hexadecimal characters; the short ID from `ollama list` is not sufficient.
Add these values to `.env`:

```dotenv
PROJECT_CREATOR_EMBEDDING_MODEL=bge-m3:567m
PROJECT_CREATOR_EMBEDDING_DIGEST=sha256:full-digest-of-installed-embedding-model
PROJECT_CREATOR_GENERATOR_MODEL=qwen2.5:7b
PROJECT_CREATOR_GENERATOR_DIGEST=sha256:full-digest-of-installed-generator
```

```bash
docker compose up -d --build --wait
docker compose ps
```

Open [http://127.0.0.1:3000](http://127.0.0.1:3000). If the port is occupied,
set `APP_PORT`, for example `APP_PORT=3100`, in `.env` and use the new port. Only
`127.0.0.1:${APP_PORT:-3000}` is published to the host. PostgreSQL, backend, and
Ollama are available on the Compose network and do not publish ports. Weights are
stored in the `ollama_data` volume; projects, requirements, indexes, and analyses
are stored in `postgres_data`. The Compose project name is part of the actual
volume names, so keep it when updating.

Weights are not downloaded automatically at startup. You can run the full stack
without AI by leaving both `MODEL`/`DIGEST` pairs unset: CRUD works, while
indexing and analysis report that the provider is not configured. Setting only
one variable in a pair is insufficient; invalid configuration prevents the
backend from starting. The full Compose setup sets
`PROJECT_CREATOR_OLLAMA_URL=http://ollama:11434`. After changing a model/digest,
recreate the backend with `docker compose up -d --wait backend`. Changing the
embedding profile requires reindexing requirements for semantic search; full
pair analysis does not depend on embeddings. Changing the generator requires a
new analysis; existing vectors remain usable.

Health checks verify PostgreSQL, that the Ollama process is available, and a real
authenticated request through backend/Next.js. The backend applies migrations
before accepting requests. `healthy` does not confirm that weights are loaded or
that the LLM is high quality. For diagnostics, use
`docker compose logs --tail=100 backend frontend ollama` and inspect the request
result (the `errors` field should be absent):

```bash
curl -fsS http://127.0.0.1:3000/api/graphql \
  -H 'Content-Type: application/json' \
  --data '{"query":"{projects(limit:1,offset:0){id}}"}'
```

Store local logs and diagnostic reports in the root `logs/` directory. NPU
runtime and CLI setup reports go in `logs/npu/`. The directory is excluded from
Git and the Docker build context. Container logs are available through
`docker compose logs`; to save them to a file, use `mkdir -p logs` and
`docker compose logs --no-color > logs/compose.log`.

Manual acceptance: create a project with a goal, then add the requirements
“Payment is allowed by card only” and “Payment is allowed in cash only.” Explicitly
index both, search, and start an analysis. Check the links, versions, and source
snapshots; a person evaluates any possible collision. If data is insufficient or
an error occurs, check the model and index, then retry. After
`docker compose restart postgres backend`, wait for `healthy` and confirm that the
project, requirements, index, and result remain. Edit one requirement: the
previous analysis should become stale. Failure to find a collision is not proof
of consistency.

### Intel Arc / Core Ultra laptops

For integrated Intel Arc, including Core Ultra 7 268V, `compose.intel.yml` is
provided: Ollama with Vulkan and the Intel Mesa driver inside the container.
Ubuntu preparation, first run, checking actual GPU offload, and returning to CPU
are described in the [Intel Arc guide](intel-arc.md).

### Experimental Intel NPU

An explicitly enabled host runtime using OpenVINO GenAI and `compose.npu.yml` is
prepared for testing Core Ultra 7 268V. Generation runs only on the NPU; BGE-M3
remains in local Ollama. This option requires driver and IR-weight preparation.
After fixes to the CLI and link normalization, fixed hardware examples, including
insufficient data and source reordering, passed on 2026-10-30. Full acceptance
with CPU/GPU comparison remains a separate task. CLI `setup --device npu`, stop,
repeat `up`, and `doctor` were checked while preserving the database and prepared
weights. Commands, results, and the way back to Ollama are in the
[NPU guide](intel-npu.en.md).

The `.npu/` directory is created separately by NPU setup commands, excluded from
Git and Docker builds, and is not created automatically by ordinary
`docker compose up`. It is not needed for the CPU/Arc Ollama option; another NPU
user must prepare their own local environment, weights, and profile following the guide.
Use the [CLI guide](setup-cli.en.md) for automated setup.

### Optional NVIDIA GPU

Using the CLI: `./project-creator setup --device nvidia`. If there are multiple
NVIDIA GPUs, set `--gpu-id` to a UUID from `nvidia-smi -L` (or an index; the CLI
stores the UUID). The selection is retained for `up`, `down`, and `doctor`.

On Linux, first install a compatible NVIDIA driver and the
[NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html),
configure it for Docker, and check that containers can access the GPU. Then run:

```bash
docker compose -f compose.yml -f compose.gpu.yml up -d --wait
```

The override requests a GPU for Ollama; the other services do not need one. Use
the same pair of `-f` options for subsequent GPU configuration changes. Docker
Desktop on macOS does not provide the Metal GPU to this Linux container; use the
CPU option or a separately and deliberately configured local runtime. Official
options are described in the [Ollama Docker documentation](https://docs.ollama.com/docker).

### GPU and integrated graphics: startup matrix

The CLI supports native Linux x86_64, Python 3, and Docker Engine/Compose 2.20+;
the NPU profile additionally uses Python 3.12 and installed uv.
A profile indicates configuration support; compatibility with specific hardware
is confirmed by a separate real `setup`/`up` run.

| CLI mode | Device and runtime | Setup / status |
| --- | --- | --- |
| `cpu` | Ollama 0.40.16 on CPU | Baseline option; no GPU required. |
| `arc` | Intel Arc dGPU/iGPU, including Core Ultra 7 268V; Ollama 0.40.16/Vulkan | Intel Linux graphics, access to `/dev/dri`; [Arc guide](intel-arc.md). Intel ICD is isolated from AMD/NVIDIA. New selection logic checked with tests without a GPU. |
| `nvidia` | Supported NVIDIA GPU; Ollama 0.40.16/CUDA | NVIDIA driver and Container Toolkit, `nvidia-smi`; hardware check of the new CLI integration not yet completed. |
| `amd` (ROCm) | Only AMD dGPU/iGPU in the Ollama 0.40.16 matrix; `ollama/ollama:0.40.16-rocm` | ROCm 7, `rocminfo`, `/dev/kfd` and `/dev/dri`. Strict GFX target check; hardware acceptance not performed. |
| `amd-vulkan` (RADV) | AMD dGPU/iGPU with working Vulkan; Ollama 0.40.16 + Mesa RADV | `amdgpu`, available `/dev/dri/renderD*`, and Vulkan support in the driver. ROCm and `/dev/kfd` are not required. Broader, but **not universal**; hardware acceptance not performed. |
| `npu` | Intel NPU; OpenVINO GenAI, embeddings on CPU | Fixed hardware examples pass on Core Ultra 7 268V; results and limitations are in the [NPU guide](intel-npu.en.md). |

**Operation on every AMD GPU or Ryzen APU cannot be guaranteed.** For ROCm,
check the model, OS, and driver in the [AMD matrix](https://rocm.docs.amd.com/en/latest/compatibility/compatibility-matrix.html)
and [Ollama 0.40.16 matrix](https://github.com/ollama/ollama/blob/v0.40.16/docs/gpu.mdx).
Even if a newer ROCm version supports an iGPU, that does not automatically extend
the capabilities of the pinned Ollama container. ROCm example:
`./project-creator setup --device amd --gpu-id GPU-...`, where the UUID comes from
`rocminfo`. The CLI accepts targets `gfx908`, `gfx90a`, `gfx942`, `gfx950`,
`gfx1030`, `gfx1100`, `gfx1101`, `gfx1102`, `gfx1150`, `gfx1151`, `gfx1200`,
`gfx1201` from the [Ollama v0.40.16 matrix](https://github.com/ollama/ollama/blob/v0.40.16/docs/gpu.mdx).
This is the upstream matrix, not a list of video cards tested by this project.
Unknown targets are rejected; `HSA_OVERRIDE_GFX_VERSION` is not used.

For another AMD GPU/iGPU, you can **try Vulkan**:
`./project-creator setup --device amd-vulkan`. The Mesa RADV driver runs without
ROCm, and the Vulkan ICD is isolated from Intel/NVIDIA. With multiple AMD render
devices, set `--gpu-id 0` (or another **Vulkan index from the Ollama logs**, not
a UUID from `rocminfo` or a `renderD*` number). If Vulkan, the model, or available
memory is incompatible, setup fails; it does not automatically switch to ROCm or
CPU. Integrated graphics use shared system memory, so working Vulkan does not
guarantee both models will load.

To change the device or adapter, run `setup` again with `--switch-device`. Data
and pinned models are preserved. `setup`/`up` check GPU offload separately for
embeddings and the generator; zero offload is an error instead of a successful
CPU fallback. The displayed GPU memory bytes allow partial offload and do not
prove full execution on the GPU. `doctor` does not run inference and reports that
no measurement is available for an unloaded model. More about selection, manual
startup, and hardware checks is in the [CLI guide](setup-cli.en.md#gpus-and-adapter-selection).

### Stop, update, and back up

`docker compose stop` stops processes; `docker compose down` also removes
containers and the network. Both commands preserve named volumes. To resume, run
`docker compose up -d --wait`. Do not change the project name or Compose file path
without migrating the volumes, or you may get a separate empty database.

Before an update, stop writes and create a dump:

```bash
mkdir -p backups
chmod 700 backups
docker compose stop frontend backend
umask 077
docker compose exec -T postgres pg_dump -U project_creator -d project_creator -Fc > backups/project-creator.dump
docker compose up -d --wait
```

Store the dump outside the application machine and test restoring it in a separate
Compose project. It contains projects, requirements, vectors, source versions,
and analysis history, but not Ollama weights. Keep a protected copy of `.env` and
the pinned digests separately from git. To preserve the downloaded weights
specifically, stop Ollama and copy its directory:

```bash
docker compose stop ollama
docker compose cp ollama:/root/.ollama backups/ollama
docker compose up -d --wait
```

To restore weights, stop Ollama and copy the saved directory contents into its
`/root/.ollama`, then start the service and check the full digests again with
`model-info`. Alternatively, download the weights again and verify the digest; an
updated tag does not guarantee the same result.

Restoring the database replaces data in the selected Compose project. First back
up its current database; use compatible PostgreSQL/pgvector and application
versions. PostgreSQL must be running and the application stopped:

```bash
docker compose up -d --wait postgres
docker compose stop frontend backend
docker compose exec -T postgres dropdb -U project_creator --if-exists project_creator
docker compose exec -T postgres createdb -U project_creator project_creator
docker compose exec -T postgres pg_restore -U project_creator -d project_creator --exit-on-error < backups/project-creator.dump
docker compose up -d --wait
```

After restoring, check readiness, project list, requirements, and index status. A
password error after changing `.env` means the password in the existing
PostgreSQL volume is still the old one; change it using the PostgreSQL role rather
than deleting the volume. To update, first save a backup, review migrations and
version changes, get the repository changes, then run
`docker compose up -d --build --wait`. Migrations run at startup; downgrades are
not supported. A PostgreSQL major-version update requires `pg_upgrade` or
dump/restore, not changing the tag on an old volume. Update weights as a separate
decision: changing the embedding profile requires reindexing for search; changing
the generator requires new pair analyses.

To **permanently delete all local data and weights** intentionally, run
`docker compose down --volumes`. This removes both named volumes for the selected
project; backups outside the volumes remain. It is not needed for ordinary shutdown.
