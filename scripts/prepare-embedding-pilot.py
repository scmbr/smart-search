"""Prepare a frozen development corpus from feed; no search, model, or holdout reads."""
import argparse
import gzip
import hashlib
from html.parser import HTMLParser
import json
from pathlib import Path
from urllib.parse import urlsplit
import xml.etree.ElementTree as ET

SCOPES = {"microsoft", "kaspersky", "astralinux", "kripto-pro", "anydesk", "cyberprotect", "npo-echelon", "webinar", "positive-technologies"}


class PlainText(HTMLParser):
    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.parts, self.hidden = [], 0

    def handle_starttag(self, tag, attrs):
        if tag in {"script", "style"}:
            self.hidden += 1

    def handle_endtag(self, tag):
        if tag in {"script", "style"}:
            self.hidden = max(0, self.hidden-1)

    def handle_data(self, data):
        if not self.hidden:
            self.parts.append(data)


def plain(text):
    parser = PlainText()
    parser.feed(text or "")
    return " ".join(" ".join(parser.parts).split())


def bounded(text):
    return text.encode("utf-8")[:2048].decode("utf-8", errors="ignore")


def prepare(path, tuning, probes, limit=512, scopes=SCOPES):
    if tuning["dataset_role"] != "development_tuning" or any(c["split"] != "tuning" for c in tuning["cases"]):
        raise ValueError("Only development tuning is permitted")
    if probes["dataset_role"] != "development_pilot_probes":
        raise ValueError("Only development probes are permitted")
    if "cases_sha256" in probes:
        digest = hashlib.sha256(json.dumps(probes["cases"], ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
        if digest != probes["cases_sha256"]:
            raise ValueError("Frozen probe judgments changed")
    with path.open("rb") as raw:
        sha = hashlib.file_digest(raw, "sha256").hexdigest()
        raw.seek(0)
        compressed = raw.read(2) == b"\x1f\x8b"
    groups, categories, stack, offer_tokens, count = {}, {}, [], 0, 0
    def text(e, group):
        category_id = e.findtext("categoryId") or ""
        ancestors, seen = [], set()
        while category_id in categories and category_id not in seen:
            seen.add(category_id)
            name, category_id = categories[category_id]
            ancestors.append(name)
        return bounded("\n".join([categories.get(group, ("", ""))[0], e.findtext("vendor") or "", " / ".join(reversed(ancestors)), e.findtext("name") or "", plain(e.findtext("description"))]).strip())
    with (gzip.open(path, "rb") if compressed else path.open("rb")) as stream:
        for event, e in ET.iterparse(stream, events=("start", "end")):
            if event == "start":
                stack.append(e)
                continue
            if e.tag == "category":
                categories[e.get("id")] = (e.text or "", e.get("parentId") or "")
            if e.tag == "offer":
                count += 1
                group = e.get("group_id") or "offer-"+(e.get("id") or "")
                uri = urlsplit(e.findtext("url") or "")
                vendor = uri.path.strip("/").split("/")[0]
                content = text(e, group)
                offer_tokens += len(content.encode("utf-8"))+16
                if uri.hostname != "store.softline.ru" or not vendor or not content:
                    raise ValueError("Invalid source scope/text")
                key = vendor+"/"+group
                roles = [p.text for p in e.findall("param") if p.get("name") == "Тип лицензии"]
                score = (int("Полная версия" in roles or "Full" in roles), int(e.get("available") == "true"))
                if key not in groups or score > groups[key][0]:
                    groups[key] = (score, {"ID": key, "GroupID": group, "Vendor": vendor, "Text": content, "SourceURL": e.findtext("url")})
            if e.tag in {"offer", "category"}:
                if len(stack) > 1:
                    stack[-2].remove(e)
                e.clear()
            stack.pop()
    core = sorted(k for k, (_, d) in groups.items() if d["Vendor"] in scopes)
    if len(core) > limit:
        raise ValueError("Core scope exceeds corpus limit")
    rest = sorted((k for k in groups if k not in set(core)), key=lambda k: hashlib.sha256(k.encode()).hexdigest())
    selected = sorted(core+rest[:limit-len(core)])
    documents = [groups[k][1] for k in selected]
    present = {d["GroupID"] for d in documents}
    queries = []
    for c in tuning["cases"]:
        queries.append({"ID": c["id"], "Text": c["query"], "Scope": c["target_scope"], "Split": "tuning", "Groups": c["group_ids"], "Vendor": c["brand_id"]})
    for c in probes["cases"]:
        queries.append({"ID": c["id"], "Text": c["query"], "Scope": "family", "Split": "pilot_probe", "Groups": c["group_ids"]})
    if any(q["Scope"] == "family" and not set(q["Groups"]).issubset(present) for q in queries):
        raise ValueError("Fixed corpus does not cover known-positive groups")
    return {"Version": "embedding-text-v1", "Role": "development_tuning_pilot", "SourceSHA": sha,
            "TuningSHA": tuning.get("cases_sha256", ""), "FullGroups": len(groups), "FullOffers": count,
            "FullGroupTokenBound": sum(len(d["Text"].encode())+16 for _, d in groups.values()),
            "FullOfferTokenBound": offer_tokens, "Documents": documents, "Queries": queries}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--feed", required=True, type=Path)
    parser.add_argument("--output", type=Path, default=Path("artifacts/embedding-pilot-dataset.json"))
    args = parser.parse_args()
    tuning = json.loads(Path("testdata/evaluation/scenarios.json").read_text(encoding="utf-8"))
    probes = json.loads(Path("testdata/evaluation/embedding-probes-v1.json").read_text(encoding="utf-8"))
    result = prepare(args.feed, tuning, probes)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, ensure_ascii=False, indent=2)+"\n", encoding="utf-8")
    print(json.dumps({"documents": len(result["Documents"]), "queries": len(result["Queries"]), "all_groups": result["FullGroups"], "source_sha": result["SourceSHA"]}))


if __name__ == "__main__":
    main()
