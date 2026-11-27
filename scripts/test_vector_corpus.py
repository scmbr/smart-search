import gzip
import importlib.util
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("corpus", Path(__file__).with_name("prepare-vector-corpus.py"))
corpus = importlib.util.module_from_spec(spec)
spec.loader.exec_module(corpus)


class VectorCorpusTests(unittest.TestCase):
    def test_conflicting_and_colliding_group_identity_is_rejected(self):
        for offers in (
            '<offer id="1" group_id="g"><group_id>other</group_id><url>https://store.softline.ru/v/1/</url><name>Product</name></offer>',
            '<offer id="1"><url>https://store.softline.ru/v/1/</url><name>SKU</name></offer><offer id="2" group_id="offer-1"><url>https://store.softline.ru/v/2/</url><name>Product</name></offer>',
        ):
            with tempfile.TemporaryDirectory() as folder:
                path = Path(folder)/"feed.xml"
                path.write_text('<yml_catalog><shop><offers>'+offers+'</offers></shop></yml_catalog>', encoding="utf-8")
                with self.assertRaises(ValueError):
                    corpus.prepare(path)

    def test_full_manifest_has_sku_fallback_and_no_labels(self):
        xml = '<yml_catalog><shop><categories><category id="g">Product</category></categories><offers><offer id="2"><url>https://store.softline.ru/v/2/</url><name>SKU</name></offer><offer id="1" group_id="g"><url>https://store.softline.ru/v/1/</url><name>Product</name></offer></offers></shop></yml_catalog>'.encode()
        with tempfile.TemporaryDirectory() as folder:
            for packed in (False, True):
                path = Path(folder)/"feed.xml"
                path.write_bytes(gzip.compress(xml) if packed else xml)
                x = corpus.prepare(path)
                self.assertEqual(x["SHA"], corpus.fingerprint(x))
                self.assertEqual(x["Offers"], 2)
                self.assertEqual([d["ID"] for d in x["Documents"]], ["v/group/g", "v/offer/2"])
                self.assertEqual(x["Documents"][1]["GroupID"], "")
                self.assertEqual(x["Documents"][1]["OfferID"], "2")
                self.assertNotIn("Queries", x)
                self.assertNotIn("TuningSHA", x)
                original = x["SHA"]
                x["Documents"][0]["Text"] += " & changed"
                self.assertNotEqual(original, corpus.fingerprint(x))
