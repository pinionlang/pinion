import json, os, re
from fastapi import FastAPI, Request
from fastapi.responses import Response
import uvicorn

app = FastAPI(openapi_url=None, docs_url=None, redoc_url=None)
books = {}
next_id = [1]
FIELDS = {"title", "author", "year", "tags"}

def J(status, obj):
    return Response(content=json.dumps(obj), status_code=status, media_type="application/json")

def err(status, msg):
    return J(status, {"error": msg})

def validate(d, partial):
    if not isinstance(d, dict):
        return "body must be an object"
    for k in d:
        if k not in FIELDS:
            return "unknown field " + k
    if not partial:
        for k in ("title", "author", "year"):
            if k not in d:
                return "missing " + k
    out = {}
    if "title" in d:
        t = d["title"]
        if not isinstance(t, str): return "bad title"
        t = t.strip()
        if not t or len(t) > 200: return "bad title"
        out["title"] = t
    if "author" in d:
        a = d["author"]
        if not isinstance(a, str): return "bad author"
        a = a.strip()
        if not a: return "bad author"
        out["author"] = a
    if "year" in d:
        y = d["year"]
        if type(y) is not int or not (0 <= y <= 2100): return "bad year"
        out["year"] = y
    if "tags" in d:
        tg = d["tags"]
        if not isinstance(tg, list) or not all(isinstance(x, str) for x in tg): return "bad tags"
        out["tags"] = list(tg)
    elif not partial:
        out["tags"] = []
    return out

async def body(req):
    raw = await req.body()
    try:
        return True, json.loads(raw.decode("utf-8"), parse_constant=lambda c: (_ for _ in ()).throw(ValueError()))
    except Exception:
        return False, None

DIG = re.compile(r"^[0-9]+$")

async def handle(req: Request):
    path = req.url.path
    m = req.method
    if path == "/books":
        if m == "GET":
            q = req.query_params
            res = [books[i] for i in sorted(books)]
            if "author" in q:
                res = [b for b in res if b["author"] == q["author"]]
            if "tag" in q:
                res = [b for b in res if q["tag"] in b["tags"]]
            off, lim = 0, None
            if "offset" in q:
                if not DIG.match(q["offset"]): return err(400, "bad offset")
                off = int(q["offset"])
            if "limit" in q:
                if not DIG.match(q["limit"]): return err(400, "bad limit")
                lim = int(q["limit"])
            res = res[off:]
            if lim is not None: res = res[:lim]
            return J(200, res)
        if m == "POST":
            ok, d = await body(req)
            if not ok: return err(400, "invalid json")
            v = validate(d, False)
            if isinstance(v, str): return err(400, v)
            b = {"id": next_id[0], **v}
            b = {k: b[k] for k in ("id", "title", "author", "year", "tags")}
            books[next_id[0]] = b
            next_id[0] += 1
            return J(201, b)
        return err(405, "method not allowed")
    if path == "/stats":
        if m != "GET": return err(405, "method not allowed")
        by, tc = {}, {}
        for b in books.values():
            by[b["author"]] = by.get(b["author"], 0) + 1
            for t in set(b["tags"]):
                tc[t] = tc.get(t, 0) + 1
        top = sorted(tc.items(), key=lambda x: (-x[1], x[0].encode("utf-8")))[:3]
        return J(200, {"count": len(books), "by_author": by, "top_tags": [list(x) for x in top]})
    mm = re.fullmatch(r"/books/([^/]+)", path)
    if mm:
        if m not in ("GET", "PUT", "PATCH", "DELETE"): return err(405, "method not allowed")
        seg = mm.group(1)
        if not DIG.match(seg): return err(404, "not found")
        bid = int(seg)
        if bid not in books: return err(404, "not found")
        b = books[bid]
        if m == "GET": return J(200, b)
        if m == "DELETE":
            del books[bid]
            return Response(status_code=204)
        ok, d = await body(req)
        if not ok: return err(400, "invalid json")
        v = validate(d, m == "PATCH")
        if isinstance(v, str): return err(400, v)
        nb = dict(b) if m == "PATCH" else {"id": bid}
        nb.update(v)
        nb = {k: nb[k] for k in ("id", "title", "author", "year", "tags")}
        books[bid] = nb
        return J(200, nb)
    return err(404, "not found")

app.add_api_route("/{p:path}", handle, methods=["GET","POST","PUT","PATCH","DELETE","HEAD","OPTIONS"])

if __name__ == "__main__":
    uvicorn.run(app, host="127.0.0.1", port=int(os.environ["PORT"]), log_level="warning")
