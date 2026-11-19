# Books CRUD API — Specification

Build an HTTP/1.1 JSON server that manages a collection of **books** in memory.

## 1. Server

- Listen on host `127.0.0.1`, port taken from the environment variable `PORT` (decimal integer).
- Storage is in-memory only; the store starts empty on every start.
- Every response that has a body is JSON, with header `Content-Type: application/json`.
- Request bodies are JSON (clients send `Content-Type: application/json`; you need not check the header).

## 2. Errors

Every error response has the body `{"error": "<message>"}` where `<message>` is any non-empty string. Only the status code and the presence of a non-empty string `error` field matter.

| Status | When |
|---|---|
| 400 | Body is not valid JSON, is not a JSON object, fails validation, has unknown fields; invalid query parameters |
| 404 | Unknown route; book id not found; id segment that is not a decimal integer |
| 405 | Known path, unsupported method |

## 3. Book

Response representation (exactly these five keys):

```json
{"id": 1, "title": "Dune", "author": "Frank Herbert", "year": 1965, "tags": ["sf", "classic"]}
```

| Field | Type | Rules |
|---|---|---|
| `id` | integer | Assigned by the server: 1, 2, 3, … in creation order. Never reused, even after deletes. Clients may not send it. |
| `title` | string | Required. After trimming leading/trailing whitespace: non-empty and at most 200 characters. Stored trimmed. |
| `author` | string | Required. After trimming: non-empty. Stored trimmed. |
| `year` | integer | Required. JSON integer (not a string, boolean, or number with a fractional part) in `0..2100` inclusive. |
| `tags` | array of strings | Optional, default `[]`. Every element must be a string. Order is preserved as given. |

Validation:
- The body must be a JSON object. Any key other than `title`, `author`, `year`, `tags` (including `id`) → 400.
- `null` is never a valid value for any field → 400.
- Wrong types → 400.

## 4. Endpoints

### POST /books
Create a book. All required fields must be present. → `201` with the created book.

```
POST /books
{"title": "  Dune ", "author": "Frank Herbert", "year": 1965}
→ 201 {"id": 1, "title": "Dune", "author": "Frank Herbert", "year": 1965, "tags": []}
```

### GET /books
→ `200` with a JSON array of books sorted by `id` ascending.

Optional query parameters (any combination):
- `author=<a>` — only books whose `author` equals `<a>` exactly (case-sensitive, after URL-decoding).
- `tag=<t>` — only books whose `tags` contain `<t>`.
- `offset=<n>` — skip the first `n` matching books (default 0).
- `limit=<n>` — return at most `n` books (default: no limit; `limit=0` returns `[]`).

Filters apply first, then `offset`, then `limit`. `limit` and `offset` must be non-negative decimal integers consisting only of digits (`-1`, `abc`, `1.5`, empty → 400). Unknown query parameters are ignored.

```
GET /books?tag=sf&limit=1&offset=1
→ 200 [{"id": 3, ...}]
```

### GET /books/{id}
→ `200` with the book, or `404` if it does not exist. `{id}` that is not a decimal integer (e.g. `/books/abc`) → 404.

### PUT /books/{id}
Full replacement. Same body rules as POST (all required fields must be present; omitted `tags` becomes `[]`). The `id` does not change.
→ `200` with the updated book; `404` if the book does not exist; `400` for an invalid body (for an existing book). If the book does not exist, respond 404 regardless of body.

### PATCH /books/{id}
Partial update: only the fields present in the body are validated and changed. An empty object `{}` is valid and changes nothing. Unknown fields or invalid values → 400 and the book is left unchanged.
→ `200` with the updated book; `404` if the book does not exist (regardless of body).

```
PATCH /books/1
{"year": 1966, "tags": ["sf"]}
→ 200 {"id": 1, "title": "Dune", "author": "Frank Herbert", "year": 1966, "tags": ["sf"]}
```

### DELETE /books/{id}
→ `204` with an empty body, or `404`.

### GET /stats
→ `200`:

```json
{"count": 3, "by_author": {"Frank Herbert": 2, "Ursula K. Le Guin": 1}, "top_tags": [["sf", 3], ["classic", 1], ["fantasy", 1]]}
```

- `count`: number of books currently stored.
- `by_author`: object mapping each author to the number of their books (key order irrelevant; authors with 0 books are absent).
- `top_tags`: up to 3 `[tag, count]` pairs, where count is the number of books whose `tags` contain that tag; sorted by count descending, then tag ascending (byte-wise string comparison). Fewer than 3 entries if fewer distinct tags exist; `[]` when there are none.

## 5. Routing

| Path | Allowed methods |
|---|---|
| `/books` | GET, POST |
| `/books/{id}` | GET, PUT, PATCH, DELETE |
| `/stats` | GET |

- Any other path (e.g. `/`, `/foo`, `/books/1/extra`) → 404.
- A path in the table with a method not listed → 405 (for `/books/{id}`, 405 applies to any id segment, even non-numeric or nonexistent).
- The query string is not part of the path.
