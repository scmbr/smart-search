"""Reproduce bounded family evidence from the supplied feed, without search or APIs."""
import argparse
from collections import Counter
from decimal import Decimal
import gzip
import hashlib
import json
from pathlib import Path
from urllib.parse import urlsplit
import xml.etree.ElementTree as ET


def audit(path, config):
    by_group = {}
    for rule in config["Rules"]:
        for group in rule["Groups"]:
            if group in by_group:
                raise ValueError("duplicate mapping group")
            by_group[group] = rule
    groups = {g: {"family_id": r["ID"], "vendor_path": r["VendorPath"],
                  "source_url": r["SourceURL"], "offers": 0, "scope_mismatches": 0,
                  "priced": 0, "available": 0, "license_parameters": Counter(),
                  "examples": []} for g, r in by_group.items()}
    with path.open("rb") as raw:
        digest = hashlib.file_digest(raw, "sha256").hexdigest()
        raw.seek(0)
        compressed = raw.read(2) == b"\x1f\x8b"
    opener = gzip.open if compressed else open
    categories = {}
    total = 0
    # Remove completed records from their parent so memory does not grow with offers.
    stack = []
    with opener(path, "rb") as stream:
        for event, element in ET.iterparse(stream, events=("start", "end")):
            if event == "start":
                stack.append(element)
                continue
            if element.tag == "category":
                categories[element.get("id")] = element.text or ""
            if element.tag == "offer":
                total += 1
                group = element.get("group_id")
                if group in groups:
                    row = groups[group]
                    row["offers"] += 1
                    url = element.findtext("url") or ""
                    parsed = urlsplit(url)
                    row["scope_mismatches"] += int(parsed.hostname != "store.softline.ru" or not parsed.path.startswith(row["vendor_path"]))
                    row["priced"] += int(Decimal(element.findtext("price") or "0") > 0)
                    row["available"] += int(element.get("available") == "true")
                    roles = [p.text or "" for p in element.findall("param") if p.get("name") == "Тип лицензии"]
                    row["license_parameters"].update(roles)
                    if len(row["examples"]) < 2:
                        row["examples"].append({"offer_id": element.get("id"), "name": element.findtext("name"), "url": url, "license_parameters": roles})
            if element.tag in {"offer", "category"}:
                if len(stack) > 1:
                    stack[-2].remove(element)
                element.clear()
            stack.pop()
    for group, row in groups.items():
        row["group_name"] = categories.get(group)
        row["license_parameters"] = dict(sorted(row["license_parameters"].items()))
        if not row["offers"] or row["scope_mismatches"] or not row["group_name"]:
            raise ValueError(f"mapping has missing group/offers or wrong vendor scope: {group}")
    return {"evidence_role": "catalog_structure_not_independent_relevance_review",
            "source_sha256": digest, "catalog_offers": total,
            "mapping_version": config["Version"], "groups": groups}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--feed", required=True, type=Path)
    parser.add_argument("--output", type=Path, default=Path("artifacts/family-evidence.json"))
    parser.add_argument("--check", type=Path, help="Compare with an already frozen evidence file")
    args = parser.parse_args()
    config = json.loads(Path("internal/infrastructure/families/rules.json").read_text(encoding="utf-8"))
    report = audit(args.feed, config)
    if args.check and report != json.loads(args.check.read_text(encoding="utf-8")):
        raise SystemExit("FAIL: catalog evidence differs; inspect mapping/feed before updating evidence")
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"groups": len(report["groups"]), "mapped_offers": sum(r["offers"] for r in report["groups"].values()), "catalog_offers": report["catalog_offers"], "source_sha256": report["source_sha256"]}))


if __name__ == "__main__":
    main()
