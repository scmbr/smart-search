"""Frozen holdout comparison. Cache-only: never enables paid API calls."""
import argparse
from collections import Counter
import hashlib
import json
from pathlib import Path
import subprocess
import time
from urllib.parse import urlsplit


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def validate_sources(manifest):
    for path, expected in manifest["files"].items():
        if digest(path) != expected:
            raise ValueError(f"Frozen input changed: {path}")


def score(case, result):
    hits = result.get("Hits") or []
    ranks = []
    for rank, hit in enumerate(hits[:4], 1):
        doc = hit["Document"]
        if case["target_scope"] == "family":
            match = doc["GroupID"] in case["group_ids"]
        elif case["target_scope"] == "brand":
            url = urlsplit(doc["URL"])
            match = url.hostname == "store.softline.ru" and url.path.startswith("/" + case["brand_id"] + "/")
        else:
            match = False
        if match:
            ranks.append(rank)
    return {"target_hit_at_4": bool(ranks), "matching_ranks": ranks,
            "returned": len(hits), "ungraded_constraints": case["constraints"]}


def validate_results(results, cases, manifest):
    if len(results) != len(cases):
        raise ValueError("Incomplete batch")
    for case, result in zip(cases, results):
        if "query" in case and result.get("Query") != case["query"]:
            raise ValueError("Returned query differs from requested query")
        # Unsupported queries legitimately do not search and have no index identity.
        if result.get("SearchMode") == "not_searched":
            continue
        if result.get("CatalogID") != manifest["catalog_id"] or result.get("IndexVersion") != manifest["index_name"]:
            raise ValueError("Search snapshot differs from frozen configuration")
        plan = result.get("QueryPlan")
        if not isinstance(plan, dict) or plan.get("Version") != manifest["query_policy"]:
            raise ValueError("Query policy differs from frozen configuration")


def validate_coverage(results):
    for result in results:
        if result.get("SearchMode") == "not_searched":
            continue
        trace = result.get("Hybrid") or {}
        if trace.get("VectorStatus") not in {"used", "no_candidates", "skipped_exact", "skipped_interpretation"}:
            raise ValueError("Incomplete vector coverage")
        if trace.get("InterpretationStatus") in {"fallback", "invalid"}:
            raise ValueError("Incomplete parser coverage")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", default="testdata/evaluation/search-freeze-v1.json")
    parser.add_argument("--exe", default="bin/search.exe")
    parser.add_argument("--corpus", default="artifacts/vector-corpus.json")
    parser.add_argument("--output", required=True)
    parser.add_argument("--require-complete", action="store_true", help="reject vector/parser fallback; still cache-only")
    args = parser.parse_args()
    output = Path(args.output)
    if output.exists():
        raise SystemExit("Refusing to overwrite a previous observation")
    manifest = json.loads(Path(args.manifest).read_text(encoding="utf-8"))
    validate_sources(manifest)
    source = json.loads(Path("testdata/evaluation/holdout-v2.json").read_text(encoding="utf-8"))
    if source["dataset_role"] != "frozen_holdout_user_reviewed":
        raise ValueError("Not an accepted frozen holdout")
    if digest(args.corpus) != manifest["corpus_file_sha256"]:
        raise ValueError("Wrong corpus file")
    executable = str(Path(args.exe).resolve())
    # ASCII JSON escapes preserve Unicode across Windows pipe/codepage boundaries.
    payload = "".join(json.dumps(c["query"], ensure_ascii=True) + "\n" for c in source["cases"])
    # No target groups, family IDs or constraints are supplied to the search process.
    report = {"manifest_sha256": digest(args.manifest), "binary_sha256": digest(executable),
              "mode": "independent_cache_only_comparison", "require_complete": args.require_complete, "runs": {},
              "limitations": "Raw group/brand hit@4 only; constraints and all-four precision ungraded. Cache misses are fallback, not full hybrid quality. CLI batch time is not request p95."}
    output.parent.mkdir(parents=True, exist_ok=True)
    # Reserve the path before execution; interrupted/failed runs remain visible.
    output.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    try:
        for mode, flags in (("baseline", []), ("hybrid_cache_only", ["-vector-corpus", args.corpus, "-parser-mode", "replay"])):
            started = time.perf_counter()
            run = subprocess.run([executable, "-batch"] + flags, input=payload,
                                 capture_output=True, text=True, encoding="utf-8", timeout=600)
            if run.returncode:
                raise RuntimeError(f"{mode}: {run.stderr.strip()}")
            results = [json.loads(line) for line in run.stdout.splitlines()]
            validate_results(results, source["cases"], manifest)
            rows = [dict(id=c["id"], scope=c["target_scope"], result=r, **score(c, r))
                    for c, r in zip(source["cases"], results)]
            report["runs"][mode] = {"batch_elapsed_ms": round((time.perf_counter()-started)*1000),
                "vector_statuses": dict(Counter((r.get("Hybrid") or {}).get("VectorStatus", "off") for r in results)),
                "rows": rows}
            output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
            if args.require_complete and mode == "hybrid_cache_only":
                validate_coverage(results)
    except Exception as error:
        report["error"] = str(error)
        raise
    finally:
        output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({m: {"cases": len(v["rows"]), "target_hits": sum(r["target_hit_at_4"] for r in v["rows"]),
                          "vector_statuses": v["vector_statuses"]} for m, v in report["runs"].items()}))


if __name__ == "__main__":
    main()
