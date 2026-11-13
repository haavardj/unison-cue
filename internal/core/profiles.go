// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package core

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Profile is a unison profile (*.prf) found in the unison directory.
type Profile struct {
	Name  string   // file name without .prf; what unison takes on the command line
	Path  string   // full path of the .prf file
	Label string   // optional "label = ..." preference
	Roots []string // "root = ..." lines, for display
}

// DisplayName is the label if the profile has one, else the name.
func (p Profile) DisplayName() string {
	if p.Label != "" {
		return p.Label
	}
	return p.Name
}

// DefaultUnisonDir mirrors how unison picks its directory: $UNISON if
// set, else ~/.unison, else (macOS) ~/Library/Application Support/Unison.
func DefaultUnisonDir() string {
	if d := os.Getenv("UNISON"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "windows" {
		if up := os.Getenv("USERPROFILE"); up != "" {
			home = up
		}
	}
	dot := filepath.Join(home, ".unison")
	if fi, err := os.Stat(dot); err == nil && fi.IsDir() {
		return dot
	}
	if runtime.GOOS == "darwin" {
		lib := filepath.Join(home, "Library", "Application Support", "Unison")
		if fi, err := os.Stat(lib); err == nil && fi.IsDir() {
			return lib
		}
	}
	return dot
}

// DiscoverProfiles lists the profiles in dir, sorted by name. Editor
// backups (foo.prf~) and hidden files are skipped.
func DiscoverProfiles(dir string) ([]Profile, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.prf"))
	if err != nil {
		return nil, err
	}
	var out []Profile
	for _, m := range matches {
		base := filepath.Base(m)
		// Hidden files, and names unison would parse as a command-line
		// option ("-foo.prf" would become the argument "-foo").
		if strings.HasPrefix(base, ".") || strings.HasPrefix(base, "-") {
			continue
		}
		fi, err := os.Stat(m)
		if err != nil || fi.IsDir() {
			continue
		}
		p := Profile{Name: strings.TrimSuffix(base, ".prf"), Path: m}
		parseProfile(&p)
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func parseProfile(p *Profile) {
	f, err := os.Open(p.Path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "root":
			p.Roots = append(p.Roots, strings.TrimSpace(v))
		case "label":
			p.Label = strings.TrimSpace(v)
		}
	}
}
