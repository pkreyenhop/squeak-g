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
	// The bytecode loop is not ported yet; Interpret must say so rather than
	// pretend to run.
	if err := interp.Interpret(); err == nil {
		t.Errorf("Interpret() should report not-implemented until the loop is ported")
	}
}

func classNameOf(o *Object) string {
	if o == nil {
		return "<nil>"
	}
	return o.ClassName()
}
