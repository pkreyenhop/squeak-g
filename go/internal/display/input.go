package display

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// pollInput reads the current mouse/keyboard state from Ebitengine and forwards
// it to the backend. Called once per Update (frame).
func (g *game) pollInput() {
	// Mouse: report position and current button bitmask every frame.
	mx, my := ebiten.CursorPosition()
	buttons := 0
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		buttons |= ButtonRed
	}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) {
		buttons |= ButtonYellow
	}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle) {
		buttons |= ButtonBlue
	}
	g.backend.Mouse(mx, my, buttons)

	// Keyboard: typed characters (runes).
	for _, r := range ebiten.AppendInputChars(nil) {
		g.backend.Key(true, 0, r)
	}
	// Raw key presses/releases (for non-character keys and modifiers).
	for _, k := range inpututil.AppendJustPressedKeys(nil) {
		g.backend.Key(true, int(k), 0)
	}
	for _, k := range inpututil.AppendJustReleasedKeys(nil) {
		g.backend.Key(false, int(k), 0)
	}
}
