package pinionrt

import "testing"

func handler(f func(req *Object) Value) Value {
	return MkFunc(1, "h", func(a []Value) Value { return f(a[0].(*Object)) })
}

func TestHTTPHandleRouting(t *testing.T) {
	app := Std("http", "App", nil)
	M(app, "get", nil, "/books/:id", handler(func(r *Object) Value {
		id, _ := GetField(r, "params").(*Map).Get("id")
		return Std("http", "text", nil, "param "+id.(string))
	}))
	M(app, "get", nil, "/books/new", handler(func(r *Object) Value { return Std("http", "text", nil, "exact") }))
	M(app, "post", nil, "/books", handler(func(r *Object) Value {
		return Std("http", "json", []string{"", "status"}, MapLit("got", GetField(r, "body")), int64(201))
	}))
	M(app, "get", nil, "/q", handler(func(r *Object) Value {
		q, _ := GetField(r, "query").(*Map).Get("a")
		h, _ := GetField(r, "headers").(*Map).Get("x-k")
		return Std("http", "text", nil, q.(string)+"/"+h.(string))
	}))
	call := func(method, path, body string, hdr Value) *Object {
		req := Std("http", "request", []string{"", "", "body", "headers"}, method, path, body, hdr)
		return M(app, "handle", nil, req).(*Object)
	}
	cases := []struct {
		method, path, body string
		status             int64
		want               string
	}{
		{"GET", "/books/7", "", 200, "param 7"},
		{"GET", "/books/new", "", 200, "exact"},
		{"GET", "/books/7/x", "", 404, `{"error":"not found"}`},
		{"DELETE", "/books/7", "", 405, `{"error":"method not allowed"}`},
		{"POST", "/books", "hi", 201, `{"got":"hi"}`},
		{"get", "/q?a=1%202&a=3", "", 200, "1 2/v"},
	}
	for _, c := range cases {
		r := call(c.method, c.path, c.body, MapLit("X-K", "v"))
		if st, body := GetField(r, "status"), GetField(r, "body"); st != c.status || body != c.want {
			t.Errorf("%s %s = %v %q, want %d %q", c.method, c.path, st, body, c.status, c.want)
		}
	}
	r := call("POST", "/books", "x", &Map{})
	if ct, _ := GetField(r, "headers").(*Map).Get("content-type"); ct != "application/json" {
		t.Errorf("content-type = %v", ct)
	}
}

func TestHTTPHelpers(t *testing.T) {
	e := Std("http", "error", nil, int64(422), "bad").(*Object)
	if GetField(e, "body") != `{"error":"bad"}` || GetField(e, "status") != int64(422) {
		t.Errorf("error = %s", Debug(e))
	}
	if b := Std("http", "empty", nil, int64(204)).(*Object); GetField(b, "body") != "" || GetField(b, "status") != int64(204) {
		t.Errorf("empty = %s", Debug(b))
	}
	j := Std("http", "json", nil, ListLit(int64(1), int64(2))).(*Object)
	if GetField(j, "status") != int64(200) || GetField(j, "body") != "[1,2]" {
		t.Errorf("json = %s", Debug(j))
	}
	got := MT(j, "json", []*Type{TK(KList, TInt)}, nil).(*Result)
	if !got.IsOk || !Equal(got.V, ListLit(int64(1), int64(2))) {
		t.Errorf("resp.json = %s", Debug(got))
	}
	bad := MT(Std("http", "text", nil, "nope"), "json", []*Type{TInt}, nil).(*Result)
	if bad.IsOk {
		t.Errorf("decoding text as json should fail")
	}
}
