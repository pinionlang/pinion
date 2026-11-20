# Bug report: wrong results in list paging, PATCH and /stats

The service is supposed to implement `SPEC.md`, but users report the three problems below. Fix them so the service behaves exactly as `SPEC.md` says. Do not change any behaviour that `SPEC.md` already gets right.

## 1. Filtered lists with `offset` return the wrong books
With books 1 (author A), 2 (author B), 3 (author A), 4 (author B):

```
GET /books?author=B&offset=1
expected: [book 4]
actual:   [book 2, book 4]
```

Paging through a filtered list (`author` and/or `tag` together with `offset`) sometimes repeats books from the previous page or skips books, while `offset` without filters works.

## 2. A rejected PATCH still changes the book
```
GET   /books/1                           → {"id": 1, "title": "Dune", ..., "year": 1965, ...}
PATCH /books/1 {"title": "Changed", "year": 3000}   → 400 (correct)
GET   /books/1                           → title is now "Changed"   (expected: "Dune", book unchanged)
```

It happens for several combinations of a valid and an invalid field in the same PATCH body. According to the spec a PATCH that fails validation must leave the book completely unchanged.

## 3. `/stats` `top_tags` picks the wrong tags when counts are equal
Three books with tags `["m"]`, `["c"]`, `["q"]` (each tag used once) and one more tag `x` used by two books:

```
GET /stats
expected: "top_tags": [["x", 2], ["c", 1], ["m", 1]]
actual:   "top_tags": [["x", 2], ["q", 1], ["m", 1]]
```

Tags with the same count must be ordered by tag ascending (byte-wise), so the alphabetically first ones are kept when the list is cut to 3 entries.
