package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The catalogue a panel reads: one small archive with what the panel shows
// for every theme (catalog.json and the previews), and one archive per theme
// with its files, which a panel downloads only to install it. The build
// workflow publishes all of them as one release per build.
const (
	catalogFormat  = 1
	catalogArchive = "catalog.tar.gz"
	catalogIndex   = "catalog.json"
	previewsDir    = "previews"
)

// catalogIndexFile is catalog.json.
type catalogIndexFile struct {
	Format int `json:"format"`
	// Build is the release tag this build is published under; a panel
	// downloads a theme's archive from the same release.
	Build  string `json:"build"`
	Commit string `json:"commit"`
	// GeneratedAt is when the build ran, in Unix seconds.
	GeneratedAt int64          `json:"generatedAt"`
	Themes      []catalogTheme `json:"themes"`
}

// catalogTheme is a theme's theme.json plus where its files are.
type catalogTheme struct {
	manifest
	// Preview is the screenshot's path inside catalog.tar.gz.
	Preview string `json:"preview"`
	// Archive is the theme's archive in the same release; its files sit at
	// the archive's root. SHA256 and Size are the archive's.
	Archive string `json:"archive"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
	Files   int    `json:"files"`
}

// buildCatalog writes the catalogue of every theme in root into out. It runs
// only after a clean check, so every theme here already passed the rules.
// The archives are reproducible: the same files give the same bytes, so a
// theme nobody changed keeps its checksum from build to build.
func buildCatalog(root, out, build, commit string, now time.Time) (catalogIndexFile, error) {
	idx := catalogIndexFile{Format: catalogFormat, Build: build, Commit: commit, GeneratedAt: now.Unix(), Themes: []catalogTheme{}}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return idx, err
	}
	top, _ := filepath.Glob(filepath.Join(root, "*"))
	previews := map[string][]byte{}
	for _, dir := range top {
		name := filepath.Base(dir)
		if name == starter || entrypoint(dir) == "" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, manifestFile))
		if err != nil {
			return idx, err
		}
		m, err := parseManifest(raw)
		if err != nil {
			return idx, fmt.Errorf("%s: %w", name, err)
		}
		files, err := themeFiles(dir)
		if err != nil {
			return idx, fmt.Errorf("%s: %w", name, err)
		}
		archive, err := tarGz(files)
		if err != nil {
			return idx, fmt.Errorf("%s: %w", name, err)
		}
		archiveName := name + "-" + m.Version + ".tar.gz"
		if err := os.WriteFile(filepath.Join(out, archiveName), archive, 0o644); err != nil {
			return idx, err
		}
		sum := sha256.Sum256(archive)
		previews[previewsDir+"/"+name+".png"] = files[previewFile]
		idx.Themes = append(idx.Themes, catalogTheme{
			manifest: m,
			Preview:  previewsDir + "/" + name + ".png",
			Archive:  archiveName,
			SHA256:   hex.EncodeToString(sum[:]),
			Size:     int64(len(archive)),
			Files:    len(files),
		})
	}
	sort.Slice(idx.Themes, func(i, j int) bool { return idx.Themes[i].Name < idx.Themes[j].Name })

	body, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return idx, err
	}
	previews[catalogIndex] = append(body, '\n')
	archive, err := tarGz(previews)
	if err != nil {
		return idx, err
	}
	return idx, os.WriteFile(filepath.Join(out, catalogArchive), archive, 0o644)
}

// themeFiles reads every file of a theme, keyed by its slash path inside the
// directory. check has already refused links and hidden entries.
func themeFiles(dir string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = raw
		return nil
	})
	return files, err
}

// tarGz packs files into a gzip'd tar with fixed names, order, modes and
// times, and no owner, so its bytes depend on the files alone.
func tarGz(files map[string][]byte) ([]byte, error) {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)

	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	tw := tar.NewWriter(zw)
	dirs := map[string]bool{}
	for _, n := range names {
		// A file's directories come before it, outermost first, so an
		// extractor that wants them gets them.
		var parents []string
		for d := parentDir(n); d != "" && !dirs[d]; d = parentDir(d) {
			dirs[d] = true
			parents = append(parents, d)
		}
		for i := len(parents) - 1; i >= 0; i-- {
			if err := tw.WriteHeader(&tar.Header{Name: parents[i] + "/", Typeflag: tar.TypeDir, Mode: 0o755, ModTime: time.Unix(0, 0), Format: tar.FormatPAX}); err != nil {
				return nil, err
			}
		}
		data := files[n]
		if err := tw.WriteHeader(&tar.Header{Name: n, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(data)), ModTime: time.Unix(0, 0), Format: tar.FormatPAX}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(data); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func parentDir(p string) string {
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return ""
	}
	return p[:i]
}
