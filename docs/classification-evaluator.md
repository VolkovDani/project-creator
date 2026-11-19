# Classification evaluator

[Русская версия](classification-evaluator.ru.md)

The synthetic Russian examples exercise both taxonomy axes, short and mixed records, and abstention. The reference labels are fixture expectations. Run the metric checks from the repository root with Python 3.10 or later and the classification contract from #49.

```sh
python3 -B scripts/classification-evaluator_test.py
python3 -B scripts/classification-evaluator.py --contract contracts/classification.json --examples testdata/classification/synthetic.json --self-check
```

For independent runs, replace `--self-check` with `--predictions FILE`. Supply a JSON object mapping each scheme name (for example `flat` or `hierarchical`) to an array of objects containing a unique example `id`, `semantic` and `representation`. Use null for an abstention. Every scheme must cover exactly the reference IDs. The evaluator reports support, precision, recall, F1, confusion, coverage and overall accuracy separately for each axis. Undefined ratios appear as null. Abstentions count in the denominator of recall and accuracy. An explicit `unknown` label participates as a class.

The self-check copies reference labels and simulates complete abstention. It exercises the evaluator without invoking a model. Choosing deployment thresholds requires independent reviewed labels, model runs and recorded checkpoint and prompting versions. Store private examples and predictions locally outside version control.
