# Classification evaluator

[Русская версия](classification-evaluator.ru.md)

The synthetic Russian examples exercise both taxonomy axes, short and mixed records, and abstention. The reference labels are fixture expectations. Run the metric checks from the repository root with Python 3.10 or later and the classification contract from #49.

```sh
python3 -B scripts/classification-evaluator_test.py
python3 -B scripts/classification-evaluator.py --contract contracts/classification.json --examples testdata/classification/synthetic.json --self-check
```

For independent runs, replace `--self-check` with `--predictions FILE`. Supply a JSON object mapping each scheme name (for example `flat` or `hierarchical`) to an array of objects containing a unique example `id`, `semantic` and `representation`. Use null for an abstention. Every scheme must cover exactly the reference IDs. The evaluator reports support, precision, recall, F1, confusion, coverage and overall accuracy separately for each axis. Undefined ratios appear as null. Abstentions count in the denominator of recall and accuracy. An explicit `unknown` label participates as a class.

The self-check copies reference labels and simulates complete abstention. It exercises the evaluator without invoking a model. Choosing deployment thresholds requires independent reviewed labels, model runs and recorded checkpoint and prompting versions. Store private examples and predictions locally outside version control.

## Frozen single-record experiment

Install no additional Python packages. Run the deterministic loopback integration checks with `python3 -B scripts/classification_benchmark_test.py`. The benchmark is separate from the application and its historical pair classifier.

The public candidate file is `testdata/classification/public-ru-proposed.json`. A human reviewer examines the original excerpts and the two axes before any inference. Keep the reviewed file separately, for example at `logs/issue50/reviewed.json`. Preserve the source links, attribution, license notices, texts, IDs and preassigned source-disjoint splits. Set the top-level `status` to `reviewed`, retain `language: ru` and the contract's `taxonomy` version, and add `reviewer` and `reviewed_at` (ISO date). Replace each row’s `proposed` object with the reviewer’s `semantic` and `representation` fields directly on that row. Exclude unresolved reference cases or explicitly label an ambiguous axis `unknown` when that matches the rubric. Keep both splits. Reassigning sources after seeing predictions invalidates the holdout.

With the reviewed file available, use a trusted loopback endpoint serving the installed `laya:322m-multilingual-bf16` GGUF under Ollama 0.40.16. The runner checks the audited manifest digest, renderer, model head size and local tag. It checks identity immediately before and after every inference request. It sends one record or excerpt plus one question at a time. Example empirical tuning targets below illustrate the CLI. Choose both values before inspecting predictions.

```sh
mkdir -p logs/issue50
python3 -B scripts/classification-evaluator.py --run-local \
  --examples logs/issue50/reviewed.json --contract contracts/classification.json \
  --port 11434 --digest c1242e4a83f0e1cb58f0f191a65bbc5d3306fae274d152b8229ec574999d465d \
  --full-state-bytes 65536 --marker-state-bytes 384 \
  --target-precision 0.90 --min-accepted 5 > logs/issue50/run.json
```

When Ollama is available only inside Compose, a Python interpreter sharing the Ollama container's network namespace can reach its loopback interface. Mount the repository read-only and direct stdout to the host report. Record the hardware and runtime configuration alongside the report. The output includes UTC start time, model and code identity, hashes of the exact examples and taxonomy, and the wire questions, mappings and raw scores for each executed step.

| Profile | Question and input |
| --- | --- |
| `flat` | One full-text choice among all labels on each axis. |
| `flat_reverse`, `flat_reword` | Change only the option order or only the Russian criteria of the flat profile. |
| `hierarchical` | Choose a broad group on each axis, then choose among its members. A single-member group resolves directly. |
| `story_probe` | Ask separately whether the full record is a user story. Compare against the reviewed representation mapped to a binary label. |
| `flat_marker` | Same questions as `flat`, on the first case-insensitive marker and the text that follows it, within the configured excerpt window. The fixed expression in the runner includes «Дано», «Когда», «Если», user-story and verification markers. A missing marker yields `no_marker`. |

### Input coverage and scoring

The default full-text ceiling is the runtime’s 64 KiB transport cap. Request framing also consumes part of that cap. Full text exceeding the chosen byte limit is left unclassified. The marker window is measured independently in UTF-8 bytes of the JSON string, including quotes and escaping. For each submitted span the report records half-open Unicode character offsets, original and submitted byte counts, and actual input/output token usage per successful step. IDs join back to the frozen text. Compare `view_comparison` counts and changes on the same eligible IDs across both views. Keep records without a marker in overall coverage.

The audited GGUF template uses a 256-token head and allows at most 48 text tokens per option. The pinned renderer can shorten the question and options when crowded. The runner uses single-letter wire keys and short Russian descriptions. Before sending each question, it checks a conservative UTF-8 byte upper bound for the rendered options, their markers and the question. The pinned GGUF renderer preserves the state suffix. The multilingual checkpoint was trained with a 1,024-token total limit. Responses above this limit retain raw evidence under `steps` and are excluded from quality/threshold metrics. Inspect `over_budget`, `reserved_token`, `runtime_rejected` (HTTP 400/413) and `outside_training_context` separately. Other runtime errors stop the run. [Pinned GGUF implementation](https://github.com/ggml-org/llama.cpp/blob/b12177/tools/server/server-decision.cpp#L1269-L1321) · [Ollama request limit](https://github.com/ollama/ollama/blob/v0.40.16/server/routes.go#L948-L1027) · [Checkpoint configuration](https://github.com/NandhaKishorM/laya/blob/main/laya-rust/README.md#checkpoints)

Each flat or binary decision uses the selected option’s score. The hierarchical decision uses the lowest selected score along its path. These scores and the runtime’s concentration measure are model outputs. For each scheme and axis, sweep the observed tuning scores and choose the greatest coverage meeting the specified accepted accuracy and minimum count. Apply that threshold unchanged to the holdout. If no threshold qualifies, abstain on all examples. `raw_metrics` score available predictions before applying a cutoff. `metrics` include abstentions; `accepted_accuracy` describes accepted predictions only. Inspect `accepted_errors`, `rejected_errors`, `below_threshold` and `unavailable` by ID and compare the class supports. A weak or unavailable decision calls for human review. No threshold from this exploratory sample is automatically adopted by the application.
