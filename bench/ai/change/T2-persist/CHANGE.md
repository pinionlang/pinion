# Change request: persist books to a file

The service currently implements `SPEC.md` and keeps everything in memory. Make the data survive restarts. Everything not mentioned here stays exactly as in `SPEC.md` (this replaces the "in-memory only; starts empty on every start" rule of section 1).

## Data file
- Path: the environment variable `DATA_FILE`; if it is unset or empty, `books.json` in the server's working directory.
- The file format is up to you (it should be JSON). Only the HTTP behaviour is tested. The directory containing the file exists; you need not create it.

## Startup
- If the file does not exist, start with no books and the next id `1`.
- If it exists, load every stored book exactly as it was (all fields, tag order) and continue the id sequence.
- Ids are still never reused, across restarts too: the next id is one greater than the largest id *ever assigned*, even if that book was deleted before the restart. (So you must persist the id counter, not derive it from the remaining books.)

## Writes
- Every successful POST, PUT, PATCH and DELETE must be written to the file **before** its response is sent. The server may be killed abruptly (SIGKILL) at any moment after a response; nothing confirmed by a response may be lost.
- Failed requests (4xx) change neither the in-memory data nor the file.

## Example
```
POST /books {"title": "A", "author": "X", "year": 1}   → 201 {"id": 1, ...}
POST /books {"title": "B", "author": "X", "year": 2}   → 201 {"id": 2, ...}
DELETE /books/2                                          → 204
<restart>
GET /books                                               → 200 [{"id": 1, "title": "A", ...}]
POST /books {"title": "C", "author": "X", "year": 3}   → 201 {"id": 3, ...}
```
