// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package platform

import (
	"os"
	"os/exec"
	"strings"
)

// pangoEscape escapes text for zenity, which interprets Pango markup.
func pangoEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// askPassCommand builds the dialog command for one prompt, using zenity
// (GNOME, installed with Ubuntu) or kdialog (KDE). ok is false when
// neither is available.
func askPassCommand(lookPath func(string) (string, error), kind promptKind, prompt string) (argv []string, ok bool) {
	text := dialogText(prompt)
	if z, err := lookPath("zenity"); err == nil {
		base := []string{z, "--title=" + askPassTitle, "--text=" + pangoEscape(text), "--timeout=180", "--width=420"}
		switch kind {
		case secretPrompt:
			return append(base, "--entry", "--hide-text"), true
		case yesNoPrompt:
			return append(base, "--question", "--default-cancel", "--ok-label=Yes", "--cancel-label=No"), true
		default:
			return append(base, "--question"), true
		}
	}
	if k, err := lookPath("kdialog"); err == nil {
		switch kind {
		case secretPrompt:
			return []string{k, "--title", askPassTitle, "--password", text}, true
		default:
			return []string{k, "--title", askPassTitle, "--yesno", text}, true
		}
	}
	return nil, false
}

// AskPass implements the SSH_ASKPASS protocol: show prompt, write the
// answer to stdout, and return the process exit code (0 = answered).
func AskPass(prompt, mode string) int {
	kind := classify(prompt, mode)
	if kind == infoPrompt {
		return 0
	}
	argv, ok := askPassCommand(exec.LookPath, kind, prompt)
	if !ok {
		return 1 // no dialog program: ssh fails the login, as without askpass
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	switch kind {
	case secretPrompt:
		cmd.Stdout = os.Stdout // the dialog prints the answer and a newline
		if cmd.Run() != nil {
			return 1
		}
	case yesNoPrompt:
		if cmd.Run() != nil {
			os.Stdout.WriteString("no\n")
		} else {
			os.Stdout.WriteString("yes\n")
		}
	default:
		if cmd.Run() != nil {
			return 1
		}
	}
	return 0
}
