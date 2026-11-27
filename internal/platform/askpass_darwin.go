// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package platform

import (
	"os"
	"os/exec"
)

// Each script reads its text from argv (see runScript), so nothing is quoted.
// "activate" brings the dialog in front of other windows.
const (
	askSecretScript = `activate
set r to display dialog (item 1 of argv) with title (item 2 of argv) default answer "" with hidden answer with icon caution buttons {"Cancel", "OK"} default button "OK" cancel button "Cancel" giving up after 180
if gave up of r then error number -128
return text returned of r`

	askYesNoScript = `activate
display dialog (item 1 of argv) with title (item 2 of argv) with icon caution buttons {"No", "Yes"} default button "No" cancel button "No" giving up after 180
if gave up of result then error number -128`

	askConfirmScript = `activate
display dialog (item 1 of argv) with title (item 2 of argv) buttons {"Cancel", "OK"} default button "OK" cancel button "Cancel" giving up after 180
if gave up of result then error number -128`
)

func osascript(script, prompt string) *exec.Cmd {
	return runScript(script, dialogText(prompt), askPassTitle)
}

// AskPass implements the SSH_ASKPASS protocol: show prompt, write the
// answer to stdout, and return the process exit code (0 = answered).
func AskPass(prompt, mode string) int {
	switch classify(prompt, mode) {
	case infoPrompt:
		return 0
	case confirmPrompt:
		if osascript(askConfirmScript, prompt).Run() != nil {
			return 1
		}
		return 0
	case yesNoPrompt:
		if osascript(askYesNoScript, prompt).Run() != nil {
			os.Stdout.WriteString("no\n")
		} else {
			os.Stdout.WriteString("yes\n")
		}
		return 0
	}
	cmd := osascript(askSecretScript, prompt)
	cmd.Stdout = os.Stdout // osascript prints the answer followed by a newline
	if cmd.Run() != nil {
		return 1
	}
	return 0
}
