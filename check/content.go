package main

import (
	"bytes"
	"fmt"
	"image"
	_ "image/png" // preview.png
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// The limits a theme in this repository stays within. The panel's catalogue
// refuses an archive above them too, so they are not a matter of taste.
const (
	maxThemeBytes   = 2 << 20
	maxThemeFiles   = 200
	maxPreviewBytes = 300 << 10
	previewFile     = "preview.png"
	readmeFile      = "README.md"
)

// allowedExt are the kinds of file a theme may hold.
var allowedExt = map[string]bool{
	".html": true, ".css": true, ".js": true, ".json": true, ".md": true,
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".svg": true,
	".woff2": true,
}

// magic is how each binary kind starts, so a renamed file is caught.
var magic = map[string][]string{
	".png":   {"\x89PNG\r\n\x1a\n"},
	".jpg":   {"\xff\xd8\xff"},
	".jpeg":  {"\xff\xd8\xff"},
	".webp":  {"RIFF"},
	".woff2": {"wOF2"},
}

// checkFiles walks a theme directory and reports every file rule it breaks:
// the kinds, the sizes, links and hidden entries, and anything the page
// would load from another host. needPreview is false for starter/, which is a
// template for authors and is not published.
func checkFiles(dir string, needPreview bool) []string {
	var out []string
	add := func(format string, a ...any) { out = append(out, fmt.Sprintf(format, a...)) }

	total, files := int64(0), 0
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == dir {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(d.Name(), ".") {
			add("%s: hidden files and directories are not allowed in a theme", rel)
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			add("%s: links are not allowed in a theme", rel)
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			add("%s: not a regular file", rel)
			return nil
		}
		ext := strings.ToLower(path.Ext(rel))
		if !allowedExt[ext] {
			add("%s: a theme may hold only %s files", rel, extList())
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files++
		total += int64(len(raw))
		if starts, ok := magic[ext]; ok && !hasAnyPrefix(raw, starts) {
			add("%s: the content is not a %s file", rel, strings.TrimPrefix(ext, "."))
		}
		for _, problem := range scan(ext, raw) {
			add("%s: %s", rel, problem)
		}
		return nil
	})
	if err != nil {
		add("cannot read the directory: %v", err)
	}
	if files > maxThemeFiles {
		add("%d files; a theme holds at most %d", files, maxThemeFiles)
	}
	if total > maxThemeBytes {
		add("%d KB in all; a theme is at most %d KB", total>>10, maxThemeBytes>>10)
	}
	if info, err := os.Stat(filepath.Join(dir, readmeFile)); err != nil || !info.Mode().IsRegular() {
		add("no %s: say what the theme looks like and who made it", readmeFile)
	}
	if needPreview {
		out = append(out, checkPreview(filepath.Join(dir, previewFile))...)
	}
	return out
}

// checkPreview reads the screenshot the panel shows for a theme.
func checkPreview(p string) []string {
	f, err := os.Open(p)
	if err != nil {
		return []string{"no " + previewFile + ": a PNG screenshot at phone width"}
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return []string{previewFile + ": " + err.Error()}
	}
	var out []string
	if info.Size() > maxPreviewBytes {
		out = append(out, fmt.Sprintf("%s is %d KB; at most %d KB", previewFile, info.Size()>>10, maxPreviewBytes>>10))
	}
	cfg, format, err := image.DecodeConfig(f)
	switch {
	case err != nil || format != "png":
		out = append(out, previewFile+" is not a PNG image")
	case cfg.Width < 300 || cfg.Width > 1600 || cfg.Height < 300 || cfg.Height > 4000:
		out = append(out, fmt.Sprintf("%s is %dx%d; keep it 300 to 1600 wide and 300 to 4000 tall", previewFile, cfg.Width, cfg.Height))
	}
	return out
}

func extList() string {
	return ".html, .css, .js, .json, .md, .png, .jpg, .webp, .svg, .woff2"
}

func hasAnyPrefix(b []byte, prefixes []string) bool {
	for _, p := range prefixes {
		if bytes.HasPrefix(b, []byte(p)) {
			return true
		}
	}
	return false
}

// ---- loading from another host ----
//
// The page is opened by someone whose VPN is not up yet, so a theme loads
// everything from its own directory. Links the user clicks (<a href>,
// window.open) may go anywhere. These scans are a guard against the honest
// mistake and the obvious attempt; a reviewer still reads every script.

// remote matches a URL that leaves the theme's origin: any scheme but data:
// and blob:, or a protocol-relative //host. A template action, a relative
// path and a #fragment do not match.
var remote = regexp.MustCompile(`(?i)^\s*(//|(?:[a-z][a-z0-9+.-]*):)`)

func isRemote(u string) bool {
	m := remote.FindStringSubmatch(u)
	if m == nil {
		return false
	}
	scheme := strings.ToLower(m[1])
	return scheme != "data:" && scheme != "blob:"
}

var (
	// tagRe finds a start tag and attrRe each attribute inside it, with a
	// quoted or bare value.
	tagRe  = regexp.MustCompile(`(?is)<([a-z][a-z0-9:-]*)\b((?:[^>"']|"[^"]*"|'[^']*')*)>`)
	attrRe = regexp.MustCompile(`(?is)([a-z][a-z0-9:_-]*)\s*=\s*("[^"]*"|'[^']*'|[^\s"'>]+)`)
	// cssURLRe and importRe find what a stylesheet loads.
	cssURLRe = regexp.MustCompile(`(?is)url\(\s*("[^"]*"|'[^']*'|[^)]*)\s*\)`)
	importRe = regexp.MustCompile(`(?is)@import\s+("[^"]*"|'[^']*'|url\([^)]*\))`)
	// jsLoadRe finds a script API that sends a request, with a literal URL as
	// its first argument; jsXHRRe the URL of an XMLHttpRequest's open(method,
	// url) (window.open(url) is a link and does not match: its first argument
	// is no method).
	jsLoadRe = regexp.MustCompile("\\b(?:fetch|WebSocket|EventSource|SharedWorker|Worker|importScripts|sendBeacon)\\s*\\(\\s*(\"[^\"]*\"|'[^']*'|`[^`]*`)")
	jsXHRRe  = regexp.MustCompile("\\.open\\s*\\(\\s*[\"'`][A-Za-z]+[\"'`]\\s*,\\s*(\"[^\"]*\"|'[^']*'|`[^`]*`)")
	// jsSrcRe finds a source set from a script: el.src = "…" or
	// setAttribute("src", "…"). An href set from a script is a link and passes.
	jsSrcRe = regexp.MustCompile("(?:\\.(?:src|srcset|action|formAction)\\s*=|setAttribute\\s*\\(\\s*[\"'](?:src|srcset|action|formaction|data)[\"']\\s*,)\\s*(\"[^\"]*\"|'[^']*'|`[^`]*`)")
	// jsBanned are constructs that run text as code or write the document.
	jsBanned = []struct {
		re   *regexp.Regexp
		what string
	}{
		{regexp.MustCompile(`\beval\s*\(`), "eval runs text as code"},
		{regexp.MustCompile(`\bnew\s+Function\s*\(`), "new Function runs text as code"},
		{regexp.MustCompile(`\bdocument\s*\.\s*write(?:ln)?\s*\(`), "document.write"},
		{regexp.MustCompile("\\bset(?:Timeout|Interval)\\s*\\(\\s*[\"'`]"), "a string passed to setTimeout or setInterval runs as code"},
		{regexp.MustCompile(`\bimport\s*\(`), "a dynamic import()"},
	}
)

// loadAttrs are the attributes through which an element loads something,
// per element; "*" applies to every element.
var loadAttrs = map[string][]string{
	"*":       {"src", "srcset", "poster", "background"},
	"link":    {"href", "imagesrcset"},
	"object":  {"data"},
	"form":    {"action"},
	"button":  {"formaction"},
	"input":   {"formaction"},
	"base":    {"href"},
	"use":     {"href", "xlink:href"},
	"image":   {"href", "xlink:href"},
	"feimage": {"href", "xlink:href"},
	"script":  {"href", "xlink:href"},
}

// scan reports what a file of kind ext would load from another host, and
// script constructs that are not allowed.
func scan(ext string, raw []byte) []string {
	text := string(raw)
	switch ext {
	case ".html":
		out := scanTags(text, false)
		out = append(out, scanCSS(text)...)
		return append(out, scanJS(text)...)
	case ".svg":
		out := scanTags(text, true)
		return append(out, scanCSS(text)...)
	case ".css":
		return scanCSS(text)
	case ".js":
		return scanJS(text)
	}
	return nil
}

func scanTags(text string, svg bool) []string {
	var out []string
	for _, t := range tagRe.FindAllStringSubmatch(text, -1) {
		tag := strings.ToLower(t[1])
		if svg {
			switch tag {
			case "script":
				out = append(out, "an svg may not hold a <script>")
			case "foreignobject":
				out = append(out, "an svg may not hold a <foreignObject>")
			}
		}
		for _, a := range attrRe.FindAllStringSubmatch(t[2], -1) {
			name := strings.ToLower(a[1])
			value := unquote(a[2])
			if svg && strings.HasPrefix(name, "on") {
				out = append(out, fmt.Sprintf("<%s %s>: an svg may not carry event handlers", tag, name))
				continue
			}
			if tag == "meta" && name == "content" && strings.Contains(strings.ToLower(t[2]), "refresh") {
				if _, after, ok := strings.Cut(strings.ToLower(value), "url="); ok && isRemote(after) {
					out = append(out, "<meta http-equiv=refresh> sends the page to another host")
				}
				continue
			}
			if !isLoadAttr(tag, name) {
				continue
			}
			for _, u := range splitSrcset(name, value) {
				if isRemote(u) {
					out = append(out, fmt.Sprintf("<%s %s=%q> loads from another host", tag, name, u))
				}
			}
		}
	}
	return out
}

func isLoadAttr(tag, name string) bool {
	for _, a := range loadAttrs["*"] {
		if a == name {
			return true
		}
	}
	for _, a := range loadAttrs[tag] {
		if a == name {
			return true
		}
	}
	return false
}

// splitSrcset returns each URL a srcset names; any other attribute is one URL.
func splitSrcset(name, value string) []string {
	if !strings.HasSuffix(name, "srcset") {
		return []string{value}
	}
	var out []string
	for _, part := range strings.Split(value, ",") {
		if f := strings.Fields(part); len(f) > 0 {
			out = append(out, f[0])
		}
	}
	return out
}

func scanCSS(text string) []string {
	var out []string
	for _, m := range importRe.FindAllStringSubmatch(text, -1) {
		u := unquote(m[1])
		if inner, ok := strings.CutPrefix(strings.ToLower(u), "url("); ok {
			u = unquote(strings.TrimSuffix(strings.TrimSpace(inner), ")"))
		}
		if isRemote(u) {
			out = append(out, fmt.Sprintf("@import %q loads from another host", u))
		}
	}
	for _, m := range cssURLRe.FindAllStringSubmatch(text, -1) {
		if u := unquote(strings.TrimSpace(m[1])); isRemote(u) {
			out = append(out, fmt.Sprintf("url(%q) loads from another host", u))
		}
	}
	return out
}

func scanJS(text string) []string {
	var out []string
	for _, re := range []*regexp.Regexp{jsLoadRe, jsXHRRe} {
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			if u := unquote(m[1]); isRemote(u) {
				out = append(out, fmt.Sprintf("a script sends a request to another host (%q)", u))
			}
		}
	}
	for _, m := range jsSrcRe.FindAllStringSubmatch(text, -1) {
		if u := unquote(m[1]); isRemote(u) {
			out = append(out, fmt.Sprintf("a script loads from another host (%q)", u))
		}
	}
	for _, b := range jsBanned {
		if b.re.MatchString(text) {
			out = append(out, "not allowed in a theme: "+b.what)
		}
	}
	return out
}

func unquote(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 {
		switch v[0] {
		case '"', '\'', '`':
			if v[len(v)-1] == v[0] {
				return v[1 : len(v)-1]
			}
		}
	}
	return v
}
