package vm

import (
	"fmt"
	"io"
	"sort"
)

// PrintDiagnostics writes a human-readable summary of a loaded image, used to
// verify the object model and loader before the interpreter exists.
func PrintDiagnostics(img *Image, w io.Writer) {
	endian := "big-endian"
	if img.LittleEndian {
		endian = "little-endian"
	}
	fmt.Fprintf(w, "image:        %s\n", img.Name)
	fmt.Fprintf(w, "version:      %d (%s%s)\n", img.Version, endian, closureNote(img))
	fmt.Fprintf(w, "old objects:  %d\n", img.OldSpaceCount)
	fmt.Fprintf(w, "old bytes:    %d\n", img.OldSpaceBytes)

	spl := img.SpecialObjectsArray
	fmt.Fprintf(w, "special objs: array of %d\n", len(spl.Pointers))
	printSpl := func(label string, idx int) {
		if o := img.SpecialObject(idx); o != nil {
			fmt.Fprintf(w, "  %-16s %s\n", label, o.SqInstName())
		}
	}
	printSpl("nil", SplObNilObject)
	printSpl("true", SplObTrueObject)
	printSpl("false", SplObFalseObject)
	if o := img.SpecialObject(SplObClassInteger); o != nil {
		fmt.Fprintf(w, "  %-16s %s\n", "SmallInteger", o.ClassName())
	}
	if o := img.SpecialObject(SplObClassString); o != nil {
		fmt.Fprintf(w, "  %-16s %s\n", "String class", o.ClassName())
	}
	if o := img.SpecialObject(SplObClassCompiledMethod); o != nil {
		fmt.Fprintf(w, "  %-16s %s\n", "CompiledMethod", o.ClassName())
	}

	// Histogram of instances by class name (top 20) — a good sanity check that
	// pointers were rectified and class names decode.
	counts := map[string]int{}
	methods := 0
	for obj := img.FirstOldObject; obj != nil; obj = obj.NextObject {
		if obj.IsMethod() {
			methods++
		}
		if obj.SqClass != nil {
			counts[obj.SqClass.ClassName()]++
		}
	}
	type kv struct {
		name  string
		count int
	}
	var sorted []kv
	for n, c := range counts {
		sorted = append(sorted, kv{n, c})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].count != sorted[j].count {
			return sorted[i].count > sorted[j].count
		}
		return sorted[i].name < sorted[j].name
	})
	fmt.Fprintf(w, "compiled methods: %d\n", methods)
	fmt.Fprintf(w, "instances by class (top 20 of %d classes):\n", len(sorted))
	for i, e := range sorted {
		if i >= 20 {
			break
		}
		fmt.Fprintf(w, "  %6d  %s\n", e.count, e.name)
	}
}

func closureNote(img *Image) string {
	if img.HasClosures {
		return ", closures"
	}
	return ""
}
