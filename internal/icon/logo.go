// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package icon

import (
	"image/color"
	"math"
	"strconv"
	"strings"
)

// uPath is the outline of the Unison logo, a "U" whose arms are two
// upward arrows, copied from icons/U.svg in the Unison repository
// (https://github.com/bcpierce00/unison, GPL-3.0). Coordinates are in
// that file's 48×48 user space.
const uPath = `M 11.501514,0.70044889 L 3.5845995,9.8069515 L 7.1943842,9.8069515 L 7.1943842,29.250565 C 7.1943803,33.458778 7.4618286,36.652879 8.0147898,38.808291 C 8.3961341,40.262350 9.1927086,41.658059 10.393966,42.992360 C 11.595209,44.309562 13.257100,45.353724 15.316400,46.191942 C 17.394740,47.013054 20.418712,47.463572 24.422902,47.463571 C 27.740620,47.463571 30.444584,47.067690 32.503898,46.315003 C 34.563154,45.545211 36.205979,44.521473 37.426332,43.238482 C 38.665682,41.955498 39.511930,40.385764 39.969589,38.521149 C 40.427171,36.656551 40.625874,33.479553 40.625914,28.963423 L 40.625914,9.8069515 L 44.399780,9.8069515 L 36.482865,0.70044889 L 28.565951,9.8069515 L 32.175736,9.8069515 L 32.175736,29.988930 C 32.175707,32.965476 32.054402,35.105119 31.806553,36.388094 C 31.558647,37.671091 30.811749,38.739167 29.591458,39.628697 C 28.371113,40.501134 26.579260,40.941351 24.176781,40.941346 C 21.812394,40.941351 19.929842,40.470413 18.557002,39.546656 C 17.203196,38.605807 16.334993,37.394424 15.972724,35.854831 C 15.762969,34.896879 15.685570,32.811870 15.685582,29.578728 L 15.685582,9.8069515 L 19.418428,9.8069515 L 11.501514,0.70044889 z`

// Official gradient (linearGradient2819/2825 in U.svg), resolved into
// user space: gradientTransform scale(0.934241,1.070387) applied to
// (19.223558,7.3200717)→(22.745747,39.056557).
var (
	gradFrom  = pt{19.223558 * 0.934241, 7.3200717 * 1.070387}
	gradTo    = pt{22.745747 * 0.934241, 39.056557 * 1.070387}
	gradStops = []struct {
		at  float64
		col color.NRGBA
	}{
		{0, color.NRGBA{0xFA, 0xFE, 0x52, 0xFF}},
		{0.675, color.NRGBA{0xEF, 0x94, 0x29, 0xFF}},
		{1, color.NRGBA{0xE5, 0x2B, 0x00, 0xFF}},
	}
	// The official outline is black, 0.5 units wide.
	logoStrokeWidth = 0.5
)

// uOutline is uPath flattened to a polygon, in SVG units.
var uOutline = flattenPath(uPath)

// Bounding box and features of the logo in SVG units.
var (
	uMinX, uMinY, uMaxX, uMaxY = bounds(uOutline)
	// The hollow between the arms (inner edges at x≈15.69 and x≈32.18,
	// bottom at y≈40.94); state badges sit here.
	cupCenter = pt{23.93, 26.5}
	cupHalfW  = (32.175736 - 15.685582) / 2
)

func gradientAt(p pt) color.NRGBA {
	dx, dy := gradTo.x-gradFrom.x, gradTo.y-gradFrom.y
	t := clamp01(((p.x-gradFrom.x)*dx + (p.y-gradFrom.y)*dy) / (dx*dx + dy*dy))
	for i := 1; i < len(gradStops); i++ {
		a, b := gradStops[i-1], gradStops[i]
		if t <= b.at {
			return lerpColor(a.col, b.col, (t-a.at)/(b.at-a.at))
		}
	}
	return gradStops[len(gradStops)-1].col
}

// placement maps SVG logo units to canvas pixels.
type placement struct{ k, ox, oy float64 }

// placeU scales the logo to height pixels, centred on (cx, cy).
func placeU(cx, cy, height float64) placement {
	k := height / (uMaxY - uMinY)
	return placement{k: k, ox: cx - (uMinX+uMaxX)/2*k, oy: cy - (uMinY+uMaxY)/2*k}
}

func (pl placement) pt(p pt) pt          { return pt{p.x*pl.k + pl.ox, p.y*pl.k + pl.oy} }
func (pl placement) inv(x, y float64) pt { return pt{(x - pl.ox) / pl.k, (y - pl.oy) / pl.k} }

func (pl placement) poly() []pt {
	out := make([]pt, len(uOutline))
	for i, p := range uOutline {
		out[i] = pl.pt(p)
	}
	return out
}

// flattenPath parses the subset of SVG path syntax used by uPath
// (absolute M, L, C, Z) and flattens curves into line segments.
func flattenPath(d string) []pt {
	f := strings.FieldsFunc(d, func(r rune) bool { return r == ' ' || r == ',' || r == '\n' || r == '\t' })
	var out []pt
	var cmd string
	num := func(i *int) float64 {
		v, err := strconv.ParseFloat(f[*i], 64)
		if err != nil {
			panic("icon: bad path number " + f[*i])
		}
		*i++
		return v
	}
	for i := 0; i < len(f); {
		if _, err := strconv.ParseFloat(f[i], 64); err != nil {
			cmd = f[i]
			i++
			continue
		}
		switch cmd {
		case "M", "L":
			out = append(out, pt{num(&i), num(&i)})
		case "C":
			p0 := out[len(out)-1]
			p1 := pt{num(&i), num(&i)}
			p2 := pt{num(&i), num(&i)}
			p3 := pt{num(&i), num(&i)}
			const steps = 12
			for s := 1; s <= steps; s++ {
				t := float64(s) / steps
				u := 1 - t
				out = append(out, pt{
					u*u*u*p0.x + 3*u*u*t*p1.x + 3*u*t*t*p2.x + t*t*t*p3.x,
					u*u*u*p0.y + 3*u*u*t*p1.y + 3*u*t*t*p2.y + t*t*t*p3.y,
				})
			}
		default:
			panic("icon: unsupported path command " + cmd)
		}
	}
	// Drop the closing point if it repeats the first.
	if n := len(out); n > 1 && math.Hypot(out[0].x-out[n-1].x, out[0].y-out[n-1].y) < 1e-9 {
		out = out[:n-1]
	}
	return out
}

// uStyle controls how drawU paints the logo.
type uStyle struct {
	solid       *color.NRGBA // nil = official gradient
	stroke      color.NRGBA
	strokeWidth float64 // pixels; 0 = no outline
	// fillFrom (0..1) leaves the top part of the U unfilled, measured
	// from the top of the logo; used for the "syncing" animation.
	fillFrom float64
	// empty is painted in the unfilled part (zero value = transparent).
	empty color.NRGBA
}

// drawU paints the Unison logo onto c.
func (c *Canvas) drawU(pl placement, st uStyle) {
	poly := pl.poly()
	top := uMinY*pl.k + pl.oy
	h := (uMaxY - uMinY) * pl.k
	if st.fillFrom > 0 && st.empty.A > 0 {
		c.fill(polyMask(c.W, poly, 0), st.empty)
	}
	body := polyMask(c.W, poly, top+st.fillFrom*h)
	if st.solid != nil {
		c.fill(body, *st.solid)
	} else {
		c.paint(body, func(x, y float64) color.NRGBA { return gradientAt(pl.inv(x, y)) })
	}
	if st.strokeWidth > 0 {
		c.fill(strokeMask(c.W, poly, st.strokeWidth), st.stroke)
	}
}
