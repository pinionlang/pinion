# Mini PinionBench results

- date: 2026-11-20T01:20:41+0900  
- machine: Darwin arm64  
- runs per measurement: 5 (median wall time, includes process start)  
- tokens: tiktoken cl100k_base  
- versions: python `3.13.5`, go `go version go1.27.4 darwin/arm64`, node `v22.17.0`, tsc `Version 5.8.3`, pinion `pinion: unknown command "version"`

## Run time (ms, median)

| task | pinion | python | go | ts | outputs match |
|---|---:|---:|---:|---:|---|
| fib | 32 | 149 | 18 | 44 | yes |
| json_roundtrip | 76 | 94 | 39 | 65 | yes |
| loop_sum | 186 | 536 | 14 | 207 | yes |
| sort | 213 | 214 | 76 | 168 | yes |
| wordfreq | 100 | 170 | 34 | 127 | yes |

## Source size (lines / bytes / tokens)

| task | pinion | python | go | ts |
|---|---:|---:|---:|---:|
| fib | 7 / 126 / 56 | 5 / 107 / 45 | 14 / 166 / 62 | 5 / 120 / 45 |
| json_roundtrip | 26 / 721 / 248 | 15 / 540 / 184 | 44 / 990 / 341 | 21 / 715 / 242 |
| loop_sum | 7 / 105 / 48 | 4 / 83 / 41 | 11 / 156 / 64 | 5 / 107 / 51 |
| sort | 24 / 660 / 234 | 18 / 456 / 177 | 36 / 787 / 297 | 18 / 636 / 227 |
| wordfreq | 31 / 824 / 297 | 16 / 483 / 192 | 43 / 928 / 328 | 19 / 666 / 268 |
| **total** | **95 / 2436 / 883** | **58 / 1669 / 639** | **148 / 3027 / 1092** | **68 / 2244 / 833** |

Expected stdout per task is the Python output; see `results.json` for raw times and stdout.
