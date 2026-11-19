package main

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

type Book struct {
	ID     int      `json:"id"`
	Title  string   `json:"title"`
	Author string   `json:"author"`
	Year   int      `json:"year"`
	Tags   []string `json:"tags"`
}

var (
	mu     sync.Mutex
	books  = map[int]*Book{}
	nextID = 1
)

func send(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	send(w, code, map[string]string{"error": msg})
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

// parseBody validates fields; if full, required fields must be present. Applies to b.
func parseBody(r *http.Request, b *Book, full bool) string {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return "cannot read body"
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	var m map[string]json.RawMessage
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return "invalid json"
	}
	if dec.More() {
		return "invalid json"
	}
	if _, err := dec.Token(); err != io.EOF {
		return "invalid json"
	}
	t := bytes.TrimSpace(raw)
	if len(t) == 0 || t[0] != '{' {
		return "body must be an object"
	}
	if err := json.Unmarshal(t, &m); err != nil {
		return "invalid json"
	}
	nb := *b
	for k := range m {
		switch k {
		case "title", "author", "year", "tags":
		default:
			return "unknown field " + k
		}
	}
	if full {
		for _, k := range []string{"title", "author", "year"} {
			if _, ok := m[k]; !ok {
				return "missing " + k
			}
		}
		nb.Tags = []string{}
	}
	isNull := func(v json.RawMessage) bool { return string(bytes.TrimSpace(v)) == "null" }
	if v, ok := m["title"]; ok {
		var s string
		if isNull(v) || json.Unmarshal(v, &s) != nil {
			return "invalid title"
		}
		s = strings.TrimSpace(s)
		if s == "" || utf8.RuneCountInString(s) > 200 {
			return "invalid title"
		}
		nb.Title = s
	}
	if v, ok := m["author"]; ok {
		var s string
		if isNull(v) || json.Unmarshal(v, &s) != nil {
			return "invalid author"
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return "invalid author"
		}
		nb.Author = s
	}
	if v, ok := m["year"]; ok {
		var f float64
		tv := bytes.TrimSpace(v)
		if len(tv) == 0 || !(tv[0] == '-' || (tv[0] >= '0' && tv[0] <= '9')) || json.Unmarshal(tv, &f) != nil {
			return "invalid year"
		}
		if f != math.Trunc(f) || f < 0 || f > 2100 {
			return "invalid year"
		}
		nb.Year = int(f)
	}
	if v, ok := m["tags"]; ok {
		var arr []json.RawMessage
		if isNull(v) || json.Unmarshal(v, &arr) != nil {
			return "invalid tags"
		}
		tags := make([]string, 0, len(arr))
		for _, e := range arr {
			var s string
			if isNull(e) || json.Unmarshal(e, &s) != nil {
				return "invalid tag"
			}
			tags = append(tags, s)
		}
		nb.Tags = tags
	}
	*b = nb
	return ""
}

func booksHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var b Book
		if msg := parseBody(r, &b, true); msg != "" {
			fail(w, 400, msg)
			return
		}
		mu.Lock()
		b.ID = nextID
		nextID++
		books[b.ID] = &b
		c := b
		mu.Unlock()
		send(w, 201, c)
	case http.MethodGet:
		q := r.URL.Query()
		offset, limit := 0, -1
		if q.Has("offset") {
			s := q.Get("offset")
			n, err := strconv.Atoi(s)
			if !isDigits(s) || err != nil {
				fail(w, 400, "invalid offset")
				return
			}
			offset = n
		}
		if q.Has("limit") {
			s := q.Get("limit")
			n, err := strconv.Atoi(s)
			if !isDigits(s) || err != nil {
				fail(w, 400, "invalid limit")
				return
			}
			limit = n
		}
		mu.Lock()
		res := []Book{}
		for _, b := range books {
			if q.Has("author") && b.Author != q.Get("author") {
				continue
			}
			if q.Has("tag") {
				found := false
				for _, t := range b.Tags {
					if t == q.Get("tag") {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}
			res = append(res, *b)
		}
		mu.Unlock()
		sort.Slice(res, func(i, j int) bool { return res[i].ID < res[j].ID })
		if offset >= len(res) {
			res = []Book{}
		} else {
			res = res[offset:]
		}
		if limit >= 0 && limit < len(res) {
			res = res[:limit]
		}
		send(w, 200, res)
	default:
		fail(w, 405, "method not allowed")
	}
}

func bookHandler(w http.ResponseWriter, r *http.Request, seg string) {
	switch r.Method {
	case http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		fail(w, 405, "method not allowed")
		return
	}
	id, err := strconv.Atoi(seg)
	if !isDigits(seg) || err != nil {
		fail(w, 404, "not found")
		return
	}
	mu.Lock()
	defer mu.Unlock()
	b, ok := books[id]
	if !ok {
		fail(w, 404, "not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		send(w, 200, b)
	case http.MethodDelete:
		delete(books, id)
		w.WriteHeader(204)
	default:
		if msg := parseBody(r, b, r.Method == http.MethodPut); msg != "" {
			fail(w, 400, msg)
			return
		}
		send(w, 200, b)
	}
}

func statsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		fail(w, 405, "method not allowed")
		return
	}
	mu.Lock()
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
	count := len(books)
	mu.Unlock()
	keys := make([]string, 0, len(tagCount))
	for k := range tagCount {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if tagCount[keys[i]] != tagCount[keys[j]] {
			return tagCount[keys[i]] > tagCount[keys[j]]
		}
		return keys[i] < keys[j]
	})
	top := [][]any{}
	for i := 0; i < len(keys) && i < 3; i++ {
		top = append(top, []any{keys[i], tagCount[keys[i]]})
	}
	send(w, 200, map[string]any{"count": count, "by_author": byAuthor, "top_tags": top})
}

func main() {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case p == "/books":
			booksHandler(w, r)
		case p == "/stats":
			statsHandler(w, r)
		case strings.HasPrefix(p, "/books/") && !strings.Contains(p[7:], "/"):
			bookHandler(w, r, p[7:])
		default:
			fail(w, 404, "not found")
		}
	})
	http.ListenAndServe("127.0.0.1:"+os.Getenv("PORT"), h)
}
