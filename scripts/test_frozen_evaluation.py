"""Offline guards for independent evaluation; never reads holdout queries."""
import importlib.util
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("frozen", Path(__file__).with_name("evaluate-frozen.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class FrozenEvaluationTests(unittest.TestCase):
    def test_complete_coverage_rejects_fallback_and_invalid_parser(self):
        for trace in ({"VectorStatus": "fallback"},
                      {"VectorStatus": "catalog_mismatch"},
                      {"VectorStatus": "used", "InterpretationStatus": "fallback"},
                      {"VectorStatus": "used", "InterpretationStatus": "invalid"}):
            with self.subTest(trace=trace), self.assertRaises(ValueError):
                module.validate_coverage([{"Hybrid": trace}])
        for status in ("used", "no_candidates", "skipped_exact", "skipped_interpretation"):
            module.validate_coverage([{"Hybrid": {"VectorStatus": status}}])
        module.validate_coverage([{"SearchMode": "not_searched"}])

    def test_changed_source_is_rejected(self):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / "source"
            path.write_text("original", encoding="utf-8")
            manifest = {"files": {str(path): module.digest(path)}}
            module.validate_sources(manifest)
            path.write_text("changed", encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "Frozen input changed"):
                module.validate_sources(manifest)

    def test_snapshot_change_and_incomplete_batch_are_rejected(self):
        manifest = {"catalog_id": "catalog", "index_name": "index", "query_policy": "policy"}
        result = {"CatalogID": "catalog", "IndexVersion": "index", "QueryPlan": {"Version": "policy"}}
        module.validate_results([result], [{}], manifest)
        for field in ("CatalogID", "IndexVersion", "QueryPlan"):
            with self.subTest(field=field), self.assertRaises(ValueError):
                module.validate_results([dict(result, **{field: "changed"})], [{}], manifest)
        with self.assertRaises(ValueError):
            module.validate_results([], [{}], manifest)

    def test_changed_query_is_not_a_valid_observation(self):
        result = {"Query": "damaged", "SearchMode": "not_searched"}
        with self.assertRaisesRegex(ValueError, "query"):
            module.validate_results([result], [{"query": "original"}], {})

    def test_brand_boundary_and_top_four_only(self):
        case = {"target_scope": "brand", "brand_id": "vendor", "constraints": ["ungraded"]}
        bad = {"Document": {"URL": "https://store.softline.ru/vendor-neighbor/a"}}
        good = {"Document": {"URL": "https://store.softline.ru/vendor/a"}}
        result = module.score(case, {"Hits": [bad]*4 + [good]})
        self.assertFalse(result["target_hit_at_4"])
        self.assertEqual(result["ungraded_constraints"], ["ungraded"])
        self.assertTrue(module.score(case, {"Hits": [good]})["target_hit_at_4"])


if __name__ == "__main__":
    unittest.main()
