// Package display is the Ebitengine-based window/input shell for the VM.
//
// Ebitengine is used because Squeak's display is fundamentally a framebuffer:
// BitBlt renders into one bitmap and the VM only needs that RGBA buffer blitted
// to a window plus mouse/keyboard events fed back in. Ebitengine requires no
// cgo on Windows and macOS (it uses purego); Linux still needs cgo for X11.
package display

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// Buttons bitmask, matching Squeak's mouse button encoding.
const (
	ButtonRed    = 4 // left
	ButtonYellow = 2 // right / middle
	ButtonBlue   = 1 // link / meta
)

// Backend is implemented by the VM. The shell drives it once per display frame.
type Backend interface {
	// Title is the window title.
	Title() string
	// Frame returns the current display as an RGBA image. It may return a
	// different size if the image resized its display; the shell adapts.
	Frame() *image.RGBA
	// Step advances the VM by roughly one display frame's worth of work.
	// Input events pushed via Mouse/Key before Step become visible to it.
	Step()
	// Mouse reports the pointer position (in display pixels) and button bitmask.
	Mouse(x, y, buttons int)
	// Key reports a keyboard event. down is true for press, false for release;
	// r is the typed rune (0 if none), keyCode is a raw key identifier.
	Key(down bool, keyCode int, r rune)
}

// game adapts a Backend to ebiten.Game.
type game struct {
	backend Backend
	canvas  *ebiten.Image
	w, h    int
}

// Run opens the window and blocks until it is closed. Must be called on the
// main goroutine.
func Run(backend Backend) error {
	frame := backend.Frame()
	w, h := frame.Rect.Dx(), frame.Rect.Dy()
	g := &game{backend: backend, w: w, h: h, canvas: ebiten.NewImage(max1(w), max1(h))}
	ebiten.SetWindowSize(w, h)
	ebiten.SetWindowTitle(backend.Title())
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	return ebiten.RunGame(g)
}

func (g *game) Update() error {
	g.pollInput()
	g.backend.Step()
	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	frame := g.backend.Frame()
	w, h := frame.Rect.Dx(), frame.Rect.Dy()
	if w != g.w || h != g.h || g.canvas == nil {
		g.w, g.h = w, h
		g.canvas = ebiten.NewImage(max1(w), max1(h))
	}
	g.canvas.WritePixels(frame.Pix)
	screen.DrawImage(g.canvas, nil)
}

func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return g.w, g.h
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}
