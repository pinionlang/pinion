import json, os, re
from urllib.parse import parse_qsl
from fastapi import FastAPI, Request
from fastapi.responses import Response
import uvicorn

app = FastAPI(docs_url=None, redoc_url=None, openapi_url=None)
books = {}
next_id = 1
FIELDS = {"title", "author", "year", "tags"}

def J(status, obj=None):
    if obj is None:
        return Response(status_code=status)
    return Response(json.dumps(obj), status_code=status, media_type="application/json")

def err(status, msg):
    return J(status, {"error": msg})

class Bad(Exception):
    pass

def validate(d, partial):
    if not isinstance(d, dict):
        raise Bad("body must be an object")
    for k in d:
        if k not in FIELDS:
            raise Bad("unknown field " + k)
    out = {}
    for k in ("title", "author"):
        if k in d:
            v = d[k]
            if not isinstance(v, str):
                raise Bad(k + " must be string")
            v = v.strip()
            if not v:
                raise Bad(k + " empty")
            if k == "title" and len(v) > 200:
                raise Bad("title too long")
            out[k] = v
        elif not partial:
            raise Bad(k + " required")
    if "year" in d:
        v = d["year"]
        if type(v) is not int or not (0 <= v <= 2100):
            raise Bad("invalid year")
        out["year"] = v
    elif not partial:
        raise Bad("year required")
    if "tags" in d:
        v = d["tags"]
        if not isinstance(v, list) or not all(isinstance(t, str) for t in v):
            raise Bad("invalid tags")
        out["tags"] = list(v)
    elif not partial:
        out["tags"] = []
    return out

async def body(request):
    raw = await request.body()
    try:
        return json.loads(raw.decode("utf-8"), parse_constant=lambda c: (_ for _ in ()).throw(Bad(c)))
    except Exception:
        raise Bad("invalid json")

ID_RE = re.compile(r"^[0-9]+$")

@app.api_route("/{path:path}", methods=["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"])
async def handle(request: Request, path: str):
    global next_id
    p = request.url.path
    m = request.method
    parts = p.split("/")
    try:
        if p == "/books":
            if m == "GET":
                q = {}
                for k, v in parse_qsl(request.url.query, keep_blank_values=True):
                    q.setdefault(k, v)
                res = [books[i] for i in sorted(books)]
                if "author" in q:
                    res = [b for b in res if b["author"] == q["author"]]
                if "tag" in q:
                    res = [b for b in res if q["tag"] in b["tags"]]
                for k in ("offset", "limit"):
                    if k in q and not re.fullmatch(r"[0-9]+", q[k]):
                        return err(400, "invalid " + k)
                off = int(q.get("offset", 0))
                res = res[off:]
                if "limit" in q:
                    res = res[: int(q["limit"])]
                return J(200, res)
            if m == "POST":
                d = validate(await body(request), False)
                b = {"id": next_id, **d}
                b = {k: b[k] for k in ("id", "title", "author", "year", "tags")}
                books[next_id] = b
                next_id += 1
                return J(201, b)
            return err(405, "method not allowed")
        if len(parts) == 3 and parts[1] == "books" and parts[2] != "":
            if m not in ("GET", "PUT", "PATCH", "DELETE"):
                return err(405, "method not allowed")
            seg = parts[2]
            if not ID_RE.fullmatch(seg) or int(seg) not in books:
                return err(404, "not found")
            bid = int(seg)
            b = books[bid]
            if m == "GET":
                return J(200, b)
            if m == "DELETE":
                del books[bid]
                return J(204)
            d = validate(await body(request), m == "PATCH")
            nb = dict(b); nb.update(d)
            books[bid] = nb
            return J(200, nb)
        if p == "/stats":
            if m != "GET":
                return err(405, "method not allowed")
            by = {}; tc = {}
            for b in books.values():
                by[b["author"]] = by.get(b["author"], 0) + 1
                for t in set(b["tags"]):
                    tc[t] = tc.get(t, 0) + 1
            top = sorted(tc.items(), key=lambda x: (-x[1], x[0].encode("utf-8")))[:3]
            return J(200, {"count": len(books), "by_author": by, "top_tags": [list(x) for x in top]})
        return err(404, "not found")
    except Bad as e:
        return err(400, str(e))

if __name__ == "__main__":
    uvicorn.run(app, host="127.0.0.1", port=int(os.environ["PORT"]), log_level="warning")
