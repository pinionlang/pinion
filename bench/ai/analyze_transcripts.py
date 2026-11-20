#!/usr/bin/env python3
"""Break down implementer-agent transcripts (Claude Code subagent JSONL) into
token usage (input / output / cache) and tool calls by category.

usage: analyze_transcripts.py MAP.json   (MAP: {"<round>/<lang>": "<path to .output JSONL>"})
writes results/transcripts.json and prints a Markdown table.
"""
import json, re, sys
from pathlib import Path

HERE = Path(__file__).resolve().parent


def classify(name, inp):
    if name in ("Read", "Glob", "Grep"):
        p = str(inp.get("file_path") or inp.get("path") or inp.get("pattern") or "")
        if "SPEC.md" in p:
            return "read_spec"
        if re.search(r"spec/|examples/|GUIDE|STDLIB|REFERENCE", p):
            return "read_docs"
        return "read_other"
    if name in ("Write", "Edit", "MultiEdit", "NotebookEdit"):
        return "write"
    if name == "Bash":
        # One shell command often does several things; give it every label that applies.
        c = inp.get("command", "")
        labels = []
        if re.search(r"(cat|head|sed -n|less|grep|ls)\b[^|;&]*(spec/|examples/|GUIDE|STDLIB|REFERENCE)", c):
            labels.append("read_docs")
        if "SPEC.md" in c:
            labels.append("read_spec")
        if re.search(r"<<\s*'?\w+'?|>\s*\S+\.(pi|py|go|ts|mod)\b|\b(RUN|BUILD)\b\s*$|> *(RUN|BUILD)\b", c, re.M):
            labels.append("write")
        if re.search(r"pinion (build|check|test|run)|go (build|vet|run)|tsc|py_compile|python3 -c .import", c):
            labels.append("build_check")
        if "curl" in c:
            labels.append("run_check")
        return labels or ["bash_other"]
    return "other:" + name


def analyze(path):
    usage = {"input": 0, "output": 0, "cache_create": 0, "cache_read": 0}
    seen = set()
    tools = {}
    calls = []
    model = None
    errors = 0
    pending = {}
    for ln in open(path, encoding="utf-8"):
        try:
            o = json.loads(ln)
        except Exception:
            continue
        msg = o.get("message") or {}
        if o.get("type") == "assistant":
            mid = msg.get("id")
            u = msg.get("usage") or {}
            if mid and mid not in seen and u:
                seen.add(mid)
                usage["input"] += u.get("input_tokens", 0)
                usage["output"] += u.get("output_tokens", 0)
                usage["cache_create"] += u.get("cache_creation_input_tokens", 0)
                usage["cache_read"] += u.get("cache_read_input_tokens", 0)
            model = msg.get("model") or model
            for b in msg.get("content") or []:
                if isinstance(b, dict) and b.get("type") == "tool_use":
                    cats = classify(b.get("name"), b.get("input") or {})
                    cats = cats if isinstance(cats, list) else [cats]
                    for cat in cats:
                        tools[cat] = tools.get(cat, 0) + 1
                    tools["calls"] = tools.get("calls", 0) + 1
                    calls.append("+".join(cats))
                    pending[b.get("id")] = "build_check" if "build_check" in cats else cats[0]
        elif o.get("type") == "user":
            for b in msg.get("content") or [] if isinstance(msg.get("content"), list) else []:
                if isinstance(b, dict) and b.get("type") == "tool_result":
                    text = json.dumps(b.get("content"))
                    cat = pending.get(b.get("tool_use_id"))
                    if cat == "build_check" and (b.get("is_error") or re.search(r"error [EW]\d{4}|E\d{4}|error TS\d+|\.go:\d+:\d+:|SyntaxError", text)):
                        errors += 1
    return {"model": model, "usage": usage, "tools": tools, "sequence": calls, "build_errors": errors}


def main():
    m = json.loads(Path(sys.argv[1]).read_text())
    out = {}
    for key, p in sorted(m.items()):
        if Path(p).exists():
            out[key] = analyze(p)
    (HERE / "results").mkdir(exist_ok=True)
    (HERE / "results" / "transcripts.json").write_text(json.dumps(out, indent=2))
    cats = ["calls", "read_spec", "read_docs", "write", "build_check", "run_check", "bash_other", "read_other"]
    print("| run | model | input | output | cache write | cache read | " + " | ".join(cats) + " | build errors |")
    print("|---" * (7 + len(cats)) + "|")
    for k, r in out.items():
        u = r["usage"]
        print(f"| {k} | {r['model']} | {u['input']} | {u['output']} | {u['cache_create']} | {u['cache_read']} | "
              + " | ".join(str(r["tools"].get(c, 0)) for c in cats) + f" | {r['build_errors']} |")


if __name__ == "__main__":
    main()
