"""Reviewed tuning diagnostics only: no holdout, no OpenAI, no eligibility claims."""
import argparse
import json
from pathlib import Path
import subprocess
import time
from urllib.parse import urlsplit


def diagnose(case, result):
    hits = result.get("Hits") or []
    matches = []
    for rank, hit in enumerate(hits[:4], 1):
        document = hit["Document"]
        if case["target_scope"] == "brand":
            parsed = urlsplit(document["URL"])
            found = parsed.hostname == "store.softline.ru" and parsed.path.startswith("/"+case["brand_id"]+"/")
        else:
            found = document["GroupID"] in case["group_ids"]
        if found:
            matches.append(rank)
    return {"draft_group_target_hit_at_4": bool(matches) if case["target_scope"] == "family" else None,
            "draft_brand_scope_hit_at_4": bool(matches) if case["target_scope"] == "brand" else None,
            "negative_empty": not hits if case["target_scope"] == "empty" else None,
            "matching_ranks": matches}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--exe", default="bin/search.exe")
    parser.add_argument("--output", default="artifacts/tuning-diagnostics.json")
    args = parser.parse_args()
    source = json.loads(Path("testdata/evaluation/scenarios.json").read_text(encoding="utf-8"))
    if source["dataset_role"] != "development_tuning" or any(c["split"] != "tuning" for c in source["cases"]):
        raise SystemExit("Refusing to run a non-tuning dataset")
    executable = str(Path(args.exe).resolve())
    rows = []
    for case in source["cases"]:
        started = time.perf_counter()
        row = {"id":case["id"], "query":case["query"], "target_scope":case["target_scope"]}
        try:
            run = subprocess.run([executable, "-query", case["query"]], capture_output=True, text=True, encoding="utf-8", timeout=40)
            if run.returncode:
                raise RuntimeError(run.stderr.strip() or f"exit {run.returncode}")
            result = json.loads(run.stdout)
            row.update(diagnose(case, result))
            row.update({"catalog_id":result["CatalogID"], "index_version":result["IndexVersion"],
                        "query_policy_version":result.get("QueryPlan",{}).get("Version","legacy_exact"),
                        "search_mode":result.get("SearchMode","legacy_exact"),
                        "elapsed_ms":round((time.perf_counter()-started)*1000), "result":result,
                        "ungraded_constraints":case["constraints"]})
        except (OSError, ValueError, KeyError, RuntimeError, subprocess.TimeoutExpired) as error:
            row["error"] = str(error)
        rows.append(row)
    report = {"dataset_role":source["dataset_role"], "label_status":source["label_status"], "dataset_cases_sha256":source["cases_sha256"],
              "cases":len(rows), "execution_errors":sum("error" in r for r in rows),
              "family_scope_cases":sum(c["target_scope"] == "family" for c in source["cases"]),
              "draft_group_target_hits_at_4":sum(bool(r.get("draft_group_target_hit_at_4")) for r in rows),
              "brand_scope_cases":sum(c["target_scope"] == "brand" for c in source["cases"]),
              "draft_brand_scope_hits_at_4":sum(bool(r.get("draft_brand_scope_hit_at_4")) for r in rows),
              "negative_cases":sum(c["target_scope"] == "empty" for c in source["cases"]),
              "negative_empty_cases":sum(bool(r.get("negative_empty")) for r in rows),
              "note":"Synthetic user-reviewed tuning diagnostics, not independent relevance/Family Precision or SKU eligibility. Only raw query is passed; constraints are recorded but not graded. Timings include CLI startup.",
              "results":rows}
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report,ensure_ascii=False,indent=2)+"\n", encoding="utf-8")
    print(json.dumps({k:v for k,v in report.items() if k != "results"},ensure_ascii=True))
    return 1 if report["execution_errors"] else 0


if __name__ == "__main__":
    raise SystemExit(main())
