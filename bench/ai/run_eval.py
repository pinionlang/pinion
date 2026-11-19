#!/usr/bin/env python3
"""Evaluate implementer runs.

Layout: <runs>/<lang>/ containing source, RUN (start command, run in that dir),
optional BUILD (build command), optional METRICS.json {"tokens","tool_uses","duration_ms"}.
Writes <results>/eval.json and <results>/RESULTS.md.

Usage: python3 run_eval.py [--runs DIR] [--results DIR] [--only lang,lang]
"""
import argparse, json, os, subprocess, sys, time
from pathlib import Path

HERE = Path(__file__).resolve().parent
TEST = HERE / "crud" / "test_api.py"
SRC_EXT = {".pi", ".py", ".go", ".ts", ".js", ".mjs", ".cjs", ".mts"}
SKIP_DIRS = {"node_modules", "dist", "build", "out", "__pycache__", ".venv", "venv", ".git", "bin", "target"}
SKIP_FILES = {"RUN", "BUILD", "METRICS.json"}

try:
    import tiktoken
    ENC = tiktoken.get_encoding("cl100k_base")
except Exception:
    ENC = None


def source_files(d: Path):
    out = []
    for root, dirs, files in os.walk(d):
        dirs[:] = [x for x in dirs if x not in SKIP_DIRS and not x.startswith(".")]
        for f in files:
            p = Path(root) / f
            if f in SKIP_FILES or f.endswith(".d.ts"):
                continue
            # compiled .js next to a .ts of the same name is build output
            if p.suffix == ".js" and p.with_suffix(".ts").exists():
                continue
            if p.suffix in SRC_EXT:
                out.append(p)
    return sorted(out)


def size(files):
    lines = nbytes = toks = 0
    for p in files:
        b = p.read_bytes()
        nbytes += len(b)
        t = b.decode(errors="replace")
        lines += sum(1 for l in t.splitlines() if l.strip())
        if ENC:
            toks += len(ENC.encode(t, disallowed_special=()))
    return {"files": [str(p) for p in files], "nonblank_lines": lines, "bytes": nbytes,
            "tokens_cl100k": toks if ENC else None}


def evaluate(d: Path):
    r = {"lang": d.name}
    run = d / "RUN"
    if not run.exists():
        r["error"] = "missing RUN"
        return r
    r["run_cmd"] = run.read_text().strip()
    r["source"] = size(source_files(d))
    if (d / "BUILD").exists():
        cmd = (d / "BUILD").read_text().strip()
        t0 = time.time()
        b = subprocess.run(cmd, shell=True, cwd=d, capture_output=True, text=True, timeout=600)
        r["build"] = {"cmd": cmd, "ok": b.returncode == 0, "seconds": round(time.time() - t0, 2),
                      "output": (b.stdout + b.stderr)[-2000:]}
        if b.returncode != 0:
            r["tests"] = {"passed": 0, "total": None, "failures": ["build failed"]}
            return r
    t = subprocess.run([sys.executable, str(TEST), "--cmd", r["run_cmd"], "--cwd", str(d)],
                       capture_output=True, text=True, timeout=900)
    try:
        r["tests"] = json.loads(t.stdout)
    except ValueError:
        r["tests"] = {"passed": 0, "total": None, "failures": [t.stdout[-1000:] + t.stderr[-1000:]]}
    m = d / "METRICS.json"
    if m.exists():
        try:
            r["metrics"] = json.loads(m.read_text())
        except ValueError:
            r["metrics"] = {"error": "bad METRICS.json"}
    return r


def fmt(v):
    return "-" if v is None else str(v)


def table(results):
    rows = ["| lang | tests passed | source tokens | source lines | agent tokens | tool uses | minutes |",
            "|---|---|---|---|---|---|---|"]
    for r in results:
        t = r.get("tests") or {}
        s = r.get("source") or {}
        m = r.get("metrics") or {}
        dur = m.get("duration_ms")
        tests = f"{t.get('passed', 0)}/{fmt(t.get('total'))}" if t else r.get("error", "-")
        rows.append(f"| {r['lang']} | {tests} | {fmt(s.get('tokens_cl100k'))} | {fmt(s.get('nonblank_lines'))} | "
                    f"{fmt(m.get('tokens'))} | {fmt(m.get('tool_uses'))} | "
                    f"{fmt(round(dur / 60000, 1) if isinstance(dur, (int, float)) else None)} |")
    return "\n".join(rows) + "\n"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--runs", default=str(HERE / "runs"))
    ap.add_argument("--results", default=str(HERE / "results"))
    ap.add_argument("--only", default="")
    a = ap.parse_args()
    only = {x for x in a.only.split(",") if x}
    runs = sorted(p for p in Path(a.runs).iterdir() if p.is_dir() and (not only or p.name in only))
    results = []
    for d in runs:
        print(f"== {d.name}", file=sys.stderr)
        r = evaluate(d)
        t = r.get("tests") or {}
        print(f"   tests {t.get('passed')}/{t.get('total')}  {r.get('error', '')}", file=sys.stderr)
        results.append(r)
    out = Path(a.results)
    out.mkdir(parents=True, exist_ok=True)
    (out / "eval.json").write_text(json.dumps(results, indent=2, ensure_ascii=False))
    (out / "RESULTS.md").write_text("# CRUD benchmark results\n\n" + table(results))
    print(table(results))


if __name__ == "__main__":
    main()
