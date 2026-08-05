package vm

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSqueak30BootsClean checks the 2001 Squeak 3.0 image (16-bit color, full
// class library with Balloon/FloatArray/Misc plugins) loads and boots to idle
// with no unhandled primitive failures / panics. It draws (BitBlt) but only
// composites its desktop to the Display in response to input, so we don't
// assert Display content here.
func TestSqueak30BootsClean(t *testing.T) {
	buf, err := os.ReadFile(filepath.Join("..", "..", "..", "images", "Squeak3.0.image"))
	if err != nil {
		t.Skipf("Squeak3.0.image not available: %v", err)
	}
	img, err := ReadImage("Squeak3.0.image", buf)
	if err != nil {
		t.Fatalf("ReadImage: %v", err)
	}
	interp, err := NewInterpreter(img)
	if err != nil {
		t.Fatalf("NewInterpreter: %v", err)
	}
	interp.SetScreenSize(800, 600)
	interp.Boot(60_000_000)
	total, _ := interp.BltStats()
	t.Logf("boot: %d bytecodes, idle=%v, bitblt=%d", interp.ByteCodeCount, interp.Idle(), total)
	if !interp.Idle() {
		t.Errorf("Squeak 3.0 did not reach idle within the boot budget")
	}
	if total == 0 {
		t.Errorf("Squeak 3.0 ran no BitBlt during boot")
	}
}
