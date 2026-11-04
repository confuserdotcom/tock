//go:build ignore

// gen renders the tock app icon (the same drawing as icon.svg) into an
// .iconset folder for iconutil. Run: go run gen.go <out.iconset>
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

func ring(x, y, r, w float64) float64 { return math.Abs(math.Hypot(x-512, y-512)-r) - w }

// arc is the brass stroke: clockwise from 12 o'clock to 9 o'clock, round caps.
func arc(x, y float64) float64 {
	th := math.Atan2(x-512, 512-y)
	if th < 0 {
		th += 2 * math.Pi
	}
	if th <= 1.5*math.Pi {
		return ring(x, y, 250, 32)
	}
	return math.Min(math.Hypot(x-512, y-262), math.Hypot(x-262, y-512)) - 32
}

// hand is the clock hand: a capsule from the centre up to y 352.
func hand(x, y float64) float64 {
	cy := math.Max(352, math.Min(512, y))
	return math.Hypot(x-512, y-cy) - 20
}

func render(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	s := 1024 / float64(size) // design units per pixel
	bgTop, bgBot := hex(0x2c313a), hex(0x111317)
	track, brassA, brassB, ink := hex(0x323843), hex(0xf4c875), hex(0xc47c1c), hex(0xeef0f3)
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			x, y := (float64(px)+0.5)*s, (float64(py)+0.5)*s
			a := cover(roundRect(x, y) / s)
			if a == 0 {
				continue
			}
			c := mix(bgTop, bgBot, (y-100)/824)
			c = mix(c, track, cover(ring(x, y, 250, 32)/s))
			c = mix(c, mix(brassA, brassB, ((x-262)+(y-262))/1000), cover(arc(x, y)/s))
			c = mix(c, ink, cover(hand(x, y)/s))
			c = mix(c, ink, cover((math.Hypot(x-512, y-512)-40)/s))
			img.SetNRGBA(px, py, color.NRGBA{uint8(c.r*255 + .5), uint8(c.g*255 + .5), uint8(c.b*255 + .5), uint8(a*255 + .5)})
		}
	}
	return img
}

func main() {
	out := os.Args[1]
	if err := os.MkdirAll(out, 0o755); err != nil {
		panic(err)
	}
	for _, n := range []int{16, 32, 128, 256, 512} {
		for _, k := range []int{1, 2} {
			name := fmt.Sprintf("icon_%dx%d.png", n, n)
			if k == 2 {
				name = fmt.Sprintf("icon_%dx%d@2x.png", n, n)
			}
			f, err := os.Create(filepath.Join(out, name))
			if err != nil {
				panic(err)
			}
			if err := png.Encode(f, render(n*k)); err != nil {
				panic(err)
			}
			if err := f.Close(); err != nil {
				panic(err)
			}
		}
	}
}
