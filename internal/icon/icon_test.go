// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package icon

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"image/png"
	"io"
	"math"
	"strings"
	"testing"
)

func TestPathParsed(t *testing.T) {
	// 22 M/L points + 15 cubics × 12 steps, minus the duplicated start.
	if len(uOutline) < 150 {
		t.Fatalf("outline has %d points", len(uOutline))
	}
	if math.Abs(uMinX-3.5846) > 1e-3 || math.Abs(uMaxX-44.3998) > 1e-3 ||
		math.Abs(uMinY-0.7004) > 1e-3 || math.Abs(uMaxY-47.4636) > 1e-2 {
		t.Errorf("bounds = %v %v %v %v", uMinX, uMinY, uMaxX, uMaxY)
	}
}

func TestGradientEnds(t *testing.T) {
	if c := gradientAt(gradFrom); c != gradStops[0].col {
		t.Errorf("top = %v", c)
	}
	if c := gradientAt(pt{24, 60}); c != gradStops[2].col {
		t.Errorf("bottom = %v", c)
	}
}

func alphaAt(b []byte, x, y int) uint8 {
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		panic(err)
	}
	_, _, _, a := img.At(x, y).RGBA()
	return uint8(a >> 8)
}

func TestTrayShapes(t *testing.T) {
	const s = 44
	pl := placeU(s/2, s/2, s*0.88)
	cup := pl.pt(cupCenter)
	arm := pl.pt(pt{11.4, 20}) // inside the left arm
	idle := PNG(Idle, 0, s, true)
	if alphaAt(idle, int(arm.x), int(arm.y)) != 255 {
		t.Error("arm not filled")
	}
	if alphaAt(idle, int(cup.x), int(cup.y)) != 0 {
		t.Error("cup not empty when idle")
	}
	if alphaAt(PNG(Due, 0, s, true), int(cup.x), int(cup.y)) != 255 {
		t.Error("no badge when due")
	}
	// Error badge has the "!" cut out at its centre line.
	if alphaAt(PNG(Error, 0, s, true), int(cup.x), int(cup.y)-2) == 255 {
		t.Error("error badge has no exclamation mark")
	}
	// Syncing: first frame is hollow, last is full.
	if alphaAt(PNG(Syncing, 0, s, true), int(arm.x), int(arm.y)) != 0 {
		t.Error("frame 0 should be hollow")
	}
	if alphaAt(PNG(Syncing, SyncFrames-1, s, true), int(arm.x), int(arm.y)) != 255 {
		t.Error("last frame should be full")
	}
}

func TestStatesDiffer(t *testing.T) {
	seen := map[string]State{}
	for _, s := range []State{Idle, Due, Syncing, Error} {
		key := string(PNG(s, 3, 32, true))
		if prev, dup := seen[key]; dup {
			t.Errorf("template icons for states %d and %d are identical", prev, s)
		}
		seen[key] = s
	}
}

func TestICOLayout(t *testing.T) {
	sizes := []int{16, 32, 48}
	b := ICO(Due, 0, sizes...)
	le := binary.LittleEndian
	if le.Uint16(b[0:]) != 0 || le.Uint16(b[2:]) != 1 || int(le.Uint16(b[4:])) != len(sizes) {
		t.Fatalf("bad ICONDIR % x", b[:6])
	}
	for i, s := range sizes {
		e := b[6+16*i:]
		size, off := le.Uint32(e[8:]), le.Uint32(e[12:])
		if int(e[0]) != s || le.Uint16(e[6:]) != 32 {
			t.Errorf("entry %d header wrong", i)
		}
		mask := ((s + 31) / 32) * 4 * s
		if int(size) != 40+s*s*4+mask || int(off+size) > len(b) {
			t.Errorf("entry %d size/offset wrong: %d at %d", i, size, off)
		}
		img := b[off:]
		if le.Uint32(img) != 40 || int(int32(le.Uint32(img[8:]))) != 2*s {
			t.Errorf("entry %d BITMAPINFOHEADER wrong", i)
		}
	}
}

func TestSVGWellFormed(t *testing.T) {
	for name, s := range map[string]string{"logo": LogoSVG(), "wordmark": WordmarkSVG()} {
		d := xml.NewDecoder(strings.NewReader(s))
		for {
			_, err := d.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
		if !strings.Contains(s, uPath) || !strings.Contains(s, `id="glow"`) {
			t.Errorf("%s: missing logo path or glow", name)
		}
	}
}

func TestAppIconHasCueLight(t *testing.T) {
	for _, size := range []int{16, 32, 128, 512} {
		l := layoutFor(size)
		c := AppIcon(size).NRGBAAt(int(l.cue.x), int(l.cue.y))
		if c.B < 0xC0 || c.R > 0x80 {
			t.Errorf("size %d: centre of cue light is %v, want blue", size, c)
		}
	}
}
