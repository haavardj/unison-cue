// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package platform

import (
	"os"
	"os/exec"
)

// Each script reads its text from the environment, so nothing is quoted.
// "activate" brings the dialog in front of other windows.
const (
	askSecretScript = `activate
set r to display dialog (system attribute "UT_PROMPT") with title (system attribute "UT_TITLE") default answer "" with hidden answer with icon caution buttons {"Cancel", "OK"} default button "OK" cancel button "Cancel" giving up after 180
if gave up of r then error number -128
return text returned of r`

	askYesNoScript = `activate
display dialog (system attribute "UT_PROMPT") with title (system attribute "UT_TITLE") with icon caution buttons {"No", "Yes"} default button "No" cancel button "No" giving up after 180
if gave up of result then error number -128`

	askConfirmScript = `activate
display dialog (system attribute "UT_PROMPT") with title (system attribute "UT_TITLE") buttons {"Cancel", "OK"} default button "OK" cancel button "Cancel" giving up after 180
if gave up of result then error number -128`
)

func osascript(script, prompt string) *exec.Cmd {
	cmd := exec.Command(osascriptPath, "-e", script)
	cmd.Env = append(os.Environ(), "UT_PROMPT="+dialogText(prompt), "UT_TITLE="+askPassTitle)
	return cmd
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
