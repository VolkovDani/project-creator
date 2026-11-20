import importlib.util
from pathlib import Path
import unittest
import json
import subprocess
import sys

SPEC = importlib.util.spec_from_file_location("evaluator", Path(__file__).with_name("classification-evaluator.py"))
evaluator = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(evaluator)


class MetricsTest(unittest.TestCase):
    def test_wrong_class_and_abstention_remain_in_recall_denominator(self):
        truth = ["a", "a", "b", "b"]
        result = evaluator.score_axis(truth, ["a", None, "a", "b"], ["a", "b"])
        self.assertEqual(result["coverage"], .75)
        self.assertEqual(result["accuracy"], .5)
        self.assertEqual(result["classes"]["a"]["precision"], .5)
        self.assertEqual(result["classes"]["a"]["recall"], .5)
        self.assertEqual(result["confusion"]["a"]["abstained"], 1)
        self.assertAlmostEqual(result["classes"]["b"]["f1"], 2/3)

    def test_scheme_comparison_joins_by_id_and_rejects_missing_predictions(self):
        rows = [{"id": "a", "semantic": "feature", "representation": "story"},
                {"id": "b", "semantic": "rule", "representation": "statement"}]
        contract = {"semantic": ["feature", "rule"], "representation": ["story", "statement"]}
        result = evaluator.evaluate(rows, {"reverse": rows[::-1]}, contract)
        self.assertEqual(result["reverse"]["semantic"]["accuracy"], 1)
        with self.assertRaises(ValueError):
            evaluator.evaluate(rows, {"missing": rows[:1]}, contract)
        with self.assertRaises(ValueError):
            evaluator.evaluate(rows, {"duplicate": [rows[0], rows[0]]}, contract)

    def test_replay_frozen_public_run(self):
        root = Path(__file__).resolve().parents[1]
        output = subprocess.check_output([
            sys.executable, "-B", "scripts/classification-evaluator.py",
            "--contract", "contracts/classification.json",
            "--examples", "testdata/classification/public-ru-reviewed.json",
            "--predictions", "testdata/classification/laya-benchmark-predictions.json",
        ], cwd=root)
        report = json.loads(output)
        self.assertEqual(report["examples"], 28)
        self.assertAlmostEqual(report["results"]["flat"]["semantic"]["accuracy"], 6/28)

    def test_bad_predictions_rejected(self):
        with self.assertRaises(ValueError):
            evaluator.score_axis(["a"], ["made_up"], ["a"])
        with self.assertRaises(ValueError):
            evaluator.score_axis(["a"], [], ["a"])


if __name__ == "__main__":
    unittest.main()
