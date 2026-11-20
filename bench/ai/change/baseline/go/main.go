package main

import (
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type Book struct {
	ID     int      `json:"id"`
	Title  string   `json:"title"`
	Author string   `json:"author"`
	Year   int      `json:"year"`
	Tags   []string `json:"tags"`
}

var (
	mu    sync.Mutex
	books = map[int]*Book{}
	next  = 1
	digit = regexp.MustCompile(`^[0-9]+$`)
)

func send(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, code int, msg string) { send(w, code, map[string]string{"error": msg}) }

func apply(r *http.Request, b *Book, full bool) string {
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return "invalid json"
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return "body must be object"
	}
	nb := *b
	if full {
		nb.Tags = []string{}
		for _, k := range []string{"title", "author", "year"} {
			if _, ok := m[k]; !ok {
				return "missing " + k
			}
		}
	}
	for k, v := range m {
		switch k {
		case "title", "author":
			s, ok := v.(string)
			s = strings.TrimSpace(s)
			if !ok || s == "" || (k == "title" && len([]rune(s)) > 200) {
				return "bad " + k
			}
			if k == "title" {
				nb.Title = s
			} else {
				nb.Author = s
			}
		case "year":
			n, ok := v.(json.Number)
			if !ok || !digitOrNeg(string(n)) {
				return "bad year"
			}
			y, err := strconv.Atoi(string(n))
			if err != nil || y < 0 || y > 2100 {
				return "bad year"
			}
			nb.Year = y
		case "tags":
			arr, ok := v.([]any)
			if !ok {
				return "bad tags"
			}
			tags := []string{}
			for _, t := range arr {
				s, ok := t.(string)
				if !ok {
					return "bad tags"
				}
				tags = append(tags, s)
			}
			nb.Tags = tags
		default:
			return "unknown field " + k
		}
	}
	*b = nb
	return ""
}

func digitOrNeg(s string) bool { return digit.MatchString(strings.TrimPrefix(s, "-")) }

func sorted() []*Book {
	out := []*Book{}
	for _, b := range books {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func handler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()
	p := r.URL.Path
	switch {
	case p == "/books":
		switch r.Method {
		case "GET":
			q := r.URL.Query()
			off, lim := 0, -1
			if q.Has("offset") {
				if !digit.MatchString(q.Get("offset")) {
					fail(w, 400, "bad offset"); return
				}
				off, _ = strconv.Atoi(q.Get("offset"))
			}
			if q.Has("limit") {
				if !digit.MatchString(q.Get("limit")) {
					fail(w, 400, "bad limit"); return
				}
				lim, _ = strconv.Atoi(q.Get("limit"))
			}
			res := []*Book{}
			for _, b := range sorted() {
				if q.Has("author") && b.Author != q.Get("author") {
					continue
				}
				if q.Has("tag") && !contains(b.Tags, q.Get("tag")) {
					continue
				}
				res = append(res, b)
			}
			if off > len(res) {
				off = len(res)
			}
			res = res[off:]
			if lim >= 0 && lim < len(res) {
				res = res[:lim]
			}
			send(w, 200, res)
		case "POST":
			var b Book
			if e := apply(r, &b, true); e != "" {
				fail(w, 400, e); return
			}
			b.ID = next
			next++
			books[b.ID] = &b
			send(w, 201, b)
		default:
			fail(w, 405, "method not allowed")
		}
	case p == "/stats":
		if r.Method != "GET" {
			fail(w, 405, "method not allowed"); return
		}
		ba, tc := map[string]int{}, map[string]int{}
		for _, b := range books {
			ba[b.Author]++
			seen := map[string]bool{}
			for _, t := range b.Tags {
				if !seen[t] {
					seen[t] = true
					tc[t]++
				}
			}
		}
		keys := []string{}
		for k := range tc {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if tc[keys[i]] != tc[keys[j]] {
				return tc[keys[i]] > tc[keys[j]]
			}
			return keys[i] < keys[j]
		})
		top := [][]any{}
		for i := 0; i < len(keys) && i < 3; i++ {
			top = append(top, []any{keys[i], tc[keys[i]]})
		}
		send(w, 200, map[string]any{"count": len(books), "by_author": ba, "top_tags": top})
	case strings.HasPrefix(p, "/books/") && len(p) > 7 && !strings.Contains(p[7:], "/"):
		switch r.Method {
		case "GET", "PUT", "PATCH", "DELETE":
		default:
			fail(w, 405, "method not allowed"); return
		}
		var b *Book
		if digit.MatchString(p[7:]) {
			if id, err := strconv.Atoi(p[7:]); err == nil {
				b = books[id]
			}
		}
		if b == nil {
			fail(w, 404, "not found"); return
		}
		switch r.Method {
		case "GET":
			send(w, 200, b)
		case "DELETE":
			delete(books, b.ID)
			w.WriteHeader(204)
		default:
			if e := apply(r, b, r.Method == "PUT"); e != "" {
				fail(w, 400, e); return
			}
			send(w, 200, b)
		}
	default:
		fail(w, 404, "not found")
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}


func main() {
	http.ListenAndServe("127.0.0.1:"+os.Getenv("PORT"), http.HandlerFunc(handler))
}
