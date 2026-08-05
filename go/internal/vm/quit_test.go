package vm

import "testing"

func TestQuitPrimitiveSetsFlag(t *testing.T) {
	img, _ := ReadImage("mini.image", miniImage(t))
	interp, err := NewInterpreter(img)
	if err != nil {
		t.Fatalf("NewInterpreter: %v", err)
	}
	if interp.Quitting() {
		t.Fatal("Quitting() true before any quit")
	}
	// SystemDictionary>>quitPrimitive is <primitive: 113>.
	if _, err := interp.Evaluate("Smalltalk quitPrimitive"); err != nil {
		t.Fatalf("evaluate quitPrimitive: %v", err)
	}
	if !interp.Quitting() {
		t.Fatal("Quitting() should be true after quitPrimitive")
	}
	t.Log("quitPrimitive set the quit flag")
}
