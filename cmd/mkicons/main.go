// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Håvard Dagenborg

// Command mkicons writes the application icons and logo images used by
// the packaging scripts and the README:
//
//	go run ./cmd/mkicons -iconset build/AppIcon.iconset   # macOS, then iconutil
//	go run ./cmd/mkicons -ico build/UnisonCue.ico        # Windows .exe icon
//	go run ./cmd/mkicons -assets assets                   # PNG logos + tray preview
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/haavardj/unison-cue/internal/icon"
)

func main() {
	iconset := flag.String("iconset", "", "write a macOS .iconset directory here")
	ico := flag.String("ico", "", "write a Windows .ico file here")
	assets := flag.String("assets", "", "write logo PNGs and a tray icon preview into this directory")
	flag.Parse()

	if *iconset != "" {
		mkdir(*iconset)
		for _, s := range []int{16, 32, 128, 256, 512} {
			writePNG(filepath.Join(*iconset, fmt.Sprintf("icon_%dx%d.png", s, s)), icon.AppIcon(s))
			writePNG(filepath.Join(*iconset, fmt.Sprintf("icon_%dx%d@2x.png", s, s)), icon.AppIcon(2*s))
		}
	}
	if *ico != "" {
		var imgs []*image.NRGBA
		for _, s := range []int{16, 24, 32, 48, 64, 128, 256} {
			imgs = append(imgs, icon.AppIcon(s))
		}
		if err := os.WriteFile(*ico, icon.EncodeICO(imgs...), 0o644); err != nil {
			log.Fatal(err)
		}
	}
	if *assets != "" {
		mkdir(*assets)
		for _, s := range []int{128, 512} {
			writePNG(filepath.Join(*assets, fmt.Sprintf("logo-%d.png", s)), icon.AppIcon(s))
		}
		writePNG(filepath.Join(*assets, "tray-states.png"), trayPreview())
	}
}

// trayPreview shows the four tray states, macOS (template, on a light
// and a dark menu bar) above Windows (colour).
func trayPreview() image.Image {
	const cell, pad = 64, 12
	states := []icon.State{icon.Idle, icon.Due, icon.Error, icon.Syncing}
	w := len(states)*(cell+pad) + pad
	h := 3*(cell+pad) + pad
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	bgs := []color.NRGBA{{0xF2, 0xF2, 0xF2, 0xFF}, {0x2A, 0x2A, 0x2E, 0xFF}, {0x20, 0x26, 0x33, 0xFF}}
	for row, bg := range bgs {
		y := pad + row*(cell+pad)
		draw.Draw(out, image.Rect(0, y-pad/2, w, y+cell+pad/2), &image.Uniform{bg}, image.Point{}, draw.Src)
		for i, s := range states {
			x := pad + i*(cell+pad)
			var img *image.NRGBA
			switch row {
			case 0:
				img = icon.Render(s, icon.SyncFrames/2, cell, true)
			case 1:
				img = tint(icon.Render(s, icon.SyncFrames/2, cell, true), color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF})
			default:
				img = icon.Render(s, icon.SyncFrames/2, cell, false)
			}
			draw.Draw(out, image.Rect(x, y, x+cell, y+cell), img, image.Point{}, draw.Over)
		}
	}
	return out
}

// tint recolours a template image, as macOS does in a dark menu bar.
func tint(img *image.NRGBA, c color.NRGBA) *image.NRGBA {
	out := image.NewNRGBA(img.Rect)
	for i := 0; i < len(img.Pix); i += 4 {
		out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = c.R, c.G, c.B, img.Pix[i+3]
	}
	return out
}

func mkdir(d string) {
	if err := os.MkdirAll(d, 0o755); err != nil {
		log.Fatal(err)
	}
}

func writePNG(path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}
