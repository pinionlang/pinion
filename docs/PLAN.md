# Pinion v0 구현 계획 (v0.5)

> 1차 목표: **작은 CLI 프로그램을 만들고, 구조화된 피드백으로 고치는 전체 흐름**
> 의미 규칙과 근거는 [DESIGN.md](DESIGN.md)에 있다. 이 계획은 DESIGN.md v0.5를 기준으로 한다.

## 1. v0의 완료 정의

다음 세 프로그램을 Pinion으로 작성할 수 있고, 각각에 대해 `pinion run`, `pinion test`, `pinion check --json`이 동작하면 v0가 끝난 것이다.

| # | 인수 프로그램 | 검증하는 기능 |
|---|---|---|
| A1 | **wc**: 파일들의 줄·단어·글자 수를 세는 CLI. 줄 = `"\n"` 개수(Unix `wc -l`과 같음), 단어 = `words()` 결과 수, 글자 = 유니코드 코드 포인트 수 | `!env`·`!fs`·`!io` 효과, `MutMap`과 `freeze`, 문자열 단위 |
| A2 | **todo**: 할 일을 JSON 파일에 저장하는 CLI (`add`, `done`, `list`) | `data`, enum과 match, `Result`·`?`·`map_err`, JSON, 테스트에서 `fs.fake` |
| A3 | **bank**: 계좌·이체 도메인 로직과 테스트 | `init` 검증, `with`, interface와 합성, `class`의 `mut fn`, 결함 중단과 런타임 진단 |

품질 기준:
- 컴파일 진단과 런타임 진단이 모두 `.pi` 파일의 줄·열을 가리킨다.
- **핵심 가이드 `spec/GUIDE.md`가 3천 토큰 이하**이고, 경계 조건은 **`spec/REFERENCE.md`**에 모두 적혀 있다.
- 미니 PinionBench(과제 5개)를 DESIGN.md 8장의 측정 규칙으로 돌린 결과를 기록한다.

## 2. 범위

### 포함

| 영역 | 항목 |
|---|---|
| 바인딩 | `let`, `var` |
| 타입 | `Int`(64비트, 오버플로 검사), `Float`(유한), `Bool`, `Str`, `()`, `Never`, `Dec`, `Bytes`, `Instant`, `Duration`, `Range`, `List`, `Map`, `Set`, `MutList`, `MutMap`, `MutSet`, 튜플, `T?`, `Result<T, E>`, `type` 별칭, 표준 오류 타입(`FsError`, `JsonError`, `ParseError`, `Utf8Error`) |
| 선언 | `fn`, `data`, `class`(`mut` 필드, `mut fn`), `init`, `with`, `interface`(**기본 메서드 없음**, 명시 구현), `enum`, `pub` |
| 제네릭 | 제네릭 함수와 타입, 내장 제약 `Eq`·`Ord`·`Hash`만 |
| 식·문 | `if`, `match`(완전성 검사, `_` 규칙), `for`, `while`, `let ... else`, `??`, 클로저(값 캡처), 문자열 보간, `?`(`Result`만) |
| 실패 | `Result` 미사용 에러, 결함 중단(종료 코드 70), `main`의 `Result` 반환 |
| 효과 | `!io`, `!fs`, `!env`, `!time`, `!rand`. 함수는 명시, 클로저는 추론, 표준 고차 함수는 효과 전달, interface 호환 검사 |
| 모듈 | 파일 하나가 모듈 하나. `import`, `pub`, 비공개 필드가 있는 타입의 생성 제한 |
| 테스트 | `test "이름" { }`, `assert`(구조적 diff), `with fs: fs.fake(...) { }` 같은 효과 대체 |
| 표준 라이브러리 | `io`, `fs`, `env`, `json`, `str`, `list`, `map`, `time`, `test` |
| 주석 | `//` 일반 주석, `///` 문서 주석(outline에 노출). 블록 주석 없음 |
| 진단 | 사람용과 `--json` 출력(`pinion.diag/1`), 진단 코드 체계, 수정 제안(`auto`·`review`·`scaffold`, 범위와 `file_hash` 포함) |
| 런타임 진단 | 위치, 메시지, 함수 이름 스택. 값 스냅샷은 `--snapshot`으로 켠다. `pinion test`에서는 **실제 효과를 선언하지 않은 테스트에만** 기본으로 켠다 |
| 테스트 실행 | 테스트마다 별도 프로세스, 순차 실행, 제한 시간 10초, 실패해도 나머지 계속 실행(`pinion.test/1`), 기본 가짜 효과로 밀폐 |
| CLI | `pinion run`, `pinion test`, `pinion check [--json]`, `pinion build [--emit go]`, `pinion fmt`, `pinion outline` |

### 제외 (v0 이후)

HTTP·웹, DB와 `sql""`, `Secret`, 크기가 정해진 정수, `Char`, 정렬된 맵·집합, 날짜·시간대, 동시성(`spawn`/`await`), capability, `extern`(Go 연동), interface 기본 메서드, 위임 문법, 사용자 정의 고차 함수의 효과 다형성, 사용자 정의 제네릭 제약, 계약, 애너테이션, 아키텍처 규칙, LSP, `pinion query`, `pinion trace`, 패키지 관리자

## 3. 아키텍처

컴파일러는 Go로 작성한다. 생성된 Go 코드는 로컬 Go 툴체인으로 빌드한다. 툴체인 번들링은 v0 이후에 한다.

```
.pi ─▶ lexer ─▶ parser ─▶ AST ─▶ resolve ─▶ check ─▶ IR ─▶ codegen ─▶ .go ─▶ go build
                              (이름 해석)  (타입·효과·mut·match)  (의미 확정)  (옮기기만)
          └─────────── diag: 모든 단계의 진단을 같은 형식으로 수집, 사람용·JSON 출력 ───────────┘
```

| 단계 | 핵심 결정 |
|---|---|
| lexer | 모든 토큰에 위치 정보(파일, 줄, 열, 오프셋)를 붙인다 |
| parser | **직접 작성한 재귀 하강 + Pratt 파서**. 에러를 만나도 복구해서 한 번에 여러 진단을 보고한다 |
| resolve | 이름 해석, import, `pub`, 비공개 필드가 있는 타입의 생성 제한 |
| check | 타입 추론(지역), `data`의 깊은 불변 검사, `mut` 검사, null 안전, `Result` 규칙, match 완전성과 `_` 규칙, 효과 검사와 추론 |
| **IR** | 검사를 통과한 프로그램의 의미를 확정한 중간 표현이다. 타입, 효과, 오버플로 검사 지점, 결함 지점이 모두 명시된다. **백엔드는 IR만 본다** |
| codegen | IR을 Go로 옮긴다. `//line file.pi:L:C`를 넣고, enum은 sealed interface로, `data`는 값 구조체로, `class`는 포인터로 바꾼다 |
| runtime (`pinionrt`) | `Option`, `Result`, 오버플로 검사 연산, 결함 중단과 런타임 진단, 디버그 표현, JSON, 효과 대체용 컨텍스트 |
| driver | 임시 디렉터리에 Go 모듈을 만들고 빌드한다. 내용 해시로 빌드를 캐시한다 |

### 저장소 구조

```
cmd/pinion/            CLI 진입점
internal/
  lexer/ parser/ ast/
  resolve/ check/
  ir/                검사된 중간 표현
  codegen/           IR → Go
  diag/              진단 모델, 코드 목록, JSON 출력
  driver/            빌드, 캐시, go build 실행
  format/            pinion fmt
  outline/           pinion outline
runtime/pinionrt/      생성 코드가 import하는 Go 런타임
std/                 표준 라이브러리 (.pi와 Go 구현)
spec/
  conformance/       규칙별 적합성 예제 (정상·에러·경계 사례). 하네스가 직접 읽는다
  GUIDE.md           핵심 작성 가이드 (3천 토큰 이하)
  REFERENCE.md       전체 의미 규칙과 경계 조건
testdata/            적합성 예제 밖의 추가 골든 테스트 (진단 JSON 전체 형식, 생성된 Go 코드 등)
examples/            인수 프로그램 A1~A3
bench/               미니 PinionBench
docs/                DESIGN.md, PLAN.md
```

### 진단 코드 체계

| 범위 | 분류 |
|---|---|
| `E0xxx` | 문법 |
| `E1xxx` | 이름, 타입, 타입 속성, 생성, 모듈 |
| `E2xxx` | match, 흐름, 실패 |
| `E3xxx` | 효과, 불변성, `mut` |
| `Wxxxx` | 경고 |
| `R0xxx` | 런타임 결함 |
| `T0xxx` | 테스트 결과 |

전체 코드 목록의 기준은 [`spec/conformance/README.md`](../spec/conformance/README.md)다. 새 코드는 적합성 예제와 함께 추가한다.

## 4. 마일스톤

각 마일스톤은 **골든 테스트가 통과해야** 완료로 본다.

### M0. 준비와 의미 명세 (코드보다 먼저)
- Go 설치(`brew install go`, 1.23 이상), 모듈 초기화, CI(GitHub Actions: `go test ./...`)
- **적합성 예제** (`spec/conformance/`): 초안 11개 파일 작성 완료(2026-11-20). 남은 영역(제네릭, 문자열 보간, `for`·`while`, 디버그 표현)을 추가한다
- **적합성 하네스**: `//# case`, `//# expect`, `//~` 형식을 읽어 사례별로 컴파일·실행하고 비교한다. 구현이 없어서 처음에는 모두 실패한다
- `testdata/` 골든 하네스: 진단 JSON 전체, 생성된 Go 코드를 비교한다. `-update` 플래그로 기대값을 갱신한다
- **`spec/REFERENCE.md` 초안**: DESIGN.md 3장 규칙을 경계 조건까지 적고, 예제는 적합성 예제에서 가져온다

**완료**: 적합성 예제와 REFERENCE를 리뷰한다. 두 하네스가 동작한다.

### M1. 문법 분석
- lexer, parser, AST. 에러를 복구해서 여러 진단을 보고한다
- 주석 처리: `//`는 버리고, `///`는 다음 선언에 붙인다(`W0201`)
- `diag` 패키지와 `pinion check --json` (문법 진단까지)

**완료**: 두 가지를 따로 확인한다.
- **정상 입력**: 의도적인 문법 오류가 없는 적합성 사례가 모두 문법 진단 없이 파싱된다.
- **문법 오류 입력**: `E0xxx`·`W0201`을 기대하는 사례가 **정확히 그 진단을 그 위치에서** 내고, 복구 후 엉뚱한 진단을 추가로 내지 않는다.

### M2. 최소 실행 경로 (끝에서 끝까지)
- 함수, `let`·`var`, `if`, `Int`·`Str`·`Bool`, 문자열 보간, `print`
- 최소 resolve와 check, **IR 최초 버전**, Go 생성, `//line` 매핑, `pinion run`, `pinion build --emit go`
- 런타임: 오버플로 검사, 결함 중단과 `R` 코드 진단

**완료**: hello world와 피보나치가 실행된다. 오버플로 진단이 `.pi` 위치를 가리킨다.

> M2를 일찍 두는 이유: 파서부터 실행까지 전체 경로를 먼저 뚫어 두면, 이후 기능은 각 단계에 하나씩 끼워 넣으면 된다.

### M3. 실패와 컬렉션
- `T?`, `??`, `let ... else`, `Result`, `?`, `map_err`, 미사용 `Result` 에러
- `List`, `Map`, `MutList`, `MutMap`, `freeze`, `for`·`while`
- 클로저와 값 캡처 규칙, 최소 제네릭

**완료**: 적합성 파일 `08-failure-tests`(테스트 실행 사례 제외)와 `11-strings`의 사례가 통과한다. `T?`를 `T`처럼 쓰면 `fix` 제안이 함께 나온다. **첫 번째 비용 측정**(4.1)을 기록한다.

### M4. 객체 모델
- `data`(깊은 불변 검사, `with`, 구조적 `==`, JSON), `class`(참조, `mut` 필드, `mut fn`)
- `init`(실패 가능한 `init`과 그에 따른 생성자·`with`의 `Result` 반환), 비공개 필드가 있는 타입의 생성 제한
- `interface`(명시 구현, 호환 규칙), `enum`과 `match` 완전성, `_` 규칙
- 디버그 표현(깊이·크기 제한, 순환 표시)

**완료**: 적합성 파일 `01-frozen`, `02-mut`, `03-construct`, `06-match`, `10-interface-subtyping`의 모든 사례와 `04-properties-json`의 Eq·Map 사례가 통과한다. (DESIGN.md의 예제는 정의를 생략한 설명용이라 완료 조건으로 쓰지 않는다.)

### M5. 효과와 표준 라이브러리
- 함수 효과 검사(`E3001`, `W3002`), 클로저 효과 추론, 표준 고차 함수의 효과 전달, interface 구현의 효과 호환
- `io`, `fs`, `env`, `json`, `str`, `list`, `map`, `time`

**완료**: 적합성 파일 `05-eval-effects`(테스트 실행 사례 제외), `04-properties-json`의 JSON 사례, `07-modules`가 통과한다. 인수 프로그램 A1(wc)과 A2(todo)가 동작한다.

### M6. 테스트와 런타임 진단
- `test` 블록, `pinion test`(필터, `--json` 결과), `assert` 실패 시 구조적 diff
- 테스트마다 프로세스 격리, 순차 실행, 제한 시간, 실패 후에도 계속 실행
- **밀폐 테스트**: 테스트 기본 효과는 모두 가짜(`io` 캡처, 빈 `fs`·`env`, 고정 `time`·`rand`). 테스트에 선언한 효과만 실제 구현
- 효과 대체(`with fs: fs.fake(...) { }`): 테스트 전용, 간접 호출에도 적용, 안쪽 우선, 블록을 벗어나면 바깥 구현으로 복구
- 런타임 진단: 기본은 위치와 스택만. `--snapshot`을 주거나, 실제 효과를 선언하지 않은 테스트에서는 `self`와 인자 스냅샷 포함

**완료**: 모든 적합성 파일의 모든 사례가 통과한다. 인수 프로그램 A3(bank)와 그 테스트가 동작한다. 실패한 테스트의 JSON 결과에 기대값, 실제값, diff가 들어간다.

### M7. 도구와 검증
- `pinion fmt`(정답이 하나인 포맷, 주석과 문서 주석의 연결 보존), `pinion outline`(문서 주석, 시그니처, 효과, `mut`, `_`가 흡수하는 조합, `main`의 효과 상한)
- `spec/GUIDE.md` 작성과 토큰 수 측정. AI가 헷갈리기 쉬운 다음 항목은 가이드에 반드시 넣는다
  - 함수의 `!fs`(사용 표시)와 테스트의 `!fs`(실제 구현 선택)의 차이
  - `pub`(외부 읽기)과 `mut`(내부 대입)의 차이. `pub mut`는 외부 쓰기가 아니다
  - `data`(값)와 `class`(참조)의 선택 기준
- 미니 PinionBench: 과제 5개. 제품 평가(Python과 비교)와 원인 분석(JSON 진단과 outline을 끄고 비교)을 모두 돌린다. 4.1에서 쌓은 측정 기록과 함께 정리한다

**완료**: 1장의 완료 정의를 모두 충족한다.

### 4.1 지속 측정 (M2부터)

AI 작성 비용 검증을 M7에 몰면, 언어를 다 만든 뒤에야 가설이 틀렸다는 것을 알게 될 수 있다. 그래서 **M2부터 마일스톤마다** 작은 측정을 돌린다.

- 그 시점에 구현된 기능만으로 풀 수 있는 작은 과제 3개를 고른다(예: M2는 피보나치·FizzBuzz, M3은 단어 세기·옵션 처리).
- 과제마다 ① 처음부터 작성 ② 미리 심어 둔 버그 고치기를 AI에게 시킨다. 같은 과제를 Python으로도 시킨다.
- 기록할 것: 입출력 토큰, 첫 시도 통과율, 수정 반복 횟수, 진단을 받고 고친 횟수. DESIGN.md 8장의 측정 규칙(모델·설정 고정, 반복 실행)을 그대로 따른다.
- 해당 기능이 구현된 시점부터는 DESIGN.md 8.1의 규칙별 지표(`E3105` 관련, 밀폐 테스트 관련)도 함께 기록한다.
- 결과는 `bench/log/`에 마일스톤별로 남긴다. **Pinion이 Python보다 확연히 나쁜 지표가 나오면** 다음 마일스톤에 들어가기 전에 원인(문법, 진단, 가이드)을 살펴보고 설계를 조정한다.

## 5. 테스트 전략

- **골든 테스트가 기본**이다. 입력 `.pi`, 기대 stdout, 기대 진단 JSON을 비교한다. 진단 형식이 곧 제품이라서 형식 자체를 회귀 테스트한다.
- **REFERENCE의 예제가 곧 테스트다.** 명세와 구현이 어긋나지 않게 한다.
- 단계별 단위 테스트: lexer와 parser는 표 기반 테스트로 검증한다.
- 생성된 Go 코드는 `gofmt`와 `go vet`을 통과해야 한다.
- 인수 프로그램 A1~A3를 CI에서 매번 빌드하고 실행한다.

## 6. 위험과 대응

| 위험 | 대응 |
|---|---|
| `data`와 `class` 구분을 AI가 헷갈림 | 진단 메시지에서 어느 쪽을 써야 하는지 `fix`로 제안한다. PinionBench에서 관련 오류 빈도를 잰다 |
| `MutMap`과 `freeze` 패턴이 장황해서 토큰이 늘어남 | A1에서 실제 코드 길이를 보고, 필요하면 표준 라이브러리 헬퍼(`count_by` 등)를 추가한다 |
| 효과 검사가 너무 엄격해서 AI가 오히려 자주 실패함 | 미니 PinionBench로 측정한다. `fix` 제안으로 빠진 효과를 바로 추가할 수 있게 한다 |
| 핵심 가이드가 3천 토큰을 넘음 | 마일스톤마다 잰다. 경계 조건은 REFERENCE로 옮기고 가이드에는 핵심만 남긴다 |
| enum과 `Result`를 옮긴 Go 코드가 느리거나 장황함 | M2~M4에서 생성 코드를 리뷰하고 간단한 벤치마크로 확인한다 |
| `go build`가 느려서 실행 피드백이 늦음 | 내용 해시 기반 캐시를 둔다 |
| 구현하면서 의미 규칙이 계속 바뀜 | 바뀐 규칙은 REFERENCE와 DESIGN.md 10장 결정 기록에 바로 반영한다 |

## 7. 시작 전에 정할 것

- [ ] Go 설치 진행 (`brew install go`)
- [ ] 최소 Go 버전: 1.23 이상 제안
- [ ] GitHub 저장소 위치: `pinionlang` 조직 또는 개인 계정
- [ ] 라이선스: MIT 또는 Apache-2.0
