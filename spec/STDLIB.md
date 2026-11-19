# Pinion v0 표준 라이브러리 API

이 문서는 **검사기(시그니처)와 런타임(구현)이 함께 따르는 계약**이다. 의미 규칙은 [DESIGN.md](../docs/DESIGN.md)를 따른다.
표기: `fn 이름(인자) -> 반환 !효과`. 효과가 없으면 생략한다. `T`, `U`, `K`, `V`는 타입 매개변수다.

## 기본 제공 (prelude, import 없이 사용)

| 이름 | 시그니처 |
|---|---|
| `print` | `fn print(s: Str) !io` — 한 줄 출력(끝에 `\n`) |
| `assert` | 문(statement): `assert <Bool 식>`. 실패 시 테스트 안이면 `T0001`, 밖이면 결함으로 중단 |
| `debug` | `fn debug<T>(x: T) -> Str` — 디버그 표현(DESIGN 3.8) |
| `todo` | `fn todo(msg: Str = "") -> Never` — 결함 `R0105` |
| `unreachable` | `fn unreachable(msg: Str = "") -> Never` — 결함 `R0106` |
| `Ok`, `Err`, `Some`, `None` | `Result`·옵션 생성자이자 패턴 |
| 컬렉션 생성 | `MutList<T>()`, `MutMap<K, V>()`, `MutSet<T>()`, `Set(xs: List<T>)` (위치 인자 하나) |
| `Duration` | `Duration(ns: Int)`, `Duration(us: Int)`, `Duration(ms: Int)`, `Duration(s: Int)`, `Duration(m: Int)`, `Duration(h: Int)` — 이름 붙은 인자 하나 |
| `Round` | `enum Round { HalfEven, HalfUp, Down, Floor, Ceil }` |
| 오류 타입 | `FsError`, `JsonError`, `JsonErrorKind`, `ParseError`, `Utf8Error` (DESIGN 3.15.2). `JsonError`에는 `fn code() -> Str` |

## 내장 타입 메서드

### `Int`
`to_float() -> Float`, `to_dec() -> Dec`, `abs() -> Int`, `max(o: Int) -> Int`, `min(o: Int) -> Int`, `pow(e: Int) -> Int`
타입 이름 함수: `Int.parse(s: Str) -> Result<Int, ParseError>`
연산: `+ - * / %`(정수 나눗셈은 0 방향 버림, 0으로 나누면 `R0103`), 비교, 오버플로 `R0101`

### `Float`
`trunc_int() -> Int`, `round_int() -> Int`, `abs() -> Float`, `max(o) -> Float`, `min(o) -> Float`, `sqrt() -> Float`(음수면 `R0104`)
`Float.parse(s: Str) -> Result<Float, ParseError>`

### `Dec`
`div(b: Dec, scale: Int, round: Round) -> Dec`, `round(scale: Int, round: Round) -> Dec`, `scale() -> Int`, `to_float() -> Float`, `trunc_int() -> Int`, `abs() -> Dec`
`Dec.parse(s: Str) -> Result<Dec, ParseError>`. `/` 연산자 금지(`E1206`)

### `Str`
`len() -> Int`(코드 포인트), `byte_len() -> Int`, `is_empty() -> Bool`, `slice(start: Int, end: Int) -> Str`, `chars() -> List<Str>`,
`trim() -> Str`, `lower() -> Str`, `upper() -> Str`, `words() -> List<Str>`, `lines() -> List<Str>`, `split(sep: Str) -> List<Str>`,
`contains(sub: Str) -> Bool`, `starts_with(p: Str) -> Bool`, `ends_with(p: Str) -> Bool`, `index_of(sub: Str) -> Int?`(코드 포인트 위치),
`count(sub: Str) -> Int`, `replace(old: Str, new: Str) -> Str`, `repeat(n: Int) -> Str`, `to_bytes() -> Bytes`,
`before(sep: Str) -> Str?`, `after(sep: Str) -> Str?`
연산: `+`(이어붙이기), 비교(코드 포인트 사전순)

### `List<T>` (불변)
`len()`, `is_empty()`, `get(i: Int) -> T?`, `first() -> T?`, `last() -> T?`, `[i]`(범위 밖 `R0102`),
`map<U>(f: fn(T) -> U) -> List<U>`, `filter(f: fn(T) -> Bool) -> List<T>`, `each(f: fn(T))`, `fold<U>(init: U, f: fn(U, T) -> U) -> U`,
`any(f) -> Bool`, `all(f) -> Bool`, `find(f) -> T?`, `sort() -> List<T>`(T: Ord, 안정), `sort_by<K>(f: fn(T) -> K) -> List<T>`(K: Ord),
`sum() -> T`(T는 `Int`·`Float`·`Dec`, 빈 리스트면 0), `join(sep: Str) -> Str`(T = `Str`), `contains(x: T) -> Bool`(T: Eq),
`append(x: T) -> List<T>`, `concat(o: List<T>) -> List<T>`, `reverse() -> List<T>`, `slice(start, end) -> List<T>`,
`to_set() -> Set<T>`(T: Eq+Hash), `enumerate() -> List<(Int, T)>`

### `MutList<T>`
`push(x: T)`, `pop() -> T?`, `set(i: Int, x: T)`, `clear()`, `freeze() -> List<T>` 그리고 `List`의 조회·고차 함수(`len`, `get`, `[i]`, `map`, `filter`, `each`, `fold`, `any`, `all`, `find`, `contains`, `join`, `sum`)

### `Map<K, V>` (불변, 삽입 순서)
`len()`, `is_empty()`, `get(k: K) -> V?`, `contains_key(k: K) -> Bool`, `keys() -> List<K>`, `values() -> List<V>`, `entries() -> List<(K, V)>`
`for (k, v) in m`

### `MutMap<K, V>`
`set(k: K, v: V)`, `remove(k: K) -> V?`, `clear()`, `freeze() -> Map<K, V>` 그리고 `Map`의 조회 메서드

### `Set<T>`, `MutSet<T>` (DESIGN 3.15.2)
`Set`: `len`, `is_empty`, `contains`, `union`, `intersect`, `diff`, `sym_diff`, `is_subset`, `is_disjoint`, `to_list`, `map -> List<U>`, `filter -> Set<T>`, `each`, `any`, `all`, `fold`, `find`
`MutSet`: `add(x) -> Bool`, `remove(x) -> Bool`, `clear()`, `contains`, `len`, `freeze() -> Set<T>`

### `Range`
`a..b`, `a..=b`. `to_list() -> List<Int>`, `rev() -> Range`, `step(n: Int) -> Range`, `contains(x: Int) -> Bool`, `len() -> Int`

### `Bytes`
`len()`, `[i] -> Int`, `slice(start, end) -> Bytes`, `to_str() -> Result<Str, Utf8Error>`, `Bytes.from_list(xs: List<Int>) -> Bytes`

### `T?`
`??`, `let ... else`, `is_some() -> Bool`, `is_none() -> Bool`, `map<U>(f: fn(T) -> U) -> U?`

### `Result<T, E>`
`?`, `ok() -> T?`, `map<U>(f) -> Result<U, E>`, `map_err<F>(f: fn(E) -> F) -> Result<T, F>`, `is_ok() -> Bool`, `is_err() -> Bool`, `unwrap_or(v: T) -> T`

### `Instant`, `Duration`
`Instant`: `add(d: Duration) -> Instant`, `sub(d: Duration) -> Instant`, `since(o: Instant) -> Duration`, `unix() -> Int`
`Duration`: `plus(o) -> Duration`, `minus(o) -> Duration`, `times(n: Int) -> Duration`, `ms() -> Int`, `ns() -> Int`

## 모듈

### `std/fs` — 효과 `!fs`
`fn read(path: Str) -> Result<Str, FsError> !fs`
`fn read_bytes(path: Str) -> Result<Bytes, FsError> !fs`
`fn write(path: Str, text: Str) -> Result<(), FsError> !fs`
`fn exists(path: Str) -> Bool !fs`
`fn remove(path: Str) -> Result<(), FsError> !fs`
효과 대체(테스트 전용): `with fs: fs.fake(files: Map<Str, Str>) { ... }`

### `std/env` — 효과 `!env`
`fn args() -> List<Str> !env` (프로그램 이름 제외)
`fn get(name: Str) -> Str? !env`
효과 대체: `with env: env.fake(args: List<Str>, vars: Map<Str, Str>) { ... }` (둘 다 생략 가능)

### `std/io` — 효과 `!io`
`fn read_line() -> Str? !io` (끝의 개행 제외, 입력 끝이면 `None`)
`fn read_all() -> Str !io`
`fn eprint(s: Str) !io` (표준 오류)
효과 대체: `with io: io.fake(input: Str) { ... }`

### `std/time` — 효과 `!time`
`fn now() -> Instant !time`, `fn now_unix() -> Int !time`, `fn sleep(d: Duration) !time`
효과 대체: `with time: time.fake(at: Int) { ... }` (유닉스 초)

### `std/rand` — 효과 `!rand`
`fn int(lo: Int, hi: Int) -> Int !rand` (`lo` 이상 `hi` 미만), `fn float() -> Float !rand` ([0, 1))
효과 대체: `with rand: rand.fake(seed: Int) { ... }`

### `std/json` — 효과 없음
`fn encode<T>(v: T) -> Str` (T: Encode)
`fn decode<T>(s: Str) -> Result<T, JsonError>` (T: Decode)
출력은 공백 없는 한 줄 JSON이다.

## 테스트 기본값 (DESIGN 3.13 규칙 9)

`io`: 출력 캡처·빈 입력, `fs`: 빈 가상 파일 시스템, `env`: 빈 인자·환경변수, `time`: `946684800`(2000-01-01T00:00:00Z), `rand`: 시드 0.
