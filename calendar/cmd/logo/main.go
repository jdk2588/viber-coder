package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
)

func main() {
	const size = 256
	const headerHeight = size / 4

	img := image.NewRGBA(image.Rect(0, 0, size, size))

	bg := color.NRGBA{R: 249, G: 248, B: 255, A: 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{C: bg}, image.Point{}, draw.Src)

	border := color.NRGBA{R: 90, G: 78, B: 199, A: 255}

	// Draw border
	for x := 0; x < size; x++ {
		img.Set(x, 0, border)
		img.Set(x, size-1, border)
	}
	for y := 0; y < size; y++ {
		img.Set(0, y, border)
		img.Set(size-1, y, border)
	}

	// Draw header bar
	headerColor := color.NRGBA{R: 224, G: 62, B: 82, A: 255}
	fillRect(img, image.Rect(1, 1, size-1, headerHeight), headerColor)

	// Draw binding rings
	ringColor := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	ringShadow := color.NRGBA{R: 200, G: 32, B: 52, A: 255}
	ringRadius := float64(size) * 0.06
	ringY := float64(headerHeight) * 0.5
	for _, cx := range []float64{float64(size) * 0.3, float64(size) * 0.7} {
		drawCircle(img, int(cx), int(ringY), int(ringRadius+2), ringShadow)
		drawCircle(img, int(cx), int(ringY), int(ringRadius), ringColor)
	}

	// Draw calendar grid area
	gridRect := image.Rect(10, headerHeight+10, size-10, size-10)
	surface := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	fillRect(img, gridRect, surface)

	gridLine := color.NRGBA{R: 220, G: 216, B: 240, A: 255}
	rows := 5
	cols := 7

	cellWidth := float64(gridRect.Dx()) / float64(cols)
	cellHeight := float64(gridRect.Dy()) / float64(rows)

	// Draw vertical lines
	for c := 1; c < cols; c++ {
		x := int(float64(gridRect.Min.X) + cellWidth*float64(c))
		for y := gridRect.Min.Y; y < gridRect.Max.Y; y++ {
			img.Set(x, y, gridLine)
		}
	}
	// Draw horizontal lines
	for r := 1; r < rows; r++ {
		y := int(float64(gridRect.Min.Y) + cellHeight*float64(r))
		for x := gridRect.Min.X; x < gridRect.Max.X; x++ {
			img.Set(x, y, gridLine)
		}
	}

	// Highlight selected day (center cell)
	highlight := color.NRGBA{R: 113, G: 102, B: 226, A: 255}
	textShade := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	selectedCol := 3
	selectedRow := 2

	cellRect := image.Rect(
		int(float64(gridRect.Min.X)+cellWidth*float64(selectedCol)),
		int(float64(gridRect.Min.Y)+cellHeight*float64(selectedRow)),
		int(float64(gridRect.Min.X)+cellWidth*float64(selectedCol+1)),
		int(float64(gridRect.Min.Y)+cellHeight*float64(selectedRow+1)),
	)
	inset := 6
	fillRect(img, cellRect.Inset(inset), highlight)

	// Add small circle as day indicator
	drawCircle(img, cellRect.Min.X+cellRect.Dx()/2, cellRect.Min.Y+cellRect.Dy()/2, int(float64(cellRect.Dx())*0.18), textShade)

	// Save file
	if err := os.MkdirAll("assets", 0o755); err != nil {
		panic(err)
	}

	f, err := os.Create("assets/calendar_logo.png")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
}

func fillRect(img *image.RGBA, rect image.Rectangle, c color.NRGBA) {
	draw.Draw(img, rect, &image.Uniform{C: c}, image.Point{}, draw.Src)
}

func drawCircle(img *image.RGBA, cx, cy, r int, c color.NRGBA) {
	rr := float64(r)
	for y := -r; y <= r; y++ {
		for x := -r; x <= r; x++ {
			if math.Hypot(float64(x), float64(y)) <= rr {
				img.Set(cx+x, cy+y, c)
			}
		}
	}
}
