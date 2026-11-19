// Command pinion is the Pinion compiler CLI.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pinionlang/pinion/internal/diag"
	"github.com/pinionlang/pinion/internal/driver"
	"github.com/pinionlang/pinion/internal/format"
	"github.com/pinionlang/pinion/internal/outline"
)

const usage = `usage:
  pinion run [--snapshot] [--no-check] <file.pi | project-dir> [args...]
  pinion build [-o out] [--emit go] [--no-check] <file.pi | project-dir>
  pinion test [--json] [--snapshot] [--no-check] [path]
  pinion check [--json] <path>
  pinion fmt [-w] <files...>
  pinion outline <path>
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var code int
	switch cmd {
	case "run":
		code = cmdRun(args)
	case "build":
		code = cmdBuild(args)
	case "test":
		code = cmdTest(args)
	case "check":
		code = cmdCheck(args)
	case "fmt":
		code = cmdFmt(args)
	case "outline":
		code = cmdOutline(args)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "pinion: unknown command %q\n%s", cmd, usage)
		code = 2
	}
	os.Exit(code)
}

func load(path string, noCheck bool) (*driver.Program, int) {
	p, err := driver.Load(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return nil, 1
	}
	if !noCheck {
		d := p.Check()
		if len(d.Items) > 0 {
			fmt.Fprint(os.Stderr, d.Human())
		}
		if d.HasErrors() {
			return nil, 1
		}
	}
	return p, 0
}

func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	snapshot := fs.Bool("snapshot", false, "include value snapshots in runtime diagnostics")
	noCheck := fs.Bool("no-check", false, "skip the static checker")
	fs.Parse(args)
	if fs.NArg() < 1 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	p, code := load(fs.Arg(0), *noCheck)
	if p == nil {
		return code
	}
	bin, err := p.Build()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	c := exec.Command(bin, fs.Args()[1:]...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	c.Env = os.Environ()
	if *snapshot {
		c.Env = append(c.Env, "PINION_SNAPSHOT=1")
	}
	if err := c.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func cmdBuild(args []string) int {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	out := fs.String("o", "", "output file")
	emit := fs.String("emit", "bin", "output kind: bin or go")
	noCheck := fs.Bool("no-check", false, "skip the static checker")
	fs.Parse(args)
	if fs.NArg() != 1 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	p, code := load(fs.Arg(0), *noCheck)
	if p == nil {
		return code
	}
	if *emit == "go" {
		src, err := p.GoSource()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if *out == "" {
			os.Stdout.Write(src)
			return 0
		}
		if err := os.WriteFile(*out, src, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
	bin, err := p.Build()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	dst := *out
	if dst == "" {
		abs, _ := filepath.Abs(fs.Arg(0))
		dst = filepath.Base(abs)
		if ext := filepath.Ext(dst); ext == ".pi" {
			dst = dst[:len(dst)-len(ext)]
		}
	}
	if err := copyFile(bin, dst); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	o, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(o, in); err != nil {
		o.Close()
		return err
	}
	return o.Close()
}

func cmdTest(args []string) int {
	fs := flag.NewFlagSet("test", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "emit pinion.test/1 JSON")
	snapshot := fs.Bool("snapshot", false, "include value snapshots also for tests declaring real effects")
	noCheck := fs.Bool("no-check", false, "skip the static checker")
	fs.Parse(args)
	path := "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}
	p, code := load(path, *noCheck)
	if p == nil {
		return code
	}
	bin, err := p.Build()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	wd, _ := os.Getwd()
	var env []string
	if *snapshot {
		env = append(os.Environ(), "PINION_SNAPSHOT=1")
	}
	res, err := driver.RunTests(bin, wd, env)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	passed := 0
	for _, r := range res {
		if r.Result == "passed" {
			passed++
		}
	}
	if *asJSON {
		doc := map[string]any{"schema": "pinion.test/1", "tests": res,
			"summary": map[string]int{"total": len(res), "passed": passed, "failed": len(res) - passed}}
		if res == nil {
			doc["tests"] = []driver.TestResult{}
		}
		b, _ := json.MarshalIndent(doc, "", "  ")
		fmt.Println(string(b))
	} else {
		for _, r := range res {
			line := fmt.Sprintf("%-8s %s", r.Result, r.Name)
			if r.Code != "" {
				line += " (" + r.Code + ")"
			}
			fmt.Println(line)
			if r.Result != "passed" {
				if len(r.Diag) > 0 {
					fmt.Println("    " + string(r.Diag))
				}
				if r.Output != "" {
					fmt.Print("    output:\n" + indent(r.Output))
				}
			}
		}
		fmt.Printf("%d/%d passed\n", passed, len(res))
	}
	if passed != len(res) {
		return 1
	}
	return 0
}

func indent(s string) string {
	out := ""
	for _, l := range splitLines(s) {
		out += "      " + l + "\n"
	}
	return out
}

func splitLines(s string) []string {
	var ls []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			ls = append(ls, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		ls = append(ls, s[start:])
	}
	return ls
}

func cmdCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "emit pinion.diag/1 JSON")
	fs.Parse(args)
	path := "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}
	p, err := driver.Load(path)
	var d *diag.List
	hashes := map[string]string{}
	if err != nil {
		de, ok := err.(*driver.ErrDiagnostics)
		if !ok {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		d = de.List
	} else {
		d = p.Check()
		for k, v := range p.Sources {
			s := sha256.Sum256(v)
			hashes[k] = "sha256:" + hex.EncodeToString(s[:])
		}
	}
	if *asJSON {
		b, _ := d.JSON(hashes)
		fmt.Println(string(b))
	} else {
		fmt.Print(d.Human())
	}
	if d.HasErrors() {
		return 1
	}
	return 0
}

func cmdFmt(args []string) int {
	fs := flag.NewFlagSet("fmt", flag.ExitOnError)
	write := fs.Bool("w", false, "write result to the file instead of stdout")
	fs.Parse(args)
	code := 0
	for _, f := range fs.Args() {
		src, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
			continue
		}
		out, d := format.Format(f, src)
		if d != nil && d.HasErrors() {
			fmt.Fprint(os.Stderr, d.Human())
			code = 1
			continue
		}
		if *write {
			if string(out) != string(src) {
				if err := os.WriteFile(f, out, 0o644); err != nil {
					fmt.Fprintln(os.Stderr, err)
					code = 1
				}
			}
		} else {
			os.Stdout.Write(out)
		}
	}
	return code
}

func cmdOutline(args []string) int {
	if len(args) != 1 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	p, err := driver.Load(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	p.Check() // annotates script mains with their inferred effects
	fmt.Print(outline.Outline(p.Files))
	return 0
}
