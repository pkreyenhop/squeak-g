package vm

import (
	"os"
	"path/filepath"
	"testing"
)

// miniImage locates the classic-format demo image shipped in the repo.
func miniImage(t *testing.T) []byte {
	t.Helper()
	// test runs in internal/vm; the image is at ../../../demo/mini.image
	path := filepath.Join("..", "..", "..", "demo", "mini.image")
	buf, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("mini.image not available: %v", err)
	}
	return buf
}

func TestLoadMiniImage(t *testing.T) {
	img, err := ReadImage("mini.image", miniImage(t))
	if err != nil {
		t.Fatalf("ReadImage: %v", err)
	}
	if img.Version != 6502 {
		t.Errorf("version = %d, want 6502", img.Version)
	}
	if img.LittleEndian {
		t.Errorf("mini.image should be big-endian")
	}
	if img.OldSpaceCount < 15000 || img.OldSpaceCount > 17000 {
		t.Errorf("OldSpaceCount = %d, want ~15893", img.OldSpaceCount)
	}

	// Well-known singletons decode correctly.
	if nilObj := img.SpecialObject(SplObNilObject); nilObj == nil || !nilObj.IsNil {
		t.Errorf("nil object not marked")
	}
	if s := img.SpecialObject(SplObClassString); s == nil || s.ClassName() != "String" {
		t.Errorf("String class name = %q, want String", classNameOf(s))
	}
	if cm := img.SpecialObject(SplObClassCompiledMethod); cm == nil || cm.ClassName() != "CompiledMethod" {
		t.Errorf("CompiledMethod class name wrong")
	}
}

func TestInitialContext(t *testing.T) {
	img, err := ReadImage("mini.image", miniImage(t))
	if err != nil {
		t.Fatalf("ReadImage: %v", err)
	}
	interp, err := NewInterpreter(img)
	if err != nil {
		t.Fatalf("NewInterpreter: %v", err)
	}
	if interp.Method == nil || !interp.Method.IsMethod() {
		t.Fatalf("initial method not a CompiledMethod")
	}
	if interp.Receiver == nil {
		t.Fatalf("initial receiver is nil pointer")
	}
	// Run a bounded number of bytecodes and confirm the interpreter makes
	// progress (executes sends) without panicking.
	interp.Run(200000)
	if interp.ByteCodeCount < 100000 {
		t.Errorf("expected the interpreter to execute bytecodes, got %d", interp.ByteCodeCount)
	}
	if interp.SendCount == 0 {
		t.Errorf("expected some message sends during execution")
	}
}

func TestBootAndRenderDisplay(t *testing.T) {
	img, err := ReadImage("mini.image", miniImage(t))
	if err != nil {
		t.Fatalf("ReadImage: %v", err)
	}
	interp, err := NewInterpreter(img)
	if err != nil {
		t.Fatalf("NewInterpreter: %v", err)
	}
	interp.Run(0) // run until idle
	total, toDisplay := interp.BltStats()
	if toDisplay == 0 {
		t.Errorf("expected BitBlts to the Display during boot (total=%d)", total)
	}
	rgba, info, err := img.RenderDisplay()
	if err != nil {
		t.Fatalf("RenderDisplay: %v", err)
	}
	if info.Width != 640 || info.Height != 480 {
		t.Errorf("display size = %dx%d, want 640x480", info.Width, info.Height)
	}
	// The booted desktop must not be entirely blank.
	nonWhite := 0
	for i := 0; i+3 < len(rgba.Pix); i += 4 {
		if rgba.Pix[i] != 0xFF || rgba.Pix[i+1] != 0xFF || rgba.Pix[i+2] != 0xFF {
			nonWhite++
		}
	}
	if nonWhite < 1000 {
		t.Errorf("expected drawn content on the desktop, only %d non-white pixels", nonWhite)
	}
}

func classNameOf(o *Object) string {
	if o == nil {
		return "<nil>"
	}
	return o.ClassName()
}
