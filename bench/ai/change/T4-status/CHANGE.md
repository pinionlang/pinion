# Change request: lending status

The service currently implements `SPEC.md`. Add a lending status to books. Everything not mentioned here stays exactly as in `SPEC.md`.

## Representation
Every book in every response has a sixth key `status`, one of `"available"`, `"borrowed"`, `"lost"`:

```json
{"id": 1, "title": "Dune", "author": "Frank Herbert", "year": 1965, "tags": [], "status": "available"}
```

- A newly created book is `"available"`.
- `status` cannot be set through POST, PUT or PATCH: a body containing the key `status` (with any value) → 400, like any other unknown field.
- PUT and PATCH never change the status (PUT replaces only title/author/year/tags).

## New endpoints
All take no request body (ignore one if sent) and respond `200` with the updated book.

| Endpoint | Allowed when status is | New status |
|---|---|---|
| `POST /books/{id}/borrow` | `available` | `borrowed` |
| `POST /books/{id}/return` | `borrowed` | `available` |
| `POST /books/{id}/lost` | `available` or `borrowed` | `lost` |

- Current status not allowed for the action → `409` with `{"error": "<non-empty string>"}`, book unchanged. (`lost` is final: every action on a lost book is 409.)
- Book does not exist, or `{id}` is not a decimal integer → 404.
- Any method other than POST on these three paths → 405 (for any id segment, like `/books/{id}`).
- Other sub-paths (e.g. `/books/1/extra`, `/books/1/borrow/x`) are still 404.

Routing table additions:

| Path | Allowed methods |
|---|---|
| `/books/{id}/borrow` | POST |
| `/books/{id}/return` | POST |
| `/books/{id}/lost` | POST |

## Filter
`GET /books?status=<s>`: only books with that status. Combines with the other filters and with `offset`/`limit` like `author`/`tag` (filters first). A value other than `available`, `borrowed`, `lost` (including empty) → 400.

## Stats
`GET /stats` gains a key `by_status`: an object with exactly the three keys `available`, `borrowed`, `lost`, each the number of stored books with that status (0 included).

```json
{"count": 2, "by_author": {...}, "top_tags": [...], "by_status": {"available": 1, "borrowed": 0, "lost": 1}}
```
