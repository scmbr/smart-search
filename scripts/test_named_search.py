import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("named_search", Path(__file__).with_name("diagnose-named-search.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class RelevanceTest(unittest.TestCase):
    def test_product_identity_and_vendor_both_required(self):
        case = {"vendor_path": "/adobe/", "product_name": "Adobe Photoshop"}
        def hit(name, url):
            return {"name": name, "selected_offer": {"url": url}}
        self.assertTrue(module.relevant(case, hit("Adobe Photoshop", "https://store.softline.ru/adobe/product/")))
        self.assertFalse(module.relevant(case, hit("ФотоШОУ PRO", "https://store.softline.ru/ams-software/product/")))
        self.assertFalse(module.relevant(case, hit("Photoshop Plug-ins", "https://store.softline.ru/adobe/product/")))
        self.assertFalse(module.relevant(case, hit("Adobe Photoshop", "https://other.example/adobe/product/")))

    def test_brand_does_not_require_a_specific_product(self):
        self.assertTrue(module.relevant({"vendor_path": "/adobe/"}, {"name": "Adobe Illustrator", "selected_offer": {"url": "https://store.softline.ru/adobe/item/"}}))
