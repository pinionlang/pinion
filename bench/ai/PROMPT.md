# Implementer prompt template

Substitute `<LANG>` and `<lang>` and paste exactly one of the language lines at `<LANGUAGE_LINE>`. Everything else stays identical across languages.

| `<LANG>` | `<lang>` | `<LANGUAGE_LINE>` |
|---|---|---|
| Pinion | pinion | Language: Pinion. Read `spec/GUIDE.md` and `spec/STDLIB.md` first; you may also look at `spec/conformance/` and `examples/`. Use the compiler at `./bin/pinion` (run `./bin/pinion` with no arguments for usage). |
| Python | python | Language: Python 3 with FastAPI (already installed, with uvicorn). Do not install packages. |
| Go | go | Language: Go 1.27 using only the standard library (`net/http`). No third-party modules. |
| TypeScript | typescript | Language: TypeScript on Node 22 using only the standard library (`node:http`). Compile with `npx tsc` (5.8, already available). No npm installs, no `package.json` dependencies. |

---

## Prompt

You are implementing a small HTTP backend. Work autonomously from the repository root `/Users/tj.k/orca/projects/AILang`.

Task: implement the specification in `bench/ai/crud/SPEC.md` in <LANG>.

<LANGUAGE_LINE>

Rules:
- Put all of your code in `bench/ai/runs/<lang>/` and do not modify anything outside that directory.
- Create `bench/ai/runs/<lang>/RUN` containing one line: the shell command that starts the server, executed with `bench/ai/runs/<lang>/` as the working directory and the env var `PORT` set (e.g. `./server`).
- If a build step is needed, create `bench/ai/runs/<lang>/BUILD` containing one line: the shell command that builds it, executed in the same directory. It will be run before `RUN`.
- The only other files you may read are `bench/ai/crud/SPEC.md` and the language documentation named above. Do not read other files under `bench/`.
- You may run the server and check it manually (e.g. with `curl`). You will NOT be given the grading test; it exercises everything in the spec, including edge cases.
- Do not leave a server process running when you finish.
- Stop when you believe the implementation is complete and correct. Reply with a one-paragraph summary.
