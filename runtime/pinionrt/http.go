package pinionrt

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

// ---------- std/http (DESIGN 3.21) ----------

var (
	HTTPRequestType  = &TypeInfo{Name: "Request", Module: "std/http"}
	HTTPResponseType = &TypeInfo{Name: "Response", Module: "std/http"}
	HTTPAppType      = &TypeInfo{Name: "App", Module: "std/http", Class: true}
)

// netReal is false in hermetic tests: app.listen is not available there.
var netReal = true

// serveMu serialises handler execution so that class state captured by
// handlers is never touched by two requests at once (no data races).
var serveMu sync.Mutex

type route struct {
	method string
	segs   []string
	h      Value
}

type appState struct {
	mu     sync.Mutex
	routes []route
}

var (
	appsMu sync.Mutex
	apps   = map[*Object]*appState{}
)

func appOf(self Value) *appState {
	appsMu.Lock()
	defer appsMu.Unlock()
	return apps[self.(*Object)]
}

func lowerHeaders(v Value) *Map {
	out := &Map{}
	if m, ok := v.(*Map); ok {
		for i, k := range m.keys.items {
			out.put(strings.ToLower(k.(string)), m.vals[i])
		}
	}
	return out
}

func queryMap(raw string) *Map {
	out := &Map{}
	for _, part := range strings.Split(raw, "&") {
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		if uk, err := url.QueryUnescape(k); err == nil {
			k = uk
		}
		if uv, err := url.QueryUnescape(v); err == nil {
			v = uv
		}
		if _, dup := out.Get(k); !dup {
			out.put(k, v)
		}
	}
	return out
}

func mkRequest(method, path, rawQuery, body string, headers *Map) *Object {
	return &Object{T: HTTPRequestType, F: []Value{strings.ToUpper(method), path, &Map{}, queryMap(rawQuery), headers, body}}
}

func mkResponse(status int64, body string, ctype string) *Object {
	h := &Map{}
	if ctype != "" {
		h.put("content-type", ctype)
	}
	return &Object{T: HTTPResponseType, F: []Value{status, body, h}}
}

func errorResponse(status int64, msg string) *Object {
	return mkResponse(status, JSONEncode(MapLit("error", msg)).(string), "application/json")
}

func splitPath(p string) []string {
	var out []string
	for _, s := range strings.Split(p, "/") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// match returns the number of literal segments matched and the params, or -1.
func (r *route) match(segs []string) (int, *Map) {
	if len(r.segs) != len(segs) {
		return -1, nil
	}
	lits := 0
	params := &Map{}
	for i, s := range r.segs {
		if strings.HasPrefix(s, ":") {
			params.put(s[1:], segs[i])
			continue
		}
		if s != segs[i] {
			return -1, nil
		}
		lits++
	}
	return lits, params
}

// dispatch routes req: the matching route with the most literal segments
// wins (exact segments beat :params; ties go to the first registered).
func (a *appState) dispatch(req *Object) Value {
	segs := splitPath(req.F[1].(string))
	method := req.F[0].(string)
	a.mu.Lock()
	routes := append([]route(nil), a.routes...)
	a.mu.Unlock()
	best, bestScore, pathMatched := -1, -1, false
	var bestParams *Map
	for i := range routes {
		score, params := routes[i].match(segs)
		if score < 0 {
			continue
		}
		pathMatched = true
		if routes[i].method == method && score > bestScore {
			best, bestScore, bestParams = i, score, params
		}
	}
	if best < 0 {
		if pathMatched {
			return errorResponse(405, "method not allowed")
		}
		return errorResponse(404, "not found")
	}
	r := &Object{T: HTTPRequestType, F: append([]Value(nil), req.F...)}
	r.F[2] = bestParams
	return handlerResult(Call(routes[best].h, r))
}

// handlerResult accepts Response or Result<Response, Response | Str>:
// Ok(r) and Err(r) are sent as is, Err(msg) becomes 400 {"error": msg}.
func handlerResult(v Value) Value {
	res, ok := v.(*Result)
	if !ok {
		return v
	}
	if msg, isStr := res.V.(string); !res.IsOk && isStr {
		return errorResponse(400, msg)
	}
	return res.V
}

func (a *appState) serveHTTP(w http.ResponseWriter, hr *http.Request) {
	b, _ := io.ReadAll(hr.Body)
	headers := &Map{}
	for k, vs := range hr.Header {
		headers.put(strings.ToLower(k), strings.Join(vs, ", "))
	}
	req := mkRequest(hr.Method, hr.URL.Path, hr.URL.RawQuery, strings.ToValidUTF8(string(b), "�"), headers)
	serveMu.Lock()
	defer serveMu.Unlock()
	var resp Value
	func() {
		defer func() {
			if r := recover(); r != nil {
				d, ok := r.(*Defect)
				if !ok {
					d = &Defect{Code: "R0100", Msg: fmt.Sprint(r)}
				}
				report(d)
				os.Exit(70)
			}
		}()
		resp = a.dispatch(req)
	}()
	flushStdout()
	o := resp.(*Object)
	if hm, ok := o.F[2].(*Map); ok {
		for i, k := range hm.keys.items {
			w.Header().Set(k.(string), Display(hm.vals[i]))
		}
	}
	w.WriteHeader(int(o.F[0].(int64)))
	io.WriteString(w, o.F[1].(string))
}

func (a *appState) listen(port int64, host string) {
	if !netReal {
		Abort("R0109", "app.listen: the network is not available in hermetic tests; use app.handle(http.request(...))")
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(host, fmt.Sprint(port)))
	if err != nil {
		flushStdout()
		fmt.Fprintln(os.Stderr, "error: listen: "+err.Error())
		os.Exit(1)
	}
	srv := &http.Server{Handler: http.HandlerFunc(a.serveHTTP), ErrorLog: nil}
	err = srv.Serve(ln)
	flushStdout()
	fmt.Fprintln(os.Stderr, "error: serve: "+err.Error())
	os.Exit(1)
}

func init() {
	HTTPRequestType.Fields = []FieldInfo{
		{Name: "method", Type: TStr, Pub: true},
		{Name: "path", Type: TStr, Pub: true},
		{Name: "params", Type: TK(KMap, TStr, TStr), Pub: true},
		{Name: "query", Type: TK(KMap, TStr, TStr), Pub: true},
		{Name: "headers", Type: TK(KMap, TStr, TStr), Pub: true},
		{Name: "body", Type: TStr, Pub: true},
	}
	HTTPResponseType.Fields = []FieldInfo{
		{Name: "status", Type: TInt, Pub: true},
		{Name: "body", Type: TStr, Pub: true},
		{Name: "headers", Type: TK(KMap, TStr, TStr), Pub: true},
	}
	for _, m := range []string{"get", "post", "put", "patch", "delete"} {
		method := strings.ToUpper(m)
		HTTPAppType.AddMethod(m, []string{"path", "h"}, func(self Value, a []Value) Value {
			st := appOf(self)
			st.mu.Lock()
			st.routes = append(st.routes, route{method: method, segs: splitPath(a[0].(string)), h: a[1]})
			st.mu.Unlock()
			return UnitV
		})
	}
	HTTPAppType.AddMethod("handle", []string{"req"}, func(self Value, a []Value) Value {
		return appOf(self).dispatch(a[0].(*Object))
	})
	HTTPAppType.AddMethod("listen", []string{"port", "host"}, func(self Value, a []Value) Value {
		host := "127.0.0.1" // loopback only unless the program asks for more
		if len(a) > 1 && a[1] != nil {
			host = a[1].(string)
		}
		appOf(self).listen(a[0].(int64), host)
		return UnitV
	})
	opt := func(name string, defs ...Value) {
		m := map[int]Value{}
		for i := 0; i+1 < len(defs); i += 2 {
			m[defs[i].(int)] = defs[i+1]
		}
		stdDefaults[name] = m
	}
	opt("http.request", 2, "", 3, &Map{})
	opt("http.json", 1, int64(200))
	opt("http.text", 1, int64(200))
	for _, t := range []*TypeInfo{HTTPRequestType, HTTPResponseType} {
		t.AddMethod("json", nil, func(self Value, _ []Value) Value { return JSONDecode(TAnyT, GetField(self, "body")) })
	}
	stdFns["http.App"] = stdFn{nil, func(a []Value) Value {
		o := &Object{T: HTTPAppType}
		o.id = nextID()
		appsMu.Lock()
		apps[o] = &appState{}
		appsMu.Unlock()
		return o
	}}
	stdFns["http.request"] = stdFn{[]string{"method", "path", "body", "headers"}, func(a []Value) Value {
		p, q, _ := strings.Cut(a[1].(string), "?")
		return mkRequest(a[0].(string), p, q, a[2].(string), lowerHeaders(a[3]))
	}}
	stdFns["http.json"] = stdFn{[]string{"v", "status"}, func(a []Value) Value {
		return mkResponse(a[1].(int64), JSONEncode(a[0]).(string), "application/json")
	}}
	stdFns["http.text"] = stdFn{[]string{"s", "status"}, func(a []Value) Value {
		return mkResponse(a[1].(int64), a[0].(string), "text/plain; charset=utf-8")
	}}
	stdFns["http.empty"] = stdFn{[]string{"status"}, func(a []Value) Value {
		return mkResponse(a[0].(int64), "", "")
	}}
	stdFns["http.error"] = stdFn{[]string{"status", "msg"}, func(a []Value) Value {
		return errorResponse(a[0].(int64), a[1].(string))
	}}
	stdFns["http.Request"] = stdFn{fieldNames(HTTPRequestType), func(a []Value) Value {
		return &Object{T: HTTPRequestType, F: a}
	}}
	stdFns["http.Response"] = stdFn{fieldNames(HTTPResponseType), func(a []Value) Value {
		return &Object{T: HTTPResponseType, F: a}
	}}
}

func fieldNames(t *TypeInfo) []string {
	var out []string
	for _, f := range t.Fields {
		out = append(out, f.Name)
	}
	return out
}

// MT is a method call with explicit type arguments (req.json<T>()).
func MT(recv Value, name string, targs []*Type, names []string, args ...Value) Value {
	if o, ok := recv.(*Object); ok && name == "json" && (o.T == HTTPRequestType || o.T == HTTPResponseType) {
		t := TAnyT
		if len(targs) > 0 {
			t = targs[0]
		}
		return JSONDecode(t, GetField(o, "body"))
	}
	return M(recv, name, names, args...)
}
