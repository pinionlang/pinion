# todo — expected behaviour

State is kept in `./todo.json` (adjacent-tagged enum JSON, one line).

```sh
cd examples/todo
rm -f todo.json
pinion run list
pinion run add buy milk
pinion run add write report
pinion run done 1
pinion run list
cat todo.json
pinion run done 42
```

stdout, in order:

```
no todos
added 1: buy milk
added 2: write report
done 1
1 [x] buy milk
2 [ ] write report
[{"id":1,"title":"buy milk","status":{"tag":"Done"}},{"id":2,"title":"write report","status":{"tag":"Open"}}]
```

The last command prints `todo: no todo with id 42` to stderr and exits 1.
Other errors: `pinion run` (no args) and unknown commands print
`todo: usage: todo add <title> | todo done <id> | todo list`; `done x` prints `todo: usage: todo done <id>`;
a corrupt `todo.json` prints `todo: corrupt store (<code> at <path>)`. All exit 1.

`pinion test` → 9 tests, all `passed` (they use `with fs: fs.fake(...)`, never the real disk).
