// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

package icon

import (
	"fmt"
	"image/color"
	"strings"
)

const svgCredit = `<!-- Unison Cue logo, GPL-3.0-or-later. The U is the Unison logo
       (icons/U.svg, https://github.com/bcpierce00/unison, GPL-3.0-or-later). -->`

func hex(c color.NRGBA) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

func alpha(c color.NRGBA) float64 { return float64(c.A) / 255 }

// svgDefs are the gradients used by the logo.
func svgDefs(l appLayout) string {
	var b strings.Builder
	// The official Unison gradient, in the logo's own user space.
	b.WriteString(`<linearGradient id="u" gradientUnits="userSpaceOnUse" x1="19.223558" y1="7.3200717" x2="22.745747" y2="39.056557" gradientTransform="scale(0.934241,1.070387)">`)
	for _, st := range gradStops {
		fmt.Fprintf(&b, `<stop offset="%g" stop-color="%s"/>`, st.at, hex(st.col))
	}
	b.WriteString(`</linearGradient>`)
	fmt.Fprintf(&b, `<linearGradient id="bg" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="%s"/><stop offset="1" stop-color="%s"/></linearGradient>`,
		hex(tileTop), hex(tileBottom))
	if l.hasGlow {
		// Same falloff as AppIcon: full strength to glowR0, then (1-t)² to glowR1.
		f := l.glowR0 / l.glowR1
		fmt.Fprintf(&b, `<radialGradient id="glow" cx="%.2f" cy="%.2f" r="%.2f" gradientUnits="userSpaceOnUse">`, l.cue.x, l.cue.y, l.glowR1)
		fmt.Fprintf(&b, `<stop offset="0" stop-color="%s" stop-opacity="%.3f"/>`, hex(cueGlow), alpha(cueGlow))
		for _, t := range []float64{0, 0.25, 0.5, 0.75, 1} {
			fmt.Fprintf(&b, `<stop offset="%.3f" stop-color="%s" stop-opacity="%.3f"/>`,
				f+(1-f)*t, hex(cueGlow), (1-t)*(1-t)*alpha(cueGlow))
		}
		b.WriteString(`</radialGradient>`)
	}
	return b.String()
}

// svgBody draws the logo for the given layout, in that size's pixel space.
func svgBody(l appLayout) string {
	var b strings.Builder
	if l.tile {
		fmt.Fprintf(&b, `<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" rx="%.2f" fill="url(#bg)"/>`,
			l.m, l.m, l.in, l.in, l.in*0.225)
	}
	fmt.Fprintf(&b, `<path transform="translate(%.3f %.3f) scale(%.5f)" d="%s" fill="url(#u)" stroke="%s" stroke-width="%.4f"/>`,
		l.u.ox, l.u.oy, l.u.k, uPath, hex(outline), l.strokeWidth/l.u.k)
	if l.hasGlow {
		fmt.Fprintf(&b, `<circle cx="%.2f" cy="%.2f" r="%.2f" fill="url(#glow)"/>`, l.cue.x, l.cue.y, l.glowR1)
	}
	fmt.Fprintf(&b, `<circle cx="%.2f" cy="%.2f" r="%.2f" fill="%s"/>`, l.cue.x, l.cue.y, l.cueR, hex(cueBlue))
	if l.hasHighlight {
		fmt.Fprintf(&b, `<circle cx="%.2f" cy="%.2f" r="%.2f" fill="%s" fill-opacity="%.3f"/>`,
			l.cue.x-l.cueR*0.3, l.cue.y-l.cueR*0.3, l.cueR*0.35, hex(cueHighlight), alpha(cueHighlight))
	}
	return b.String()
}

const svgArtboard = 1024

// LogoSVG is the application icon as SVG on a 1024×1024 artboard.
func LogoSVG() string {
	l := layoutFor(svgArtboard)
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %[1]d %[1]d" width="%[1]d" height="%[1]d">
  %[2]s
  <title>Unison Cue</title>
  <defs>%[3]s</defs>
  %[4]s
</svg>
`, svgArtboard, svgCredit, svgDefs(l), svgBody(l))
}

// WordmarkSVG is the icon followed by the name, for READMEs and web pages.
// The name is set in the system UI font; the colours are the logo's red
// and the cue light's blue.
func WordmarkSVG() string {
	l := layoutFor(svgArtboard)
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 180" width="640" height="180">
  %s
  <title>Unison Cue</title>
  <defs>%s</defs>
  <g transform="scale(%.6f)">%s</g>
  <text x="182" y="119" font-family="-apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif" font-size="86" letter-spacing="-1"><tspan font-weight="700" fill="#e5532b">Unison</tspan><tspan font-weight="400" fill="%s"> Cue</tspan></text>
</svg>
`, svgCredit, svgDefs(l), 180.0/svgArtboard, svgBody(l), hex(cueBlue))
}
