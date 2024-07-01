package main

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/exp/constraints"
	"image/color"
)

func abs(x int) int {
	if x < 0 {
		return -x
	} else {
		return x
	}
}

func max[T constraints.Ordered](x, y T) T {
	if x > y {
		return x
	} else {
		return y
	}
}

var blackImage = (func() *ebiten.Image {
	img := ebiten.NewImage(1, 1)
	img.Fill(color.White)
	return img
})()

// draws an equilateral triangle on image `img`, centered on coordinates `xy`,
// with radius `r`, rotated by `a` (radians), of gray shade `c`
func drawTriangle(img *ebiten.Image, x, y, r int, a float64, c uint8) {
	xf, yf, rf := float32(x), float32(y), float64(r)
	x1 := xf + float32(rf*math.Cos(a))
	y1 := yf + float32(rf*math.Sin(a))
	x2 := xf + float32(rf*math.Cos(a+2*math.Pi/3))
	y2 := yf + float32(rf*math.Sin(a+2*math.Pi/3))
	x3 := xf + float32(rf*math.Cos(a-2*math.Pi/3))
	y3 := yf + float32(rf*math.Sin(a-2*math.Pi/3))

	cf := float32(c) / 255

	vertices := []ebiten.Vertex{
		{DstX: x1, DstY: y1, ColorR: cf, ColorG: cf, ColorB: cf, ColorA: 1.0},
		{DstX: x2, DstY: y2, ColorR: cf, ColorG: cf, ColorB: cf, ColorA: 1.0},
		{DstX: x3, DstY: y3, ColorR: cf, ColorG: cf, ColorB: cf, ColorA: 1.0},
	}

	img.DrawTriangles(vertices, []uint16{0, 1, 2}, blackImage, nil)
}
