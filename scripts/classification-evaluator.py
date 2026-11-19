"""Score labeled two-axis classification outputs without sending source texts anywhere."""
import argparse
from collections import Counter
import json
from pathlib import Path


def score_axis(expected, predicted, labels):
    if not expected or len(expected) != len(predicted):
        raise ValueError("Expected a prediction or explicit abstention for every example")
    confusion = {label: Counter() for label in labels}
    for truth, guess in zip(expected, predicted, strict=True):
        if truth not in confusion or (guess is not None and guess not in confusion):
            raise ValueError("Unknown classification label")
        confusion[truth][guess if guess is not None else "abstained"] += 1
    classes = {}
    for label, row in confusion.items():
        hits = row[label]
        positive = sum(other[label] for other in confusion.values())
        support = sum(row.values())
        classes[label] = {
            "support": support,
            "precision": hits / positive if positive else None,
            "recall": hits / support if support else None,
            "f1": 2 * hits / (support + positive) if support + positive else None,
        }
    return {
        "coverage": sum(p is not None for p in predicted) / len(expected),
        "accuracy": sum(a == b for a, b in zip(expected, predicted)) / len(expected),
        "classes": classes,
        "confusion": confusion,
    }


def by_id(rows):
    index = {row["id"]: row for row in rows}
    if len(index) != len(rows) or not index:
        raise ValueError("Example IDs must be unique and the dataset nonempty")
    return index


def evaluate(examples, predictions, contract):
    expected = by_id(examples)
    results = {}
    for scheme, rows in predictions.items():
        outputs = by_id(rows)
        if expected.keys() != outputs.keys():
            raise ValueError("Prediction IDs differ from the labeled dataset")
        results[scheme] = {
            axis: score_axis([r[axis] for r in expected.values()],
                             [outputs[key][axis] for key in expected], contract[axis])
            for axis in ("semantic", "representation")
        }
    if not results:
        raise ValueError("No schemes supplied")
    return results


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--examples", type=Path, required=True)
    parser.add_argument("--contract", type=Path, required=True)
    modes = parser.add_mutually_exclusive_group(required=True)
    modes.add_argument("--predictions", type=Path)
    modes.add_argument("--self-check", action="store_true")
    args = parser.parse_args()
    examples = json.loads(args.examples.read_text())
    contract = json.loads(args.contract.read_text())
    if args.self_check:
        predictions = {
            "copy_reference": examples,
            "abstain": [{"id": r["id"], "semantic": None, "representation": None} for r in examples],
        }
    else:
        predictions = json.loads(args.predictions.read_text())
    print(json.dumps({"mode": "self_check" if args.self_check else "score_predictions",
                      "taxonomy": contract["version"],
                      "examples": len(examples),
                      "results": evaluate(examples, predictions, contract)}, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
