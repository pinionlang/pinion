# 구현 노트

## AST changes

- `ast.File.Script`, `ast.FnDecl.Script`: 스크립트 모드(DESIGN 3.20). 파서가 `fn main` 없는 파일의 최상위 문과 `let`/`var`를 소스 순서대로 모아 암묵적 `FnDecl{Name: "main", Script: true}`를 만든다(최상위 `let`은 `LetDecl`에서 빠진다). 검사기는 이 main의 효과를 추론해 `FnDecl.Effects`에 채운다(outline이 읽는다).

## 프론트엔드 규약 (lexer/parser)

- 위치: `BinaryExpr`, `CallExpr`, `MemberExpr`, `IndexExpr`, `TryExpr`, `RangeExpr`의 `Pos`는 식의 **시작 위치**(가장 왼쪽 피연산자)다. 선언의 `Pos`는 첫 토큰(`pub` 포함), `Block.Pos`/`End`는 `{`/`}`.
- 연산자 우선순위(낮음→높음): `||`, `&&`, 비교, `??`, `..`/`..=`, `+ -`, `* / %`, 단항, 후위. `??`와 산술·비교를 괄호 없이 섞으면 파서가 `E0102`(`??` 연산자 위치).
- `T??`는 `OptionalType{OptionalType{T}}`로 만든다(E1204는 checker).
- 람다 본문이 대입이면(`x => sum = sum + x`) `BlockExpr{AssignStmt}`로 감싼다.
- 식 위치의 `{`: `{}` 또는 첫 식 뒤에 `:`가 오면 `MapLit`, 아니면 `BlockExpr`. match 갈래와 `=>` 뒤의 `{`는 블록.
- 문자열: `{`가 보간을 시작한다. 단, 보간 내용에 이스케이프(`\`)가 먼저 나오면 그 `{`는 글자 그대로다(`"{\"name\":\"a\"}"` 같은 JSON 텍스트 — 적합성 예제가 이렇게 쓴다).
- 문 시작의 식별자 `assert`는 `AssertStmt`.
- `parser.ParseWithInfo`는 포매터용 힌트(맵 중괄호, 제네릭 `<>`, 단항·효과 `!` 등)를 함께 돌려준다.

## 런타임 (std/http, 보조 메서드)

- `std/http`: `runtime/pinionrt/http.go`. `Request`·`Response`는 내장 `TypeInfo` 객체, `App`은 클래스 객체(상태는 Go 쪽 표). 실제 서버는 `net/http`, 핸들러 실행은 전역 잠금 `serveMu`로 직렬화한다. 테스트에서는 `netReal = false`라 `listen`이 `R0109`.
- 타입 인자가 있는 메서드 호출(`req.json<T>()`)은 `rt.MT(recv, name, []*rt.Type{...}, ...)`로 만든다. 타입 인자는 디코드에만 쓰고, 나머지 수신자는 `rt.M`으로 넘긴다.
- 표준 함수의 생략 가능한 인자 기본값은 `stdDefaults`(이름 → 인자 위치 → 값)에 둔다.
