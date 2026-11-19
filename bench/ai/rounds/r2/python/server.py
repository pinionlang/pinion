import json, os, re
import uvicorn
from fastapi import FastAPI, Request
from fastapi.responses import Response

app = FastAPI()
books = {}
next_id = [1]

def j(status, obj=None):
    if obj is None:
        return Response(status_code=status)
    return Response(json.dumps(obj), status_code=status, media_type="application/json")

def err(status, msg):
    return j(status, {"error": msg})

class Bad(Exception):
    pass

def validate(d, partial):
    if not isinstance(d, dict):
        raise Bad("body must be object")
    for k in d:
        if k not in ("title", "author", "year", "tags"):
            raise Bad("unknown field " + k)
    if not partial:
        for k in ("title", "author", "year"):
            if k not in d:
                raise Bad("missing " + k)
    out = {}
    if "title" in d:
        t = d["title"]
        if not isinstance(t, str) or not t.strip() or len(t.strip()) > 200:
            raise Bad("invalid title")
        out["title"] = t.strip()
    if "author" in d:
        a = d["author"]
        if not isinstance(a, str) or not a.strip():
            raise Bad("invalid author")
        out["author"] = a.strip()
    if "year" in d:
        y = d["year"]
        if isinstance(y, bool):
            raise Bad("invalid year")
        if isinstance(y, float) and y.is_integer():
            y = int(y)
        if not isinstance(y, int) or not (0 <= y <= 2100):
            raise Bad("invalid year")
        out["year"] = y
    if "tags" in d:
        tg = d["tags"]
        if not isinstance(tg, list) or not all(isinstance(x, str) for x in tg):
            raise Bad("invalid tags")
        out["tags"] = list(tg)
    elif not partial:
        out["tags"] = []
    return out

async def body(req):
    raw = await req.body()
    try:
        return json.loads(raw)
    except Exception:
        raise Bad("invalid json")

def nonneg(v):
    if not re.fullmatch(r"[0-9]+", v or ""):
        raise Bad("invalid int")
    return int(v)

@app.api_route("/{path:path}", methods=["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"])
async def handle(path: str, req: Request):
    parts = req.url.path.split("/")[1:]
    m = req.method
    try:
        if parts == ["books"]:
            if m == "POST":
                b = validate(await body(req), False)
                bid = next_id[0]; next_id[0] += 1
                book = {"id": bid, **b}
                books[bid] = book
                return j(201, book)
            if m == "GET":
                q = req.query_params
                res = [books[k] for k in sorted(books)]
                if "author" in q:
                    res = [b for b in res if b["author"] == q["author"]]
                if "tag" in q:
                    res = [b for b in res if q["tag"] in b["tags"]]
                off = nonneg(q["offset"]) if "offset" in q else 0
                lim = nonneg(q["limit"]) if "limit" in q else None
                res = res[off:]
                if lim is not None:
                    res = res[:lim]
                return j(200, res)
            return err(405, "method not allowed")
        if len(parts) == 2 and parts[0] == "books":
            if m not in ("GET", "PUT", "PATCH", "DELETE"):
                return err(405, "method not allowed")
            if not re.fullmatch(r"[0-9]+", parts[1]) or int(parts[1]) not in books:
                return err(404, "not found")
            bid = int(parts[1])
            if m == "GET":
                return j(200, books[bid])
            if m == "DELETE":
                del books[bid]
                return j(204)
            b = validate(await body(req), m == "PATCH")
            book = {"id": bid, **(b if m == "PUT" else {**books[bid], **b})}
            book["id"] = bid
            books[bid] = {k: book[k] for k in ("id", "title", "author", "year", "tags")}
            return j(200, books[bid])
        if parts == ["stats"]:
            if m != "GET":
                return err(405, "method not allowed")
            by, tc = {}, {}
            for b in books.values():
                by[b["author"]] = by.get(b["author"], 0) + 1
                for t in set(b["tags"]):
                    tc[t] = tc.get(t, 0) + 1
            top = sorted(tc.items(), key=lambda kv: (-kv[1], kv[0].encode()))[:3]
            return j(200, {"count": len(books), "by_author": by, "top_tags": [list(x) for x in top]})
        return err(404, "not found")
    except Bad as e:
        return err(400, str(e))

if __name__ == "__main__":
    uvicorn.run(app, host="127.0.0.1", port=int(os.environ["PORT"]), log_level="warning")
