"""Small offline checks for the evidence extractor; production search is not used."""
import gzip
import hashlib
import importlib.util
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("family_audit", Path(__file__).with_name("audit-family-mapping.py"))
audit_module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit_module)


class FamilyAuditTests(unittest.TestCase):
    def test_plain_and_gzip_use_actual_format_and_preserve_evidence(self):
        config = {"Version": "test", "Rules": [{"ID": "family", "Groups": ["g"], "VendorPath": "/v/", "SourceURL": "https://example.invalid/family"}]}
        xml = b'<yml_catalog><shop><categories><category id="g">Product</category></categories><offers><offer id="a" group_id="g" available="true"><name>License</name><url>https://store.softline.ru/v/a/</url><price>10.00</price><param name="other">ignored</param></offer></offers></shop></yml_catalog>'
        with tempfile.TemporaryDirectory() as folder:
            for compressed in (False, True):
                with self.subTest(compressed=compressed):
                    path = Path(folder) / "feed.yml.gz.xml"
                    content = gzip.compress(xml) if compressed else xml
                    path.write_bytes(content)
                    result = audit_module.audit(path, config)
                    self.assertEqual(result["source_sha256"], hashlib.sha256(content).hexdigest())
                    self.assertEqual(result["catalog_offers"], 1)
                    row = result["groups"]["g"]
                    self.assertEqual((row["offers"], row["priced"], row["available"]), (1, 1, 1))
                    self.assertEqual(row["group_name"], "Product")
                    self.assertEqual(row["examples"][0]["offer_id"], "a")
                    config["Rules"][0]["VendorPath"] = "/wrong/"
                    with self.assertRaises(ValueError):
                        audit_module.audit(path, config)
                    config["Rules"][0]["VendorPath"] = "/v/"
            config["Rules"][0]["Groups"] = ["missing"]
            with self.assertRaises(ValueError):
                audit_module.audit(path, config)


if __name__ == "__main__":
    unittest.main()
