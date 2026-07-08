package clientapp

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// trayIconPNG returns a small tray icon (amber on transparent).
func trayIconPNG() []byte {
	return encodePNG(drawTrayIcon(false))
}

// trayTemplateIconPNG returns a monochrome template icon for macOS menu bar.
func trayTemplateIconPNG() []byte {
	return encodePNG(drawTrayIcon(true))
}

func drawTrayIcon(template bool) *image.RGBA {
	const size = 44
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	ink := color.RGBA{R: 232, G: 161, B: 60, A: 255}
	if template {
		ink = color.RGBA{A: 255}
	}
	center := size / 2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x-center), float64(y-center)
			d := dx*dx + dy*dy
			if d >= float64(center-6)*float64(center-6) && d <= float64(center-2)*float64(center-2) {
				img.SetRGBA(x, y, ink)
			}
		}
	}
	// Speaker cone.
	for y := center - 8; y <= center+8; y++ {
		for x := center - 2; x <= center+2; x++ {
			img.SetRGBA(x, y, ink)
		}
	}
	for _, offset := range []int{10, 16} {
		for y := center - offset; y <= center+offset; y++ {
			dy := float64(y - center)
			edge := float64(offset)
			if dy*dy > edge*edge {
				continue
			}
			x := center + offset
			if x < size {
				img.SetRGBA(x, y, ink)
			}
		}
	}
	return img
}

func encodePNG(img image.Image) []byte {
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
