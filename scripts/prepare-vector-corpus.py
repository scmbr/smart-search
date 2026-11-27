"""Prepare every catalog group/SKU from feed only; no labels, query files, or API."""
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path

spec = importlib.util.spec_from_file_location("pilot_reader", Path(__file__).with_name("prepare-embedding-pilot.py"))
reader = importlib.util.module_from_spec(spec)
spec.loader.exec_module(reader)


def fingerprint(x):
    fields = [x["Version"], x["TextVersion"], x["Model"], str(x["Dimensions"]), x["SourceSHA"], str(x["Offers"]), str(len(x["Documents"]))]
    for d in x["Documents"]:
        fields += [d[k] for k in ("ID", "GroupID", "OfferID", "Vendor", "Text", "SourceURL")]
    h = hashlib.sha256()
    for s in fields:
        raw = s.encode("utf-8")
        h.update(len(raw).to_bytes(4, "big"))
        h.update(raw)
    return h.hexdigest()


def prepare(path):
    sha, groups, count, _ = reader.read_catalog(path)
    documents = []
    for _, doc in groups.values():
        d = dict(doc)
        if d["OfferID"]:
            d["GroupID"] = ""
        identity = "offer/"+d["OfferID"] if d["OfferID"] else "group/"+d["GroupID"]
        d["ID"] = d["Vendor"]+"/"+identity
        documents.append(d)
    documents.sort(key=lambda d: d["ID"])
    x = dict(Version="vector-corpus-v1", TextVersion="embedding-text-v1", Model="text-embedding-3-small", Dimensions=1536,
             SourceSHA=sha, Offers=count, Documents=documents)
    x["SHA"] = fingerprint(x)
    return x


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--feed", required=True, type=Path)
    parser.add_argument("--output", type=Path, default=Path("artifacts/vector-corpus.json"))
    args = parser.parse_args()
    x = prepare(args.feed)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(x, ensure_ascii=False, indent=2)+"\n", encoding="utf-8")
    print(json.dumps(dict(documents=len(x["Documents"]), offers=x["Offers"], fallback_skus=sum(bool(d["OfferID"]) for d in x["Documents"]), source_sha=x["SourceSHA"], corpus_sha=x["SHA"])))


if __name__ == "__main__":
    main()
