# Pinion v0 표준 라이브러리 API

> AI용 요약: [REFERENCE-CARD.md](REFERENCE-CARD.md)(핵심 규칙 + 자주 쓰는 API + CRUD 예제, 한 문서).

이 문서는 **검사기(시그니처)와 런타임(구현)이 함께 따르는 계약**이다. 의미 규칙은 [DESIGN.md](../docs/DESIGN.md)를 따른다.
표기: `fn 이름(인자) -> 반환 !효과`. 효과가 없으면 생략한다. `이름: T = 기본값`은 생략할 수 있는 인자다(표준 라이브러리만). `T`, `U`, `K`, `V`는 타입 매개변수다.

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
`before(sep: Str) -> Str?`, `after(sep: Str) -> Str?`, `pad_left(n: Int, ch: Str = " ") -> Str`, `pad_right(n: Int, ch: Str = " ") -> Str`(코드 포인트 `n`개가 될 때까지 채움)
연산: `+`(이어붙이기), 비교(코드 포인트 사전순)

### `List<T>` (불변)
`len()`, `is_empty()`, `get(i: Int) -> T?`, `first() -> T?`, `last() -> T?`, `[i]`(범위 밖 `R0102`),
`map<U>(f: fn(T) -> U) -> List<U>`, `filter(f: fn(T) -> Bool) -> List<T>`, `each(f: fn(T))`, `fold<U>(init: U, f: fn(U, T) -> U) -> U`,
`any(f) -> Bool`, `all(f) -> Bool`, `find(f) -> T?`, `sort() -> List<T>`(T: Ord, 안정), `sort_by<K>(f: fn(T) -> K) -> List<T>`(K: Ord),
`sum() -> T`(T는 `Int`·`Float`·`Dec`, 빈 리스트면 0), `join(sep: Str) -> Str`(T = `Str`), `contains(x: T) -> Bool`(T: Eq),
`append(x: T) -> List<T>`, `concat(o: List<T>) -> List<T>`, `reverse() -> List<T>`, `slice(start, end) -> List<T>`,
`to_set() -> Set<T>`(T: Eq+Hash), `enumerate() -> List<(Int, T)>`,
`count_by<K>(f: fn(T) -> K) -> Map<K, Int>`, `group_by<K>(f: fn(T) -> K) -> Map<K, List<T>>`(키는 처음 나온 순서), `take(n: Int) -> List<T>`, `drop(n: Int) -> List<T>`(음수 `R0102`),
`max_by<K>(f: fn(T) -> K) -> T?`, `min_by<K>(f) -> T?`(같으면 처음 원소), `sum_by<U>(f: fn(T) -> U) -> U`(빈 리스트면 0), `index_of(x: T) -> Int?`, `distinct() -> List<T>`(처음 나온 순서)

### `MutList<T>`
`push(x: T)`, `pop() -> T?`, `set(i: Int, x: T)`, `clear()`, `freeze() -> List<T>`, `remove_at(i: Int) -> T`(범위 밖 `R0102`), `insert(i: Int, x: T)`(0..=len), `retain(f: fn(T) -> Bool)` 그리고 `List`의 조회·고차 함수 전부(결과는 불변 `List`)

### `Map<K, V>` (불변, 삽입 순서)
`len()`, `is_empty()`, `get(k: K) -> V?`, `contains_key(k: K) -> Bool`, `keys() -> List<K>`, `values() -> List<V>`, `entries() -> List<(K, V)>`, `get_or(k: K, d: V) -> V`,
`map_values<U>(f: fn(V) -> U) -> Map<K, U>`, `filter(f: fn(K, V) -> Bool) -> Map<K, V>`
`for (k, v) in m`

### `MutMap<K, V>`
`set(k: K, v: V)`, `remove(k: K) -> V?`, `clear()`, `freeze() -> Map<K, V>`, `inc(k: K)`(V = `Int`; 없으면 1, 있으면 +1), `update(k: K, default: V, f: fn(V) -> V)`(없으면 `f(default)`) 그리고 `Map`의 조회 메서드(`get_or`, `map_values`, `filter`는 불변 `Map`을 돌려준다)

### `Set<T>`, `MutSet<T>` (DESIGN 3.15.2)
`Set`: `len`, `is_empty`, `contains`, `union`, `intersect`, `diff`, `sym_diff`, `is_subset`, `is_disjoint`, `to_list`, `map -> List<U>`, `filter -> Set<T>`, `each`, `any`, `all`, `fold`, `find`
`MutSet`: `add(x) -> Bool`, `remove(x) -> Bool`, `clear()`, `contains`, `len`, `freeze() -> Set<T>`

### `Range`
`a..b`, `a..=b`. `to_list() -> List<Int>`, `rev() -> Range`, `step(n: Int) -> Range`, `contains(x: Int) -> Bool`, `len() -> Int` 그리고 `List<Int>`의 조회·고차 함수(`join` 제외). 결과는 `List`: `(0..n).map(i => i * i)`

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
`fn parse(s: Str) -> Result<Json, JsonError>` — 엄격한 RFC 8259(뒤따르는 문자, `NaN`·`Infinity`, 끝 쉼표는 `syntax`, 중복 키는 `duplicate_key`, 범위를 넘는 수는 `out_of_range`)
출력은 공백 없는 한 줄 JSON이다.

동적 JSON 값(DESIGN 3.7.1). `import std/json` 뒤 `json.Json`으로 쓴다.
```
enum Json { Null, Bool(Bool), Int(Int), Float(Float), Str(Str), Arr(List<Json>), Obj(Map<Str, Json>) }
```
생성은 `json.Json.Int(3)`, `json.Json.Null`, 패턴은 이름만(`Int(n) => ...`). Frozen·Eq·Hash·Encode·Decode. `encode`는 태그 없는 일반 JSON으로 쓰고, `decode<T>`에서 `json.Json` 위치(필드 등)는 아무 JSON이나 그대로 받는다.
수: `.`·`e`·`E`가 없고 Int 범위면 `Int`, 아니면 `Float`. `Obj`는 키 삽입 순서를 지킨다.
메서드: `get(key: Str) -> Json?`(Obj가 아니거나 키가 없으면 `None`), `at(i: Int) -> Json?`, `as_str() -> Str?`, `as_int() -> Int?`(`Int`만), `as_float() -> Float?`(`Int`·`Float`), `as_bool() -> Bool?`, `as_list() -> List<Json>?`, `as_obj() -> Map<Str, Json>?`, `is_null() -> Bool`, `has(key: Str) -> Bool`(값이 `null`이어도 키가 있으면 true), `keys() -> List<Str>`(Obj가 아니면 빈 리스트), `kind() -> Str`(`"null"`, `"bool"`, `"int"`, `"float"`, `"str"`, `"arr"`, `"obj"`)
`get`, `at`, `as_*`는 `Json?` 수신자에도 쓸 수 있다(`None`이면 `None`): `body.get("user").get("name").as_str()`
검증 보조(오류는 `"필드: 문제"` 문장, 수신자가 Obj가 아니면 `"body must be a JSON object"`):
`only(keys: List<Str>) -> Result<(), Str>`(목록 밖 키 → `"unknown field: k"`),
`req_str(k: Str, min: Int = _, max: Int = _) -> Result<Str, Str>`(코드 포인트 길이), `opt_str(k, min, max) -> Result<Str?, Str>`,
`req_int(k: Str, min: Int = _, max: Int = _) -> Result<Int, Str>`, `opt_int(...) -> Result<Int?, Str>`,
`req_float(k: Str, min: Float = _, max: Float = _) -> Result<Float, Str>`(`Int`도 받음), `opt_float(...) -> Result<Float?, Str>`,
`req_bool(k: Str) -> Result<Bool, Str>`, `opt_bool(k) -> Result<Bool?, Str>`, `str_list(k: Str) -> Result<List<Str>?, Str>`.
`req_*`: 없으면 `"k: required"`. `opt_*`·`str_list`: 없으면 `Ok(None)`, `null`이나 다른 타입이면 `Err`(`"k: must be a string"`, `an integer`, `a number`, `a boolean`, `a list of strings`). 범위: `"k: must be >= 0"`, `"k: must not be empty"`(`min: 1`), `"k: must have at most 100 characters"`. `_` 기본값은 "제한 없음"이다.

### `std/http` — 효과 `!net` (DESIGN 3.21)
```
data Request  { pub method: Str, pub path: Str, pub params: Map<Str, Str>, pub query: Map<Str, Str>, pub headers: Map<Str, Str>, pub body: Str }
data Response { pub status: Int, pub body: Str, pub headers: Map<Str, Str> }
class App
```
`fn App() -> App`
`App`: `get(path: Str, h: fn(Request) -> Response)`, `post`, `put`, `patch`, `delete`(같은 시그니처, 핸들러의 효과를 전달), `handle(req: Request) -> Response`(네트워크 없이 라우팅), `listen(port: Int, host: Str = "127.0.0.1") !net`(막고 서비스, 기본은 루프백만, 핸들러는 직렬 실행)
핸들러 반환 타입은 `Response`, `Result<Response, Response>`(Err 응답을 그대로 보냄), `Result<Response, Str>`(`Err(msg)` → 400 `{"error": msg}`) 중 하나다. 셋이 아니면 `E1201`.
`Request.json<T>() -> Result<T, JsonError>`, `Response.json<T>() -> Result<T, JsonError>` (T: Decode, 본문을 디코드)
`Request.json_value() -> Result<json.Json, JsonError>`, `Response.json_value()` (`json.parse(body)`의 줄임)
`Request.json_obj() -> Result<json.Json, Str>` (파싱 실패 `"invalid json"`, Obj가 아니면 `"body must be a JSON object"`)
`fn request(method: Str, path: Str, body: Str = "", headers: Map<Str, Str> = {}) -> Request` — 테스트용. `path`의 `?` 뒤는 `query`로, 헤더 이름은 소문자로
`fn json<T>(v: T, status: Int = 200) -> Response` (T: Encode — `data`, `List<data>`, `Map<Str, V>`, `json.Json` 등, `content-type: application/json`)
`fn text(s: Str, status: Int = 200) -> Response` (`text/plain; charset=utf-8`), `fn empty(status: Int) -> Response`, `fn error(status: Int, msg: Str) -> Response` (`{"error": msg}` JSON)
경로 패턴: `"/books/:id"` → `req.params.get_or("id", "")`. 경로 없음 404 `{"error":"not found"}`, 메서드 없음 405 `{"error":"method not allowed"}`.

## 테스트 기본값 (DESIGN 3.13 규칙 9)

`io`: 출력 캡처·빈 입력, `fs`: 빈 가상 파일 시스템, `env`: 빈 인자·환경변수, `time`: `946684800`(2000-01-01T00:00:00Z), `rand`: 시드 0, `net`: 없음(`app.listen`은 `R0109`, `test "x" !net`이면 실제 네트워크).
