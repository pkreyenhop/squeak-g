package display

import (
	"image"
	"image/color"
)

// DemoBackend is a placeholder Backend used to exercise the window/input shell
// before the interpreter is wired in. It draws a gradient with a crosshair at
// the mouse position, tinted while a mouse button is held.
type DemoBackend struct {
	img       *image.RGBA
	mouseX    int
	mouseY    int
	buttons   int
	frame     int
	lastRunes []rune
}

// NewDemoBackend returns a demo backend of the given size.
func NewDemoBackend(w, h int) *DemoBackend {
	return &DemoBackend{img: image.NewRGBA(image.Rect(0, 0, w, h))}
}

func (d *DemoBackend) Title() string { return "Squeak-G (display shell demo — VM not yet wired)" }

func (d *DemoBackend) Frame() *image.RGBA { return d.img }

func (d *DemoBackend) Step() {
	d.frame++
	b := d.img.Bounds()
	tint := d.buttons != 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r := uint8((x + d.frame) & 0xFF)
			g := uint8((y - d.frame) & 0xFF)
			bl := uint8((x + y) & 0xFF)
			if tint {
				r, g = g, r
			}
			d.img.SetRGBA(x, y, color.RGBA{r, g, bl, 0xFF})
		}
	}
	// Crosshair at mouse.
	white := color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
	for x := b.Min.X; x < b.Max.X; x++ {
		d.img.SetRGBA(x, d.mouseY, white)
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		d.img.SetRGBA(d.mouseX, y, white)
	}
}

func (d *DemoBackend) Mouse(x, y, squeakButtons int) {
	d.mouseX, d.mouseY, d.buttons = x, y, squeakButtons
}

func (d *DemoBackend) Key(squeakKeyCode int) {
	d.lastRunes = append(d.lastRunes, rune(squeakKeyCode&0xFF))
}
