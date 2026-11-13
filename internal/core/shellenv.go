// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package core

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"
)

// shellEnvMarker separates shell start-up noise (motd, prompts, plugin
// output) from the environment dump that follows it.
const shellEnvMarker = "__UNISON_CUE_ENV_START__"

// IsUnisonVar reports whether name is one of unison's environment
// variables (UNISON, UNISONLOCALHOSTNAME, UNISONBACKUPDIR, ...).
func IsUnisonVar(name string) bool {
	return strings.HasPrefix(name, "UNISON") && !strings.HasPrefix(name, "UNISON_CUE_")
}

// ImportShellEnv copies unison's variables from the user's login shell
// into this process, for those not already set.
//
// On macOS, apps started from the Finder or at login get their
// environment from launchd, not from ~/.zshrc or ~/.bash_profile. A
// variable such as UNISONLOCALHOSTNAME exported there would be missing,
// and unison would then look for the wrong archives. The shell is started
// as an interactive login shell so that both kinds of start-up file are
// read. It returns the names it imported. It does nothing on Windows,
// where GUI apps inherit the user's environment variables.
func ImportShellEnv(timeout time.Duration) ([]string, error) {
	if runtime.GOOS == "windows" {
		return nil, nil
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, "-l", "-i", "-c",
		"printf '\\n"+shellEnvMarker+"\\n'; /usr/bin/env -0")
	cmd.Stdin = nil
	cmd.Stderr = nil
	prepareCmd(cmd) // own session: an interactive shell must not grab a terminal
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return nil, err
	}
	vars := parseEnvDump(out)
	var imported []string
	for k, v := range vars {
		if !IsUnisonVar(k) {
			continue
		}
		if _, set := os.LookupEnv(k); set {
			continue
		}
		if os.Setenv(k, v) == nil {
			imported = append(imported, k)
		}
	}
	sort.Strings(imported)
	return imported, nil
}

// parseEnvDump reads the NUL-separated output of `env -0` after the marker.
func parseEnvDump(out []byte) map[string]string {
	i := bytes.LastIndex(out, []byte(shellEnvMarker+"\n"))
	if i < 0 {
		return nil
	}
	vars := map[string]string{}
	for _, kv := range bytes.Split(out[i+len(shellEnvMarker)+1:], []byte{0}) {
		k, v, ok := strings.Cut(string(kv), "=")
		if ok && k != "" {
			vars[k] = v
		}
	}
	return vars
}
