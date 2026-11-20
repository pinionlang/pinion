#!/usr/bin/env python3
"""Generate results/SUMMARY.md from raw records only:
results/raw/*.json (latest evaluation per round/lang), results/transcripts.json,
results/incidents.json. Nothing in the summary is typed by hand.
"""
import json, statistics
from pathlib import Path

HERE = Path(__file__).resolve().parent
R = HERE / "results"

# Relative price weights (input = 1). Assumption: cache write 1.25x, cache read 0.1x,
# output 5x input — Anthropic's usual ratios. Change here if the price sheet differs.
W = {"input": 1.0, "cache_create": 1.25, "cache_read": 0.1, "output": 5.0}


def latest_raw():
    best = {}
    for f in sorted((R / "raw").glob("*.json")):
        r = json.loads(f.read_text())
        k = (r["round"], r["lang"])
        if k not in best or r["evaluated_at"] > best[k]["evaluated_at"]:
            best[k] = r
    return best


def mean(xs):
    return statistics.mean(xs) if xs else float("nan")


def rng(xs):
    return f"{min(xs):,.0f}–{max(xs):,.0f}" if xs else "-"


def main():
    raw = latest_raw()
    tr = json.loads((R / "transcripts.json").read_text()) if (R / "transcripts.json").exists() else {}
    inc = json.loads((R / "incidents.json").read_text()) if (R / "incidents.json").exists() else []
    langs = sorted({l for (_, l) in raw})
    rounds = sorted({r for (r, _) in raw})
    out = ["# CRUD 백엔드 AI 작성 비용 벤치마크 (자동 생성: summarize.py)", ""]
    out.append(f"회차: {', '.join(rounds)}. 원자료: `results/raw/`, `results/transcripts.json`, `results/incidents.json`.")
    out.append("")
    out.append("## 채점 결과와 비용 (회차 평균, 괄호는 범위)")
    out.append("")
    out.append("| 언어 | 통과 | 소스 토큰 | 출력 토큰 | 캐시 쓰기 | 캐시 읽기 | 가중 비용 | 도구 호출 | 문서 조회 | 빌드 오류 |")
    out.append("|---|---|---:|---:|---:|---:|---:|---:|---:|---:|")
    for l in langs:
        rs = [raw[(r, l)] for r in rounds if (r, l) in raw]
        ts = [tr.get(f"{r}/{l}") for r in rounds if tr.get(f"{r}/{l}")]
        passed = " ".join(f"{x['tests']['passed']}/{x['tests']['total']}" for x in rs)
        src = [x["source"]["tokens_cl100k"] for x in rs]
        o = [t["usage"]["output"] for t in ts]
        cw = [t["usage"]["cache_create"] for t in ts]
        cr = [t["usage"]["cache_read"] for t in ts]
        cost = [sum(W[k] * t["usage"][k] for k in W) for t in ts]
        calls = [t["tools"].get("calls", 0) for t in ts]
        docs = [t["tools"].get("read_docs", 0) for t in ts]
        errs = [t["build_errors"] for t in ts]
        out.append(f"| {l} | {passed} | {mean(src):,.0f} ({rng(src)}) | {mean(o):,.0f} | {mean(cw):,.0f} | {mean(cr):,.0f} | "
                   f"{mean(cost):,.0f} ({rng(cost)}) | {mean(calls):.1f} | {mean(docs):.1f} | {mean(errs):.1f} |")
    out.append("")
    out.append("- 가중 비용 = 입력×1 + 캐시 쓰기×1.25 + 캐시 읽기×0.1 + 출력×5 (입력 토큰 가격 기준 상대값). 가중치는 가정이며 `summarize.py`의 `W`에 있다.")
    out.append("- 도구 호출 분류는 정규식 기반 근사다(`analyze_transcripts.py`). 한 Bash 명령이 여러 분류에 동시에 들어갈 수 있다.")
    out.append("- 빌드 오류 = 빌드·검사 명령 결과에 컴파일 오류가 보인 횟수.")
    others = sorted(k for k in tr if k.split("/")[0] not in rounds)
    if others:
        out.append("")
        out.append("## 이전 Pinion 버전 (각 1회)")
        out.append("")
        out.append("| 실행 | 출력 토큰 | 캐시 읽기 | 가중 비용 | 도구 호출 | 문서 조회 | 빌드 오류 |")
        out.append("|---|---:|---:|---:|---:|---:|---:|")
        for k in others:
            t = tr[k]
            c = sum(W[x] * t["usage"][x] for x in W)
            out.append(f"| {k} | {t['usage']['output']:,} | {t['usage']['cache_read']:,} | {c:,.0f} | {t['tools'].get('calls', 0)} | {t['tools'].get('read_docs', 0)} | {t['build_errors']} |")
    if inc:
        out.append("")
        out.append("## 사건 기록 (최초 결과와 재채점 분리)")
        out.append("")
        for i in inc:
            out.append(f"- **{i['run']}** — 최초: {i['initial']}. 원인: {i['cause']}. 조치: {i['fix']}. 재채점: {i['rescored']}.")
    meta = next(iter(raw.values()), {})
    if meta:
        out.append("")
        out.append(f"채점 환경: 컴파일러 `{meta.get('compiler_commit')}`, 모델 `{next(iter(tr.values()), {}).get('model')}`, 해시 {json.dumps(meta.get('hashes'))}")
    (R / "SUMMARY.md").write_text("\n".join(out) + "\n")
    print("\n".join(out))


if __name__ == "__main__":
    main()
