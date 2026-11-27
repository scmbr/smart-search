"""Run the provided reference smoke against the baseline CLI; no OpenAI calls."""
import argparse
import json
from pathlib import Path
import subprocess
import time
from urllib.parse import urlparse


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--exe", default="bin/search.exe")
    parser.add_argument("--output", default="artifacts/reference-baseline.json")
    args = parser.parse_args()
    source = json.loads(Path("testdata/evaluation/source-queries.json").read_text(encoding="utf-8"))
    mapping = json.loads(Path("internal/infrastructure/families/rules.json").read_text(encoding="utf-8"))
    family_by_group = {group: rule["ID"] for rule in mapping["Rules"] for group in rule["Groups"]}
    executable = str(Path(args.exe).resolve())
    rows = []
    for case in source["cases"]:
        started = time.perf_counter()
        try:
            run = subprocess.run([executable, "-query", case["query"]], capture_output=True, text=True, encoding="utf-8", timeout=60)
            if run.returncode:
                raise RuntimeError(run.stderr.strip() or f"exit {run.returncode}")
            result = json.loads(run.stdout)
        except (OSError, ValueError, subprocess.TimeoutExpired, RuntimeError) as error:
            rows.append({"id": case["id"], "query": case["query"], "target_kind":case["target_kind"], "error": str(error)})
            continue
        expected = {family_by_group[g] for g in case.get("group_ids", []) if g in family_by_group}
        hits = result.get("Hits") or []
        matching_ranks = []
        for rank, hit in enumerate(hits[:4], start=1):
            document = hit["Document"]
            if expected:
                matches = document["FamilyID"] in expected and document["FamilyConfirmed"]
            else:
                matches = urlparse(document["URL"]).path.startswith(case["vendor_path"])
                if case.get("category_id"):
                    matches = matches and document["CategoryID"] == case["category_id"]
            if matches:
                matching_ranks.append(rank)
        rows.append({
            "id": case["id"], "query": case["query"], "target_kind":case["target_kind"],
            "target_hit_at_4": bool(matching_ranks),
            "family_hit_at_4":bool(matching_ranks) if expected else None,
            "first_matching_rank": min(matching_ranks) if matching_ranks else None,
            "cli_elapsed_ms": round((time.perf_counter() - started) * 1000),
            "catalog_id": result["CatalogID"], "index_version": result["IndexVersion"],
            "query_policy_version":result.get("QueryPlan",{}).get("Version","legacy_exact"),
            "search_mode":result.get("SearchMode","legacy_exact"),
            "four_families_confirmed": result["FourFamiliesConfirmed"], "hits": hits,
        })
    errors = sum("error" in row for row in rows)
    successes = sum(row.get("target_hit_at_4", False) for row in rows)
    family_rows=[row for row in rows if row.get("target_kind")=="product_group"]
    family_hits=sum(bool(row.get("family_hit_at_4")) for row in family_rows)
    report = {"dataset_role": source["dataset_role"], "cases": len(rows), "execution_errors": errors,
              "reference_target_hits_at_4": successes, "reference_target_hit_rate_at_4": None if errors else successes / len(rows),
              "family_cases":len(family_rows),"family_hits_at_4":family_hits,
              "broad_target_cases":len(rows)-len(family_rows),"broad_target_hits_at_4":successes-family_hits,
              "four_families_confirmed_cases":sum(bool(row.get("four_families_confirmed")) for row in rows),
              "note": "Reference smoke only. Vendor/category scopes are coarse targets, not verified family sets. CLI timings include process startup; SKU eligibility and independent relevance are not measured.",
              "results": rows}
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps({key: value for key, value in report.items() if key != "results"}, ensure_ascii=True))
    return 1 if errors else 0


if __name__ == "__main__":
    raise SystemExit(main())
