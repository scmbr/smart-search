"""Offline checks for tracked repository content; no third-party packages."""

import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys
from urllib.parse import unquote, urlsplit


def tracked_files():
    result = subprocess.run(
        ["git", "ls-files", "-z"], check=True, capture_output=True
    )
    return [Path(p) for p in result.stdout.decode("utf-8").split("\0") if p]


def index_text(path):
    # Validate the proposed commit, not unstaged changes from another task.
    result = subprocess.run(
        ["git", "show", ":" + path.as_posix()], check=True, capture_output=True
    )
    return result.stdout.decode("utf-8")


def layout(files):
    names = {p.as_posix() for p in files}
    if any(p.suffix == ".go" for p in files) and "go.mod" not in names:
        raise ValueError("Tracked Go code requires a tracked go.mod")
    frontend_sources = any(
        p.parts[0] == "frontend" and p.suffix in {".ts", ".tsx", ".js", ".jsx", ".mjs"}
        for p in files
    )
    if frontend_sources and "frontend/package.json" not in names:
        raise ValueError("Frontend source requires frontend/package.json and CI scripts")
    return "go.mod" in names, "frontend/package.json" in names


def check_contents(files):
    names = {p.as_posix() for p in files}
    for path in files:
        if path.suffix not in {".md", ".json", ".py", ".mjs", ".yaml", ".yml"}:
            continue
        text = index_text(path)
        for number, line in enumerate(text.splitlines(), 1):
            if line != line.rstrip():
                raise ValueError(f"{path}:{number}: trailing whitespace")
        if path.suffix == ".json":
            json.loads(text)
        if path.suffix != ".md":
            continue
        # Ignore illustrative links inside fenced code blocks.
        prose = re.sub(r"(?ms)^```.*?^```[^\n]*", "", text)
        for _, target in re.findall(r"\[([^\]]+)\]\(([^)]+)\)", prose):
            target = target.strip("<>")
            parsed = urlsplit(target)
            if parsed.scheme or parsed.netloc or not parsed.path:
                continue
            destination = Path(os.path.normpath(path.parent / unquote(parsed.path))).as_posix()
            if destination in names:
                continue
            if any(name.startswith(destination.rstrip("/") + "/") for name in names):
                continue
            raise ValueError(f"{path}: link target is not tracked: {target}")


def check_pr_naming():
    if os.environ.get("GITHUB_EVENT_NAME") != "pull_request":
        return
    event = json.loads(Path(os.environ["GITHUB_EVENT_PATH"]).read_text(encoding="utf-8"))
    pr = event["pull_request"]
    types = r"(?:feat|fix|docs|chore|refactor|test)"
    if not re.fullmatch(types + r"/[a-z0-9]+(?:-[a-z0-9]+)*", pr["head"]["ref"]):
        raise ValueError("PR branch must be <type>/<kebab-case-slug>")
    if not re.fullmatch(types + r": \S.*", pr["title"]):
        raise ValueError("PR title must be <type>: <description>")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--layout", action="store_true")
    parser.add_argument("--go-format", action="store_true")
    args = parser.parse_args()
    files = tracked_files()
    go_present, frontend_present = layout(files)
    if args.layout:
        version = ""
        if go_present:
            match = re.search(r"(?m)^go\s+(\d+\.\d+)(?:\.\d+)?\s*$", index_text(Path("go.mod")))
            if not match:
                raise ValueError("Cannot determine Go version from go.mod")
            version = match[1] + ".x"
        print(f"go={str(go_present).lower()}")
        print(f"frontend={str(frontend_present).lower()}")
        print(f"go_version={version}")
        return
    if args.go_format:
        go_files = [str(p) for p in files if p.suffix == ".go"]
        if not go_files:
            raise ValueError("Go formatting job has no tracked Go files")
        result = subprocess.run(["gofmt", "-l", *go_files], check=True, capture_output=True, text=True)
        if result.stdout.strip():
            raise ValueError("gofmt required:\n" + result.stdout)
        print("PASS: Go formatting")
        return
    check_contents(files)
    check_pr_naming()
    print(f"PASS: tracked UTF-8 content, Markdown links, JSON and PR naming ({len(files)} files)")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        print(f"ERROR: {error}", file=sys.stderr)
        sys.exit(1)
