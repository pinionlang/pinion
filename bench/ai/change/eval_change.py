#!/usr/bin/env python3
"""Evaluate change-task runs.

For every bench/ai/change/runs/<T>/<lang>/: run BUILD (if present), then the hidden
grading test <private>/<T>/test_change.py against RUN; measure the diff against the
starting point (META.json "baseline", else T3-bugfix/<lang> for T3-bugfix and
baseline/<lang> otherwise). Writes one append-only record per run to
bench/ai/change/results/raw/<T>-<lang>-<timestamp>.json and prints a table.

Usage: python3 eval_change.py [--runs DIR] [--private DIR] [--only T1-isbn,T4-status/pinion]
"""
import argparse, difflib, hashlib, json, os, platform, subprocess, sys, time
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[2]
SKIP_DIRS = {"node_modules", "dist", "build", "out", "__pycache__", ".venv", "venv", ".git", "bin", "target"}
SKIP_FILES = {"META.json", "METRICS.json", "server", "books.json"}
TEXT_EXT = {".pi", ".py", ".go", ".ts", ".js", ".mjs", ".cjs", ".mts", ".mod", ".json", ".toml", ".txt", ""}

try:
    import tiktoken
    ENC = tiktoken.get_encoding("cl100k_base")
except Exception:
    ENC = None


def ntok(s):
    return len(ENC.encode(s, disallowed_special=())) if ENC else None


def tracked(d: Path):
    out = {}
    if not d.is_dir():
        return out
    for root, dirs, files in os.walk(d):
        dirs[:] = [x for x in dirs if x not in SKIP_DIRS and not x.startswith(".")]
        for f in files:
            p = Path(root) / f
            if f in SKIP_FILES or f.endswith(".d.ts") or p.suffix not in TEXT_EXT:
                continue
            if p.suffix == ".js" and p.with_suffix(".ts").exists():
                continue
            try:
                out[str(p.relative_to(d))] = p.read_text()
            except (UnicodeDecodeError, OSError):
                pass
    return out


def diff_stats(start: Path, end: Path):
    a, b = tracked(start), tracked(end)
    chunks, added, removed, changed = [], 0, 0, []
    for name in sorted(set(a) | set(b)):
        x, y = a.get(name, "").splitlines(keepends=True), b.get(name, "").splitlines(keepends=True)
        if x == y:
            continue
        lines = list(difflib.unified_diff(x, y, "a/" + name, "b/" + name))
        chunks.extend(l if l.endswith("\n") else l + "\n" for l in lines)
        for l in lines:
            if l.startswith(("+++", "---")):
                continue
            if l.startswith("+"):
                added += 1
                changed.append(l[1:])
            elif l.startswith("-"):
                removed += 1
                changed.append(l[1:])
    text = "".join(chunks)
    return {"start": str(start), "files_changed": len({n for n in set(a) | set(b) if a.get(n) != b.get(n)}),
            "lines_added": added, "lines_removed": removed,
            "diff_tokens_cl100k": ntok(text), "changed_tokens_cl100k": ntok("".join(changed)),
            "unified_diff": text}


def start_dir(task, lang, meta):
    b = meta.get("baseline")
    if b:
        p = Path(b)
        return p if p.is_absolute() else ROOT / p
    return HERE / ("T3-bugfix" if task.startswith("T3") else "baseline") / lang


def evaluate(d: Path, task: str, private: Path):
    lang = d.name
    r = {"task": task, "lang": lang, "run_dir": str(d)}
    meta = {}
    if (d / "META.json").exists():
        try:
            meta = json.loads((d / "META.json").read_text())
        except ValueError:
            meta = {"error": "bad META.json"}
    r["meta"] = meta
    if (d / "METRICS.json").exists():
        try:
            r["metrics"] = json.loads((d / "METRICS.json").read_text())
        except ValueError:
            r["metrics"] = {"error": "bad METRICS.json"}
    r["diff"] = diff_stats(start_dir(task, lang, meta), d)
    run = d / "RUN"
    if not run.exists():
        r["error"] = "missing RUN"
        r["tests"] = {"passed": 0, "total": None, "failures": ["missing RUN"]}
        return r
    r["run_cmd"] = run.read_text().strip()
    if (d / "BUILD").exists():
        cmd = (d / "BUILD").read_text().strip()
        t0 = time.time()
        b = subprocess.run(cmd, shell=True, cwd=d, capture_output=True, text=True, timeout=600)
        r["build"] = {"cmd": cmd, "ok": b.returncode == 0, "seconds": round(time.time() - t0, 2),
                      "output": (b.stdout + b.stderr)[-2000:]}
        if b.returncode != 0:
            r["tests"] = {"passed": 0, "total": None, "failures": ["build failed"]}
            return r
    test = private / task / "test_change.py"
    r["test_sha256"] = hashlib.sha256(test.read_bytes()).hexdigest()[:12] if test.exists() else None
    t = subprocess.run([sys.executable, str(test), "--cmd", r["run_cmd"], "--cwd", str(d)],
                       capture_output=True, text=True, timeout=900)
    try:
        r["tests"] = json.loads(t.stdout)
    except ValueError:
        r["tests"] = {"passed": 0, "total": None, "failures": [t.stdout[-1000:] + t.stderr[-1000:]]}
    return r


def provenance():
    def sh(cmd):
        try:
            return subprocess.run(cmd, cwd=ROOT, capture_output=True, text=True, timeout=30).stdout.strip()
        except Exception:
            return ""

    def h(p):
        f = ROOT / p
        return hashlib.sha256(f.read_bytes()).hexdigest()[:12] if f.exists() else None

    docs = ["bench/ai/crud/SPEC.md", "spec/REFERENCE-CARD.md", "spec/GUIDE.md", "spec/STDLIB.md", "bench/ai/change/PROMPT.md"]
    docs += [str(p.relative_to(ROOT)) for p in sorted(HERE.glob("T*/CHANGE.md"))]
    return {
        "evaluated_at": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
        "compiler_commit": sh(["git", "rev-parse", "--short", "HEAD"]),
        "compiler_dirty": bool(sh(["git", "status", "--porcelain", "--", "cmd", "internal", "runtime"])),
        "hashes": {p: h(p) for p in docs},
        "tools": {"python": platform.python_version(), "go": sh(["go", "version"]), "node": sh(["node", "--version"]), "os": platform.platform()},
    }


def fmt(v):
    return "-" if v is None else str(v)


def table(results):
    rows = ["| task | lang | tests | +lines | -lines | changed tokens | agent tokens | tool uses | minutes |",
            "|---|---|---|---|---|---|---|---|---|"]
    for r in results:
        t, df, m = r.get("tests") or {}, r.get("diff") or {}, r.get("metrics") or {}
        dur = m.get("duration_ms")
        rows.append(f"| {r['task']} | {r['lang']} | {t.get('passed', 0)}/{fmt(t.get('total'))} | {fmt(df.get('lines_added'))} | "
                    f"{fmt(df.get('lines_removed'))} | {fmt(df.get('changed_tokens_cl100k'))} | {fmt(m.get('tokens'))} | "
                    f"{fmt(m.get('tool_uses'))} | {fmt(round(dur / 60000, 1) if isinstance(dur, (int, float)) else None)} |")
    return "\n".join(rows) + "\n"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--runs", default=str(HERE / "runs"))
    ap.add_argument("--private", default="/tmp/pinion-change-private")
    ap.add_argument("--only", default="", help="comma list of <T> or <T>/<lang>")
    a = ap.parse_args()
    only = {x for x in a.only.split(",") if x}
    runs = Path(a.runs)
    dirs = sorted(d for t in runs.iterdir() if t.is_dir() for d in t.iterdir() if d.is_dir()) if runs.exists() else []
    dirs = [d for d in dirs if not only or d.parent.name in only or f"{d.parent.name}/{d.name}" in only]
    meta = provenance()
    raw = HERE / "results" / "raw"
    raw.mkdir(parents=True, exist_ok=True)
    stamp = time.strftime("%Y%m%dT%H%M%S")
    results = []
    for d in dirs:
        print(f"== {d.parent.name}/{d.name}", file=sys.stderr)
        r = evaluate(d, d.parent.name, Path(a.private))
        t = r.get("tests") or {}
        print(f"   tests {t.get('passed')}/{t.get('total')}  {r.get('error', '')}", file=sys.stderr)
        rid = f"{r['task']}-{r['lang']}-{stamp}"
        path = raw / f"{rid}.json"
        n = 1
        while path.exists():
            n += 1
            path = raw / f"{rid}-{n}.json"
        path.write_text(json.dumps({"eval_id": path.stem, **meta, **r}, indent=2, ensure_ascii=False))
        results.append(r)
    print(table(results))


if __name__ == "__main__":
    main()
