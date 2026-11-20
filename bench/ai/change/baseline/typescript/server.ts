declare const require: any;
declare const process: any;
const http = require("node:http");

interface Book { id: number; title: string; author: string; year: number; tags: string[] }
const books = new Map<number, Book>();
let next = 1;
const DIG = /^[0-9]+$/;

function send(res: any, code: number, v?: unknown) {
  if (v === undefined) { res.writeHead(code); res.end(); return; }
  res.writeHead(code, { "Content-Type": "application/json" });
  res.end(JSON.stringify(v));
}
const fail = (res: any, code: number, msg: string) => send(res, code, { error: msg });

function apply(text: string, b: Book, full: boolean): string | null {
  let d: any;
  try { d = JSON.parse(text); } catch { return "invalid json"; }
  if (typeof d !== "object" || d === null || Array.isArray(d)) return "body must be object";
  const nb: Book = { ...b };
  if (full) {
    nb.tags = [];
    for (const k of ["title", "author", "year"]) if (!(k in d)) return "missing " + k;
  }
  for (const [k, v] of Object.entries(d)) {
    if (k === "title" || k === "author") {
      if (typeof v !== "string" || !v.trim()) return "bad " + k;
      const s = v.trim();
      if (k === "title" && [...s].length > 200) return "title too long";
      nb[k] = s;
    } else if (k === "year") {
      if (typeof v !== "number" || !Number.isInteger(v) || v < 0 || v > 2100) return "bad year";
      nb.year = v;
    } else if (k === "tags") {
      if (!Array.isArray(v) || !v.every((t) => typeof t === "string")) return "bad tags";
      nb.tags = [...v];
    } else return "unknown field " + k;
  }
  Object.assign(b, nb);
  return null;
}

const sorted = () => [...books.values()].sort((a, b) => a.id - b.id);

function handle(req: any, res: any, body: string) {
  const url = new URL(req.url, "http://x");
  const p = url.pathname, m = req.method, q = url.searchParams;
  if (p === "/books") {
    if (m === "GET") {
      for (const k of ["offset", "limit"]) if (q.has(k) && !DIG.test(q.get(k)!)) return fail(res, 400, "bad " + k);
      let r = sorted();
      if (q.has("author")) r = r.filter((b) => b.author === q.get("author"));
      if (q.has("tag")) r = r.filter((b) => b.tags.includes(q.get("tag")!));
      r = r.slice(q.has("offset") ? Number(q.get("offset")) : 0);
      if (q.has("limit")) r = r.slice(0, Number(q.get("limit")));
      return send(res, 200, r);
    }
    if (m === "POST") {
      const b = { id: 0, title: "", author: "", year: 0, tags: [] } as Book;
      const e = apply(body, b, true);
      if (e) return fail(res, 400, e);
      b.id = next++;
      books.set(b.id, b);
      return send(res, 201, { id: b.id, title: b.title, author: b.author, year: b.year, tags: b.tags });
    }
    return fail(res, 405, "method not allowed");
  }
  if (p === "/stats") {
    if (m !== "GET") return fail(res, 405, "method not allowed");
    const ba: Record<string, number> = {};
    const tc = new Map<string, number>();
    for (const b of books.values()) {
      ba[b.author] = (ba[b.author] ?? 0) + 1;
      for (const t of new Set(b.tags)) tc.set(t, (tc.get(t) ?? 0) + 1);
    }
    const top = [...tc.entries()].sort((a, b) => b[1] - a[1] || (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0)).slice(0, 3);
    return send(res, 200, { count: books.size, by_author: ba, top_tags: top });
  }
  const mm = /^\/books\/([^/]+)$/.exec(p);
  if (!mm) return fail(res, 404, "not found");
  if (!["GET", "PUT", "PATCH", "DELETE"].includes(m)) return fail(res, 405, "method not allowed");
  const b = DIG.test(mm[1]) ? books.get(Number(mm[1])) : undefined;
  if (!b) return fail(res, 404, "not found");
  if (m === "GET") return send(res, 200, b);
  if (m === "DELETE") { books.delete(b.id); return send(res, 204); }
  const e = apply(body, b, m === "PUT");
  if (e) return fail(res, 400, e);
  return send(res, 200, b);
}

http.createServer((req: any, res: any) => {
  let body = "";
  req.setEncoding("utf8");
  req.on("data", (c: string) => (body += c));
  req.on("end", () => handle(req, res, body));
}).listen(Number(process.env.PORT), "127.0.0.1");
