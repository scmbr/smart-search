"""Check the 100-case catalog facts against the feed without executing search."""
import argparse
import importlib.util
import json
from pathlib import Path

spec = importlib.util.spec_from_file_location("family_audit", Path(__file__).with_name("audit-family-mapping.py"))
family_audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(family_audit)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--feed", required=True, type=Path)
    parser.add_argument("--output", type=Path, default=Path("artifacts/evaluation-catalog-audit.json"))
    args = parser.parse_args()
    datasets = [json.loads((Path("testdata/evaluation") / f).read_text(encoding="utf-8")) for f in ("scenarios.json", "holdout-v2.json")]
    rules = []
    for data in datasets:
        for ident, family in data["families"].items():
            groups = family["group_ids"]
            rules.append({"ID":ident, "Groups":groups, "VendorPath":"/"+family["brand_id"]+"/", "SourceURL":data["catalog_evidence"][groups[0]]["example_url"]})
    report = family_audit.audit(args.feed, {"Version":"evaluation-v3", "Rules":rules})
    for data in datasets:
        if data["source_sha256"] != report["source_sha256"]:
            raise SystemExit("FAIL: source feed differs from the frozen dataset snapshot")
        for group, expected in data["catalog_evidence"].items():
            actual = report["groups"][group]
            example = actual["examples"][0]
            if (expected["group_name"], expected["offers"], expected["example_offer_id"], expected["example_url"]) != (actual["group_name"], actual["offers"], example["offer_id"], example["url"]):
                raise SystemExit(f"FAIL: catalog facts differ for group {group}")
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2)+"\n", encoding="utf-8")
    print(f'PASS: catalog facts for {len(report["groups"])} groups verified; search not run, relevance not reviewed')


if __name__ == "__main__":
    main()
