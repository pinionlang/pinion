<p align="center"><img src="assets/branding/pinion-icon.svg" width="128" alt="Pinion .pi"></p>

# Pinion

AI가 작성하고 디버깅하기 쉬운 객체 지향 언어. 확장자는 `.pi`, Go로 컴파일된다.

> 상태: **설계 단계 (v0)**. 컴파일러는 아직 없다.

- [설계 문서](docs/DESIGN.md): 의미 규칙과 결정 기록
- [구현 계획](docs/PLAN.md): v0 범위와 마일스톤
- [적합성 예제](spec/conformance/): 규칙별 정상·에러·경계 사례

```pinion
data User {
  pub name: Str
  pub email: Str?
}

fn greet(u: User) -> Str {
  "hello, {u.name}"
}

fn main() !io {
  print(greet(User(name: "pinion", email: None)))
}
```

## 라이선스

[Apache License 2.0](LICENSE)
