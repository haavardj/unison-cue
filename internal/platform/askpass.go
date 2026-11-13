// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package platform

import "strings"

// promptKind is what ssh is asking for when it runs the askpass helper.
type promptKind int

const (
	secretPrompt  promptKind = iota // passphrase or password: hidden text field
	yesNoPrompt                     // unknown host key: answer "yes" or "no" on stdout
	confirmPrompt                   // SSH_ASKPASS_PROMPT=confirm: answer by exit code
	infoPrompt                      // SSH_ASKPASS_PROMPT=none: notice only, nothing to answer
)

const askPassTitle = "Unison Cue: SSH"

func classify(prompt, mode string) promptKind {
	switch mode {
	case "confirm":
		return confirmPrompt
	case "none":
		return infoPrompt
	}
	if strings.Contains(prompt, "(yes/no") {
		return yesNoPrompt
	}
	return secretPrompt
}

// dialogText adds context so the user knows which app is asking and why.
func dialogText(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		prompt = "SSH needs a password or passphrase."
	}
	return prompt + "\n\n(Unison Cue is connecting to a remote root.)"
}
