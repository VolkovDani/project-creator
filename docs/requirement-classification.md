# Requirement classification

[Русская версия](requirement-classification.ru.md)

[Reviewing classifications](classification-review.md)

Rule version 2 adds `test_case` to the representation axis. Meaning and form are independent. Mixed or ambiguous records can have an unknown label on one axis and a known label on the other. Keep the full original text.

| Code | Definition | Example | Counterexample |
| --- | --- | --- | --- |
| `business_requirement` | Business objective | Reduce waiting time by half | Show the queue |
| `stakeholder_requirement` | Stakeholder need | The dispatcher needs to see urgent orders | Save a timestamp |
| `functional_requirement` | System behavior | Save the selected interval | Finish migration by May |
| `non_functional_requirement` | Quality attribute | Respond within one second | Send a receipt |
| `transition_requirement` | Temporary rollout need | Import existing orders before launch | Keep an order history permanently |
| `business_rule` | Domain policy | Only members qualify for discounts | Render the amount in bold |
| `constraint` | Imposed implementation restriction | Use the existing local database | Reduce waiting time |
| `statement` | Declarative assertion or directive | Save the selected interval | Given … when … then … |
| `user_story` | Role, need and purpose | As a clerk I want a queue to select the next order | The system exports a file |
| `use_case` | Actor goal and flow with possible alternatives | Submit order, validate, accept or return for correction | The order is pending |
| `scenario` | Concrete sequence in context | A dispatcher closes an empty queue, reopens it and sees zero orders | Respond within one second |
| `acceptance_criterion` | Verifiable condition for acceptance | Reopening an empty queue must show zero orders | Improve usability |
| `test_case` | Repeatable check with preconditions or test data, steps and an expected result | Start with no orders in queue A. Close and reopen queue A. Check that its order count is 0. | Reopening an empty queue must show zero orders |
| `other` | Recognizable content outside the categories | Meeting note (semantic axis) | Unclear fragment |
| `unknown` | Insufficient evidence for one label | Faster (unspecified object) | A clearly specified export action |

The three empty-queue examples describe the same system behavior (`functional_requirement`). The scenario recounts what the dispatcher does and sees. The acceptance criterion sets the condition for accepting the behavior. The test case tells a tester how to reproduce and check it.

New manual confirmations record rules version 2. Earlier decisions retain their original labels and rules version in history.

Existing records start as `unclassified`. The `confirmRequirementClassification` operation requires the source version and current decision revision. Editing the source makes its previous decision `stale`; history retains the original status. Filters return the latest revision’s types together with its status. The API and UI share the `classifying` state. Alternative scores belong to the model. Automatic classification depends on benchmark #50.

## Local model evidence and background work

With the default configuration, users classify and confirm records manually. Set `PROJECT_CREATOR_RECORD_CLASSIFIER=systemone-review` to process new, edited and existing records with a local System One model.

A new local model is compatible when Ollama 0.40.16 reports the `decision` capability and a positive `num_ctx` in its model parameters. To select it, supply only its exact tag and full SHA-256 manifest digest from the local `/api/tags` response. For the known Laya and D3 manifests, the adapter also verifies the GGUF format, llama.cpp runner and model-specific metadata. The example below selects `d3:4b`, whose model parameters specify `num_ctx 4096`.

```dotenv
PROJECT_CREATOR_RECORD_CLASSIFIER=systemone-review
PROJECT_CREATOR_RECORD_CLASSIFIER_MODEL=d3:4b
PROJECT_CREATOR_RECORD_CLASSIFIER_DIGEST=2c571f91caf64c47b6ca6bd3adc1daa07dbb84ead8e9489758498cc7e11e533d
```

Existing installations can keep `PROJECT_CREATOR_RECORD_CLASSIFIER=laya-review` as shorthand for the pinned multilingual BF16 Laya model. None of the evaluated Laya schemes qualified for automatic labels. [Laya benchmark results](classification-evaluator.md#measured-result-on-20-november-2026)

The adapter sends the original text in full and asks a separate question for each axis. Reviewers can inspect the model’s uncalibrated scores for the alternatives. Both the semantic and representation types stay `unknown` with status `needs_review` until a person confirms them.

Set `PROJECT_CREATOR_RECORD_CLASSIFIER_RUNTIME=managed` to use a decision profile from Models Configurator. The application still records the pinned model digest and applies the same taxonomy and review rules. It schedules fresh suggestions for active versions without a confirmed classification when the effective profile changes. [Managed classifier setup](setup-cli.en.md#record-classification-through-models-configurator)

The direct Ollama adapter uses `PROJECT_CREATOR_RECORD_CLASSIFIER_URL`, then the configured local Ollama URL, then loopback on port 11434. It accepts loopback HTTP and the Compose service name `ollama`. Restart the backend after changing configuration. Unsupported input or token counts are recorded separately. Runtime and pin failures have a failed/retry path. No source text is sent to another provider on error.

PostgreSQL records the source version, effective classifier profile and each job's lease. A worker processes one record at a time independently of pair analysis and search. Repeated requests and delayed responses are checked against the current source and lease. A confirmed choice remains effective across explicit reanalysis of the same version. Model evidence from that reanalysis is available in history. Editing the text makes its earlier decision stale. Use project-scoped progress and failed-only retry to inspect the backlog.
