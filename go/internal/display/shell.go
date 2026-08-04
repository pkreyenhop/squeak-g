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

// Backend is implemented by the VM. The shell drives it once per display frame.
// Input is delivered already encoded in Squeak's conventions (see input.go).
type Backend interface {
	// Title is the window title.
	Title() string
	// Frame returns the current display as an RGBA image. It may return a
	// different size if the image resized its display; the shell adapts.
	Frame() *image.RGBA
	// Step advances the VM by roughly one display frame's worth of work.
	Step()
	// Mouse reports the pointer position (in display pixels) and the full Squeak
	// buttons word (mouse bits OR keyboard-modifier bits).
	Mouse(x, y, squeakButtons int)
	// Key enqueues a Squeak keyboard code (modifiers<<8 | charCode).
	Key(squeakKeyCode int)
}

// HostScreenSizer is an optional Backend extension: if implemented, the shell
// MonitorSize returns the primary monitor's pixel size. Callable before Run so
// the VM can boot the Squeak display at full resolution. Returns 0,0 if unknown.
func MonitorSize() (int, int) {
	if m := ebiten.Monitor(); m != nil {
		return m.Size()
	}
	return 0, 0
}

type game struct {
	backend Backend
	canvas  *ebiten.Image
	w, h    int
}

// Run opens the window and blocks until it is closed. Must be called on the
// main goroutine. If fullscreen is true the window covers the whole monitor.
// scale (>=1) magnifies the Squeak display with crisp nearest-neighbor pixels,
// so a scale of 2 makes fonts and everything else twice as big.
func Run(backend Backend, fullscreen bool, scale int) error {
	if scale < 1 {
		scale = 1
	}
	frame := backend.Frame()
	w, h := frame.Rect.Dx(), frame.Rect.Dy()
	g := &game{backend: backend, w: w, h: h, canvas: ebiten.NewImage(max1(w), max1(h))}
	if fullscreen {
		ebiten.SetFullscreen(true)
	}
	// The window is scale times the logical display; Ebitengine scales content
	// up to fill it (Layout returns the logical size).
	ebiten.SetWindowSize(w*scale, h*scale)
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
