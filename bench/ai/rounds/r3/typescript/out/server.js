"use strict";
const http = require("node:http");
const books = new Map();
let nextId = 1;
const KEYS = new Set(["title", "author", "year", "tags"]);
class Bad extends Error {
}
function validate(o, full) {
    if (typeof o !== "object" || o === null || Array.isArray(o))
        throw new Bad("body must be object");
    const out = {};
    for (const k of Object.keys(o))
        if (!KEYS.has(k))
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
        if (!Array.isArray(o.tags) || !o.tags.every((t) => typeof t === "string"))
            throw new Bad("tags");
        out.tags = [...o.tags];
    }
    else if (full)
        out.tags = [];
    return out;
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
function parseBody(raw) { try {
    return JSON.parse(raw);
}
catch {
    throw new Bad("invalid json");
} }
const nonNeg = (v) => { if (!/^[0-9]+$/.test(v))
    throw new Bad("bad int"); return Number(v); };
function handle(req, res, raw) {
    const url = req.url || "/";
    const qi = url.indexOf("?");
    const path = qi >= 0 ? url.slice(0, qi) : url;
    const params = new URLSearchParams(qi >= 0 ? url.slice(qi + 1) : "");
    const m = req.method;
    try {
        if (path === "/books") {
            if (m === "GET") {
                const author = params.get("author"), tag = params.get("tag");
                const off = params.has("offset") ? nonNeg(params.get("offset")) : 0;
                const lim = params.has("limit") ? nonNeg(params.get("limit")) : Infinity;
                let list = [...books.values()].sort((a, b) => a.id - b.id);
                if (author !== null)
                    list = list.filter(b => b.author === author);
                if (tag !== null)
                    list = list.filter(b => b.tags.includes(tag));
                list = list.slice(off);
                if (lim !== Infinity)
                    list = list.slice(0, lim);
                return send(res, 200, list);
            }
            if (m === "POST") {
                const v = validate(parseBody(raw), true);
                const b = { id: nextId++, title: v.title, author: v.author, year: v.year, tags: v.tags };
                books.set(b.id, b);
                return send(res, 201, b);
            }
            return err(res, 405, "method not allowed");
        }
        if (path === "/stats") {
            if (m !== "GET")
                return err(res, 405, "method not allowed");
            const by = Object.create(null);
            const tc = new Map();
            for (const b of books.values()) {
                by[b.author] = (by[b.author] || 0) + 1;
                for (const t of new Set(b.tags))
                    tc.set(t, (tc.get(t) || 0) + 1);
            }
            const top = [...tc.entries()].sort((a, b) => b[1] - a[1] || (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0)).slice(0, 3);
            return send(res, 200, { count: books.size, by_author: { ...by }, top_tags: top });
        }
        const mm = /^\/books\/([^/]*)$/.exec(path);
        if (mm) {
            if (!["GET", "PUT", "PATCH", "DELETE"].includes(m))
                return err(res, 405, "method not allowed");
            if (!/^[0-9]+$/.test(mm[1]))
                return err(res, 404, "not found");
            const b = books.get(Number(mm[1]));
            if (!b)
                return err(res, 404, "not found");
            if (m === "GET")
                return send(res, 200, b);
            if (m === "DELETE") {
                books.delete(b.id);
                return send(res, 204);
            }
            const v = validate(parseBody(raw), m === "PUT");
            Object.assign(b, v);
            return send(res, 200, b);
        }
        return err(res, 404, "not found");
    }
    catch (e) {
        if (e instanceof Bad)
            return err(res, 400, e.message);
        return err(res, 500, "internal error");
    }
}
http.createServer((req, res) => {
    const chunks = [];
    req.on("data", (c) => chunks.push(c));
    req.on("end", () => handle(req, res, Buffer.concat(chunks).toString("utf8")));
}).listen(Number(process.env.PORT), "127.0.0.1");
