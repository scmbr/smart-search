import gzip
import importlib.util
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("pilot", Path(__file__).with_name("prepare-embedding-pilot.py"))
pilot = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pilot)


class PreparationTests(unittest.TestCase):
    def test_bounded_deterministic_and_no_holdout(self):
        xml = '<yml_catalog><shop><categories><category id="g">Product</category></categories><offers><offer id="1" group_id="g" available="true"><url>https://store.softline.ru/v/1/</url><name>SKU</name><description>&lt;script&gt;hidden&lt;/script&gt;Useful описание</description><param name="Тип лицензии">Full</param></offer></offers></shop></yml_catalog>'.encode()
        tuning = {"dataset_role": "development_tuning", "cases": [{"id": "tune-01", "query": "Product", "split": "tuning", "target_scope": "family", "group_ids": ["g"], "brand_id": "v"}]}
        probes = {"dataset_role": "development_pilot_probes", "cases": []}
        with tempfile.TemporaryDirectory() as folder:
            for packed in (False, True):
                p = Path(folder)/"feed.yml.gz.xml"
                p.write_bytes(gzip.compress(xml) if packed else xml)
                r = pilot.prepare(p, tuning, probes, limit=4, scopes={"v"})
                self.assertEqual(r, pilot.prepare(p, tuning, probes, limit=4, scopes={"v"}))
                self.assertNotIn("hidden", r["Documents"][0]["Text"])
                self.assertIn("Useful", r["Documents"][0]["Text"])
                tuning["cases"][0]["split"] = "holdout"
                with self.assertRaises(ValueError):
                    pilot.prepare(p, tuning, probes, scopes={"v"})
                tuning["cases"][0]["split"] = "tuning"
        self.assertLessEqual(len(pilot.bounded("я"*3000).encode()), 2048)
