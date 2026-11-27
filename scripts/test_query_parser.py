"""Offline bounds and data-minimization checks for the development runner."""
import contextlib
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("parser_experiment", Path(__file__).with_name("evaluate-query-parser.py"))
experiment = importlib.util.module_from_spec(spec)
spec.loader.exec_module(experiment)


class ParserExperimentTests(unittest.TestCase):
    def run_plan(self, extra):
        calls = []

        def cli(args, **kwargs):
            calls.append(args)
            self.assertEqual(args[1:3], ["-mode", "plan"])
            self.assertEqual(args[3], "-query")
            self.assertEqual(len(args), 5)  # No targets, families or catalog texts.
            report = {"Plan": {"Key": "a"*64, "CostUpperNano": 800_000}}
            return subprocess.CompletedProcess(args, 0, json.dumps(report), "")

        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)/"plan.json"
            with patch("sys.argv", ["evaluate-query-parser", "--output", str(output)]+extra), patch.object(experiment.subprocess, "run", side_effect=cli), contextlib.redirect_stdout(io.StringIO()):
                experiment.main()
            return json.loads(output.read_text(encoding="utf-8")), calls

    def test_plan_never_calls_live_or_sends_targets(self):
        report, calls = self.run_plan([])
        self.assertEqual(len(calls), 16)
        self.assertEqual(report["cost_upper_nano"], 12_800_000)

    def test_live_refuses_unapproved_fingerprint_before_payment(self):
        with self.assertRaisesRegex(SystemExit, "exact planned"):
            self.run_plan(["--mode", "live", "--approved-sha", "wrong"])

    def test_complete_plan_must_fit_run_cap(self):
        with self.assertRaisesRegex(SystemExit, "does not fit"):
            self.run_plan(["--max-cost-nano", "1"])


if __name__ == "__main__":
    unittest.main()
