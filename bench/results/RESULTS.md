# Mini PinionBench results

- date: 2026-11-20T08:02:17+0900  
- machine: Darwin arm64  
- runs per measurement: 3 (median wall time, includes process start)  
- tokens: tiktoken cl100k_base  
- versions: python `3.13.5`, go `go version go1.27.4 darwin/arm64`, node `v22.17.0`, tsc `Version 5.8.3`, pinion `pinion: unknown command "version"`

## Run time (ms, median)

| task | pinion | python | go | ts | outputs match |
|---|---:|---:|---:|---:|---|
| fib | 45 | 153 | 13 | 50 | yes |
| json_roundtrip | 73 | 92 | 31 | 65 | yes |
| loop_sum | 192 | 556 | 18 | 208 | yes |
| sort | 237 | 248 | 79 | 180 | yes |
| wordfreq | 68 | 162 | 27 | 133 | yes |

## Source size (lines / bytes / tokens)

| task | pinion | python | go | ts |
|---|---:|---:|---:|---:|
| fib | 5 / 106 / 48 | 5 / 107 / 45 | 14 / 166 / 62 | 5 / 120 / 45 |
| json_roundtrip | 23 / 647 / 223 | 15 / 540 / 184 | 44 / 990 / 341 | 21 / 715 / 242 |
| loop_sum | 5 / 77 / 37 | 4 / 83 / 41 | 11 / 156 / 64 | 5 / 107 / 51 |
| sort | 22 / 616 / 217 | 18 / 456 / 177 | 36 / 787 / 297 | 18 / 636 / 227 |
| wordfreq | 14 / 501 / 192 | 16 / 483 / 192 | 43 / 928 / 328 | 19 / 666 / 268 |
| **total** | **69 / 1947 / 717** | **58 / 1669 / 639** | **148 / 3027 / 1092** | **68 / 2244 / 833** |

Expected stdout per task is the Python output; see `results.json` for raw times and stdout.
