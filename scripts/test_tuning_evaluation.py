import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("tuning",Path(__file__).with_name("evaluate-tuning.py"))
tuning = importlib.util.module_from_spec(spec)
spec.loader.exec_module(tuning)


class TuningDiagnosticsTests(unittest.TestCase):
    def test_group_brand_and_empty_are_distinct_diagnostics(self):
        result = {"Hits":[{"Document":{"GroupID":"other","URL":"https://store.softline.ru/vendor/a/"}}]}
        case = {"target_scope":"family","group_ids":["g"],"brand_id":"vendor"}
        self.assertFalse(tuning.diagnose(case,result)["draft_group_target_hit_at_4"])
        case["target_scope"]="brand"
        self.assertTrue(tuning.diagnose(case,result)["draft_brand_scope_hit_at_4"])
        self.assertIsNone(tuning.diagnose(case,result)["draft_group_target_hit_at_4"])
        result["Hits"][0]["Document"]["URL"]="https://evil.invalid/vendor/a/"
        self.assertFalse(tuning.diagnose(case,result)["draft_brand_scope_hit_at_4"])
        case["target_scope"]="empty"
        self.assertFalse(tuning.diagnose(case,result)["negative_empty"])
        self.assertTrue(tuning.diagnose(case,{"Hits":[]})["negative_empty"])


if __name__ == "__main__":
    unittest.main()
