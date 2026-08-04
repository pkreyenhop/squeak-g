package display

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Squeak button/modifier bits (from vm.input.js).
const (
	mouseBlue   = 1 // right
	mouseYellow = 2 // middle
	mouseRed    = 4 // left
	modShift    = 8
	modCtrl     = 16
	modOption   = 32
	modCmd      = 64
)

// pollInput reads Ebitengine input, encodes it in Squeak's conventions, and
// forwards it to the backend once per frame.
func (g *game) pollInput() {
	mods := currentModifiers()

	// Mouse: position (clamped to the display) plus buttons OR modifiers.
	mx, my := ebiten.CursorPosition()
	if mx < 0 {
		mx = 0
	} else if mx >= g.w {
		mx = g.w - 1
	}
	if my < 0 {
		my = 0
	} else if my >= g.h {
		my = g.h - 1
	}
	buttons := 0
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		buttons |= mouseRed
	}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle) {
		buttons |= mouseYellow
	}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) {
		buttons |= mouseBlue
	}
	g.backend.Mouse(mx, my, buttons|mods)

	// Keyboard: typed characters first, then special keys. Squeak keycode is
	// (modifiers>>3)<<8 | charCode.
	modByte := mods >> 3

	// Cmd/Alt-P is bound to "do it": inject the keystroke the image recognizes
	// as do-it (Cmd-d) and swallow the raw P for this frame.
	doIt := (mods&(modCmd|modOption)) != 0 && inpututil.IsKeyJustPressed(ebiten.KeyP)
	if doIt {
		g.backend.Key((modCmd>>3)<<8 | doItChar)
	}

	for _, r := range ebiten.AppendInputChars(nil) {
		if doIt {
			continue
		}
		g.backend.Key(modByte<<8 | int(r))
	}
	for _, k := range inpututil.AppendJustPressedKeys(nil) {
		if doIt && k == ebiten.KeyP {
			continue
		}
		if code, ok := specialKey(k); ok {
			g.backend.Key(modByte<<8 | code)
		}
	}
}

// doItChar is the character the Squeak editor maps to "do it" (Cmd-d).
const doItChar = 'd'

func currentModifiers() int {
	m := 0
	if ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight) {
		m |= modShift
	}
	if ebiten.IsKeyPressed(ebiten.KeyControlLeft) || ebiten.IsKeyPressed(ebiten.KeyControlRight) {
		m |= modCtrl
	}
	if ebiten.IsKeyPressed(ebiten.KeyAltLeft) || ebiten.IsKeyPressed(ebiten.KeyAltRight) {
		m |= modOption
	}
	if ebiten.IsKeyPressed(ebiten.KeyMetaLeft) || ebiten.IsKeyPressed(ebiten.KeyMetaRight) {
		m |= modCmd
	}
	return m
}

// specialKey maps non-character keys to Squeak/Mac key codes. Returns false for
// keys handled via AppendInputChars (letters, digits, punctuation).
func specialKey(k ebiten.Key) (int, bool) {
	switch k {
	case ebiten.KeyBackspace:
		return 8, true
	case ebiten.KeyTab:
		return 9, true
	case ebiten.KeyEnter, ebiten.KeyNumpadEnter:
		return 13, true
	case ebiten.KeyEscape:
		return 27, true
	case ebiten.KeyDelete:
		return 127, true
	case ebiten.KeyHome:
		return 1, true
	case ebiten.KeyEnd:
		return 4, true
	case ebiten.KeyPageUp:
		return 11, true
	case ebiten.KeyPageDown:
		return 12, true
	case ebiten.KeyLeft:
		return 28, true
	case ebiten.KeyRight:
		return 29, true
	case ebiten.KeyUp:
		return 30, true
	case ebiten.KeyDown:
		return 31, true
	}
	return 0, false
}
