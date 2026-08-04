package main

import (
	"image"

	"squeakg/internal/vm"
)

// vmBackend drives a live Interpreter for the Ebitengine display shell: each
// frame it runs one UI cycle, renders the Squeak Display, and forwards input.
type vmBackend struct {
	interp *vm.Interpreter
	img    *vm.Image
	title  string
	frame  *image.RGBA
}

func newVMBackend(interp *vm.Interpreter, img *vm.Image, title string) *vmBackend {
	b := &vmBackend{interp: interp, img: img, title: title}
	b.render()
	return b
}

func (b *vmBackend) Title() string { return b.title }

func (b *vmBackend) Frame() *image.RGBA { return b.frame }

func (b *vmBackend) Step() {
	// Run until the image relinquishes the processor (polls the Sensor / goes
	// idle), capped so a busy image can't freeze the window.
	b.interp.Run(5_000_000)
	b.render()
}

func (b *vmBackend) render() {
	if rgba, _, err := b.img.RenderDisplay(); err == nil {
		b.frame = rgba
	} else if b.frame == nil {
		b.frame = image.NewRGBA(image.Rect(0, 0, 640, 480))
	}
}

func (b *vmBackend) Mouse(x, y, squeakButtons int) {
	b.interp.SetMouse(x, y, squeakButtons)
}

func (b *vmBackend) Key(squeakKeyCode int) {
	b.interp.PushKey(squeakKeyCode)
}
