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
func errJSON(w http.ResponseWriter, code int, msg string) {
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

// parseBody applies fields onto b; full requires all required fields.
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
		var y int64
		if i, err := strconv.ParseInt(n.String(), 10, 64); err == nil {
			y = i
		} else {
			f, err := strconv.ParseFloat(n.String(), 64)
			if err != nil || f != math.Trunc(f) || f < 0 || f > 2100 {
				return "year must be integer"
			}
			y = int64(f)
		}
		if y < 0 || y > 2100 {
			return "year out of range"
		}
		b.Year = y
	}
	if v, ok := obj["tags"]; ok {
		arr, ok := v.([]any)
		if !ok {
			return "tags must be array"
		}
		tags := make([]string, 0, len(arr))
		for _, t := range arr {
			s, ok := t.(string)
			if !ok {
				return "tags must be strings"
			}
			tags = append(tags, s)
		}
		b.Tags = tags
	}
	return ""
}

func copyBook(b *Book) Book {
	c := *b
	c.Tags = append([]string{}, b.Tags...)
	return c
}

func handleBooks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		b := &Book{}
		if msg := parseBody(r, b, true); msg != "" {
			errJSON(w, 400, msg)
			return
		}
		mu.Lock()
		b.ID = nextID
		nextID++
		books[b.ID] = b
		out := copyBook(b)
		mu.Unlock()
		writeJSON(w, 201, out)
	case http.MethodGet:
		q := r.URL.Query()
		offset, limit := 0, -1
		if q.Has("offset") {
			s := q.Get("offset")
			n, err := strconv.Atoi(s)
			if !isDigits(s) || err != nil {
				errJSON(w, 400, "invalid offset")
				return
			}
			offset = n
		}
		if q.Has("limit") {
			s := q.Get("limit")
			n, err := strconv.Atoi(s)
			if !isDigits(s) || err != nil {
				errJSON(w, 400, "invalid limit")
				return
			}
			limit = n
		}
		mu.Lock()
		list := []Book{}
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
			list = append(list, copyBook(b))
		}
		mu.Unlock()
		sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
		if offset >= len(list) {
			list = []Book{}
		} else {
			list = list[offset:]
		}
		if limit >= 0 && limit < len(list) {
			list = list[:limit]
		}
		writeJSON(w, 200, list)
	default:
		errJSON(w, 405, "method not allowed")
	}
}

func handleBook(w http.ResponseWriter, r *http.Request, seg string) {
	switch r.Method {
	case http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		errJSON(w, 405, "method not allowed")
		return
	}
	if !isDigits(seg) {
		errJSON(w, 404, "not found")
		return
	}
	id, err := strconv.ParseInt(seg, 10, 64)
	if err != nil {
		errJSON(w, 404, "not found")
		return
	}
	mu.Lock()
	defer mu.Unlock()
	b, ok := books[id]
	if !ok {
		errJSON(w, 404, "not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, copyBook(b))
	case http.MethodDelete:
		delete(books, id)
		w.WriteHeader(204)
	case http.MethodPut, http.MethodPatch:
		nb := copyBook(b)
		if msg := parseBody(r, &nb, r.Method == http.MethodPut); msg != "" {
			errJSON(w, 400, msg)
			return
		}
		*b = nb
		writeJSON(w, 200, copyBook(b))
	}
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errJSON(w, 405, "method not allowed")
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
	type tc struct {
		t string
		c int
	}
	var tcs []tc
	for t, c := range tagCount {
		tcs = append(tcs, tc{t, c})
	}
	sort.Slice(tcs, func(i, j int) bool {
		if tcs[i].c != tcs[j].c {
			return tcs[i].c > tcs[j].c
		}
		return tcs[i].t < tcs[j].t
	})
	top := [][]any{}
	for i := 0; i < len(tcs) && i < 3; i++ {
		top = append(top, []any{tcs[i].t, tcs[i].c})
	}
	writeJSON(w, 200, map[string]any{"count": count, "by_author": byAuthor, "top_tags": top})
}

func router(w http.ResponseWriter, r *http.Request) {
	p := r.URL.EscapedPath()
	switch {
	case p == "/books":
		handleBooks(w, r)
	case p == "/stats":
		handleStats(w, r)
	case strings.HasPrefix(p, "/books/") && !strings.Contains(p[7:], "/"):
		handleBook(w, r, p[7:])
	default:
		errJSON(w, 404, "not found")
	}
}

func main() {
	srv := &http.Server{Addr: "127.0.0.1:" + os.Getenv("PORT"), Handler: http.HandlerFunc(router)}
	if err := srv.ListenAndServe(); err != nil {
		panic(err)
	}
}
