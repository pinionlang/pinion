#!/usr/bin/env python3
"""Prepare a run directory BEFORE an implementer agent starts and record provenance
at that moment (compiler commit, doc hashes). Optionally copy a baseline codebase in.

usage: start_run.py <run_dir> [--baseline DIR] [--task ID]
writes <run_dir>/META.json
"""
import argparse, hashlib, json, shutil, subprocess, time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
DOCS = ["spec/REFERENCE-CARD.md", "spec/GUIDE.md", "spec/STDLIB.md", "bench/ai/crud/SPEC.md"]


def sh(cmd):
    return subprocess.run(cmd, cwd=ROOT, capture_output=True, text=True).stdout.strip()


def h(p):
    f = ROOT / p
    return hashlib.sha256(f.read_bytes()).hexdigest()[:12] if f.exists() else None


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("run_dir")
    ap.add_argument("--baseline")
    ap.add_argument("--task")
    a = ap.parse_args()
    d = Path(a.run_dir)
    if d.exists() and any(d.iterdir()):
        raise SystemExit(f"{d} is not empty")
    d.mkdir(parents=True, exist_ok=True)
    if a.baseline:
        shutil.copytree(a.baseline, d, dirs_exist_ok=True,
                        ignore=shutil.ignore_patterns("server", "out", "__pycache__", "node_modules", "METRICS.json", "META.json"))
    extra = [f"bench/ai/change/{a.task}/CHANGE.md"] if a.task else []
    meta = {
        "started_at": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
        "compiler_commit": sh(["git", "rev-parse", "--short", "HEAD"]),
        "compiler_dirty": bool(sh(["git", "status", "--porcelain", "--", "cmd", "internal", "runtime", "spec"])),
        "task": a.task,
        "baseline": a.baseline,
        "doc_hashes": {p: h(p) for p in DOCS + extra},
    }
    (d / "META.json").write_text(json.dumps(meta, indent=2))
    print(json.dumps(meta))


if __name__ == "__main__":
    main()
