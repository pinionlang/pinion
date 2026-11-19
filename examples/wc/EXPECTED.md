# wc — expected behaviour

Counts: lines = number of `"\n"` (same as `wc -l`), words = `text.words().len()`, chars = Unicode code points.

```sh
cd examples/wc
printf '가나 다\n라마\n' > a.txt
printf 'one two three\nfour\n' > b.txt
pinion run a.txt b.txt
```

stdout:

```
2 3 8 a.txt
2 4 19 b.txt
4 7 27 total
```

exit code 0. With one file there is no `total` row:

```sh
pinion run a.txt
```

```
2 3 8 a.txt
```

Missing file (other files are still counted; error goes to stderr; `main` returns `Err`):

```sh
pinion run a.txt nope.txt
```

stdout:

```
2 3 8 a.txt
2 3 8 total
```

stderr contains `wc: nope.txt: no such file`; exit code 1.

No arguments: stderr `usage: wc <file>...`, exit code 1.

`pinion test` → 7 tests, all `passed`.
