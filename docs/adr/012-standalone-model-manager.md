# ADR-012. Optional same-host model manager

[Русская версия](012-standalone-model-manager.ru.md)

## Decision

Project Creator starts with PostgreSQL, the API and the UI. Applications on the same host may share [Models Configurator](https://github.com/VolkovDani/models-configurator/tree/develop) over its versioned HTTP API. Its sources, containers and credentials have a separate lifecycle. The base installation requires neither a submodule nor a live provider.

The manager owns inference transport, model artifact pins, provider credentials on managed routes, weights and runtime diagnostics. Project Creator owns source access, prompts, validation of model output, classification decisions, history and indexes. The existing OpenAI-compatible client serves both managed and direct calls. Each role is explicit. A failed local profile never selects a hosted one.

## Connect a Compose installation

1. Start the manager first, configure at least one model and read its full immutable ID from `GET http://127.0.0.1:8317/v1/models`. The manager Compose project creates the shared clients network. Only the Project Creator backend joins it.
2. For each desired role, place its settings in the private Project Creator `.env`. Omit the entire group for a disabled role. The generation model must accept strict JSON Schema. The embedding profile must return 1024 components.

```dotenv
PROJECT_CREATOR_MODELS_NETWORK=models-configurator-clients
PROJECT_CREATOR_GENERATOR_RUNTIME=openai-compatible
PROJECT_CREATOR_GENERATOR_URL=http://models-configurator:8317/v1
PROJECT_CREATOR_GENERATOR_MODEL=<chat-profile-id>
PROJECT_CREATOR_EMBEDDING_RUNTIME=openai-compatible
PROJECT_CREATOR_EMBEDDING_URL=http://models-configurator:8317/v1
PROJECT_CREATOR_EMBEDDING_MODEL=<embedding-profile-id>
```

3. Run `./project-creator setup --device remote` (add `--switch-device` for an existing local installation). Manual Compose users can include `-f compose.yml -f compose.models-configurator.yml`. An application running directly on the host uses the loopback URL instead of the Compose service name. A direct external deployment needs only its own URL, model ID and optional key for each role. [Installation guide](../setup-cli.en.md)

The profile ID includes the provider and a fingerprint covering the upstream, model, role, local digest or external revision. Select a new ID and restart the backend after changing a profile. Old IDs are rejected by the manager. Project Creator incorporates the ID into its existing result and index profiles. The external operator supplies the upstream revision; an unannounced change behind that revision cannot be detected reliably. Credentials stay in the manager on managed routes. Only trusted same-host applications may use the shared client network.

## Compatibility and verification

Existing CPU, GPU and NPU deployments can continue to use their saved installer state, Compose project name and volumes. The pilot creates a separate Ollama volume for new managed installations. Switching providers leaves the old data available. The standalone OpenVINO NPU route accepts the manager's immutable chat ID. Project Creator canonicalizes the pair, maps source references and validates findings on both NPU routes. The managed route has its own provenance and cache identity. Classifier migration remains separate. Hardware acceptance remains specific to the measured device.

The cross-repository test uses two synthetic upstream roles and the real manager binary. Set `TEST_DATABASE_URL` to a disposable PostgreSQL database with pgvector, then run:

```bash
# From the manager checkout:
go build -o /tmp/models-configurator-test ./cmd/models-configurator
# From the Project Creator checkout:
MODELS_CONFIGURATOR_TEST_BIN=/tmp/models-configurator-test go test -count=1 ./internal/api -run 'Test(ModelManagerWorkerIntegration|CompatibleProvidersWithoutLocalRuntime)$'
```

The test checks persisted summaries, indexing and search, collision analysis and workspace isolation through the API. Ordinary Go tests use deterministic providers and can run without the manager checkout.

To run the same three hardware fixtures against the managed client, set `MANAGED_NPU_REAL_SMOKE=1`, `MANAGED_NPU_URL=http://127.0.0.1:8317/v1` and `MANAGED_NPU_MODEL` to its full NPU ID, then run `go test -v -count=1 ./internal/analysis -run '^TestManagedOpenVINORealNPUSmoke$'`.
