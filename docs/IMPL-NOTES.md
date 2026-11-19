# 구현 노트

## AST changes

- 없음 (프론트엔드는 기존 `internal/ast`, `internal/token` 계약만 사용).

## 프론트엔드 규약 (lexer/parser)

- 위치: `BinaryExpr`, `CallExpr`, `MemberExpr`, `IndexExpr`, `TryExpr`, `RangeExpr`의 `Pos`는 식의 **시작 위치**(가장 왼쪽 피연산자)다. 선언의 `Pos`는 첫 토큰(`pub` 포함), `Block.Pos`/`End`는 `{`/`}`.
- 연산자 우선순위(낮음→높음): `||`, `&&`, 비교, `??`, `..`/`..=`, `+ -`, `* / %`, 단항, 후위. `??`와 산술·비교를 괄호 없이 섞으면 파서가 `E0102`(`??` 연산자 위치).
- `T??`는 `OptionalType{OptionalType{T}}`로 만든다(E1204는 checker).
- 람다 본문이 대입이면(`x => sum = sum + x`) `BlockExpr{AssignStmt}`로 감싼다.
- 식 위치의 `{`: `{}` 또는 첫 식 뒤에 `:`가 오면 `MapLit`, 아니면 `BlockExpr`. match 갈래와 `=>` 뒤의 `{`는 블록.
- 문자열: `{`가 보간을 시작한다. 단, 보간 내용에 이스케이프(`\`)가 먼저 나오면 그 `{`는 글자 그대로다(`"{\"name\":\"a\"}"` 같은 JSON 텍스트 — 적합성 예제가 이렇게 쓴다).
- 문 시작의 식별자 `assert`는 `AssertStmt`.
- `parser.ParseWithInfo`는 포매터용 힌트(맵 중괄호, 제네릭 `<>`, 단항·효과 `!` 등)를 함께 돌려준다.
