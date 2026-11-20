import json, re
from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse, Response

app = FastAPI()
books, nxt = {}, [1]
FIELDS = {"title", "author", "year", "tags"}

def err(code, msg): return JSONResponse({"error": msg}, status_code=code)

def validate(d, full, out=None):
    if not isinstance(d, dict): return None, "body must be object"
    if set(d) - FIELDS: return None, "unknown fields"
    if full and not {"title", "author", "year"} <= set(d): return None, "missing fields"
    out = {} if out is None else out
    for k, v in d.items():
        if k in ("title", "author"):
            if not isinstance(v, str) or not v.strip(): return None, f"bad {k}"
            v = v.strip()
            if k == "title" and len(v) > 200: return None, "title too long"
        elif k == "year":
            if type(v) is not int or not 0 <= v <= 2100: return None, "bad year"
        elif k == "tags":
            if not isinstance(v, list) or not all(isinstance(x, str) for x in v): return None, "bad tags"
        out[k] = v
    if full: out.setdefault("tags", [])
    return out, None

async def body(req):
    try: return json.loads(await req.body()), None
    except Exception: return None, "invalid json"

def nonneg(v):
    return int(v) if v is not None and re.fullmatch(r"[0-9]+", v) else (None if v is None else -1)

@app.api_route("/{path:path}", methods=["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"])
async def route(path: str, req: Request):
    m = req.method
    if path == "books":
        if m == "GET":
            q = req.query_params
            off, lim = nonneg(q.get("offset")), nonneg(q.get("limit"))
            if off == -1 or lim == -1: return err(400, "bad limit/offset")
            res = [b for _, b in sorted(books.items())]
            if "author" in q: res = [b for b in res if b["author"] == q["author"]]
            if "tag" in q: res = [b for b in res if q["tag"] in b["tags"]]
            res = res[off or 0:]
            if lim is not None: res = res[:lim]
            return res
        if m == "POST":
            d, e = await body(req)
            if e: return err(400, e)
            d, e = validate(d, True)
            if e: return err(400, e)
            b = {"id": nxt[0], **{k: d[k] for k in ("title", "author", "year", "tags")}}
            books[nxt[0]] = b; nxt[0] += 1
            return JSONResponse(b, status_code=201)
        return err(405, "method not allowed")
    if path == "stats":
        if m != "GET": return err(405, "method not allowed")
        ba, tc = {}, {}
        for b in books.values():
            ba[b["author"]] = ba.get(b["author"], 0) + 1
            for t in set(b["tags"]): tc[t] = tc.get(t, 0) + 1
        top = sorted(tc.items(), key=lambda x: (-x[1], x[0]))[:3]
        return {"count": len(books), "by_author": ba, "top_tags": [list(x) for x in top]}
    mm = re.fullmatch(r"books/([^/]+)", path)
    if not mm: return err(404, "not found")
    if m not in ("GET", "PUT", "PATCH", "DELETE"): return err(405, "method not allowed")
    s = mm.group(1)
    b = books.get(int(s)) if s.isdigit() and s.isascii() else None
    if b is None: return err(404, "not found")
    if m == "GET": return b
    if m == "DELETE":
        del books[b["id"]]; return Response(status_code=204)
    d, e = await body(req)
    if e: return err(400, e)
    d, e = validate(d, m == "PUT", None if m == "PUT" else dict(b))
    if e: return err(400, e)
    b.update(d)
    return b
