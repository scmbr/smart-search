"""Build compact names from feed only; no labels or API calls."""
import argparse
import hashlib
import json
from pathlib import Path
from urllib.parse import urlparse
import xml.etree.ElementTree as ET


def prepare(path):
    digest = hashlib.sha256()
    with path.open('rb') as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b''):
            digest.update(chunk)
    categories, products, brands = {}, {}, set()
    for _, node in ET.iterparse(path, events=('end',)):
        if node.tag == 'category':
            categories[node.get('id')] = (node.text or '').strip()
            node.clear()
        elif node.tag == 'offer':
            u = urlparse(node.findtext('url') or '')
            parts = u.path.strip('/').split('/')
            if u.scheme == 'https' and u.netloc == 'store.softline.ru' and len(parts) >= 2:
                scope = '/' + parts[0] + '/'
                vendor = (node.findtext('vendor') or '').strip()
                if vendor:
                    brands.add((vendor, scope))
                group = node.get('group_id') or node.findtext('group_id')
                name = categories.get(group)
                if name:
                    products.setdefault((name, scope), set()).add(group)
            node.clear()
    entries = [{'Entity': {'Kind': 'brand', 'Name': name, 'Scope': scope}, 'Groups': []}
               for name, scope in sorted(brands)]
    entries += [{'Entity': {'Kind': 'product', 'Name': name, 'Scope': scope}, 'Groups': sorted(groups)}
                for (name, scope), groups in sorted(products.items())]
    return {'Version': 'name-catalog-v1', 'SourceSHA': digest.hexdigest(), 'Entries': entries}


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--feed', required=True, type=Path)
    p.add_argument('--output', type=Path, default=Path('artifacts/name-catalog.json'))
    a = p.parse_args()
    result = prepare(a.feed)
    a.output.parent.mkdir(parents=True, exist_ok=True)
    a.output.write_text(json.dumps(result, ensure_ascii=False) + '\n', encoding='utf-8')
    print(json.dumps({'entries': len(result['Entries']), 'source_sha': result['SourceSHA'], 'bytes': a.output.stat().st_size}))


if __name__ == '__main__':
    main()
