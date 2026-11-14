// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package platform

import "testing"

func TestInDir(t *testing.T) {
	cases := []struct {
		path, dir string
		want      bool
	}{
		{`C:\Users\me\AppData\Local\Temp\x.zip.4c5\UnisonCue.exe`, `C:\Users\me\AppData\Local\Temp`, true},
		{`c:\users\ME\appdata\local\temp\UnisonCue.exe`, `C:\Users\me\AppData\Local\Temp`, true},
		{`C:\Users\me\Apps\UnisonCue.exe`, `C:\Users\me\AppData\Local\Temp`, false},
		{`C:\Users\me\AppData\Local\Temporary\UnisonCue.exe`, `C:\Users\me\AppData\Local\Temp`, false},
		{`D:\Temp\UnisonCue.exe`, `C:\Temp`, false},
	}
	for _, c := range cases {
		if got := inDir(c.path, c.dir); got != c.want {
			t.Errorf("inDir(%q, %q) = %v, want %v", c.path, c.dir, got, c.want)
		}
	}
}
