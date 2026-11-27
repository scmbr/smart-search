"""Plan/replay an explicitly bounded parser experiment on frozen development probes."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--exe", default="bin/query-parse.exe")
    parser.add_argument("--mode", choices=["plan", "replay", "live"], default="plan")
    parser.add_argument("--approved-sha", default="")
    parser.add_argument("--max-cost-nano", type=int, default=20_000_000)
    parser.add_argument("--output", default="artifacts/query-parser-plan.json")
    args = parser.parse_args()
    if not 0 < args.max_cost_nano <= 20_000_000:
        parser.error("run cap must be positive and at most $0.02")
    source = json.loads(Path("testdata/evaluation/embedding-probes-v1.json").read_text(encoding="utf-8"))
    digest = hashlib.sha256(json.dumps(source["cases"], ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
    if source["dataset_role"] != "development_pilot_probes" or digest != "af7a01d9bf904948afc338accfc70a6003a8931680b1b77c10f5b3daf538dc84":
        raise SystemExit("Frozen development probes changed; refusing")
    # No target/group/constraint field is passed to the executable or provider.
    executable = str(Path(args.exe).resolve())

    def call(query, mode):
        run = subprocess.run([executable, "-mode", mode, "-query", query],
                             capture_output=True, text=True, encoding="utf-8", timeout=55)
        try:
            report = json.loads(run.stdout)
        except ValueError:
            report = {"Error": "CLI returned no valid report"}
        return report, run.returncode

    plans = []
    for case in source["cases"]:
        result, code = call(case["query"], "plan")
        if code or "Plan" not in result:
            raise SystemExit("Cannot prepare every query before live")
        plans.append({"id": case["id"], "query": case["query"], "plan": result["Plan"]})
    fingerprint = hashlib.sha256(json.dumps(plans, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
    upper = sum(row["plan"]["CostUpperNano"] for row in plans)
    if upper > args.max_cost_nano:
        raise SystemExit("Complete run does not fit its cap")
    if args.mode == "live" and args.approved_sha != fingerprint:
        raise SystemExit("Live requires the exact planned --approved-sha")
    report = {"mode": args.mode, "dataset_role": source["dataset_role"], "sha256": fingerprint,
              "queries": len(plans), "cost_upper_nano": upper, "run_cap_nano": args.max_cost_nano,
              "note": "Development parser outputs only, not independent relevance; no holdout.", "results": plans}
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)

    def save():
        output.write_text(json.dumps(report, ensure_ascii=False, indent=2)+"\n", encoding="utf-8")

    save()
    if args.mode != "plan":
        used = 0
        for row in plans:
            if used + row["plan"]["CostUpperNano"] > args.max_cost_nano:
                raise SystemExit("Run cap exhausted before another request")
            result, code = call(row["query"], args.mode)
            row["execution"] = result
            row["exit_code"] = code
            # Includes uncertainty and known rejections; no automatic retry.
            if result.get("Execution", {}).get("Requests", 0):
                used += row["plan"]["CostUpperNano"]
            report["reserved_upper_nano"] = used
            save()
            if code and args.mode == "live":
                raise SystemExit("Stopped at first unsuccessful live query; inspect saved report")
    print(json.dumps({key: value for key, value in report.items() if key != "results"}, ensure_ascii=False))


if __name__ == "__main__":
    main()
