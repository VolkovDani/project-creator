# Using an AI assistant to install and verify local AI

[Русская версия](ai-installation.ru.md)

Use this guide when an AI assistant is helping a person install the existing
local-model runtime. The assistant should first read the README and setup guide
from the user's selected checkout, then determine the OS and architecture before
suggesting commands. Do not assume that a different checkout or remembered
instructions match the files on the user's machine. The repository supports
native Linux x86_64 with Python 3 for the CLI and Docker Engine with Compose
v2.20+. Windows, macOS, ARM, and Docker Desktop are outside the current CLI
support matrix. NPU additionally requires Python 3.12 and uv 0.12.13 or later.
This guide covers the existing CLI, not an MCP client or server; MCP remains a
proposal (see [MCP integration](mcp-integration.md)).

## 1. Discover the machine and choose a profile

Check the available device and prerequisites before setup:

```bash
uname -s
uname -m
python3 --version
docker info
docker compose version
```

Choose `cpu` for the broadest local setup. Select `arc`, `nvidia`, `amd`, or
`amd-vulkan` only after installing and verifying the corresponding host driver
and container support. The CLI does not install system drivers. For NPU, follow
the prerequisites in the [CLI setup guide](setup-cli.en.md#system-prerequisites)
and [Intel NPU guide](intel-npu.md) first. The device guide documents current
resource budgets and hardware caveats; a detected device alone does not prove
that inference will run on it.

## 2. Preserve an existing installation

If this repository already has an installation, run setup from the same
repository directory, retaining its `.env` and exact Compose project name. If
the old installation used `docker compose -p`, supply that name on the first
CLI run. A different name creates a separate Docker volume namespace.

The CLI preserves existing secrets, settings, database volumes, and model
weights. Do not remove `.env`, `.project-creator/`, `.npu/`, or Docker volumes
when resuming or diagnosing an installation. The CLI does not run
`down --volumes` or migrate data between project names. Read the CLI guide's
[existing installation and data preservation](setup-cli.en.md#existing-installations-and-data-preservation)
section before switching profiles.

## 3. Set up and run

From the repository root, explicitly choose the local inference device. For
example:

```bash
./project-creator setup --device cpu
```

Other profiles are `arc`, `nvidia`, `amd`, `amd-vulkan`, and `npu`; supported
NPU + Arc combinations and adapter selection are described in the CLI guide.
`setup` explicitly downloads local weights and containers, starts the chosen
runtime, and checks the application plus a real AI workflow. Review the model
and container downloads before running it. A later start can use:

```bash
./project-creator up
./project-creator doctor
```

`doctor` checks environment, UI/API availability, and model pins; it does not
run inference. `up` runs the AI scenarios again. Stop the stack while preserving
data with:

```bash
./project-creator down
```

## 4. Verify inference, not just containers

A successful container start or health check is not evidence that the selected
model completed inference. Confirm that `setup` or `up` completed its synthetic
embedding and generation scenarios. For a GPU profile, check the reported model
and positive VRAM offload measurement; this confirms offload, not that every
operation ran on the GPU. Hardware and model limitations remain profile-specific.

Use diagnostics when setup fails:

```bash
./project-creator doctor
docker compose ps
docker compose logs --tail=100 backend frontend ollama
```

Keep reports in the ignored root `logs/` directory; NPU runtime and installation
reports use `logs/npu/`. For example:

```bash
mkdir -p logs
docker compose logs --no-color > logs/compose.log
```

Do not publish `.env`, logs, or installation reports: logs can contain local
paths, hardware details, or operational context. Redact sensitive information
before sharing diagnostics. Do not paste secrets into an assistant or issue.

## Permission and cost boundaries

`setup` is an explicit action that downloads model weights, containers, and
dependencies for the selected profile. Review the downloads and available disk
space first. Do not use destructive cleanup such as `docker compose down
--volumes`, deleting project data, or changing a production installation unless
the data owner has approved the specific operation and a backup exists. System
driver changes, privileged access, and reboots are separate operations.

This documented setup uses local models. Any future cloud model integration
must be configured explicitly and requires consent before project context is
sent. Local inference failures must not silently cause cloud processing. Paid
cloud services or subscriptions require separate user approval; this guide does
not enable them.

For exact prerequisites, device selection, recovery, and error codes, see the
[complete CLI guide](setup-cli.en.md).
