# Change request: ISBN field

The service currently implements `SPEC.md`. Add an optional, unique `isbn` field to books. Everything not mentioned here stays exactly as in `SPEC.md`.

## Representation
Every book in every response (POST, GET, PUT, PATCH, list) has a sixth key `isbn`:
- the stored ISBN string, or
- `null` if the book has no ISBN.

```json
{"id": 1, "title": "Dune", "author": "Frank Herbert", "year": 1965, "tags": [], "isbn": null}
```

## Request body
- `isbn` is now an accepted key in POST, PUT and PATCH bodies (other unknown keys are still 400).
- Valid value: a JSON string consisting of exactly 10 or exactly 13 ASCII digits `0-9`, nothing else (no hyphens, spaces, `X`; no trimming). Anything else → 400, including `null`, numbers and the empty string.
- POST: absent → the book has no ISBN (`null`).
- PUT (full replacement): absent → the book's ISBN becomes `null`.
- PATCH: absent → ISBN unchanged. (PATCH cannot remove an ISBN.)

## Uniqueness
- No two stored books may have the same ISBN. Books without ISBN never conflict.
- POST/PUT/PATCH whose `isbn` equals the ISBN of a *different* existing book → `409` with `{"error": "<non-empty string>"}`; nothing is changed and (for POST) no id is consumed, i.e. the next successful create gets the id it would have gotten without the failed request.
- Setting a book's ISBN to its own current value is not a conflict.
- Once a book is deleted (or its ISBN changed/cleared), its old ISBN is free again.
- Precedence: 404 (book missing) first, then 400 (invalid body), then 409.

## Filter
`GET /books?isbn=<s>` returns only books whose `isbn` equals `<s>` exactly. It combines with the other filters and with `offset`/`limit` like `author`/`tag` do (filters first). The value is not validated: a value that matches no book returns `[]`.
