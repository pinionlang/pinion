# Pinion reference card (read this first)

Enough to write CLI scripts and HTTP/JSON backends. Full docs: `spec/GUIDE.md`, `spec/STDLIB.md`, `docs/DESIGN.md`. Every `pinion` block below compiles (`internal/driver/refcard_test.go`).

## Rules AI gets wrong
- `let` immutable, `var` reassignable. Last expression is the value; `return` is for early exit. No `null`: `T?` with `None`/`Some(x)`.
- `data` = immutable value (`==`, JSON, `v.with(f: x)`); fields only immutable types (no class/`Mut*`). `class` = mutable reference: changeable fields `mut`, assigned only in `mut fn` as `self.f = ...`; no `==`/JSON. `pub` = visible outside the file.
- Construct with named args: `User(name: "a", email: None)`. No static methods: write a module-level `fn`.
- Effects after the return type: `fn f() -> Result<(), fs.FsError> !io !fs`. `!io` print/stdin, `!fs`, `!env`, `!time`, `!rand`, `!net`. Named functions declare all effects their body and callees use; closures and script top level infer.
- `?` only on `Result`, and the error types must match: `r.map_err(e => "...")?`. No auto `Ok` wrap: end with `Ok(x)`. Result -> optional: `r.ok()`.
- `T?`: `opt ?? d`, `let x = opt else { return ... }` (else must leave). `let ... else` does not accept `Result` (use `.ok()`). Parenthesize `(a ?? 0) + 1`.
- Unused `Result` is an error: `let _ = r`.
- `match` is exhaustive; enum arms use bare variant names (`Circle(r) =>`), combine `A | B`; no `_` arm for a single enum. Construct variants qualified: `Shape.Circle(r: 2)`, `json.Json.Int(3)`.
- Strings: `"hi {name}"` interpolates Int/Float/Dec/Bool/Str only (others: `debug(x)`, `.len()`, `?? "-"`). Literal braces: `\{` `\}`. No `s[i]`.
- No mixing numbers: `n.to_float()`. Int overflow / `xs[i]` out of range abort; `xs.get(i)` is `T?`.
- Closures can't assign captured `var`s: accumulate in `MutList`/`MutMap`. No tuple `.0`: destructure `let (a, b) = t`, `for (k, v) in m`.
- Modules: one file = one module; `import std/json` then `json.parse(...)` (always qualified). `src/a/b.pi` is `import a/b`, used as `b.X`. No cycles.
- Script mode: a single file without `fn main` runs top-level statements in order.
- Tools: `pinion run f.pi`, `pinion test`, `pinion check --json`, `pinion fmt`.

```pinion
import std/env

enum Shape {
  Circle(r: Int)
  Rect(w: Int, h: Int)
}

data Item {
  pub name: Str
  pub qty: Int = 1
}

class Counter {
  mut n: Int = 0
  pub mut fn add(k: Int) { self.n = self.n + k }
  pub fn get() -> Int { self.n }
}

fn area(s: Shape) -> Int {
  match s {
    Circle(r) => r * r * 3
    Rect(w, h) => w * h
  }
}

fn parse_qty(s: Str) -> Result<Int, Str> {
  let n = Int.parse(s.trim()).map_err(e => "bad qty: {s}")?
  if n < 0 { return Err("negative") }
  Ok(n)
}

let name = env.args().first() ?? "world"
let c = Counter()
c.add(area(Shape.Rect(w: 2, h: 3)))
let it = Item(name: name).with(qty: parse_qty(" 4").unwrap_or(0))
print("hi {it.name} x{it.qty} \{{c.get()}\}")
```

## Prelude
`print(s) !io`, `debug(x) -> Str`, `todo()`, `unreachable()`, `assert cond`, `Ok Err Some None`, `MutList<T>()`, `MutMap<K, V>()`, `MutSet<T>()`, `Set(xs)`, `Duration(ms: 5)`. Test: `test "name" { assert ... }`.

## Types (methods)
- `Int`: `Int.parse(s) -> Result<Int, ParseError>`, `to_float to_dec abs max(o) min(o) pow(e)`; `/` truncates.
- `Float`: `Float.parse(s)`, `trunc_int round_int abs max min sqrt`. `Dec` (`1.50d`): `Dec.parse`, `div(b, scale:, round: Round.HalfEven)`, `round(scale:, round:)`.
- `Str`: `len is_empty trim lower upper chars words lines split(sep) contains(s) starts_with(p) ends_with(p) index_of(s) -> Int? count(s) replace(old, new) repeat(n) slice(a, b) before(sep) -> Str? after(sep) -> Str? pad_left(n, ch) pad_right(n, ch)`; `+` concat.
- `List<T>`: `len is_empty get(i) -> T? first last [i] map filter each fold(init, f) any all find -> T? sort sort_by(f) sum sum_by(f) join(sep) contains(x) index_of(x) append(x) concat(xs) reverse slice(a, b) take(n) drop(n) distinct enumerate -> List<(Int, T)> count_by(f) -> Map<K, Int> group_by(f) -> Map<K, List<T>> max_by(f) min_by(f) -> T? to_set`.
- `MutList<T>`: `push(x) pop() -> T? set(i, x) insert(i, x) remove_at(i) -> T retain(f) clear freeze -> List<T>` + all List reads.
- `Map<K, V>` (insertion order): `len is_empty get(k) -> V? get_or(k, d) contains_key(k) keys values entries map_values(f) filter((k, v) => b)`; `for (k, v) in m`.
- `MutMap<K, V>`: `set(k, v) remove(k) -> V? clear inc(k) update(k, default, f) freeze` + Map reads.
- `Set<T>`: `len contains union intersect diff is_subset to_list map filter`; `MutSet`: `add(x) -> Bool remove(x) -> Bool contains len freeze`.
- `T?`: `?? is_some is_none map(f)`. `Result<T, E>`: `? ok() map(f) map_err(f) is_ok is_err unwrap_or(v)`.
- Ranges: `0..n`, `1..=n`; `(0..n).map(i => i * i)` gives a `List`.

## Modules
- `std/fs` `!fs`: `read(path) -> Result<Str, FsError>`, `write(path, text) -> Result<(), FsError>`, `exists(path) -> Bool`, `remove(path) -> Result<(), FsError>`, `read_bytes(path)`.
- `std/env` `!env`: `args() -> List<Str>` (no program name), `get(name) -> Str?`.
- `std/io` `!io`: `read_line() -> Str?`, `read_all() -> Str`, `eprint(s)`.
- `std/time` `!time`: `now() -> Instant`, `now_unix() -> Int`, `sleep(d)`. `std/rand` `!rand`: `int(lo, hi)`, `float()`.
- `std/json`: `encode(v) -> Str` (data, List, Map, Json), `decode<T>(s) -> Result<T, JsonError>`, `parse(s) -> Result<json.Json, JsonError>`.
- `json.Json` = `Null | Bool(Bool) | Int(Int) | Float(Float) | Str(Str) | Arr(List<Json>) | Obj(Map<Str, Json>)`. Reads (also on `Json?`): `get(k) -> Json?`, `at(i) -> Json?`, `as_str as_int as_float as_bool as_list as_obj -> T?`; `has(k) is_null keys kind`.
- Validation (Err = `"field: problem"` message; non-object -> `"body must be a JSON object"`):
  `only(keys: List<Str>) -> Result<(), Str>` (unknown key -> Err),
  `req_str(k, min:, max:) -> Result<Str, Str>` (length bounds optional), `opt_str(k, min:, max:) -> Result<Str?, Str>`,
  `req_int(k, min:, max:)`, `opt_int`, `req_float(k, min:, max:)` (accepts Int), `opt_float`, `req_bool(k)`, `opt_bool(k)`,
  `str_list(k) -> Result<List<Str>?, Str>`. `opt_*`: missing -> `Ok(None)`; `null` or wrong type -> Err.
- `std/http` `!net`: `http.App()`; `app.get/post/put/patch/delete(path, req => ...)`; `app.listen(port, host: "127.0.0.1")` (default loopback; `host: "0.0.0.0"` for all); `app.handle(http.request("POST", "/x", body: "..."))` routes without network (tests).
- Responses: `http.json(v, status: 201)`, `http.text(s)`, `http.empty(204)`, `http.error(404, "msg")` (`{"error":"msg"}`).
- Request: `req.method path params query headers body` (Map fields: `req.params.get_or("id", "")`); `req.json<T>()`, `req.json_value() -> Result<json.Json, JsonError>`, `req.json_obj() -> Result<json.Json, Str>` (Err "invalid json" / not an object).
- A handler returns `Response`, or `Result<Response, Response>` (Err sent as is), or `Result<Response, Str>` (Err(msg) -> 400 `{"error": msg}`) so `?` works. In a Result handler every exit is `Ok(...)`/`Err(...)`.
- Routes: `"/items/:id"`, literal segments beat params; unknown path 404, wrong method 405. Handlers run one at a time (shared `MutMap` state is safe). Tests have no network.

## Example: JSON CRUD service
```pinion
import std/http

data Todo {
  pub id: Int
  pub title: Str
  pub done: Bool
}

fn app() -> http.App {
  let todos = MutMap<Int, Todo>()
  let next = MutList<Int>()
  let a = http.App()
  a.get("/todos", req => http.json(todos.values()))
  a.post("/todos", req => {
    let b = req.json_obj()?
    b.only(["title", "done"])?
    next.push(next.len() + 1)
    let t = Todo(id: next.len(), title: b.req_str("title", min: 1, max: 100)?, done: b.opt_bool("done")? ?? false)
    todos.set(t.id, t)
    Ok(http.json(t, status: 201))
  })
  a.patch("/todos/:id", req => {
    let t = todos.get(Int.parse(req.params.get_or("id", "")).ok() ?? 0) else { return Ok(http.error(404, "not found")) }
    let b = req.json_obj()?
    b.only(["title", "done"])?
    let u = t.with(title: b.opt_str("title", min: 1)? ?? t.title, done: b.opt_bool("done")? ?? t.done)
    todos.set(u.id, u)
    Ok(http.json(u))
  })
  a.delete("/todos/:id", req => {
    let _ = todos.remove(Int.parse(req.params.get_or("id", "")).ok() ?? 0) else { return http.error(404, "not found") }
    http.empty(204)
  })
  a
}

fn main() !net {
  app().listen(8080)
}

test "crud" {
  let a = app()
  assert a.handle(http.request("POST", "/todos", body: "\{\"title\":\"x\"\}")).status == 201
  assert a.handle(http.request("POST", "/todos", body: "\{\"title\":\"\"\}")).body == "\{\"error\":\"title: must not be empty\"\}"
  assert a.handle(http.request("PATCH", "/todos/1", body: "\{\"done\":true\}")).body == "\{\"id\":1,\"title\":\"x\",\"done\":true\}"
  assert a.handle(http.request("DELETE", "/todos/1")).status == 204
  assert a.handle(http.request("DELETE", "/todos/1")).status == 404
}
```
