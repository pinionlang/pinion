# bank — expected behaviour

```sh
cd examples/bank
pinion run
```

stdout:

```
transfer alice->bob 30: ok
transfer bob->alice 500: insufficient funds: balance 50, needed 500
transfer alice->alice 1: cannot transfer from alice to itself
transfer alice->carol 1: unknown account carol
deposit bob -5: invalid amount -5
open dave -1: negative balance -1
alice: 70
bob: 50
total: 120
```

exit code 0.

`pinion test` → 12 tests, all `passed`: init validation (negative balance, empty id), `with` re-running `init`,
withdraw/deposit errors, transfer success, failed transfer leaving the ledger untouched (checked through the
shared `MemoryLedger.writes` counter), same-account transfer, duplicate open, unknown account.

Defect vs failure: `Bank.total()` uses checked `Int` arithmetic, so a ledger whose balances sum past
2^63-1 aborts with `R0101` (exit code 70, runtime diagnostic pointing at `src/bank/ledger.pi`) instead of
returning an `Err`.
