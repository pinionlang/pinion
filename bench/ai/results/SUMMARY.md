# CRUD 백엔드 AI 작성 비용 벤치마크 (자동 생성: summarize.py)

회차: r1, r2, r3. 원자료: `results/raw/`, `results/transcripts.json`, `results/incidents.json`.

## 채점 결과와 비용 (회차 평균, 괄호는 범위)

| 언어 | 통과 | 소스 토큰 | 출력 토큰 | 캐시 쓰기 | 캐시 읽기 | 가중 비용 | 도구 호출 | 문서 조회 | 빌드 오류 |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
| go | 74/74 74/74 74/74 | 2,057 (1,974–2,137) | 4,928 | 20,111 | 88,569 | 58,645 (51,458–72,719) | 4.0 | 0.0 | 0.0 |
| pinion | 74/74 74/74 74/74 | 1,302 (1,204–1,493) | 4,415 | 19,588 | 169,303 | 63,504 (61,423–64,969) | 6.0 | 2.0 | 0.3 |
| python | 74/74 74/74 74/74 | 1,243 (1,213–1,296) | 3,353 | 18,790 | 77,671 | 48,027 (42,870–57,192) | 3.7 | 0.0 | 0.0 |
| typescript | 74/74 74/74 74/74 | 1,474 (1,442–1,514) | 4,035 | 19,368 | 89,090 | 53,304 (36,952–67,972) | 4.0 | 0.0 | 1.0 |

- 가중 비용 = 입력×1 + 캐시 쓰기×1.25 + 캐시 읽기×0.1 + 출력×5 (입력 토큰 가격 기준 상대값). 가중치는 가정이며 `summarize.py`의 `W`에 있다.
- 도구 호출 분류는 정규식 기반 근사다(`analyze_transcripts.py`). 한 Bash 명령이 여러 분류에 동시에 들어갈 수 있다.
- 빌드 오류 = 빌드·검사 명령 결과에 컴파일 오류가 보인 횟수.

## 이전 Pinion 버전 (각 1회)

| 실행 | 출력 토큰 | 캐시 읽기 | 가중 비용 | 도구 호출 | 문서 조회 | 빌드 오류 |
|---|---:|---:|---:|---:|---:|---:|
| v1/pinion | 11,690 | 498,144 | 155,073 | 13 | 5 | 3 |
| v2/pinion | 5,507 | 238,749 | 83,111 | 8 | 2 | 1 |

## 사건 기록 (최초 결과와 재채점 분리)

- **r2/pinion, r3/pinion** — 최초: 0/1 (server did not accept connections) at compiler 01668be..d37697d. 원인: pinion build -o truncated an existing executable in place; macOS SIGKILLed the new binary (stale code-signature cache). Compiler bug, not the agents' code. 조치: commit 13df10b: write to a temp file and rename. 재채점: 74/74 each, same agent source unchanged.
- **r1-r3 pinion (v3)** — 최초: 74/74. 원인: all three agents copied the REFERENCE-CARD example's id counter workaround (MutList<Int> grown on every create, id = len()); functionally correct but unbounded growth. 조치: REFERENCE-CARD example now uses class Store { mut next_id } with a mut fn (not yet re-measured). 재채점: n/a.
- **r1-r3 (all languages)** — 최초: provenance recorded at evaluation time, not at agent run time. 원인: run_eval.py records doc hashes when grading; the Pinion agents read spec/REFERENCE-CARD.md as of commit d37697d, before the example fix. 조치: future runs must record compiler commit and doc hashes when the agent starts (to do in the run launcher). 재채점: n/a.

채점 환경: 컴파일러 `13df10b`, 모델 `bedrock-claude-opus-5-5`, 해시 {"bench/ai/crud/SPEC.md": "95c26e8998f6", "bench/ai/crud/test_api.py": "02b0c9d39a93", "spec/REFERENCE-CARD.md": "db5c977f9eb8", "spec/GUIDE.md": "e9b24066e108", "spec/STDLIB.md": "e9c2404c2c27"}
