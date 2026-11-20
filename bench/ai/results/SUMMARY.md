# CRUD 백엔드 AI 작성 비용 벤치마크 (자동 생성: summarize.py)

회차: r1, r2, r3. 원자료: `results/raw/`, `results/transcripts.json`, `results/incidents.json`.

## 채점 결과와 비용 (회차 평균, 괄호는 범위)

| 언어 | 통과 | 소스 토큰 | 출력 토큰 | 캐시 쓰기 | 캐시 읽기 | 가중 비용 | 도구 호출 | 문서 조회 | 빌드 오류 |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
| go | 74/74 74/74 74/74 | 2,057 (1,974–2,137) | nan | nan | nan | nan (-) | nan | nan | nan |
| pinion | 74/74 74/74 74/74 | 1,302 (1,204–1,493) | nan | nan | nan | nan (-) | nan | nan | nan |
| python | 74/74 74/74 74/74 | 1,243 (1,213–1,296) | nan | nan | nan | nan (-) | nan | nan | nan |
| typescript | 74/74 74/74 74/74 | 1,474 (1,442–1,514) | nan | nan | nan | nan (-) | nan | nan | nan |

- 가중 비용 = 입력×1 + 캐시 쓰기×1.25 + 캐시 읽기×0.1 + 출력×5 (입력 토큰 가격 기준 상대값). 가중치는 가정이며 `summarize.py`의 `W`에 있다.
- 도구 호출 분류는 정규식 기반 근사다(`analyze_transcripts.py`). 한 Bash 명령이 여러 분류에 동시에 들어갈 수 있다.
- 빌드 오류 = 빌드·검사 명령 결과에 컴파일 오류가 보인 횟수.

## 사건 기록 (최초 결과와 재채점 분리)

- **r2/pinion, r3/pinion** — 최초: 0/1 (서버가 연결을 받지 않음), 컴파일러 01668be..d37697d. 원인: pinion build -o가 기존 실행 파일을 그 자리에서 덮어써서 macOS가 새 실행 파일을 SIGKILL로 종료함. 에이전트 코드가 아니라 컴파일러 버그. 조치: 커밋 13df10b: 임시 파일에 쓴 뒤 이름 변경. 재채점: 각각 74/74, 에이전트 소스는 변경하지 않음.
- **r1-r3 pinion (v3)** — 최초: 74/74. 원인: 세 에이전트가 모두 참조 카드 예제의 ID 카운터 우회 패턴(생성할 때마다 MutList<Int>에 추가하고 len()을 ID로 사용)을 그대로 복사함. 동작은 맞지만 리스트가 계속 커짐. 조치: 참조 카드 예제를 class Store { mut next_id }와 mut fn 방식으로 수정(아직 재측정하지 않음). 재채점: 해당 없음.
- **r1-r3 (모든 언어)** — 최초: 재현 정보가 에이전트 실행 시점이 아니라 채점 시점에 기록됨. 원인: run_eval.py가 채점할 때 문서 해시를 기록함. Pinion 에이전트가 실제로 읽은 참조 카드는 예제를 고치기 전인 커밋 d37697d 시점의 것. 조치: 이후 실행은 start_run.py로 에이전트 시작 시점에 컴파일러 커밋과 문서 해시를 기록. 재채점: 해당 없음.

채점 환경: 컴파일러 `13df10b`, 모델 `None`, 해시 {"bench/ai/crud/SPEC.md": "95c26e8998f6", "bench/ai/crud/test_api.py": "02b0c9d39a93", "spec/REFERENCE-CARD.md": "db5c977f9eb8", "spec/GUIDE.md": "e9b24066e108", "spec/STDLIB.md": "e9c2404c2c27"}
