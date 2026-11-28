"""Development HTTP regression probe. Run only against a cache-only local API."""
import argparse
import json
from pathlib import Path
import urllib.request
from urllib.parse import urlparse
import uuid


def relevant(case, match):
    offer = match.get("selected_offer", {})
    source = urlparse(offer.get("url", ""))
    if source.hostname != "store.softline.ru" or not source.path.startswith(case["vendor_path"]):
        return False
    expected = case.get("product_name")
    return expected is None or match.get("name", "").casefold() == expected.casefold()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", default="artifacts/named-search-baseline.json")
    args = parser.parse_args()
    dataset = json.loads(Path("testdata/evaluation/named-search-development.json").read_text(encoding="utf-8"))
    rows = []
    for case in dataset["cases"]:
        request = urllib.request.Request(
            "http://127.0.0.1:8080/api/v1/search",
            data=json.dumps({"query": case["query"], "session_id": "named-diagnostic", "limit": 4}).encode(),
            headers={"Content-Type": "application/json", "Idempotency-Key": str(uuid.uuid4())},
        )
        with urllib.request.urlopen(request, timeout=20) as response:
            result = json.load(response)
        hits = result.get("matches", [])[:4]
        rows.append({"query": case["query"], "top1_relevant": bool(hits) and relevant(case, hits[0]),
                     "relevant_at_4": sum(relevant(case, hit) for hit in hits), "response": result})
    report = {"role": "development_regression_not_independent", "cases": rows,
              "top1_hits": sum(row["top1_relevant"] for row in rows), "count": len(rows)}
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"top1: {report['top1_hits']}/{len(rows)}; {output}")


if __name__ == "__main__":
    main()
