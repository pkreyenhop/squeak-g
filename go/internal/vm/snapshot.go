package vm

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

// primitiveSnapshot implements primitive 97 (SystemDictionary>>snapshot): write
// the current object memory to the image file. Like the real VM it "returns
// twice" — true when a saved image is later resumed, false to continue now.
func (p *Primitives) primitiveSnapshot(argCount int) bool {
	p.vm.popNandPush(1, p.vm.TrueObj) // seen by a resumed saved image
	buf := p.vm.SnapshotBuffer()
	name := p.vm.Image.Name
	if err := os.WriteFile(name, buf, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "snapshot: could not write "+name+":", err)
	} else {
		fmt.Printf("snapshot: saved image to %s (%d bytes)\n", name, len(buf))
	}

	p.vm.popNandPush(1, p.vm.FalseObj) // continue the running image
	return true
}

// Image snapshot: write the object memory back out in the classic (non-Spur)
// image format. This is the inverse of the loader in image.go / object.go.

// SnapshotBuffer saves the interpreter's current execution state into the
// active process and serializes the whole object memory to a classic-format
// image buffer (does not touch the filesystem).
func (vm *Interpreter) SnapshotBuffer() []byte {
	vm.storeContextRegisters()
	vm.prim.activeProcess().Pointers[ProcSuspendedContext] = vm.ActiveContext
	return vm.Image.WriteToBuffer()
}

// snapshotSize returns the number of extra header words and body words (body
// includes the always-present base header word) this object occupies.
func (o *Object) snapshotSize() (header, body int) {
	var nWords int
	switch {
	case o.IsFloat:
		nWords = 2
	case o.Words != nil:
		nWords = len(o.Words)
	case o.Pointers != nil:
		nWords = len(o.Pointers)
	}
	if o.Bytes != nil {
		nWords += (len(o.Bytes) + 3) >> 2
	}
	nWords++ // base header word
	switch {
	case nWords > 63:
		header = 2
	case o.SqClass != nil && o.SqClass.IsCompact:
		header = 0
	default:
		header = 1
	}
	return header, nWords
}

func (o *Object) totalBytes() int {
	h, b := o.snapshotSize()
	return (h + b) * 4
}

func (o *Object) addr() int {
	h, _ := o.snapshotSize()
	return o.Oop - h*4
}

func (img *Image) objectToOop(v Value) uint32 {
	if n, ok := v.(int); ok {
		return uint32(n)<<1 | 1 // tag SmallInteger
	}
	o := v.(*Object)
	if o.Oop < 0 {
		panic("temporary oop during snapshot")
	}
	return uint32(o.Oop)
}

func (img *Image) compactClassIndex(cls *Object) int {
	for i, c := range img.CompactClasses {
		if c == cls {
			return i + 1
		}
	}
	return 0
}

// writeObject writes this object's 1-3 header words then its body into buf at
// pos, returning the new position (Squeak.Object>>writeTo).
func (o *Object) writeObject(buf []byte, pos int, img *Image) int {
	if o.Bytes != nil {
		o.Format |= (-len(o.Bytes)) & 3 // encode byte padding in low format bits
	}
	before := pos
	header, body := o.snapshotSize()
	formatAndHash := uint32((o.Format&15)<<8) | uint32((o.Hash&4095)<<17)
	put := func(w uint32) {
		binary.BigEndian.PutUint32(buf[pos:], w)
		pos += 4
	}
	switch header {
	case 2:
		put(uint32(body)<<2 | HeaderTypeSizeAndClass)
		put(uint32(o.SqClass.Oop) | HeaderTypeSizeAndClass)
		put(formatAndHash | HeaderTypeSizeAndClass)
	case 1:
		put(uint32(o.SqClass.Oop) | HeaderTypeClass)
		put(formatAndHash | uint32(body)<<2 | HeaderTypeClass)
	case 0:
		classIndex := img.compactClassIndex(o.SqClass)
		put(formatAndHash | uint32(classIndex)<<12 | uint32(body)<<2 | HeaderTypeShort)
	}
	switch {
	case o.IsFloat:
		binary.BigEndian.PutUint64(buf[pos:], math.Float64bits(o.Float))
		pos += 8
	case o.Words != nil:
		for _, w := range o.Words {
			put(w)
		}
	case o.Pointers != nil:
		for _, p := range o.Pointers {
			put(img.objectToOop(p))
		}
	}
	if o.Bytes != nil {
		copy(buf[pos:], o.Bytes)
		pos += len(o.Bytes)
		pos += (-len(o.Bytes)) & 3 // pad to word
	}
	if pos != before+o.totalBytes() {
		panic("written size does not match")
	}
	return pos
}

// assignAddresses walks every object reachable from the special objects array
// (which reaches nil/true/false, all classes, the scheduler and its processes/
// contexts, and the Smalltalk globals), assigns each a snapshot address, and
// relinks them into the old-space list in that order. Replaces the JS full GC.
func (img *Image) assignAddresses() []*Object {
	visited := make(map[*Object]bool, img.OldSpaceCount+1024)
	order := make([]*Object, 0, img.OldSpaceCount+1024)
	stack := make([]*Object, 0, 1024)
	visit := func(v Value) {
		if o, ok := v.(*Object); ok && o != nil && !visited[o] {
			visited[o] = true
			order = append(order, o)
			stack = append(stack, o)
		}
	}
	visit(img.SpecialObjectsArray)
	for len(stack) > 0 {
		o := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		visit(o.SqClass)
		for _, p := range o.Pointers {
			visit(p)
		}
	}
	addr := 0
	for _, o := range order {
		h, _ := o.snapshotSize()
		o.Oop = addr + h*4
		addr += o.totalBytes()
	}
	for i, o := range order {
		if i+1 < len(order) {
			o.NextObject = order[i+1]
		} else {
			o.NextObject = nil
		}
	}
	if len(order) > 0 {
		img.FirstOldObject = order[0]
	}
	img.OldSpaceBytes = addr
	img.OldSpaceCount = len(order)
	return order
}

// WriteToBuffer serializes the whole object memory to a classic-format image.
func (img *Image) WriteToBuffer() []byte {
	order := img.assignAddresses()
	const headerSize = 64
	buf := make([]byte, headerSize+img.OldSpaceBytes)
	pos := 0
	put := func(w uint32) {
		binary.BigEndian.PutUint32(buf[pos:], w)
		pos += 4
	}
	put(6502) // classic 32-bit big-endian format version
	put(headerSize)
	put(uint32(img.OldSpaceBytes))
	put(uint32(img.FirstOldObject.addr()))
	put(img.objectToOop(img.SpecialObjectsArray))
	put(img.lastHash)
	put((800 << 16) + 600) // saved window size (unused on load)
	for pos < headerSize {
		put(0)
	}
	for _, o := range order {
		pos = o.writeObject(buf, pos, img)
	}
	if pos != len(buf) {
		panic(fmt.Sprintf("wrong image size: wrote %d, expected %d", pos, len(buf)))
	}
	return buf
}
