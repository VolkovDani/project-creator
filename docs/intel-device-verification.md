# Intel Core Ultra 7 268V device verification

[Русская версия](intel-device-verification.ru.md) · [NPU setup](intel-npu.en.md)

## Same-weight generation comparison, 9 November 2026

At application commit `52a05514ca4964a82fd9b8a6439f52426690bafb`, OpenVINO 2026.4.1 and GenAI 2026.4.1.0 ran the same Qwen2.5-7B-Instruct symmetric INT4 export on CPU, Intel Arc GPU and Intel NPU. Source revision `a09a35458c702b33eeacc393d103063234e8bc28`, manifest SHA256 `f36b13cbad8752f05d141eba6f817045986a615d13ffb5688fe417faeebca271`. The three synthetic cases from TestOpenVINORealNPUSmoke used 923, 899 and 893 prompt tokens, with a 512-token output limit. Each device ran the same cases twice in one process and once in a new process with the compiled cache.

| Measurement | CPU | Arc GPU | NPU |
| --- | ---: | ---: | ---: |
| Pipeline creation, empty compile cache (s) | 1.93 | 6.19 | 3.86 |
| Pipeline creation, reused compile cache (s) | 1.97 | 1.66 | 2.81 |
| First collision comparison, empty cache (s) | 39.23 | 11.36 | 9.23 |
| Median of three second-pass comparisons (s) | 9.54 | 5.62 | 14.61 |
| Maximum process peak RSS (GiB) | 8.28 | 1.23 | 6.43 |
| Expected verdicts over nine comparisons | 6/9 | 7/9 | 9/9 |

CPU and GPU errors concerned the insufficient-information case. The NPU column uses the separately confirmed run after worker recovery. Process RSS excludes some driver/device allocations. Pipeline creation excludes Python import and model-integrity checks. The OS page cache was retained. Timing and verdicts apply to these inputs, model and runtime.

## Standalone BGE-M3 embeddings, 9 November 2026

BGE-M3 FP16 at revision `5617a9f61b028005a4858fdac845db406aefb181` produced normalized 1024-dimensional dense vectors on both static NPU shapes. Tokenization, CLS pooling and L2 normalization ran on CPU. OpenVINO reported NPU execution and the hardware busy counter advanced. Dynamic shape compilation failed. The 128-token window explicitly rejected the 195-token input on both passes. The 512-token window accepted all 19 inputs twice.

| Measurement | 128 tokens | 512 tokens |
| --- | ---: | ---: |
| Valid vectors | 36 | 38 |
| Median encoder inference (ms) | 14.05 | 68.27 |
| Minimum cosine against PyTorch FP32 | 0.99994995 | 0.99994995 |
| Minimum cosine against Ollama F16 | 0.99999395 | 0.99997855 |
| Expected source ranked first | 6/6 | 6/6 |

The runtime configuration still selects Ollama for application embeddings. These fixed examples establish feasibility with static windows. Switching embedding providers requires profile tracking, long-input handling and deliberate reindexing.

Local diagnostic records for these measurements are in `logs/npu/benchmark22-20261109-130800/` and `logs/npu/bge-m3-20261109/`.

## Application adapters, 18 November 2026

At `1b5b390e68d742de3c7f434e30cd10af27ac5814` we called the Go adapters with the same three synthetic pairs in order, twice per device. Ollama CPU and Arc used separate local 0.40.16 containers, the same read-only Qwen2.5:7b GGUF Q4_K_M weights (digest `845dbda0ea48ed749caafd9e6037047aa19acfcfd82e704d7ca97d631a0b697e`) and 32768 context. The existing OpenVINO NPU worker used the INT4 manifest above. Its busy counter advanced by 42,455,285 μs. The Ollama API reported zero model bytes on GPU in the CPU run and all 6,428,924,312 bytes on GPU in the Arc run.

| Go adapter call (s) | CPU | Arc | NPU |
| --- | ---: | ---: | ---: |
| Collision, first / repeat | 60.41 / 32.98 | 22.34 / 17.91 | 6.52 / 6.04 |
| Compatible, first / repeat | 33.97 / 35.07 | 24.46 / 27.12 | 8.61 / 8.42 |
| Insufficient, first / repeat | 30.82 / 30.98 | 24.24 / 27.10 | 7.97 / 7.82 |

Each adapter returned all six expected verdicts and source references. Ollama started with fresh processes. The NPU worker was already warm. We timed complete adapter calls including verification and allowed retries. Weights, prompts and runtime configuration differ across adapters. The table cannot isolate a hardware speedup. Logs are in `logs/npu/issue22-20261118/`.
