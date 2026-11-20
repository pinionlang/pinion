#!/usr/bin/env python3
"""Generate results/SUMMARY.md for the change benchmark from raw records only."""
import json
from pathlib import Path
R = Path(__file__).resolve().parent / "results"
W = {"input": 1.0, "cache_create": 1.25, "cache_read": 0.1, "output": 5.0}  # assumed price ratios
best = {}
for f in sorted((R / "raw").glob("*.json")):
    r = json.loads(f.read_text()); k = (r["task"], r["lang"])
    if k not in best or f.name > best[k][0]: best[k] = (f.name, r)
tr = json.loads((R / "transcripts.json").read_text())
rows, tot = [], {}
for (t, l), (_, r) in sorted(best.items()):
    u = tr[f"{t}/{l}"]["usage"]; c = sum(W[x] * u[x] for x in W); tot.setdefault(l, [0, 0, 0])
    tot[l][0] += c; tot[l][1] += r.get("diff", {}).get("changed_tokens_cl100k", 0); tot[l][2] += 1
    d = r.get("diff", {})
    rows.append(f"| {t} | {l} | {r['tests']['passed']}/{r['tests']['total']} | {d.get('changed_tokens_cl100k','-')} | {u['output']:,} | {u['cache_read']:,} | {c:,.0f} | {tr[f'{t}/{l}']['tools'].get('calls',0)} | {tr[f'{t}/{l}']['tools'].get('read_docs',0)} | {tr[f'{t}/{l}']['build_errors']} |")
out = ["# 수정 과제 벤치마크 (자동 생성: summarize_change.py)", "",
       "| 과제 | 언어 | 테스트 | 변경 토큰 | 출력 토큰 | 캐시 읽기 | 가중 비용 | 도구 호출 | 문서 조회 | 빌드 오류 |",
       "|---|---|---|---:|---:|---:|---:|---:|---:|---:|", *rows, "",
       "| 언어 | 가중 비용 합계 | 변경 토큰 합계 |", "|---|---:|---:|",
       *[f"| {l} | {v[0]:,.0f} | {v[1]:,} |" for l, v in sorted(tot.items())], "",
       "- 가중 비용 = 입력×1 + 캐시 쓰기×1.25 + 캐시 읽기×0.1 + 출력×5 (가정한 가격 비율).",
       "- 과제·언어당 1회. 원자료: `results/raw/`, `results/transcripts.json`, 실행 시작 시 재현 정보는 각 `runs/<T>/<lang>/META.json`."]
(R / "SUMMARY.md").write_text("\n".join(out) + "\n"); print("\n".join(out))
