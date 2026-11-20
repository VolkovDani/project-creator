"""Score two-axis classifications offline or explicitly run a pinned local experiment."""
import argparse
from collections import Counter
from datetime import datetime, timezone
from hashlib import sha256
import json
from pathlib import Path
import subprocess

import classification_benchmark as bench


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


def threshold_report(data, labels, target_precision, min_accepted):
    """Choose the greatest coverage meeting a predeclared empirical tuning target."""
    splits = {name: [r for r in data if r[4] == name] for name in ("tuning", "holdout")}
    tally = {}
    for _, truth, label, score, _ in splits["tuning"]:
        if label is not None:
            totals = tally.setdefault(score, [0, 0])
            totals[0] += 1
            totals[1] += truth == label
    threshold = None
    accepted_count = correct = 0
    curve = []
    for cutoff in sorted(tally, reverse=True):
        count, hits = tally[cutoff]
        accepted_count += count
        correct += hits
        precision = correct / accepted_count if accepted_count else None
        curve.append({"cutoff": cutoff, "accepted": accepted_count, "correct": correct,
                      "accepted_accuracy": precision})
        if accepted_count >= min_accepted and precision >= target_precision:
            threshold = cutoff
    result = {"threshold": threshold, "tuning_curve": curve}
    for name, rows in splits.items():
        truth = [r[1] for r in rows]
        raw = [r[2] for r in rows]
        predictions, errors, rejected, missing, low_errors = [], [], [], [], []
        for key, ref, guess, score, _ in rows:
            accepted = score is not None and threshold is not None and score >= threshold
            predictions.append(guess if accepted else None)
            if guess is None:
                missing.append(key)
            elif not accepted:
                rejected.append(key)
                if ref != guess:
                    low_errors.append(key)
            elif ref != guess:
                errors.append(key)
        count = sum(label is not None for label in predictions)
        result[name] = {"raw_metrics": score_axis(truth, raw, labels),
                        "metrics": score_axis(truth, predictions, labels),
                        "accepted_accuracy": (count - len(errors)) / count if count else None,
                        "accepted_errors": errors, "below_threshold": rejected,
                        "rejected_errors": low_errors, "unavailable": missing}
    return result


def score_experiments(experiment, corpus, contract, target_precision, min_accepted):
    references = by_id(corpus["examples"])
    results = {}
    for name, scheme in experiment["schemes"].items():
        axes = {bench.PROBE: ("user_story", "not_user_story")} if name == "story_probe" else {
            axis: contract[axis] for axis in bench.AXES}
        outputs = by_id(scheme["predictions"])
        if outputs.keys() != references.keys():
            raise ValueError("Experiment IDs differ from references")
        results[name] = {}
        for axis, labels in axes.items():
            data = []
            for key, row in references.items():
                if axis == bench.PROBE:
                    truth = "user_story" if row["representation"] == "user_story" else "not_user_story"
                else:
                    truth = row[axis]
                guess = outputs[key][axis]
                data.append((key, truth, guess["label"], guess["score"], row["split"]))
            results[name][axis] = threshold_report(data, labels, target_precision, min_accepted)
    return results


def compare_views(experiment, corpus, marker_bytes):
    full = by_id(experiment["schemes"]["flat"]["predictions"])
    marker = by_id(experiment["schemes"]["flat_marker"]["predictions"])
    report = {}
    for split in ("tuning", "holdout"):
        rows = [row for row in corpus["examples"] if row["split"] == split]
        report[split] = {}
        for axis in bench.AXES:
            values = [(row["id"], row[axis], full[row["id"]][axis]["label"],
                       marker[row["id"]][axis]["label"]) for row in rows]
            paired = [item for item in values if item[2] is not None and item[3] is not None]
            report[split][axis] = {
                "records": len(rows), "longer_than_marker_window": sum(
                    full[key]["input"]["original_state_bytes"] > marker_bytes for key, *_ in values),
                "full_available": sum(original is not None for _, _, original, _ in values),
                "marker_available": sum(fragment is not None for _, _, _, fragment in values),
                "paired_ids": [key for key, *_ in paired],
                "full_correct_on_paired": sum(truth == original for _, truth, original, _ in paired),
                "marker_correct_on_paired": sum(truth == fragment for _, truth, _, fragment in paired),
                "changed_ids": [key for key, _, original, fragment in paired if original != fragment],
            }
    return report


def local_report(args, corpus, contract, corpus_bytes, contract_bytes):
    bench.validate_corpus(corpus, contract)
    if (not args.digest or args.target_precision is None or not 0 < args.target_precision <= 1
            or args.min_accepted is None or args.min_accepted < 1):
        raise ValueError("Local runs require --digest, --target-precision in (0,1], and positive --min-accepted")
    if not (4 <= args.marker_state_bytes <= bench.RUNTIME_TEXT_LIMIT
            and 4 <= args.full_state_bytes <= bench.RUNTIME_TEXT_LIMIT):
        raise ValueError("Input policy byte limits must be in [4,65536]")
    runtime = bench.LocalLaya(args.port, args.digest)
    version = runtime.request("/api/version")["version"]
    if version != bench.OLLAMA_VERSION:
        raise ValueError(f"Expected Ollama {bench.OLLAMA_VERSION} for the verified SystemOne input policy")
    bench.check_pin(runtime.request("/api/tags"), runtime.digest)
    show = runtime.request("/api/show", {"model": bench.MODEL})
    details = show.get("details", {})
    info = show.get("model_info", {})
    if (details.get("format") != "gguf" or details.get("runner") != "llamacpp"
            or info.get("modern-bert.decision.type") != "laya"
            or info.get("modern-bert.decision.max_head_tokens") != bench.HEAD_LIMIT
            or "decision" not in show.get("capabilities", [])):
        raise ValueError("Expected the audited llama.cpp Laya GGUF decision renderer")
    started = datetime.now(timezone.utc).isoformat()
    experiment = bench.run_experiments(corpus, contract, runtime.ask, args.full_state_bytes, args.marker_state_bytes)
    here = Path(__file__).resolve()
    commit = None
    try:
        git = subprocess.run(["git", "-C", str(here.parent), "rev-parse", "HEAD"],
                             capture_output=True, text=True)
        if git.returncode == 0:
            commit = git.stdout.strip()
    except FileNotFoundError:
        pass  # Minimal Python containers can reproduce the run using the script hashes.

    return {"mode": "local_single_record", "taxonomy": contract["version"],
            "provenance": {"model": bench.MODEL, "digest": "sha256:" + runtime.digest,
                           "ollama_version": version, "endpoint": f"http://127.0.0.1:{args.port}/v1/systemone",
                           "started_utc": started, "git_commit": commit,
                           "scripts_sha256": {p.name: sha256(p.read_bytes()).hexdigest()
                                              for p in (here, Path(bench.__file__))},
                           "examples_sha256": sha256(corpus_bytes).hexdigest(),
                           "contract_sha256": sha256(contract_bytes).hexdigest(),
                           "reviewer": corpus["reviewer"], "reviewed_at": corpus["reviewed_at"],
                           "full_state_bytes": args.full_state_bytes, "marker_state_bytes": args.marker_state_bytes,
                           "training_context_tokens": bench.TRAINING_CONTEXT, "head_tokens": bench.HEAD_LIMIT,
                           "runner": details["runner"], "gguf_context": info.get("modern-bert.context_length"),
                           "option_tokens": bench.OPTION_LIMIT, "format": "gguf", "keep_alive": "5m",
                           "min_accepted": args.min_accepted, "target_precision": args.target_precision},
            **experiment, "view_comparison": compare_views(experiment, corpus, args.marker_state_bytes),
            "results": score_experiments(experiment, corpus, contract,
                                                       args.target_precision, args.min_accepted)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--examples", type=Path, required=True)
    parser.add_argument("--contract", type=Path, required=True)
    modes = parser.add_mutually_exclusive_group(required=True)
    modes.add_argument("--predictions", type=Path)
    modes.add_argument("--self-check", action="store_true")
    modes.add_argument("--run-local", action="store_true", help="Explicitly run the pinned single-record Laya experiment")
    parser.add_argument("--digest")
    parser.add_argument("--port", type=int, default=11434)
    parser.add_argument("--full-state-bytes", type=int, default=bench.RUNTIME_TEXT_LIMIT)
    parser.add_argument("--marker-state-bytes", type=int, default=384)
    parser.add_argument("--target-precision", type=float)
    parser.add_argument("--min-accepted", type=int)
    args = parser.parse_args()
    corpus_bytes = args.examples.read_bytes()
    contract_bytes = args.contract.read_bytes()
    examples = json.loads(corpus_bytes)
    contract = json.loads(contract_bytes)
    if args.run_local:
        try:
            report = local_report(args, examples, contract, corpus_bytes, contract_bytes)
        except (OSError, ValueError, KeyError) as exc:
            parser.exit(1, f"Experiment failed: {exc}\n")
        print(json.dumps(report, ensure_ascii=False, indent=2))
        return
    if isinstance(examples, dict):
        bench.validate_corpus(examples, contract)
        examples = examples["examples"]
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
