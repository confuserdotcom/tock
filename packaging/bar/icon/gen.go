//go:build ignore

// gen renders the tock icon, the "day ring" (the same drawing as icon.svg):
// a clock whose ring is the day split into deep, shallow and distracted time.
//
//	go run gen.go <out.iconset> [glyph dir] [favicon dir]
//
// The iconset goes to iconutil. The glyph dir gets MenuGlyph.png and
// MenuGlyph@2x.png, the menu bar template image. The favicon dir gets
// favicon.png (32 px) and apple-touch-icon.png (180 px).
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

type rgb struct{ r, g, b float64 }

func hex(v uint32) rgb {
	return rgb{float64(v>>16&0xff) / 255, float64(v>>8&0xff) / 255, float64(v&0xff) / 255}
}

func mix(a, b rgb, t float64) rgb {
	t = math.Max(0, math.Min(1, t))
	return rgb{a.r + (b.r-a.r)*t, a.g + (b.g-a.g)*t, a.b + (b.b-a.b)*t}
}

// cover turns a signed distance in pixels into antialiased coverage.
func cover(d float64) float64 { return math.Max(0, math.Min(1, 0.5-d)) }

// roundRect is the macOS icon body: 824 units centred in the 1024 canvas.
func roundRect(x, y float64) float64 {
	const c, h, r = 512.0, 412.0, 185.0
	qx, qy := math.Abs(x-c)-(h-r), math.Abs(y-c)-(h-r)
	out := math.Hypot(math.Max(qx, 0), math.Max(qy, 0))
	return out + math.Min(math.Max(qx, qy), 0) - r
}

// angle is clockwise from 12 o'clock, in degrees [0, 360), around (cx, cy).
func angle(x, y, cx, cy float64) float64 {
	a := math.Atan2(x-cx, cy-y) * 180 / math.Pi
	if a < 0 {
		a += 360
	}
	return a
}

// arc is a stroke of half-width w on the circle of radius r around (cx, cy),
// clockwise from a0 to a1 degrees. Round caps when round, else butt ends.
func arc(x, y, cx, cy, r, w, a0, a1 float64, round bool) float64 {
	d := math.Hypot(x-cx, y-cy)
	if a := angle(x, y, cx, cy); a >= a0 && a <= a1 {
		return math.Abs(d-r) - w
	}
	if !round {
		return math.Inf(1)
	}
	end := func(a float64) float64 {
		t := a * math.Pi / 180
		return math.Hypot(x-(cx+r*math.Sin(t)), y-(cy-r*math.Cos(t))) - w
	}
	return math.Min(end(a0), end(a1))
}

// capsule is a stroke of half-width w from (x0, y0) to (x0, y1), y1 < y0.
func capsule(x, y, x0, y0, y1, w float64) float64 {
	return math.Hypot(x-x0, y-math.Max(y1, math.Min(y0, y))) - w
}

// The day ring's three parts, clockwise from 12 o'clock (icon.svg's arcs).
var parts = []struct {
	a0, a1 float64
	c      uint32
}{
	{0, 108, 0x4fa89a},       // deep
	{127.2, 206.4, 0x6f86b5}, // shallow
	{230.7, 301.9, 0xc9625a}, // distracted
}

func render(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	s := 1024 / float64(size) // design units per pixel
	bgTop, bgBot, track, ink := hex(0x26282d), hex(0x0e0f11), hex(0x2c2e34), hex(0xe6e6e8)
	white := rgb{1, 1, 1}
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			x, y := (float64(px)+0.5)*s, (float64(py)+0.5)*s
			a := cover(roundRect(x, y) / s)
			if a == 0 {
				continue
			}
			c := mix(bgTop, bgBot, (y-100)/824)
			c = mix(c, white, 0.10*math.Max(0, 1-(y-100)/412)) // sheen
			c = mix(c, track, cover(arc(x, y, 512, 512, 250, 32, 0, 360, false)/s))
			for _, p := range parts {
				c = mix(c, hex(p.c), cover(arc(x, y, 512, 512, 250, 32, p.a0, p.a1, true)/s))
			}
			c = mix(c, ink, cover(capsule(x, y, 512, 512, 350, 20)/s))
			c = mix(c, ink, cover((math.Hypot(x-512, y-512)-40)/s))
			img.SetNRGBA(px, py, color.NRGBA{uint8(c.r*255 + .5), uint8(c.g*255 + .5), uint8(c.b*255 + .5), uint8(a*255 + .5)})
		}
	}
	return img
}

// glyph is the menu bar template image at size px for a 16 pt square: black
// with alpha, a ring with a gap at 8 o'clock and one hand. macOS tints it.
func glyph(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	s := 16 / float64(size) // points per pixel
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			x, y := (float64(px)+0.5)*s, (float64(py)+0.5)*s
			d := math.Min(arc(x, y, 8, 8, 6, 0.8, 0, 248, false), arc(x, y, 8, 8, 6, 0.8, 286.5, 360, false))
			d = math.Min(d, capsule(x, y, 8, 8, 4.2, 0.8))
			if a := cover(d / s); a > 0 {
				img.SetNRGBA(px, py, color.NRGBA{0, 0, 0, uint8(a*255 + .5)})
			}
		}
	}
	return img
}

func write(path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
	if err := f.Close(); err != nil {
		panic(err)
	}
}

func main() {
	out := os.Args[1]
	if err := os.MkdirAll(out, 0o755); err != nil {
		panic(err)
	}
	for _, n := range []int{16, 32, 128, 256, 512} {
		write(filepath.Join(out, fmt.Sprintf("icon_%dx%d.png", n, n)), render(n))
		write(filepath.Join(out, fmt.Sprintf("icon_%dx%d@2x.png", n, n)), render(2*n))
	}
	if len(os.Args) > 2 {
		write(filepath.Join(os.Args[2], "MenuGlyph.png"), glyph(16))
		write(filepath.Join(os.Args[2], "MenuGlyph@2x.png"), glyph(32))
	}
	if len(os.Args) > 3 {
		write(filepath.Join(os.Args[3], "favicon.png"), render(32))
		write(filepath.Join(os.Args[3], "apple-touch-icon.png"), render(180))
	}
}
