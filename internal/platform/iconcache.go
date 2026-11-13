// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package platform

import (
	"sync"

	"github.com/haavardj/unison-cue/internal/icon"
)

var (
	iconMu    sync.Mutex
	iconCache = map[[2]int][]byte{}
)

// cachedIcon renders each (state, frame) once; the syncing animation
// would otherwise redraw the same frames every half second.
func cachedIcon(s icon.State, frame int, render func() []byte) []byte {
	if s != icon.Syncing {
		frame = 0
	}
	key := [2]int{int(s), frame}
	iconMu.Lock()
	defer iconMu.Unlock()
	if b, ok := iconCache[key]; ok {
		return b
	}
	b := render()
	iconCache[key] = b
	return b
}
