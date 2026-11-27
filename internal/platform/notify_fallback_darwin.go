// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package platform

// This file is separate because cgo allows only declarations in the
// preamble of a file that exports a function.

import "C"

import "log"

// ucNotifyFallback is called when macOS refuses a native notification,
// for instance for a build without a usable code signature.
//
//export ucNotifyFallback
func ucNotifyFallback(title, body, reason *C.char) {
	log.Printf("native notification failed (%s); using osascript", C.GoString(reason))
	if err := notifyScript(C.GoString(title), C.GoString(body)); err != nil {
		log.Printf("notification failed: %v", err)
	}
}
