package vm

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSqueak11BootsAndDraws boots the 1996 Squeak 1.1 image (Apple Smalltalk-80
// descendant) the way the GUI does and checks it renders its desktop.
func TestSqueak11BootsAndDraws(t *testing.T) {
	buf, err := os.ReadFile(filepath.Join("..", "..", "..", "images", "Squeak1.1.image"))
	if err != nil {
		t.Skipf("Squeak1.1.image not available: %v", err)
	}
	img, err := ReadImage("Squeak1.1.image", buf)
	if err != nil {
		t.Fatalf("ReadImage: %v", err)
	}
	interp, err := NewInterpreter(img)
	if err != nil {
		t.Fatalf("NewInterpreter: %v", err)
	}
	interp.SetScreenSize(1024, 768)
	interp.Boot(60_000_000)
	total, toDisplay := interp.BltStats()
	t.Logf("boot: %d bytecodes, idle=%v, bitblt %d (%d to Display)",
		interp.ByteCodeCount, interp.Idle(), total, toDisplay)
	if toDisplay == 0 {
		t.Errorf("Squeak 1.1 drew nothing to the Display during boot")
	}
}
