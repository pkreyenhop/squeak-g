package vm

import "testing"

func TestEvalPrintIt(t *testing.T) {
	img, err := ReadImage("mini.image", miniImage(t))
	if err != nil {
		t.Fatalf("ReadImage: %v", err)
	}
	interp, err := NewInterpreter(img)
	if err != nil {
		t.Fatalf("NewInterpreter: %v", err)
	}
	for _, expr := range []string{
		"3 + 4",
		"(3 + 4) * 2",
		"100 factorial",
		"'hello' reversed",
		"3 > 4",
		"$A asInteger",
		"#(1 2 3 4) inject: 0 into: [:a :b | a + b]",
	} {
		out, err := interp.PrintIt(expr)
		if err != nil {
			t.Errorf("%s => error %v", expr, err)
			continue
		}
		t.Logf("%s => %s", expr, out)
	}
}
