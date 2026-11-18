import importlib.util
from pathlib import Path
import unittest

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

    def test_bad_predictions_rejected(self):
        with self.assertRaises(ValueError):
            evaluator.score_axis(["a"], ["made_up"], ["a"])
        with self.assertRaises(ValueError):
            evaluator.score_axis(["a"], [], ["a"])


if __name__ == "__main__":
    unittest.main()
