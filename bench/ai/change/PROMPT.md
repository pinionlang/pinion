# Change-task implementer prompt template

One run = one task `<T>` × one language. Before the agent starts, prepare its directory:

```
python3 bench/ai/start_run.py bench/ai/change/runs/<T>/<lang> --baseline <START> --task <T>
```

where `<START>` is `bench/ai/change/T3-bugfix/<lang>` for `T3-bugfix` and `bench/ai/change/baseline/<lang>` for every other task. Tasks: `T1-isbn`, `T2-persist`, `T3-bugfix`, `T4-status`, `T5-envelope`.

Substitute `<T>`, `<LANG>`, `<lang>` and paste exactly one language line at `<LANGUAGE_LINE>`. Everything else stays identical across languages and tasks.

| `<LANG>` | `<lang>` | `<LANGUAGE_LINE>` |
|---|---|---|
| Pinion | pinion | Language: Pinion. Read `spec/REFERENCE-CARD.md` first; you may also consult `spec/STDLIB.md` and `spec/GUIDE.md`. Use the compiler at `./bin/pinion` (run `./bin/pinion` with no arguments for usage). |
| Python | python | Language: Python 3 with FastAPI (already installed, with uvicorn). Do not install packages. |

---

## Prompt

You are changing an existing small HTTP backend. Work autonomously from the repository root `/Users/tj.k/orca/projects/AILang`.

The service in `bench/ai/change/runs/<T>/<lang>/` is written in <LANG> and implements `bench/ai/crud/SPEC.md`. Apply the change described in `bench/ai/change/<T>/CHANGE.md` to it. Behaviour that `CHANGE.md` does not mention must stay exactly as in `SPEC.md`.

<LANGUAGE_LINE>

Rules:
- Modify the existing code in `bench/ai/change/runs/<T>/<lang>/`; do not rewrite it from scratch, and do not modify anything outside that directory.
- Keep `RUN` (the one-line start command, executed in that directory with the env var `PORT` set) and `BUILD` (if present: the one-line build command run before `RUN`) working. Change them only if your change requires it.
- The only files you may read are `bench/ai/change/<T>/CHANGE.md`, `bench/ai/crud/SPEC.md`, the files in your run directory, and the language documentation named above. Do not read any other files under `bench/` (no other tasks, runs, baselines or tests).
- You may build and run the server and check it manually (e.g. with `curl`). You will NOT be given the grading test; it checks the change, including edge cases, and re-checks everything in `SPEC.md`.
- Do not leave a server process running, and do not leave data files or build output you created for manual testing in the run directory (build output that `BUILD` regenerates is fine).
- Stop when you believe the change is complete and correct. Reply with a one-paragraph summary.
