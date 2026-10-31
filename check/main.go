// Command check parses every theme in this repository the way the panel
// does, so a template error fails CI instead of sending a user's browser to
// the panel's fallback page.
//
// It catches what a parse can: unbalanced actions, unclosed comments, a
// function the panel does not offer, a directory name the panel would skip.
// It cannot catch a field that does not exist; open the theme on a panel with
// /page added to a subscription address for that.
//
//	go run ./check
package main

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
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

func main() {
	// Every top-level directory is a theme (so a clone of this repository can
	// be the panel's themes directory), and builtin/ holds the panel's own
	// pages one level further down.
	dirs, _ := filepath.Glob("*")
	more, _ := filepath.Glob("builtin/*")
	dirs = append(dirs, more...)

	failed, checked := false, 0
	for _, dir := range dirs {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			continue
		}
		entry := ""
		for _, name := range entrypoints {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				entry = name
				break
			}
		}
		if entry == "" {
			continue
		}
		checked++
		if !validName.MatchString(filepath.Base(dir)) {
			fmt.Printf("%s: the panel skips a directory with this name (letters, digits, '.', '_' and '-' only)\n", dir)
			failed = true
			continue
		}
		path := filepath.Join(dir, entry)
		if _, err := template.New(entry).Funcs(funcs).ParseFiles(path); err != nil {
			fmt.Printf("%s: %v\n", path, err)
			failed = true
			continue
		}
		fmt.Printf("ok  %s\n", path)
	}
	if checked == 0 {
		fmt.Println("no themes found; run from the repository root")
		failed = true
	}
	if failed {
		os.Exit(1)
	}
}
