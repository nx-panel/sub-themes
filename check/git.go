package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// changedThemes lists the top-level directories a pull request touches,
// comparing base with HEAD in the repository at root.
func changedThemes(root, base string) (map[string]bool, error) {
	out, err := git(root, "diff", "--name-only", "--no-renames", base, "HEAD")
	if err != nil {
		return nil, err
	}
	dirs := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if dir, _, ok := strings.Cut(strings.TrimSpace(line), "/"); ok {
			dirs[dir] = true
		}
	}
	return dirs, nil
}

// baseManifest is a theme's theme.json as base had it; ok is false when base
// had none (a new theme, or one from before theme.json).
func baseManifest(root, base, dir string) (manifest, bool, error) {
	if _, err := git(root, "cat-file", "-e", base+":"+dir+"/"+manifestFile); err != nil {
		return manifest{}, false, nil
	}
	raw, err := git(root, "show", base+":"+dir+"/"+manifestFile)
	if err != nil {
		return manifest{}, false, err
	}
	m, err := parseManifest(raw)
	if err != nil {
		return manifest{}, false, fmt.Errorf("the base's %s/%s: %w", dir, manifestFile, err)
	}
	return m, true, nil
}

func git(root string, args ...string) ([]byte, error) {
	var stderr bytes.Buffer
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
