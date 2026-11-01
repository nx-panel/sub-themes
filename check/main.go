// Command check holds every theme in this repository to the rules a theme
// must meet to be accepted, and parses it the way the panel does, so a
// mistake fails CI instead of reaching a user's browser.
//
// For each top-level theme it checks theme.json, the files (kinds, sizes, no
// links or hidden entries, a README and a preview), that nothing is loaded
// from another host, and that the page parses with the functions the panel
// offers. builtin/ is only parsed: those are the panel's own pages.
//
// With -base (a pull request's base commit) it also requires every theme the
// pull request changes to raise its version, and warns when the theme's
// author is not the one opening the pull request (-author).
//
// With -catalog it then writes the catalogue a panel reads into that
// directory (catalog.go); the build workflow publishes it as a release. It
// writes nothing when a check fails.
//
// It cannot catch a field that does not exist, or every way a script can
// reach another host; open the theme on a panel with /page added to a
// subscription address, and read its scripts.
//
//	go run ./check
//	go run ./check -base origin/main -author octocat
//	go run ./check -catalog dist -build catalog-20261101-120000-abc1234 -commit "$(git rev-parse HEAD)"
package main

import (
	"flag"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// funcs names every helper the panel gives a theme. Parsing only checks that
// a name exists, so the bodies are placeholders; keep the names in step with
// the theme guide's function table.
var funcs = template.FuncMap{}

func init() {
	for _, name := range []string{
		"bytes", "datetime", "date", "when", "json", "qr", "dataURI",
		"b64", "default", "hasPrefix", "urlquery", "num",
	} {
		funcs[name] = func(...any) string { return "" }
	}
}

// entrypoints are the files the panel looks for, in its order.
var entrypoints = []string{"index.html", "sub.html"}

// validName is the panel's rule for a theme directory.
var validName = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._-]{0,63}$`)

// starter is the template for authors: held to every rule but the preview,
// and never published to the catalogue.
const starter = "starter"

type options struct {
	root   string
	base   string
	author string
}

type catalogOptions struct {
	out, build, commit string
}

// result is what a run found: a line per theme or problem, the warnings,
// and whether anything failed.
type result struct {
	lines    []string
	warnings []string
	failed   bool
}

func (r *result) fail(format string, a ...any) {
	r.lines = append(r.lines, fmt.Sprintf(format, a...))
	r.failed = true
}

func main() {
	var o options
	var c catalogOptions
	flag.StringVar(&o.base, "base", "", "the pull request's base commit; themes changed since then must raise their version")
	flag.StringVar(&o.author, "author", "", "the GitHub account opening the pull request")
	flag.StringVar(&c.out, "catalog", "", "after a clean check, write the catalogue into this directory")
	flag.StringVar(&c.build, "build", "", "with -catalog: the release tag the build is published under")
	flag.StringVar(&c.commit, "commit", "", "with -catalog: the commit it is built from")
	flag.Parse()
	o.root = "."
	if c.out != "" && (c.build == "" || c.commit == "") {
		fmt.Fprintln(os.Stderr, "-catalog needs -build and -commit")
		os.Exit(2)
	}

	r := run(o)
	for _, l := range r.lines {
		fmt.Println(l)
	}
	for _, w := range r.warnings {
		if os.Getenv("GITHUB_ACTIONS") == "true" {
			fmt.Println("::warning::" + w)
		} else {
			fmt.Println("warning: " + w)
		}
	}
	if r.failed {
		os.Exit(1)
	}
	if c.out != "" {
		idx, err := buildCatalog(o.root, c.out, c.build, c.commit, time.Now())
		if err != nil {
			fmt.Fprintln(os.Stderr, "catalogue:", err)
			os.Exit(1)
		}
		fmt.Printf("catalogue %s: %d themes in %s\n", idx.Build, len(idx.Themes), c.out)
	}
}

func run(o options) result {
	var r result
	// Every top-level directory with a page is a theme (so a clone of this
	// repository can be the panel's themes directory), and builtin/ holds the
	// panel's own pages one level further down.
	top, _ := filepath.Glob(filepath.Join(o.root, "*"))
	builtin, _ := filepath.Glob(filepath.Join(o.root, "builtin", "*"))

	heads := map[string]manifest{}
	checked := 0
	for _, dir := range top {
		entry := entrypoint(dir)
		if entry == "" {
			continue
		}
		checked++
		name := filepath.Base(dir)
		before := len(r.lines)
		if !validName.MatchString(name) {
			r.fail("%s: the panel skips a directory with this name (letters, digits, '.', '_' and '-' only)", name)
			continue
		}
		if err := parse(filepath.Join(dir, entry)); err != nil {
			r.fail("%s: %v", name, err)
		}
		if m, ok := readManifest(&r, dir); ok {
			heads[name] = m
		}
		for _, p := range checkFiles(dir, name != starter) {
			r.fail("%s: %s", name, p)
		}
		if len(r.lines) == before {
			r.lines = append(r.lines, "ok  "+name)
		}
	}
	for _, dir := range builtin {
		entry := entrypoint(dir)
		if entry == "" {
			continue
		}
		checked++
		rel := "builtin/" + filepath.Base(dir)
		if err := parse(filepath.Join(dir, entry)); err != nil {
			r.fail("%s: %v", rel, err)
			continue
		}
		r.lines = append(r.lines, "ok  "+rel+" (parsed only)")
	}
	if checked == 0 {
		r.fail("no themes found; run from the repository root")
	}
	if o.base != "" {
		compareWithBase(&r, o, heads)
	}
	return r
}

// compareWithBase holds every theme a pull request changes to a higher
// version than base had, and warns when someone other than its author
// changes it.
func compareWithBase(r *result, o options, heads map[string]manifest) {
	changed, err := changedThemes(o.root, o.base)
	if err != nil {
		r.fail("cannot compare with %s: %v", o.base, err)
		return
	}
	names := make([]string, 0, len(changed))
	for n := range changed {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		head, ok := heads[name]
		if !ok {
			continue // removed, not a theme, or its theme.json already failed
		}
		was, ok, err := baseManifest(o.root, o.base, name)
		if err != nil {
			r.fail("%s: %v", name, err)
			continue
		}
		if !ok {
			continue // new here
		}
		if semverRe.MatchString(was.Version) && semverRe.MatchString(head.Version) && !newer(head.Version, was.Version) {
			r.fail("%s: its files changed, so its version must rise above %s (it is %s)", name, was.Version, head.Version)
		}
		if o.author != "" && !strings.EqualFold(o.author, was.Author.GitHub) {
			r.warnings = append(r.warnings, fmt.Sprintf("%s is by %s, and this pull request is by %s: ask them to review it", name, was.Author.GitHub, o.author))
		}
	}
}

// readManifest reads and checks a theme's theme.json, failing r on every
// problem; ok is whether it parsed at all.
func readManifest(r *result, dir string) (manifest, bool) {
	name := filepath.Base(dir)
	raw, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		r.fail("%s: no %s (see CONTRIBUTING.md)", name, manifestFile)
		return manifest{}, false
	}
	m, err := parseManifest(raw)
	if err != nil {
		r.fail("%s: %v", name, err)
		return manifest{}, false
	}
	for _, p := range m.problems(name) {
		r.fail("%s: %s: %s", name, manifestFile, p)
	}
	return m, true
}

// entrypoint is the page file a directory holds, or "" when it is no theme.
func entrypoint(dir string) string {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return ""
	}
	for _, name := range entrypoints {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && info.Mode().IsRegular() {
			return name
		}
	}
	return ""
}

func parse(path string) error {
	_, err := template.New(filepath.Base(path)).Funcs(funcs).ParseFiles(path)
	return err
}
