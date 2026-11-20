# Change request: paged list envelope

The service currently implements `SPEC.md`. Clients need the total number of matches to render paging controls, so `GET /books` changes its response shape. Everything not mentioned here stays exactly as in `SPEC.md`.

## GET /books
Respond `200` with a JSON object with exactly these four keys instead of a bare array:

| Key | Value |
|---|---|
| `items` | the array that `SPEC.md` returns today (filters, then `offset`, then `limit`; sorted by `id`) |
| `total` | number of books matching the filters (`author`, `tag`), counted **before** `offset` and `limit` are applied |
| `offset` | the effective offset: the `offset` query value as an integer, `0` when absent |
| `limit` | the `limit` query value as an integer, or `null` when absent |

```
GET /books?tag=sf&limit=1&offset=1
→ 200 {"items": [{"id": 3, ...}], "total": 3, "offset": 1, "limit": 1}

GET /books
→ 200 {"items": [], "total": 0, "offset": 0, "limit": null}
```

- `offset` past the end: `items` is `[]`, `total` is still the number of matches, `offset` is echoed as given.
- Invalid `offset`/`limit` → 400 exactly as before.
- No other endpoint changes (single-book responses, POST/PUT/PATCH/DELETE, /stats).
