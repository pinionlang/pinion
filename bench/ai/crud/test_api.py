#!/usr/bin/env python3
"""Black-box test for the Books CRUD API (see SPEC.md).

Usage: python3 test_api.py --cmd "<start command>" --cwd <dir> [--timeout 60]
Prints {"passed": n, "total": m, "failures": [...]} and exits 0 iff all pass.
"""
import argparse, tempfile, json, os, signal, socket, subprocess, sys, time
import urllib.request, urllib.error

BASE = ""


def free_port():
    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    p = s.getsockname()[1]
    s.close()
    return p


def req(method, path, body=None, raw=None):
    data = raw if raw is not None else (json.dumps(body).encode() if body is not None else None)
    r = urllib.request.Request(BASE + path, data=data, method=method)
    if data is not None:
        r.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(r, timeout=10) as resp:
            status, text, ctype = resp.status, resp.read(), resp.headers.get("Content-Type", "")
    except urllib.error.HTTPError as e:
        status, text, ctype = e.code, e.read(), e.headers.get("Content-Type", "")
    try:
        js = json.loads(text) if text else None
    except ValueError:
        js = "<<non-json>>"
    return status, js, text, ctype


class T:
    def __init__(self):
        self.passed, self.total, self.failures = 0, 0, []

    def check(self, name, fn):
        self.total += 1
        try:
            msg = fn()
        except Exception as e:  # noqa
            msg = f"exception: {type(e).__name__}: {e}"
        if msg in (None, True):
            self.passed += 1
        else:
            self.failures.append({"name": name, "detail": str(msg)[:500]})


def expect(res, status, body=..., err=False):
    st, js, text, ctype = res
    if st != status:
        return f"status {st} != {status}; body={text[:200]!r}"
    if err:
        if not (isinstance(js, dict) and isinstance(js.get("error"), str) and js["error"]):
            return f"bad error body {text[:200]!r}"
    if body is not ...:
        if js != body:
            return f"body {js!r} != {body!r}"
    if st != 204 and "application/json" not in ctype.lower():
        return f"content-type {ctype!r}"
    return None


def book(i, title, author, year, tags):
    return {"id": i, "title": title, "author": author, "year": year, "tags": tags}


def run_checks(t):
    c = t.check
    # empty state
    c("list empty", lambda: expect(req("GET", "/books"), 200, []))
    c("stats empty", lambda: expect(req("GET", "/stats"), 200, {"count": 0, "by_author": {}, "top_tags": []}))
    # create
    c("create trims, default tags", lambda: expect(
        req("POST", "/books", {"title": "  Dune ", "author": " Frank Herbert", "year": 1965}),
        201, book(1, "Dune", "Frank Herbert", 1965, [])))
    c("create with tags", lambda: expect(
        req("POST", "/books", {"title": "Children of Dune", "author": "Frank Herbert", "year": 1976, "tags": ["sf", "classic"]}),
        201, book(2, "Children of Dune", "Frank Herbert", 1976, ["sf", "classic"])))
    c("create third", lambda: expect(
        req("POST", "/books", {"title": "Earthsea", "author": "Ursula K. Le Guin", "year": 0, "tags": ["fantasy", "sf"]}),
        201, book(3, "Earthsea", "Ursula K. Le Guin", 0, ["fantasy", "sf"])))
    c("create year 2100 boundary, 200-char title", lambda: expect(
        req("POST", "/books", {"title": "x" * 200, "author": "Z", "year": 2100, "tags": ["sf"]}),
        201, book(4, "x" * 200, "Z", 2100, ["sf"])))
    # create validation
    bad = [
        ("invalid json", None, b"{not json"),
        ("non-object body", None, b"[1,2]"),
        ("missing title", {"author": "A", "year": 1}, None),
        ("missing author", {"title": "T", "year": 1}, None),
        ("missing year", {"title": "T", "author": "A"}, None),
        ("blank title", {"title": "   ", "author": "A", "year": 1}, None),
        ("title 201 chars", {"title": "y" * 201, "author": "A", "year": 1}, None),
        ("empty author", {"title": "T", "author": "", "year": 1}, None),
        ("year too big", {"title": "T", "author": "A", "year": 2101}, None),
        ("year negative", {"title": "T", "author": "A", "year": -1}, None),
        ("year string", {"title": "T", "author": "A", "year": "1999"}, None),
        ("year float", {"title": "T", "author": "A", "year": 1999.5}, None),
        ("year bool", {"title": "T", "author": "A", "year": True}, None),
        ("title number", {"title": 5, "author": "A", "year": 1}, None),
        ("tags not list", {"title": "T", "author": "A", "year": 1, "tags": "sf"}, None),
        ("tags non-string elem", {"title": "T", "author": "A", "year": 1, "tags": ["a", 1]}, None),
        ("null field", {"title": "T", "author": "A", "year": 1, "tags": None}, None),
        ("unknown field", {"title": "T", "author": "A", "year": 1, "isbn": "x"}, None),
        ("id field", {"id": 99, "title": "T", "author": "A", "year": 1}, None),
    ]
    for name, body, raw in bad:
        c("create 400: " + name, lambda body=body, raw=raw: expect(req("POST", "/books", body, raw), 400, err=True))
    c("failed creates did not store", lambda: None if req("GET", "/stats")[1].get("count") == 4 else "count != 4")
    # get
    c("get one", lambda: expect(req("GET", "/books/2"), 200, book(2, "Children of Dune", "Frank Herbert", 1976, ["sf", "classic"])))
    c("get missing 404", lambda: expect(req("GET", "/books/999"), 404, err=True))
    c("get non-int 404", lambda: expect(req("GET", "/books/abc"), 404, err=True))
    # list + filters
    ids = lambda path: [b["id"] for b in req("GET", path)[1]]
    def eq(path, want):
        r = req("GET", path)
        e = expect(r, 200)
        if e:
            return e
        got = [b["id"] for b in r[1]]
        return None if got == want else f"{path}: ids {got} != {want}"
    c("list sorted", lambda: eq("/books", [1, 2, 3, 4]))
    c("filter author", lambda: eq("/books?author=Frank%20Herbert", [1, 2]))
    c("filter author exact (no partial)", lambda: eq("/books?author=Frank", []))
    c("filter tag", lambda: eq("/books?tag=sf", [2, 3, 4]))
    c("filter author+tag", lambda: eq("/books?author=Frank+Herbert&tag=sf", [2]))
    c("limit/offset", lambda: eq("/books?tag=sf&limit=1&offset=1", [3]))
    c("limit 0", lambda: eq("/books?limit=0", []))
    c("offset past end", lambda: eq("/books?offset=10", []))
    for q in ["limit=-1", "limit=abc", "offset=1.5", "offset=-2", "limit="]:
        c("list 400: " + q, lambda q=q: expect(req("GET", "/books?" + q), 400, err=True))
    # put
    c("put replace, tags reset", lambda: expect(
        req("PUT", "/books/2", {"title": "Dune Messiah", "author": "Frank Herbert", "year": 1969}),
        200, book(2, "Dune Messiah", "Frank Herbert", 1969, [])))
    c("put missing field 400", lambda: expect(req("PUT", "/books/2", {"title": "X", "author": "Y"}), 400, err=True))
    c("put unknown field 400", lambda: expect(req("PUT", "/books/2", {"title": "X", "author": "Y", "year": 1, "foo": 1}), 400, err=True))
    c("put missing book 404", lambda: expect(req("PUT", "/books/999", {"title": "X", "author": "Y", "year": 1}), 404, err=True))
    c("put 400 left unchanged", lambda: expect(req("GET", "/books/2"), 200, book(2, "Dune Messiah", "Frank Herbert", 1969, [])))
    # patch
    c("patch partial", lambda: expect(req("PATCH", "/books/1", {"year": 1966, "tags": ["sf"]}),
                                      200, book(1, "Dune", "Frank Herbert", 1966, ["sf"])))
    c("patch empty object", lambda: expect(req("PATCH", "/books/1", {}), 200, book(1, "Dune", "Frank Herbert", 1966, ["sf"])))
    c("patch trims title", lambda: expect(req("PATCH", "/books/1", {"title": " Dune! "}), 200, book(1, "Dune!", "Frank Herbert", 1966, ["sf"])))
    c("patch invalid year 400", lambda: expect(req("PATCH", "/books/1", {"year": 3000}), 400, err=True))
    c("patch mixed valid+invalid 400", lambda: expect(req("PATCH", "/books/1", {"title": "New", "author": ""}), 400, err=True))
    c("patch unknown field 400", lambda: expect(req("PATCH", "/books/1", {"id": 5}), 400, err=True))
    c("patch null 400", lambda: expect(req("PATCH", "/books/1", {"title": None}), 400, err=True))
    c("patch invalid json 400", lambda: expect(req("PATCH", "/books/1", raw=b"nope"), 400, err=True))
    c("patch failures left unchanged", lambda: expect(req("GET", "/books/1"), 200, book(1, "Dune!", "Frank Herbert", 1966, ["sf"])))
    c("patch missing 404", lambda: expect(req("PATCH", "/books/999", {"year": 1}), 404, err=True))
    # stats: books 1(FH,[sf]) 2(FH,[]) 3(UKLG,[fantasy,sf]) 4(Z,[sf])
    req("PATCH", "/books/2", {"tags": ["classic", "zz"]})
    c("stats", lambda: expect(req("GET", "/stats"), 200, {
        "count": 4, "by_author": {"Frank Herbert": 2, "Ursula K. Le Guin": 1, "Z": 1},
        "top_tags": [["sf", 3], ["classic", 1], ["fantasy", 1]]}))
    # delete
    def delete_ok():
        st, js, text, _ = req("DELETE", "/books/3")
        if st != 204:
            return f"status {st}"
        return None if text == b"" else f"non-empty body {text!r}"
    c("delete 204 empty", delete_ok)
    c("delete again 404", lambda: expect(req("DELETE", "/books/3"), 404, err=True))
    c("get deleted 404", lambda: expect(req("GET", "/books/3"), 404, err=True))
    c("id not reused", lambda: expect(req("POST", "/books", {"title": "New", "author": "Q", "year": 5}), 201, book(5, "New", "Q", 5, [])))
    c("list after delete", lambda: eq("/books", [1, 2, 4, 5]))
    c("stats after delete", lambda: expect(req("GET", "/stats"), 200, {
        "count": 4, "by_author": {"Frank Herbert": 2, "Z": 1, "Q": 1},
        "top_tags": [["sf", 2], ["classic", 1], ["zz", 1]]}))
    # routing
    for p in ["/", "/foo", "/books/1/extra", "/statsx"]:
        c("404 route " + p, lambda p=p: expect(req("GET", p), 404, err=True))
    for m, p in [("DELETE", "/books"), ("PUT", "/books"), ("POST", "/books/1"), ("POST", "/books/999"), ("POST", "/stats"), ("DELETE", "/stats")]:
        c(f"405 {m} {p}", lambda m=m, p=p: expect(req(m, p, {} if m in ("POST", "PUT") else None), 405, err=True))


def main():
    global BASE
    ap = argparse.ArgumentParser()
    ap.add_argument("--cmd", required=True)
    ap.add_argument("--cwd", default=".")
    ap.add_argument("--timeout", type=float, default=60)
    a = ap.parse_args()
    port = free_port()
    BASE = f"http://127.0.0.1:{port}"
    env = dict(os.environ, PORT=str(port))
    errf = tempfile.TemporaryFile()
    proc = subprocess.Popen(a.cmd, shell=True, cwd=a.cwd, env=env, stdout=subprocess.DEVNULL,
                            stderr=errf, start_new_session=True)
    t = T()
    try:
        deadline = time.time() + a.timeout
        up = False
        while time.time() < deadline:
            if proc.poll() is not None:
                break
            try:
                socket.create_connection(("127.0.0.1", port), timeout=1).close()
                up = True
                break
            except OSError:
                time.sleep(0.2)
        if not up:
            err = ""
            if proc.poll() is not None:
                errf.seek(0)
                err = errf.read().decode(errors="replace")[-1000:]
            t.total = 1
            t.failures.append({"name": "server start", "detail": "server did not accept connections " + err})
        else:
            run_checks(t)
    finally:
        try:
            os.killpg(proc.pid, signal.SIGKILL)
        except Exception:
            pass
        proc.wait()
    print(json.dumps({"passed": t.passed, "total": t.total, "failures": t.failures}, indent=2))
    sys.exit(0 if t.passed == t.total and t.total > 0 else 1)


if __name__ == "__main__":
    main()
