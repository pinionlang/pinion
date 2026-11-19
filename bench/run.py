#!/usr/bin/env python3
"""Mini PinionBench runner: Pinion vs Python vs Go vs TypeScript.

For every task in bench/tasks/<task>/ it
  1. builds Go (`go build`), TypeScript (`npx tsc`, run with node) and Pinion
     (`pinion build`, binary from $PINION or ./bin/pinion; skipped if missing),
  2. runs each language N times (default 5) and records the median wall time,
  3. checks that every language prints byte-identical stdout (reference: Python),
  4. measures source size: lines, bytes and LLM tokens.

Token count: uses `tiktoken` (cl100k_base) when importable; otherwise a documented
heuristic: number of matches of  [A-Za-z_][A-Za-z0-9_]* | \\d+ | \\S  (identifiers,
digit runs and single punctuation characters). The method used is recorded in the output.

Usage: python3 bench/run.py [-n 5] [--tasks fib,sort] [--langs python,go,ts,pinion]
Writes bench/results/results.json and bench/results/RESULTS.md. Python 3.13, stdlib only.
"""

from __future__ import annotations

import argparse
import json
import os
import platform
import re
import shutil
import statistics
import subprocess
import sys
import time
from pathlib import Path

BENCH = Path(__file__).resolve().parent
ROOT = BENCH.parent
TASKS = BENCH / "tasks"
BUILD = BENCH / "build"
RESULTS = BENCH / "results"
LANGS = ["pinion", "python", "go", "ts"]
RUN_TIMEOUT = 300


# ---------------------------------------------------------------- token counting

def make_token_counter():
    try:
        import tiktoken  # type: ignore

        enc = tiktoken.get_encoding("cl100k_base")
        enc.encode("probe")
        return (lambda text: len(enc.encode(text))), "tiktoken cl100k_base"
    except Exception:
        pat = re.compile(r"[A-Za-z_][A-Za-z0-9_]*|\d+|\S")
        return (lambda text: len(pat.findall(text))), r"heuristic regex [A-Za-z_]\w*|\d+|\S"


# ---------------------------------------------------------------- helpers

def sh(cmd: list[str], cwd: Path | None = None, timeout: int = RUN_TIMEOUT) -> subprocess.CompletedProcess:
    return subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=timeout)


def pinion_binary() -> Path | None:
    env = os.environ.get("PINION")
    cand = Path(env) if env else ROOT / "bin" / "pinion"
    if cand.is_file() and os.access(cand, os.X_OK):
        return cand.resolve()
    found = shutil.which("pinion") if not env else None
    return Path(found) if found else None


def source_files(task: Path, lang: str) -> list[Path]:
    if lang == "pinion":
        return sorted((task / "pinion" / "src").rglob("*.pi"))
    ext = {"python": "py", "go": "go", "ts": "ts"}[lang]
    p = task / f"main.{ext}"
    return [p] if p.exists() else []


def size_of(files: list[Path], count_tokens) -> dict:
    text = "".join(f.read_text(encoding="utf-8") for f in files)
    lines = [ln for ln in text.splitlines()]
    return {
        "lines": len(lines),
        "nonblank_lines": sum(1 for ln in lines if ln.strip()),
        "bytes": len(text.encode("utf-8")),
        "tokens": count_tokens(text),
    }


# ---------------------------------------------------------------- build

def build(task: Path, lang: str, pinion: Path | None) -> tuple[list[str] | None, str]:
    """Returns (command to run, note). Command None means skipped/failed."""
    out = BUILD / task.name / lang
    out.mkdir(parents=True, exist_ok=True)
    if lang == "python":
        return [sys.executable, str(task / "main.py")], ""
    if lang == "go":
        exe = out / "main"
        r = sh(["go", "build", "-o", str(exe), str(task / "main.go")])
        return ([str(exe)], "") if r.returncode == 0 else (None, "go build failed: " + r.stderr.strip())
    if lang == "ts":
        r = sh(["npx", "--yes", "tsc", "--target", "es2022", "--module", "commonjs", "--strict",
                "--outDir", str(out), str(task / "main.ts")])
        js = out / "main.js"
        if r.returncode != 0 or not js.exists():
            return None, "tsc failed: " + (r.stdout + r.stderr).strip()
        return ["node", str(js)], ""
    if lang == "pinion":
        if pinion is None:
            return None, "skipped: pinion binary not found (set PINION or build ./bin/pinion)"
        proj = task / "pinion"
        exe = out / "main"
        attempts = [[str(pinion), "build", "-o", str(exe)], [str(pinion), "build", "-o", str(exe), "."],
                    [str(pinion), "build"]]
        errors = []
        for cmd in attempts:
            try:
                r = sh(cmd, cwd=proj, timeout=600)
            except subprocess.TimeoutExpired:
                errors.append(f"{' '.join(cmd[1:])}: timeout")
                continue
            if r.returncode == 0:
                if exe.exists():
                    return [str(exe)], ""
                for c in [proj / "bin" / task.name, proj / task.name, proj / "main", proj / "out" / task.name]:
                    if c.is_file() and os.access(c, os.X_OK):
                        return [str(c)], ""
                errors.append(f"{' '.join(cmd[1:])}: built but binary not found")
            else:
                errors.append(f"{' '.join(cmd[1:])}: {(r.stdout + r.stderr).strip()[:2000]}")
        return None, "pinion build failed: " + " | ".join(errors)
    raise ValueError(lang)


# ---------------------------------------------------------------- run

def run_n(cmd: list[str], n: int) -> dict:
    times, outputs, codes = [], set(), set()
    last_err = ""
    for _ in range(n):
        t0 = time.perf_counter()
        try:
            r = subprocess.run(cmd, capture_output=True, timeout=RUN_TIMEOUT)
        except subprocess.TimeoutExpired:
            return {"ok": False, "error": "timeout"}
        times.append(time.perf_counter() - t0)
        outputs.add(r.stdout)
        codes.add(r.returncode)
        last_err = r.stderr.decode("utf-8", "replace")
    stdout = next(iter(outputs)).decode("utf-8", "replace")
    res = {
        "ok": codes == {0} and len(outputs) == 1,
        "median_s": statistics.median(times),
        "min_s": min(times),
        "times_s": times,
        "exit_codes": sorted(codes),
        "stdout": stdout,
        "deterministic": len(outputs) == 1,
    }
    if codes != {0}:
        res["error"] = f"exit {sorted(codes)}: {last_err.strip()[:2000]}"
    return res


# ---------------------------------------------------------------- report

def fmt_ms(sec: float | None) -> str:
    return "–" if sec is None else f"{sec * 1000:.0f}"


def write_markdown(data: dict) -> str:
    langs = data["langs"]
    out = ["# Mini PinionBench results", ""]
    out.append(f"- date: {data['date']}  ")
    out.append(f"- machine: {data['machine']}  ")
    out.append(f"- runs per measurement: {data['runs']} (median wall time, includes process start)  ")
    out.append(f"- tokens: {data['token_method']}  ")
    out.append("- versions: " + ", ".join(f"{k} `{v}`" for k, v in data["versions"].items()))
    out.append("")
    out.append("## Run time (ms, median)")
    out.append("")
    out.append("| task | " + " | ".join(langs) + " | outputs match |")
    out.append("|---|" + "---:|" * len(langs) + "---|")
    for t in data["tasks"]:
        cells = []
        for lg in langs:
            r = t["langs"][lg]
            cells.append(fmt_ms(r.get("median_s")) if r.get("ok") else ("skip" if r.get("skipped") else "FAIL"))
        out.append(f"| {t['task']} | " + " | ".join(cells) + f" | {t['match']} |")
    out.append("")
    out.append("## Source size (lines / bytes / tokens)")
    out.append("")
    out.append("| task | " + " | ".join(langs) + " |")
    out.append("|---|" + "---:|" * len(langs))
    totals = {lg: [0, 0, 0] for lg in langs}
    for t in data["tasks"]:
        cells = []
        for lg in langs:
            s = t["langs"][lg]["size"]
            totals[lg] = [totals[lg][0] + s["lines"], totals[lg][1] + s["bytes"], totals[lg][2] + s["tokens"]]
            cells.append(f"{s['lines']} / {s['bytes']} / {s['tokens']}")
        out.append(f"| {t['task']} | " + " | ".join(cells) + " |")
    out.append("| **total** | " + " | ".join(f"**{a} / {b} / {c}**" for a, b, c in totals.values()) + " |")
    out.append("")
    notes = [(t["task"], lg, t["langs"][lg]["note"]) for t in data["tasks"] for lg in langs if t["langs"][lg].get("note")]
    if notes:
        out.append("## Notes")
        out.append("")
        for task, lg, note in notes:
            out.append(f"- {task}/{lg}: {note.splitlines()[0][:300]}")
        out.append("")
    out.append("Expected stdout per task is the Python output; see `results.json` for raw times and stdout.")
    return "\n".join(out) + "\n"


def version(cmd: list[str]) -> str:
    try:
        r = sh(cmd, timeout=60)
        return (r.stdout or r.stderr).strip().splitlines()[0]
    except Exception as e:  # noqa: BLE001
        return f"unavailable ({e.__class__.__name__})"


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("-n", "--runs", type=int, default=5)
    ap.add_argument("--tasks", default="", help="comma-separated subset")
    ap.add_argument("--langs", default=",".join(LANGS), help="comma-separated subset of " + ",".join(LANGS))
    args = ap.parse_args()

    langs = [lg for lg in args.langs.split(",") if lg]
    tasks = sorted(p for p in TASKS.iterdir() if p.is_dir())
    if args.tasks:
        want = set(args.tasks.split(","))
        tasks = [t for t in tasks if t.name in want]
    count_tokens, token_method = make_token_counter()
    pinion = pinion_binary()

    versions = {"python": platform.python_version(), "go": version(["go", "version"]),
                "node": version(["node", "--version"]), "tsc": version(["npx", "--yes", "tsc", "--version"]),
                "pinion": version([str(pinion), "version"]) if pinion else "not found"}
    data = {"date": time.strftime("%Y-%m-%dT%H:%M:%S%z"), "machine": f"{platform.system()} {platform.machine()}",
            "runs": args.runs, "token_method": token_method, "versions": versions, "langs": langs, "tasks": []}

    failed = False
    for task in tasks:
        print(f"== {task.name}", flush=True)
        entry = {"task": task.name, "langs": {}}
        for lg in langs:
            rec: dict = {"size": size_of(source_files(task, lg), count_tokens)}
            cmd, note = build(task, lg, pinion)
            if cmd is None:
                rec.update({"ok": False, "skipped": note.startswith("skipped"), "note": note})
                if not rec["skipped"]:
                    failed = True
            else:
                rec.update(run_n(cmd, args.runs))
                if not rec["ok"]:
                    rec["note"] = rec.get("error", "nondeterministic output")
                    failed = True
            status = f"{rec['median_s'] * 1000:8.0f} ms" if rec.get("ok") else ("    skip" if rec.get("skipped") else "    FAIL")
            print(f"   {lg:7s} {status}   {rec['size']['tokens']:4d} tokens", flush=True)
            entry["langs"][lg] = rec
        outs = {lg: r["stdout"] for lg, r in entry["langs"].items() if r.get("ok")}
        ref = outs.get("python", next(iter(outs.values()), None))
        mismatched = [lg for lg, o in outs.items() if o != ref]
        entry["expected_stdout"] = ref
        entry["match"] = "yes" if not mismatched and len(outs) > 1 else ("n/a" if len(outs) <= 1 else "NO: " + ",".join(mismatched))
        if mismatched:
            failed = True
            for lg in mismatched:
                entry["langs"][lg]["note"] = "stdout differs from reference"
        print(f"   outputs match: {entry['match']}", flush=True)
        data["tasks"].append(entry)

    RESULTS.mkdir(exist_ok=True)
    (RESULTS / "results.json").write_text(json.dumps(data, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    (RESULTS / "RESULTS.md").write_text(write_markdown(data), encoding="utf-8")
    print(f"wrote {RESULTS / 'results.json'} and {RESULTS / 'RESULTS.md'}")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
