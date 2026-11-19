const http = require("node:http");
const books = new Map();
let nextId = 1;
class Bad extends Error {
}
function send(res, status, body) {
    if (body === undefined) {
        res.writeHead(status);
        res.end();
        return;
    }
    const s = JSON.stringify(body);
    res.writeHead(status, { "Content-Type": "application/json", "Content-Length": Buffer.byteLength(s) });
    res.end(s);
}
const err = (res, st, m) => send(res, st, { error: m });
function parseObj(raw) {
    let v;
    try {
        v = JSON.parse(raw);
    }
    catch {
        throw new Bad("invalid json");
    }
    if (v === null || typeof v !== "object" || Array.isArray(v))
        throw new Bad("body must be object");
    return v;
}
function validate(o, full) {
    const out = {};
    for (const k of Object.keys(o))
        if (!["title", "author", "year", "tags"].includes(k))
            throw new Bad("unknown field " + k);
    const has = (k) => Object.prototype.hasOwnProperty.call(o, k);
    if (full)
        for (const k of ["title", "author", "year"])
            if (!has(k))
                throw new Bad("missing " + k);
    if (has("title")) {
        if (typeof o.title !== "string")
            throw new Bad("title");
        const t = o.title.trim();
        const n = [...t].length;
        if (n === 0 || n > 200)
            throw new Bad("title");
        out.title = t;
    }
    if (has("author")) {
        if (typeof o.author !== "string")
            throw new Bad("author");
        const a = o.author.trim();
        if (!a)
            throw new Bad("author");
        out.author = a;
    }
    if (has("year")) {
        const y = o.year;
        if (typeof y !== "number" || !Number.isInteger(y) || y < 0 || y > 2100)
            throw new Bad("year");
        out.year = y;
    }
    if (has("tags")) {
        if (!Array.isArray(o.tags) || !o.tags.every(t => typeof t === "string"))
            throw new Bad("tags");
        out.tags = [...o.tags];
    }
    else if (full)
        out.tags = [];
    return out;
}
const nonNeg = (s, def) => {
    if (s === null)
        return def;
    if (!/^[0-9]+$/.test(s))
        throw new Bad("bad int");
    return Number(s);
};
function handle(req, res, body) {
    const url = req.url || "/";
    const qi = url.indexOf("?");
    const path = qi >= 0 ? url.slice(0, qi) : url;
    const query = new URLSearchParams(qi >= 0 ? url.slice(qi + 1) : "");
    const m = req.method || "";
    if (path === "/stats") {
        if (m !== "GET")
            return err(res, 405, "method not allowed");
        const by = {};
        const tc = new Map();
        for (const b of books.values()) {
            by[b.author] = (by[b.author] || 0) + 1;
            for (const t of new Set(b.tags))
                tc.set(t, (tc.get(t) || 0) + 1);
        }
        const top = [...tc.entries()].sort((a, b) => b[1] - a[1] || (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0)).slice(0, 3);
        return send(res, 200, { count: books.size, by_author: by, top_tags: top });
    }
    if (path === "/books") {
        if (m === "GET") {
            const offset = nonNeg(query.get("offset"), 0);
            const limit = nonNeg(query.get("limit"), null);
            const author = query.get("author");
            const tag = query.get("tag");
            let list = [...books.values()].sort((a, b) => a.id - b.id)
                .filter(b => (author === null || b.author === author) && (tag === null || b.tags.includes(tag)));
            list = list.slice(offset);
            if (limit !== null)
                list = list.slice(0, limit);
            return send(res, 200, list);
        }
        if (m === "POST") {
            const v = validate(parseObj(body), true);
            const b = { id: nextId++, title: v.title, author: v.author, year: v.year, tags: v.tags };
            books.set(b.id, b);
            return send(res, 201, b);
        }
        return err(res, 405, "method not allowed");
    }
    const mm = /^\/books\/([^/]*)$/.exec(path);
    if (mm) {
        if (!["GET", "PUT", "PATCH", "DELETE"].includes(m))
            return err(res, 405, "method not allowed");
        const seg = mm[1];
        const b = /^[0-9]+$/.test(seg) ? books.get(Number(seg)) : undefined;
        if (!b)
            return err(res, 404, "not found");
        if (m === "GET")
            return send(res, 200, b);
        if (m === "DELETE") {
            books.delete(b.id);
            return send(res, 204);
        }
        const v = validate(parseObj(body), m === "PUT");
        Object.assign(b, v);
        return send(res, 200, { id: b.id, title: b.title, author: b.author, year: b.year, tags: b.tags });
    }
    return err(res, 404, "not found");
}
http.createServer((req, res) => {
    const chunks = [];
    req.on("data", (c) => chunks.push(c));
    req.on("end", () => {
        try {
            handle(req, res, Buffer.concat(chunks).toString("utf8"));
        }
        catch (e) {
            if (e instanceof Bad)
                err(res, 400, e.message);
            else
                err(res, 500, "internal");
        }
    });
}).listen(Number(process.env.PORT), "127.0.0.1");
