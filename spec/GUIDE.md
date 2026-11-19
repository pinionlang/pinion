# Pinion core guide (for AI agents)

Pinion (`.pi`) is a small, strict, object-oriented language compiled to Go. Write it like Kotlin/Rust/TypeScript, but follow these rules exactly. Full semantics: `docs/DESIGN.md`; std API: `spec/STDLIB.md`.

## Shape of a program
```pinion
import std/fs                      // always use module-qualified names: fs.read(...)

/// Doc comments (///) describe the next declaration.
pub data User {                    // data = immutable value
  pub name: Str
  pub email: Str?                  // T? = optional; None is the empty value
}

fn main() -> Result<(), fs.FsError> !io !fs {   // effects after the return type
  let text = fs.read("users.txt")?             // ? propagates Err (Result only)
  for line in text.lines() {
    print("user {line}")                       // interpolation: Int Float Dec Bool Str Instant Duration
  }
  Ok(())                                       // no automatic Ok wrapping
}
```

## data vs class
- `data`: value, deeply immutable. Fields may only hold immutable types (no class, no interface, no `Mut*`). Has `==`, JSON, `v.with(field: x)`.
- `class`: reference with state. Changeable fields are `mut`; only `mut fn` may assign them, and only as `self.field = ...`. No `==`, no `with`, no JSON.
- `pub` = readable from other modules. `pub mut` does NOT allow outside assignment.
- A non-mut method may not call `self`'s mut methods (E3103).

## Construction
- Auto constructor with named args: `User(name: "a", email: None)`. Omitted fields use defaults.
- `init(...)` (data: params must equal the fields) validates. All construction, including `with`, goes through `init`. A fallible `init` returns `Result<Self, E>`, so the constructor returns `Result` too. Inside `init` build with `Self { a: a, b: b }`.
- Other input shapes: write a module-level `pub fn` factory. No static methods on user types.

## Failure
- Expected failure: `Result<T, E>` / `T?`. Bugs (index out of range, overflow, `todo()`): abort.
- `?` only on `Result`. Convert errors explicitly: `x.map_err(AppError.Db)?`.
- `T?`: `opt ?? default`, `let v = opt else return ...`. To treat a Result as optional: `r.ok() ?? d`.
- Unused `Result` is an error; discard with `let _ = ...`.
- Write `(a ?? 0) + 1` — mixing `??` with other operators needs parentheses.

## Effects
- Effects: `!io` (print/stdin), `!fs`, `!env`, `!time`, `!rand`. Every named function declares what its body (and callees, and callbacks passed to `map`/`each`/...) uses. Closures infer theirs.
- A pure function can be passed where `fn(..) !io` is expected, not the reverse.
- Tests run hermetically: fake io/fs/env/time/rand by default. `test "name" !fs { }` uses the real file system. Swap fakes inside tests with `with fs: fs.fake(files: {"a.txt": "hi"}) { ... }`.
- A function's `!fs` means "uses fs"; a test's `!fs` means "use the real fs".

## Enums and match
```pinion
enum Shape {
  Circle(r: Int)
  Rect(w: Int, h: Int)
}
let s = Shape.Circle(r: 2)          // construct with the type name
match s {                           // patterns use bare variant names
  Circle(r) => r * r * 3
  Rect(w, h) => w * h
}
```
- Match must be exhaustive. For a single enum, no catch-all `_`/`x` arm: list variants, combine with `A | B`.
- Guarded arms don't count toward exhaustiveness.

## Interfaces
- `data Coupon : Discount { ... }` — implementation is explicit. A `Coupon` can be passed as `Discount`. `List<Coupon>` is NOT a `List<Discount>`: convert with `xs.map(c => c)` and a type annotation.
- No inheritance, no default methods, no downcasts.

## Collections
- `List<T>`, `Map<K, V>`, `Set<T>` are immutable; `MutList`, `MutMap`, `MutSet` are mutable references; `.freeze()` copies to immutable.
- Map/Set iterate in insertion order. `xs[i]` aborts out of range; `xs.get(i)` returns `T?`.
- `map`/`filter`/`each`/`fold` call the callback once per element in order; `sort_by(key)` is stable.
- Closures capture by value; you can't assign a captured `var` — accumulate in a `MutList`/`MutMap`.

## Strings and numbers
- `Str` length/slice are in code points; no `s[i]`. `lines()`, `words()`, `split`, `trim`.
- `{` in a string starts interpolation. Write literal braces as `\{` `\}`: `"\{\"id\":1\}"` is the JSON `{"id":1}`.
- `Int` is 64-bit and overflow aborts. `Float` is always finite. `Dec` (`12.30d`) is exact; divide with `a.div(b, scale: 2, round: Round.HalfEven)`. No mixing numeric types: `n.to_float()`.

## Modules
- One file = one module. `src/todo/store.pi` is `import todo/store`, used as `store.Todo`, `store.load()`. No selective imports, no cycles. Without `pub`, a name is private to its file.

## Tools
- `pinion check --json` gives diagnostics with codes and fixes; `pinion outline` gives signatures without bodies; `pinion test`, `pinion run`, `pinion fmt`.
