// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package platform

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		prompt, mode string
		want         promptKind
	}{
		{"Enter passphrase for key '/Users/me/.ssh/id_ed25519': ", "", secretPrompt},
		{"me@host's password: ", "", secretPrompt},
		{"Are you sure you want to continue connecting (yes/no/[fingerprint])? ", "", yesNoPrompt},
		{"Please type 'yes', 'no' or the fingerprint: ", "", yesNoPrompt},
		{"Allow use of key?", "confirm", confirmPrompt},
		{"Confirm user presence for key", "none", infoPrompt},
	}
	for _, c := range cases {
		if got := classify(c.prompt, c.mode); got != c.want {
			t.Errorf("classify(%q, %q) = %d, want %d", c.prompt, c.mode, got, c.want)
		}
	}
}
