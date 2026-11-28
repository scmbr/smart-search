import importlib.util
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('name_catalog', Path(__file__).with_name('prepare-name-catalog.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class NameCatalogTest(unittest.TestCase):
    def test_names_are_derived_only_from_own_groups_and_trusted_vendors(self):
        xml = '''<root><categories><category id="g">Synthetic Editor</category><category id="unused">Unused</category></categories><offers>
        <offer group_id="g"><vendor>Synthetic</vendor><url>https://store.softline.ru/synthetic/p/</url><description>Other Product</description></offer>
        <offer group_id="g"><vendor>Foreign</vendor><url>https://evil.example/synthetic/p/</url></offer></offers></root>'''
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'feed.xml'
            path.write_text(xml, encoding='utf-8')
            result = module.prepare(path)
        self.assertEqual(len(result['Entries']), 2)
        self.assertEqual(result['Entries'][1]['Entity']['Name'], 'Synthetic Editor')
        self.assertEqual(result['Entries'][1]['Groups'], ['g'])
