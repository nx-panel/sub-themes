package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// manifestFile is the file every theme in this repository carries beside its
// page. The panel's catalogue reads the same fields.
const manifestFile = "theme.json"

// locales are the page languages a theme's title and description are
// written in, the panel's four.
var locales = []string{"en", "fa", "ru", "zh"}

// manifest is theme.json.
type manifest struct {
	// Name is the directory's name.
	Name string `json:"name"`
	// Version is MAJOR.MINOR.PATCH; it rises with every change to the theme.
	Version string `json:"version"`
	// Title and Description are keyed by locale.
	Title       map[string]string `json:"title"`
	Description map[string]string `json:"description"`
	Author      struct {
		Name   string `json:"name"`
		GitHub string `json:"github"`
	} `json:"author"`
	// PanelMin is the oldest panel release the theme renders on.
	PanelMin string `json:"panelMin"`
}

var (
	semverRe = regexp.MustCompile(`^(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})$`)
	// githubLogin is GitHub's rule for an account name.
	githubLogin = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9]|-[A-Za-z0-9]){0,38}$`)
)

const (
	maxTitle       = 60
	maxDescription = 300
	maxAuthor      = 60
)

// parseManifest reads theme.json strictly: an unknown field, a trailing value
// or a missing one is an error, so a typo never passes as an absent field.
func parseManifest(raw []byte) (manifest, error) {
	var m manifest
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return m, fmt.Errorf("%s does not parse: %w", manifestFile, err)
	}
	if dec.More() {
		return m, fmt.Errorf("%s holds more than one JSON value", manifestFile)
	}
	return m, nil
}

// problems lists everything wrong with a manifest for the directory dir.
func (m manifest) problems(dir string) []string {
	var out []string
	add := func(format string, a ...any) { out = append(out, fmt.Sprintf(format, a...)) }

	if m.Name != dir {
		add("name is %q; it must be the directory's name, %q", m.Name, dir)
	}
	if !semverRe.MatchString(m.Version) {
		add("version %q is not MAJOR.MINOR.PATCH", m.Version)
	}
	if !semverRe.MatchString(m.PanelMin) {
		add("panelMin %q is not a panel release such as 0.1.0", m.PanelMin)
	}
	out = append(out, localized("title", m.Title, maxTitle)...)
	out = append(out, localized("description", m.Description, maxDescription)...)
	if name := strings.TrimSpace(m.Author.Name); name == "" || utf8.RuneCountInString(name) > maxAuthor {
		add("author.name must be 1 to %d characters", maxAuthor)
	}
	if !githubLogin.MatchString(m.Author.GitHub) {
		add("author.github %q is not a GitHub account name", m.Author.GitHub)
	}
	return out
}

// localized checks a field keyed by locale: every locale, nothing else, no
// blank text and nothing longer than limit.
func localized(field string, texts map[string]string, limit int) []string {
	var out []string
	for _, l := range locales {
		text, ok := texts[l]
		switch {
		case !ok:
			out = append(out, fmt.Sprintf("%s has no %q text", field, l))
		case strings.TrimSpace(text) == "":
			out = append(out, fmt.Sprintf("%s.%s is blank", field, l))
		case utf8.RuneCountInString(text) > limit:
			out = append(out, fmt.Sprintf("%s.%s is longer than %d characters", field, l, limit))
		}
	}
	for l := range texts {
		if !isLocale(l) {
			out = append(out, fmt.Sprintf("%s has text for %q, which is not a page language (%s)", field, l, strings.Join(locales, ", ")))
		}
	}
	return out
}

func isLocale(l string) bool {
	for _, x := range locales {
		if x == l {
			return true
		}
	}
	return false
}

// newer reports whether version a is above version b. Both must already
// match semverRe.
func newer(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := range 3 {
		x, _ := strconv.Atoi(pa[i])
		y, _ := strconv.Atoi(pb[i])
		if x != y {
			return x > y
		}
	}
	return false
}
