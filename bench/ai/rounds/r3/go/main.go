package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

type Book struct {
	ID     int64    `json:"id"`
	Title  string   `json:"title"`
	Author string   `json:"author"`
	Year   int64    `json:"year"`
	Tags   []string `json:"tags"`
}

var (
	mu     sync.Mutex
	books  = map[int64]*Book{}
	nextID int64 = 1
)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// parseBody validates fields; full requires title/author/year.
func parseBody(r *http.Request, b *Book, full bool) string {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return "read error"
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return "invalid json"
	}
	if _, err := dec.Token(); err != io.EOF {
		return "invalid json"
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return "body must be object"
	}
	for k := range obj {
		switch k {
		case "title", "author", "year", "tags":
		default:
			return "unknown field " + k
		}
	}
	if full {
		for _, k := range []string{"title", "author", "year"} {
			if _, ok := obj[k]; !ok {
				return "missing " + k
			}
		}
		b.Tags = []string{}
	}
	if v, ok := obj["title"]; ok {
		s, ok := v.(string)
		if !ok {
			return "title must be string"
		}
		s = strings.TrimSpace(s)
		if s == "" || utf8.RuneCountInString(s) > 200 {
			return "invalid title"
		}
		b.Title = s
	}
	if v, ok := obj["author"]; ok {
		s, ok := v.(string)
		if !ok {
			return "author must be string"
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return "invalid author"
		}
		b.Author = s
	}
	if v, ok := obj["year"]; ok {
		n, ok := v.(json.Number)
		if !ok {
			return "year must be integer"
		}
		y, err := strconv.ParseInt(string(n), 10, 64)
		if err != nil || y < 0 || y > 2100 {
			return "invalid year"
		}
		b.Year = y
	}
	if v, ok := obj["tags"]; ok {
		arr, ok := v.([]any)
		if !ok {
			return "tags must be array"
		}
		tags := make([]string, 0, len(arr))
		for _, e := range arr {
			s, ok := e.(string)
			if !ok {
				return "tags must be strings"
			}
			tags = append(tags, s)
		}
		b.Tags = tags
	}
	return ""
}

func sortedBooks() []*Book {
	out := make([]*Book, 0, len(books))
	for _, b := range books {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func handle(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()
	path := r.URL.Path
	switch {
	case path == "/books":
		switch r.Method {
		case http.MethodGet:
			listBooks(w, r)
		case http.MethodPost:
			b := &Book{}
			if msg := parseBody(r, b, true); msg != "" {
				fail(w, 400, msg)
				return
			}
			b.ID = nextID
			nextID++
			books[b.ID] = b
			writeJSON(w, 201, b)
		default:
			fail(w, 405, "method not allowed")
		}
	case path == "/stats":
		if r.Method != http.MethodGet {
			fail(w, 405, "method not allowed")
			return
		}
		stats(w)
	case strings.HasPrefix(path, "/books/") && !strings.Contains(path[len("/books/"):], "/") && len(path) > len("/books/"):
		seg := path[len("/books/"):]
		switch r.Method {
		case http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:
			fail(w, 405, "method not allowed")
			return
		}
		var b *Book
		if isDigits(seg) {
			if id, err := strconv.ParseInt(seg, 10, 64); err == nil {
				b = books[id]
			}
		}
		if b == nil {
			fail(w, 404, "not found")
			return
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, 200, b)
		case http.MethodDelete:
			delete(books, b.ID)
			w.WriteHeader(204)
		case http.MethodPut, http.MethodPatch:
			nb := *b
			nb.Tags = append([]string{}, b.Tags...)
			if msg := parseBody(r, &nb, r.Method == http.MethodPut); msg != "" {
				fail(w, 400, msg)
				return
			}
			*b = nb
			writeJSON(w, 200, b)
		}
	default:
		fail(w, 404, "not found")
	}
}

func listBooks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	offset, limit := 0, -1
	if _, ok := q["offset"]; ok {
		s := q.Get("offset")
		n, err := strconv.Atoi(s)
		if !isDigits(s) || err != nil {
			fail(w, 400, "invalid offset")
			return
		}
		offset = n
	}
	if _, ok := q["limit"]; ok {
		s := q.Get("limit")
		n, err := strconv.Atoi(s)
		if !isDigits(s) || err != nil {
			fail(w, 400, "invalid limit")
			return
		}
		limit = n
	}
	_, hasA := q["author"]
	_, hasT := q["tag"]
	a, t := q.Get("author"), q.Get("tag")
	out := []*Book{}
	for _, b := range sortedBooks() {
		if hasA && b.Author != a {
			continue
		}
		if hasT {
			found := false
			for _, x := range b.Tags {
				if x == t {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		out = append(out, b)
	}
	if offset >= len(out) {
		out = []*Book{}
	} else {
		out = out[offset:]
	}
	if limit >= 0 && limit < len(out) {
		out = out[:limit]
	}
	writeJSON(w, 200, out)
}

func stats(w http.ResponseWriter) {
	byAuthor := map[string]int{}
	tagCount := map[string]int{}
	for _, b := range books {
		byAuthor[b.Author]++
		seen := map[string]bool{}
		for _, t := range b.Tags {
			if !seen[t] {
				seen[t] = true
				tagCount[t]++
			}
		}
	}
	type kv struct {
		k string
		v int
	}
	list := []kv{}
	for k, v := range tagCount {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].v != list[j].v {
			return list[i].v > list[j].v
		}
		return list[i].k < list[j].k
	})
	top := [][]any{}
	for i := 0; i < len(list) && i < 3; i++ {
		top = append(top, []any{list[i].k, list[i].v})
	}
	writeJSON(w, 200, map[string]any{"count": len(books), "by_author": byAuthor, "top_tags": top})
}

func main() {
	http.ListenAndServe("127.0.0.1:"+os.Getenv("PORT"), http.HandlerFunc(handle))
}
