// Package diag holds compiler diagnostics and their JSON form (schema pinion.diag/1).
package diag

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/pinionlang/pinion/internal/token"
)

type Severity string

const (
	Error   Severity = "error"
	Warning Severity = "warning"
)

type FixKind string

const (
	FixAuto     FixKind = "auto"
	FixReview   FixKind = "review"
	FixScaffold FixKind = "scaffold"
)

type Edit struct {
	Start token.Pos
	End   token.Pos
	Text  string
}

type Fix struct {
	Kind  FixKind
	Title string
	Edits []Edit
}

type Diagnostic struct {
	Code     string
	Severity Severity
	Pos      token.Pos
	End      token.Pos // optional
	Msg      string
	Fixes    []Fix
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s: %s %s: %s", d.Pos, d.Severity, d.Code, d.Msg)
}

// List collects diagnostics. The zero value is ready to use.
type List struct {
	Items []Diagnostic
}

func (l *List) Add(d Diagnostic) { l.Items = append(l.Items, d) }

func (l *List) Errorf(code string, pos token.Pos, format string, args ...any) {
	l.Add(Diagnostic{Code: code, Severity: Error, Pos: pos, Msg: fmt.Sprintf(format, args...)})
}

func (l *List) Warnf(code string, pos token.Pos, format string, args ...any) {
	l.Add(Diagnostic{Code: code, Severity: Warning, Pos: pos, Msg: fmt.Sprintf(format, args...)})
}

func (l *List) HasErrors() bool {
	for _, d := range l.Items {
		if d.Severity == Error {
			return true
		}
	}
	return false
}

// Sort orders diagnostics by file, line, column, code.
func (l *List) Sort() {
	sort.SliceStable(l.Items, func(i, j int) bool {
		a, b := l.Items[i].Pos, l.Items[j].Pos
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Col != b.Col {
			return a.Col < b.Col
		}
		return l.Items[i].Code < l.Items[j].Code
	})
}

// Human renders diagnostics for terminals.
func (l *List) Human() string {
	var b strings.Builder
	for _, d := range l.Items {
		b.WriteString(d.String())
		b.WriteByte('\n')
	}
	return b.String()
}

type jsonPos [2]int

type jsonRange struct {
	Start jsonPos `json:"start"`
	End   jsonPos `json:"end"`
}

type jsonEdit struct {
	Range jsonRange `json:"range"`
	Text  string    `json:"text"`
}

type jsonFix struct {
	Kind     FixKind    `json:"kind"`
	Title    string     `json:"title"`
	FileHash string     `json:"file_hash,omitempty"`
	Edits    []jsonEdit `json:"edits"`
}

type jsonDiag struct {
	Code     string    `json:"code"`
	Severity Severity  `json:"severity"`
	File     string    `json:"file"`
	Range    jsonRange `json:"range"`
	Msg      string    `json:"msg"`
	Fixes    []jsonFix `json:"fixes,omitempty"`
}

// JSON renders the pinion.diag/1 document. hashes maps file path -> "sha256:..."
func (l *List) JSON(hashes map[string]string) ([]byte, error) {
	out := struct {
		Schema      string     `json:"schema"`
		Diagnostics []jsonDiag `json:"diagnostics"`
	}{Schema: "pinion.diag/1", Diagnostics: []jsonDiag{}}
	for _, d := range l.Items {
		end := d.End
		if !end.IsValid() {
			end = d.Pos
		}
		jd := jsonDiag{Code: d.Code, Severity: d.Severity, File: d.Pos.File,
			Range: jsonRange{jsonPos{d.Pos.Line, d.Pos.Col}, jsonPos{end.Line, end.Col}}, Msg: d.Msg}
		for _, f := range d.Fixes {
			jf := jsonFix{Kind: f.Kind, Title: f.Title, FileHash: hashes[d.Pos.File]}
			for _, e := range f.Edits {
				jf.Edits = append(jf.Edits, jsonEdit{jsonRange{jsonPos{e.Start.Line, e.Start.Col}, jsonPos{e.End.Line, e.End.Col}}, e.Text})
			}
			jd.Fixes = append(jd.Fixes, jf)
		}
		out.Diagnostics = append(out.Diagnostics, jd)
	}
	return json.MarshalIndent(out, "", "  ")
}
