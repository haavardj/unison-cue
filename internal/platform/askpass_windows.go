// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package platform

import (
	"os"
	"os/exec"
)

// Each script reads its text from the environment, so nothing is quoted.
const (
	askSecretPS = `
Add-Type -AssemblyName System.Windows.Forms
[Console]::OutputEncoding = [Text.Encoding]::UTF8
$f = New-Object Windows.Forms.Form -Property @{Text=$env:UT_TITLE; Width=480; Height=200; StartPosition='CenterScreen'; TopMost=$true; FormBorderStyle='FixedDialog'; MaximizeBox=$false; MinimizeBox=$false}
$l = New-Object Windows.Forms.Label -Property @{Text=$env:UT_PROMPT; Left=12; Top=12; Width=440; Height=64}
$t = New-Object Windows.Forms.TextBox -Property @{Left=12; Top=80; Width=440; UseSystemPasswordChar=$true}
$ok = New-Object Windows.Forms.Button -Property @{Text='OK'; Left=296; Top=116; Width=75; DialogResult='OK'}
$c = New-Object Windows.Forms.Button -Property @{Text='Cancel'; Left=377; Top=116; Width=75; DialogResult='Cancel'}
$f.Controls.AddRange(@($l, $t, $ok, $c)); $f.AcceptButton = $ok; $f.CancelButton = $c
$f.Add_Shown({ $f.Activate(); $t.Focus() })
if ($f.ShowDialog() -eq 'OK') { [Console]::Out.Write($t.Text + "` + "`n" + `"); exit 0 } else { exit 1 }
`
	askButtonsPS = `
Add-Type -AssemblyName System.Windows.Forms
$r = [Windows.Forms.MessageBox]::Show($env:UT_PROMPT, $env:UT_TITLE, $env:UT_BUTTONS, 'Question')
if ($r -eq 'Yes' -or $r -eq 'OK') { exit 0 } else { exit 1 }
`
)

func powershell(script, prompt string, extra ...string) *exec.Cmd {
	cmd := hidden(exec.Command(system32(`WindowsPowerShell\v1.0\powershell.exe`), "-NoProfile", "-NonInteractive",
		"-ExecutionPolicy", "Bypass", "-Command", script))
	cmd.Env = append(os.Environ(), append([]string{"UT_PROMPT=" + dialogText(prompt), "UT_TITLE=" + askPassTitle}, extra...)...)
	return cmd
}

// AskPass implements the SSH_ASKPASS protocol: show prompt, write the
// answer to stdout, and return the process exit code (0 = answered).
func AskPass(prompt, mode string) int {
	switch classify(prompt, mode) {
	case infoPrompt:
		return 0
	case confirmPrompt:
		if powershell(askButtonsPS, prompt, "UT_BUTTONS=OKCancel").Run() != nil {
			return 1
		}
		return 0
	case yesNoPrompt:
		if powershell(askButtonsPS, prompt, "UT_BUTTONS=YesNo").Run() != nil {
			os.Stdout.WriteString("no\n")
		} else {
			os.Stdout.WriteString("yes\n")
		}
		return 0
	}
	cmd := powershell(askSecretPS, prompt)
	cmd.Stdout = os.Stdout
	if cmd.Run() != nil {
		return 1
	}
	return 0
}
