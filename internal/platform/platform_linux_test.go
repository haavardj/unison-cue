// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package platform

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func only(names ...string) func(string) (string, error) {
	return func(n string) (string, error) {
		for _, x := range names {
			if x == n {
				return "/usr/bin/" + n, nil
			}
		}
		return "", errors.New("not found")
	}
}

func TestTerminalCommand(t *testing.T) {
	argv, err := terminalCommand(only("gnome-terminal", "xterm"), "/usr/bin/unison", "my profile; rm -rf ~")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/usr/bin/gnome-terminal", "--", "sh", "-c", terminalScript, "/usr/bin/unison", "my profile; rm -rf ~"}
	if strings.Join(argv, "|") != strings.Join(want, "|") {
		t.Errorf("argv = %q", argv)
	}
	if argv, _ := terminalCommand(only("x-terminal-emulator", "gnome-terminal"), "u", "p"); argv[0] != "/usr/bin/x-terminal-emulator" || argv[1] != "-e" {
		t.Errorf("x-terminal-emulator not preferred: %q", argv)
	}
	if _, err := terminalCommand(only(), "u", "p"); err == nil {
		t.Error("expected an error with no terminal")
	}
}

// The profile name must reach unison as one argument, not as shell code.
func TestTerminalScriptQuoting(t *testing.T) {
	out, err := exec.Command("sh", "-c", strings.Replace(terminalScript, "; s=$?", "; exit; s=$?", 1),
		"printf", "[%s]", "a b; echo injected").Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "[a b; echo injected]" {
		t.Errorf("got %q", out)
	}
}

func TestAskPassCommand(t *testing.T) {
	argv, ok := askPassCommand(only("zenity"), secretPrompt, "Enter passphrase for <key> & more: ")
	if !ok || argv[0] != "/usr/bin/zenity" || !strings.Contains(strings.Join(argv, " "), "--hide-text") {
		t.Fatalf("argv = %q", argv)
	}
	if !strings.Contains(strings.Join(argv, " "), "&lt;key&gt; &amp; more") {
		t.Errorf("markup not escaped: %q", argv)
	}
	argv, _ = askPassCommand(only("kdialog"), yesNoPrompt, "continue (yes/no)?")
	if argv[0] != "/usr/bin/kdialog" || argv[3] != "--yesno" {
		t.Errorf("kdialog argv = %q", argv)
	}
	if _, ok := askPassCommand(only(), secretPrompt, "x"); ok {
		t.Error("expected no dialog program")
	}
}

func TestAutostartEntry(t *testing.T) {
	e := autostartEntry(`/opt/My "Apps"/unison-cue`)
	if !strings.Contains(e, `Exec="/opt/My \\"Apps\\"/unison-cue"`) {
		t.Errorf("entry:\n%s", e)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if AutostartEnabled() {
		t.Fatal("enabled before SetAutostart")
	}
	if err := SetAutostart(true); err != nil || !AutostartEnabled() {
		t.Fatalf("enable: %v", err)
	}
	if filepath.Base(autostartPath()) != "unison-cue.desktop" {
		t.Errorf("path = %s", autostartPath())
	}
	if err := SetAutostart(false); err != nil || AutostartEnabled() {
		t.Fatalf("disable: %v", err)
	}
}
