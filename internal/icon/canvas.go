// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package icon

import (
	"image"
	"image/color"
	"math"
	"sort"
)

// pt is a point in pixel coordinates (y down).
type pt struct{ x, y float64 }

// Canvas is a small anti-aliased software rasterizer: shapes produce a
// per-pixel coverage mask, which is then painted or erased.
// Pixels are stored as premultiplied RGBA in 0..1.
type Canvas struct {
	W          int
	r, g, b, a []float64
}

func NewCanvas(size int) *Canvas {
	n := size * size
	return &Canvas{W: size, r: make([]float64, n), g: make([]float64, n), b: make([]float64, n), a: make([]float64, n)}
}

// mask is per-pixel coverage, 0..1.
type mask []float64

// paint composites colour (a function of the pixel centre) over the canvas
// wherever m covers it.
func (c *Canvas) paint(m mask, col func(x, y float64) color.NRGBA) {
	for i, cov := range m {
		if cov <= 0 {
			continue
		}
		x, y := float64(i%c.W)+0.5, float64(i/c.W)+0.5
		k := col(x, y)
		sa := math.Min(cov, 1) * float64(k.A) / 255
		c.r[i] = float64(k.R)/255*sa + c.r[i]*(1-sa)
		c.g[i] = float64(k.G)/255*sa + c.g[i]*(1-sa)
		c.b[i] = float64(k.B)/255*sa + c.b[i]*(1-sa)
		c.a[i] = sa + c.a[i]*(1-sa)
	}
}

func (c *Canvas) fill(m mask, k color.NRGBA) {
	c.paint(m, func(float64, float64) color.NRGBA { return k })
}

// erase makes the canvas transparent where m covers it.
func (c *Canvas) erase(m mask) {
	for i, cov := range m {
		if cov <= 0 {
			continue
		}
		f := 1 - math.Min(cov, 1)
		c.r[i] *= f
		c.g[i] *= f
		c.b[i] *= f
		c.a[i] *= f
	}
}

// Image returns the canvas as a non-premultiplied image.
func (c *Canvas) Image() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, c.W, c.W))
	for i := range c.a {
		a := c.a[i]
		if a <= 0 {
			continue
		}
		img.Pix[4*i+0] = uint8(math.Round(255 * math.Min(c.r[i]/a, 1)))
		img.Pix[4*i+1] = uint8(math.Round(255 * math.Min(c.g[i]/a, 1)))
		img.Pix[4*i+2] = uint8(math.Round(255 * math.Min(c.b[i]/a, 1)))
		img.Pix[4*i+3] = uint8(math.Round(255 * math.Min(a, 1)))
	}
	return img
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

const subRows = 5 // vertical supersampling; horizontal coverage is exact

// polyMask fills a closed polygon (even-odd rule) with scanlines. Only
// sub-rows with y >= fromY are filled, which clips the fill from the top.
func polyMask(size int, poly []pt, fromY float64) mask {
	m := make(mask, size*size)
	var xs []float64
	for py := 0; py < size; py++ {
		row := m[py*size : (py+1)*size]
		for s := 0; s < subRows; s++ {
			y := float64(py) + (float64(s)+0.5)/subRows
			if y < fromY {
				continue
			}
			xs = xs[:0]
			for i := range poly {
				p, q := poly[i], poly[(i+1)%len(poly)]
				if (p.y <= y && q.y > y) || (q.y <= y && p.y > y) {
					xs = append(xs, p.x+(y-p.y)*(q.x-p.x)/(q.y-p.y))
				}
			}
			sort.Float64s(xs)
			for i := 0; i+1 < len(xs); i += 2 {
				addSpan(row, xs[i], xs[i+1], 1.0/subRows)
			}
		}
	}
	return m
}

func addSpan(row mask, x0, x1, w float64) {
	x0, x1 = math.Max(x0, 0), math.Min(x1, float64(len(row)))
	for px := int(x0); px < len(row) && float64(px) < x1; px++ {
		ov := math.Min(x1, float64(px+1)) - math.Max(x0, float64(px))
		if ov > 0 {
			row[px] += ov * w
		}
	}
}

// strokeMask covers pixels within width/2 of the closed polygon's edges.
func strokeMask(size int, poly []pt, width float64) mask {
	m := make(mask, size*size)
	half := width / 2
	minX, minY, maxX, maxY := bounds(poly)
	for py := 0; py < size; py++ {
		y := float64(py) + 0.5
		if y < minY-half-1 || y > maxY+half+1 {
			continue
		}
		for px := 0; px < size; px++ {
			x := float64(px) + 0.5
			if x < minX-half-1 || x > maxX+half+1 {
				continue
			}
			d := math.Inf(1)
			for i := range poly {
				d = math.Min(d, segDist(pt{x, y}, poly[i], poly[(i+1)%len(poly)]))
			}
			m[py*size+px] = clamp01(half - d + 0.5)
		}
	}
	return m
}

func segDist(p, a, b pt) float64 {
	dx, dy := b.x-a.x, b.y-a.y
	l2 := dx*dx + dy*dy
	t := 0.0
	if l2 > 0 {
		t = clamp01(((p.x-a.x)*dx + (p.y-a.y)*dy) / l2)
	}
	return math.Hypot(p.x-(a.x+t*dx), p.y-(a.y+t*dy))
}

func bounds(poly []pt) (minX, minY, maxX, maxY float64) {
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	for _, p := range poly {
		minX, maxX = math.Min(minX, p.x), math.Max(maxX, p.x)
		minY, maxY = math.Min(minY, p.y), math.Max(maxY, p.y)
	}
	return
}

// sdfMask turns a signed distance function (pixels, negative inside)
// into an anti-aliased mask.
func sdfMask(size int, sdf func(x, y float64) float64) mask {
	m := make(mask, size*size)
	for i := range m {
		x, y := float64(i%size)+0.5, float64(i/size)+0.5
		m[i] = clamp01(0.5 - sdf(x, y))
	}
	return m
}

func circleMask(size int, cx, cy, r float64) mask {
	return sdfMask(size, func(x, y float64) float64 { return math.Hypot(x-cx, y-cy) - r })
}

// roundRectMask covers the rectangle [x0,x1]×[y0,y1] with corner radius r.
func roundRectMask(size int, x0, y0, x1, y1, r float64) mask {
	cx, cy := (x0+x1)/2, (y0+y1)/2
	hx, hy := (x1-x0)/2-r, (y1-y0)/2-r
	return sdfMask(size, func(x, y float64) float64 {
		qx, qy := math.Abs(x-cx)-hx, math.Abs(y-cy)-hy
		out := math.Hypot(math.Max(qx, 0), math.Max(qy, 0))
		return out + math.Min(math.Max(qx, qy), 0) - r
	})
}

func lerpColor(a, b color.NRGBA, t float64) color.NRGBA {
	l := func(x, y uint8) uint8 { return uint8(math.Round(float64(x) + (float64(y)-float64(x))*t)) }
	return color.NRGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), l(a.A, b.A)}
}
