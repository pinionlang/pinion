// Package conformance loads spec/conformance/*.pi cases (format: spec/conformance/README.md).
package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// CaseFile is one source or data file of a case. For single-file cases Path is
// "main.pi". Content line N (1-based) corresponds to marker line N.
type CaseFile struct {
	Path    string
	Content string
}

// Marker is an expected diagnostic: code at File:Line.
type Marker struct {
	File string
	Line int
	Code string
}

type TestExpect struct {
	Name   string
	Result string // "passed", "failed", "aborted", "timeout"
	Code   string // "T0001", "R0102", ... or ""
}

type Case struct {
	SpecFile  string // e.g. "01-frozen.pi"
	Name      string
	Expect    string // ok | error | warning | run | abort | tests
	AbortCode string // for expect abort
	Stdout    []string
	Exit      int
	ExitSet   bool
	Tests     []TestExpect
	Files     []CaseFile
	Markers   []Marker
	MultiFile bool
}

// ID is "specfile#case".
func (c *Case) ID() string { return strings.TrimSuffix(c.SpecFile, ".pi") + "#" + c.Name }

// IsSource reports whether a case file is Pinion source.
func IsSource(path string) bool { return strings.HasSuffix(path, ".pi") }

// DefaultExit returns the expected exit code per README rules.
func (c *Case) DefaultExit() int {
	switch c.Expect {
	case "error":
		return 1
	case "abort":
		return 70
	case "tests":
		for _, t := range c.Tests {
			if t.Result != "passed" {
				return 1
			}
		}
		return 0
	}
	return 0
}

func (c *Case) WantExit() int {
	if c.ExitSet {
		return c.Exit
	}
	return c.DefaultExit()
}

var (
	reDirective = regexp.MustCompile(`^//#\s*(\w+):\s*(.*)$`)
	reMarker    = regexp.MustCompile(`//~((?:\s+[EWRT]\d{4})+)\s*$`)
	reTest      = regexp.MustCompile(`^(.*?)\s*->\s*(passed|failed|aborted|timeout)(?:\s+([A-Z]\d{4}))?\s*$`)
)

// LoadDir loads every *.pi spec file in dir, sorted by name.
func LoadDir(dir string) ([]*Case, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.pi"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var all []*Case
	for _, p := range paths {
		src, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		cs, err := Parse(filepath.Base(p), string(src))
		if err != nil {
			return nil, err
		}
		all = append(all, cs...)
	}
	return all, nil
}

// Parse splits one spec file into cases.
func Parse(specName, src string) ([]*Case, error) {
	lines := strings.Split(src, "\n")
	var cases []*Case
	var cur *Case
	var curFile *CaseFile
	var buf []string
	inHeader := false

	flushFile := func() {
		if cur == nil {
			return
		}
		if curFile != nil {
			curFile.Content = strings.Join(trimTrailingBlank(buf), "\n") + "\n"
			cur.Files = append(cur.Files, *curFile)
		}
		curFile = nil
		buf = nil
	}
	flushCase := func() {
		if cur == nil {
			return
		}
		flushFile()
		cases = append(cases, cur)
	}

	for i, line := range lines {
		m := reDirective.FindStringSubmatch(strings.TrimSpace(line))
		if m != nil {
			key, val := m[1], strings.TrimSpace(m[2])
			switch key {
			case "case":
				flushCase()
				cur = &Case{SpecFile: specName, Name: val}
				curFile = nil
				buf = nil
				inHeader = true
				continue
			case "file":
				if cur == nil {
					return nil, fmt.Errorf("%s:%d: file directive outside case", specName, i+1)
				}
				if !cur.MultiFile {
					// discard anything collected before the first file
					buf = nil
					curFile = nil
				} else {
					flushFile()
				}
				cur.MultiFile = true
				curFile = &CaseFile{Path: val}
				buf = nil
				inHeader = false
				continue
			}
			if cur == nil {
				return nil, fmt.Errorf("%s:%d: directive outside case", specName, i+1)
			}
			if inHeader {
				switch key {
				case "expect":
					parts := strings.Fields(val)
					cur.Expect = parts[0]
					if cur.Expect == "abort" && len(parts) > 1 {
						cur.AbortCode = parts[1]
					}
				case "stdout":
					cur.Stdout = append(cur.Stdout, m[2])
				case "exit":
					n, err := strconv.Atoi(val)
					if err != nil {
						return nil, fmt.Errorf("%s:%d: bad exit", specName, i+1)
					}
					cur.Exit, cur.ExitSet = n, true
				case "test":
					tm := reTest.FindStringSubmatch(val)
					if tm == nil {
						return nil, fmt.Errorf("%s:%d: bad test line %q", specName, i+1, val)
					}
					cur.Tests = append(cur.Tests, TestExpect{Name: tm[1], Result: tm[2], Code: tm[3]})
				default:
					return nil, fmt.Errorf("%s:%d: unknown directive %q", specName, i+1, key)
				}
				continue
			}
		}
		if cur == nil {
			continue // file header comments before first case
		}
		if inHeader {
			inHeader = false
			if !cur.MultiFile {
				curFile = &CaseFile{Path: "main.pi"}
				buf = nil
			}
		}
		if curFile == nil && !cur.MultiFile {
			curFile = &CaseFile{Path: "main.pi"}
		}
		if curFile == nil {
			continue
		}
		buf = append(buf, line)
		if mm := reMarker.FindStringSubmatch(line); mm != nil {
			for _, code := range strings.Fields(mm[1]) {
				cur.Markers = append(cur.Markers, Marker{File: curFile.Path, Line: len(buf), Code: code})
			}
		}
	}
	flushCase()
	for _, c := range cases {
		if c.Expect == "" {
			return nil, fmt.Errorf("%s: case %s has no expect", specName, c.Name)
		}
	}
	return cases, nil
}

func trimTrailingBlank(ls []string) []string {
	for len(ls) > 0 && strings.TrimSpace(ls[len(ls)-1]) == "" {
		ls = ls[:len(ls)-1]
	}
	return ls
}
