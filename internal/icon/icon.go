// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

// Package icon draws the tray and application icons at runtime from the
// Unison logo (see logo.go), so the app needs no image assets.
//
// Tray icon states: the plain U means everything is up to date; a dot in
// the U's hollow means a sync is due; a dot with "!" means the last run
// failed or left conflicts; while syncing, the U fills up from the bottom
// (an animation of SyncFrames frames).
package icon

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
)

// State selects the icon variant.
type State int

const (
	Idle    State = iota // everything synced recently
	Due                  // at least one profile should be synced
	Syncing              // a sync is running (animated)
	Error                // the last run of some profile failed or had conflicts
)

// SyncFrames is the number of frames in the syncing animation.
const SyncFrames = 8

var (
	black    = color.NRGBA{0, 0, 0, 0xFF}
	outline  = color.NRGBA{0x1A, 0x1A, 0x1A, 0xFF}
	white    = color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}
	dueBlue  = color.NRGBA{0x25, 0x63, 0xEB, 0xFF}
	errorRed = color.NRGBA{0xC8, 0x1E, 0x1E, 0xFF}
	// unfilled part of the U while syncing, colour icons only
	syncEmpty = color.NRGBA{0xFF, 0xFF, 0xFF, 0xC8}
)

// Render draws the tray icon at size×size pixels. With template set, the
// image is black on transparent: macOS tints template images to suit a
// light or dark menu bar. Otherwise the logo's own colours are used.
// frame selects the animation frame for Syncing and is ignored otherwise.
func Render(state State, frame, size int, template bool) *image.NRGBA {
	s := float64(size)
	c := NewCanvas(size)
	height := s * 0.98
	if template {
		height = s * 0.88 // menu bar glyphs sit with a little air around them
	}
	pl := placeU(s/2, s/2, height)

	st := uStyle{}
	if template {
		st.solid = &black
	} else {
		st.stroke, st.strokeWidth = outline, math.Max(logoStrokeWidth*pl.k, 0.8)
	}
	if state == Syncing {
		f := ((frame % SyncFrames) + SyncFrames) % SyncFrames
		st.fillFrom = 1 - float64(f)/float64(SyncFrames-1)
		if template {
			st.stroke, st.strokeWidth = black, math.Max(s*0.036, 1)
		} else {
			st.empty = syncEmpty
		}
	}
	c.drawU(pl, st)

	if state == Due || state == Error {
		ctr := pl.pt(cupCenter)
		r := cupHalfW * pl.k * 0.8
		badge := circleMask(size, ctr.x, ctr.y, r)
		col := dueBlue
		if state == Error {
			col = errorRed
		}
		if template {
			col = black
		}
		c.fill(badge, col)
		if state == Error {
			bar := roundRectMask(size, ctr.x-0.13*r, ctr.y-0.62*r, ctr.x+0.13*r, ctr.y+0.18*r, 0.13*r)
			dot := circleMask(size, ctr.x, ctr.y+0.45*r, 0.15*r)
			if template {
				c.erase(bar)
				c.erase(dot)
			} else {
				c.fill(bar, white)
				c.fill(dot, white)
			}
		}
	}
	return c.Image()
}

// PNG encodes a tray icon as PNG.
func PNG(state State, frame, size int, template bool) []byte {
	return encodePNG(Render(state, frame, size, template))
}

// ICO encodes a colour tray icon as a Windows .ico with one image per size.
func ICO(state State, frame int, sizes ...int) []byte {
	imgs := make([]*image.NRGBA, len(sizes))
	for i, s := range sizes {
		imgs[i] = Render(state, frame, s, false)
	}
	return EncodeICO(imgs...)
}

func encodePNG(img image.Image) []byte {
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// AppIcon draws the application icon: the Unison logo resting in a tray,
// on a dark rounded square. Small sizes (≤ 32 px) get the bare logo,
// which stays legible where the full design would turn to mush.
func AppIcon(size int) *image.NRGBA {
	s := float64(size)
	c := NewCanvas(size)
	if size <= 32 {
		pl := placeU(s/2, s/2, s*0.96)
		c.drawU(pl, uStyle{stroke: outline, strokeWidth: math.Max(logoStrokeWidth*pl.k, 0.7)})
		return c.Image()
	}

	// Background tile, following the macOS icon grid (824/1024 artwork).
	m := s * 0.1
	in := s - 2*m
	top, bottom := color.NRGBA{0x3A, 0x47, 0x5C, 0xFF}, color.NRGBA{0x14, 0x1B, 0x29, 0xFF}
	tile := roundRectMask(size, m, m, s-m, s-m, in*0.225)
	c.paint(tile, func(x, y float64) color.NRGBA { return lerpColor(top, bottom, clamp01((y-m)/in)) })

	// The tray: a shallow dish under the logo.
	trayY, trayW, trayT := m+in*0.80, in*0.62, in*0.045
	trayCol := color.NRGBA{0xE2, 0xE8, 0xF0, 0xFF}
	c.fill(roundRectMask(size, s/2-trayW/2, trayY-trayT/2, s/2+trayW/2, trayY+trayT/2, trayT/2), trayCol)
	for _, sx := range []float64{-1, 1} {
		x := s/2 + sx*(trayW/2-trayT/2)
		c.fill(roundRectMask(size, x-trayT/2, trayY-in*0.09, x+trayT/2, trayY+trayT/2, trayT/2), trayCol)
	}

	pl := placeU(s/2, m+in*0.42, in*0.62)
	c.drawU(pl, uStyle{stroke: outline, strokeWidth: logoStrokeWidth * pl.k * 1.5})
	return c.Image()
}

// EncodeICO packs images into a Windows .ico, each as a classic 32-bit
// BMP entry (these load everywhere; PNG entries need Vista+ loaders).
func EncodeICO(imgs ...*image.NRGBA) []byte {
	entries := make([][]byte, len(imgs))
	for i, im := range imgs {
		entries[i] = bmpEntry(im)
	}
	var buf bytes.Buffer
	le := binary.LittleEndian
	binary.Write(&buf, le, [3]uint16{0, 1, uint16(len(imgs))}) // ICONDIR
	offset := 6 + 16*len(imgs)
	for i, im := range imgs {
		dim := uint8(im.Rect.Dx())
		if im.Rect.Dx() >= 256 {
			dim = 0
		}
		buf.Write([]byte{dim, dim, 0, 0})
		binary.Write(&buf, le, uint16(1))  // planes
		binary.Write(&buf, le, uint16(32)) // bpp
		binary.Write(&buf, le, uint32(len(entries[i])))
		binary.Write(&buf, le, uint32(offset))
		offset += len(entries[i])
	}
	for _, e := range entries {
		buf.Write(e)
	}
	return buf.Bytes()
}

// bmpEntry is a BITMAPINFOHEADER + bottom-up BGRA pixels + AND mask.
func bmpEntry(img *image.NRGBA) []byte {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	maskStride := ((w + 31) / 32) * 4
	var buf bytes.Buffer
	le := binary.LittleEndian
	binary.Write(&buf, le, uint32(40))
	binary.Write(&buf, le, int32(w))
	binary.Write(&buf, le, int32(h*2)) // XOR + AND masks
	binary.Write(&buf, le, uint16(1))
	binary.Write(&buf, le, uint16(32))
	binary.Write(&buf, le, uint32(0)) // BI_RGB
	binary.Write(&buf, le, uint32(w*h*4+maskStride*h))
	binary.Write(&buf, le, [4]uint32{})
	for y := h - 1; y >= 0; y-- {
		for x := 0; x < w; x++ {
			c := img.NRGBAAt(x, y)
			buf.Write([]byte{c.B, c.G, c.R, c.A})
		}
	}
	mask := make([]byte, maskStride)
	for y := h - 1; y >= 0; y-- {
		clear(mask)
		for x := 0; x < w; x++ {
			if img.NRGBAAt(x, y).A == 0 {
				mask[x/8] |= 0x80 >> (x % 8)
			}
		}
		buf.Write(mask)
	}
	return buf.Bytes()
}
