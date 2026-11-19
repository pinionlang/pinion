declare const require: any; declare const process: any;
const http = require("node:http");
type Book = { id: number; title: string; author: string; year: number; tags: string[] };
const books = new Map<number, Book>();
let nextId = 1;
class E extends Error { constructor(public status: number, m: string) { super(m); } }
function send(res: any, status: number, body?: unknown) {
  if (body === undefined) { res.writeHead(status); res.end(); return; }
  const s = JSON.stringify(body);
  res.writeHead(status, { "Content-Type": "application/json", "Content-Length": Buffer.byteLength(s) });
  res.end(s);
}
declare const Buffer: any;
const KEYS = new Set(["title", "author", "year", "tags"]);
function validate(raw: string, full: boolean): Partial<Book> {
  let b: any;
  try { b = JSON.parse(raw); } catch { throw new E(400, "invalid JSON"); }
  if (b === null || typeof b !== "object" || Array.isArray(b)) throw new E(400, "body must be an object");
  const out: Partial<Book> = {};
  for (const k of Object.keys(b)) if (!KEYS.has(k)) throw new E(400, "unknown field " + k);
  if (full) for (const k of ["title", "author", "year"]) if (!(k in b)) throw new E(400, "missing " + k);
  if ("title" in b) {
    if (typeof b.title !== "string") throw new E(400, "bad title");
    const t = b.title.trim(); const n = [...t].length;
    if (n === 0 || n > 200) throw new E(400, "bad title"); out.title = t;
  }
  if ("author" in b) {
    if (typeof b.author !== "string" || b.author.trim() === "") throw new E(400, "bad author");
    out.author = b.author.trim();
  }
  if ("year" in b) {
    const y = b.year;
    if (typeof y !== "number" || !Number.isInteger(y) || y < 0 || y > 2100) throw new E(400, "bad year");
    out.year = y;
  }
  if ("tags" in b) {
    if (!Array.isArray(b.tags) || !b.tags.every((t: any) => typeof t === "string")) throw new E(400, "bad tags");
    out.tags = [...b.tags];
  } else if (full) out.tags = [];
  return out;
}
function nonneg(v: string | null, name: string): number | undefined {
  if (v === null) return undefined;
  if (!/^[0-9]+$/.test(v)) throw new E(400, "bad " + name);
  return Number(v);
}
function handle(method: string, url: string, body: string): [number, unknown?] {
  const q = url.indexOf("?");
  const path = q >= 0 ? url.slice(0, q) : url;
  const params = new URLSearchParams(q >= 0 ? url.slice(q + 1) : "");
  if (path === "/books") {
    if (method === "GET") {
      const offset = nonneg(params.get("offset"), "offset") ?? 0;
      const limit = nonneg(params.get("limit"), "limit");
      const a = params.get("author"), t = params.get("tag");
      let list = [...books.values()].sort((x, y) => x.id - y.id)
        .filter(b => (a === null || b.author === a) && (t === null || b.tags.includes(t)));
      list = list.slice(offset);
      if (limit !== undefined) list = list.slice(0, limit);
      return [200, list];
    }
    if (method === "POST") {
      const v = validate(body, true) as Book;
      const b: Book = { id: nextId++, title: v.title, author: v.author, year: v.year, tags: v.tags };
      books.set(b.id, b); return [201, b];
    }
    throw new E(405, "method not allowed");
  }
  if (path === "/stats") {
    if (method !== "GET") throw new E(405, "method not allowed");
    const by: Record<string, number> = {}; const tc = new Map<string, number>();
    for (const b of books.values()) {
      by[b.author] = (by[b.author] ?? 0) + 1;
      for (const t of new Set(b.tags)) tc.set(t, (tc.get(t) ?? 0) + 1);
    }
    const top = [...tc.entries()].sort((x, y) => y[1] - x[1] || (x[0] < y[0] ? -1 : x[0] > y[0] ? 1 : 0)).slice(0, 3);
    return [200, { count: books.size, by_author: by, top_tags: top }];
  }
  const m = /^\/books\/([^/]*)$/.exec(path);
  if (m) {
    if (!["GET", "PUT", "PATCH", "DELETE"].includes(method)) throw new E(405, "method not allowed");
    if (!/^[0-9]+$/.test(m[1])) throw new E(404, "not found");
    const id = Number(m[1]); const b = books.get(id);
    if (!b) throw new E(404, "not found");
    if (method === "GET") return [200, b];
    if (method === "DELETE") { books.delete(id); return [204]; }
    const v = validate(body, method === "PUT");
    Object.assign(b, v); return [200, b];
  }
  throw new E(404, "not found");
}
http.createServer((req: any, res: any) => {
  const chunks: any[] = [];
  req.on("data", (c: any) => chunks.push(c));
  req.on("end", () => {
    try { const [s, b] = handle(req.method, req.url, Buffer.concat(chunks).toString("utf8")); send(res, s, b); }
    catch (e: any) { if (e instanceof E) send(res, e.status, { error: e.message }); else send(res, 500, { error: "internal" }); }
  });
}).listen(Number(process.env.PORT), "127.0.0.1");
