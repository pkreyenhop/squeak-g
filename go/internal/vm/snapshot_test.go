package vm

import "testing"

func TestSnapshotPersistsChange(t *testing.T) {
	img, err := ReadImage("mini.image", miniImage(t))
	if err != nil {
		t.Fatalf("ReadImage: %v", err)
	}
	interp, err := NewInterpreter(img)
	if err != nil {
		t.Fatalf("NewInterpreter: %v", err)
	}
	// Make a persistent change, then snapshot to a buffer.
	if _, err := interp.Evaluate("Smalltalk at: #MySaveMarker put: 12345"); err != nil {
		t.Fatalf("evaluate change: %v", err)
	}
	buf := interp.SnapshotBuffer()
	t.Logf("snapshot: %d bytes, %d objects", len(buf), img.OldSpaceCount)

	// Reload the saved image and confirm the change is present.
	img2, err := ReadImage("saved.image", buf)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	interp2, err := NewInterpreter(img2)
	if err != nil {
		t.Fatalf("interpret reloaded: %v", err)
	}
	got, err := interp2.PrintIt("MySaveMarker")
	if err != nil {
		t.Fatalf("read marker from reloaded image: %v", err)
	}
	if got != "12345" {
		t.Fatalf("MySaveMarker = %q in reloaded image, want 12345", got)
	}
	t.Logf("change persisted across save/reload: MySaveMarker = %s", got)
}
