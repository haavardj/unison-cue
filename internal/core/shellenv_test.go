// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseEnvDump(t *testing.T) {
	out := []byte("Welcome!\nprompt noise " + shellEnvMarker + " not a line\n\n" +
		shellEnvMarker + "\nUNISONLOCALHOSTNAME=laptop\x00MULTI=a\nb\x00EMPTY=\x00")
	got := parseEnvDump(out)
	if got["UNISONLOCALHOSTNAME"] != "laptop" || got["MULTI"] != "a\nb" || len(got) != 3 {
		t.Errorf("parsed %q", got)
	}
	if parseEnvDump([]byte("no marker")) != nil {
		t.Error("expected nil without marker")
	}
}

func TestImportShellEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no login shell import on Windows")
	}
	// A fake shell that prints start-up noise, then what the real command
	// would print.
	sh := filepath.Join(t.TempDir(), "fakeshell")
	script := "#!/bin/sh\necho 'motd noise'\nprintf '\\n" + shellEnvMarker + "\\n'\n" +
		"printf 'UNISONLOCALHOSTNAME=from-shell\\000UNISONBACKUPDIR=/b\\000UNISON=/already\\000HOME=/x\\000'\n"
	if err := os.WriteFile(sh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", sh)
	t.Setenv("UNISON", "/kept") // already set: must not be overwritten
	os.Unsetenv("UNISONLOCALHOSTNAME")
	os.Unsetenv("UNISONBACKUPDIR")
	t.Cleanup(func() { os.Unsetenv("UNISONLOCALHOSTNAME"); os.Unsetenv("UNISONBACKUPDIR") })

	names, err := ImportShellEnv(5 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(names, ","); got != "UNISONBACKUPDIR,UNISONLOCALHOSTNAME" {
		t.Errorf("imported %q", got)
	}
	if os.Getenv("UNISONLOCALHOSTNAME") != "from-shell" || os.Getenv("UNISON") != "/kept" {
		t.Errorf("env: host=%q unison=%q", os.Getenv("UNISONLOCALHOSTNAME"), os.Getenv("UNISON"))
	}
	if os.Getenv("HOME") == "/x" {
		t.Error("imported a non-UNISON variable")
	}
}

func TestRunnerEnvOverrides(t *testing.T) {
	t.Setenv("UNISONLOCALHOSTNAME", "inherited")
	exe, argsFile := fakeUnison(t, `echo "$UNISONLOCALHOSTNAME" > "$(dirname "$0")/host"`)
	r := &Runner{UnisonPath: exe, LogDir: t.TempDir(), Env: map[string]string{"UNISONLOCALHOSTNAME": "configured"}}
	r.Run(context.Background(), "p")
	host, _ := os.ReadFile(filepath.Join(filepath.Dir(argsFile), "host"))
	if got := strings.TrimSpace(string(host)); got != "configured" {
		t.Errorf("unison saw UNISONLOCALHOSTNAME=%q, want configured", got)
	}

	r.Env = nil
	r.Run(context.Background(), "p")
	host, _ = os.ReadFile(filepath.Join(filepath.Dir(argsFile), "host"))
	if got := strings.TrimSpace(string(host)); got != "inherited" {
		t.Errorf("unison saw UNISONLOCALHOSTNAME=%q, want inherited", got)
	}
}

func TestUnisonDirFor(t *testing.T) {
	t.Setenv("UNISON", "/from/env")
	if got := UnisonDirFor(map[string]string{"UNISON": "/from/config"}); got != "/from/config" {
		t.Errorf("config: %q", got)
	}
	if got := UnisonDirFor(nil); got != "/from/env" {
		t.Errorf("env: %q", got)
	}
}
