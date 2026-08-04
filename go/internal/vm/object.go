package vm

import (
	"encoding/binary"
	"math"
	"strconv"
	"strings"
)

// Value is a Smalltalk object reference as stored in an instance variable or
// indexable slot. It is either an int (an immediate SmallInteger) or an
// *Object. This mirrors SqueakJS, where SmallIntegers are plain numbers and
// every other object is a Squeak.Object instance.
type Value = any

// Object is a Squeak object: the Go equivalent of a Squeak.Object instance.
// Exactly one of Pointers / Words / Bytes / (IsFloat+Float) carries the body,
// except CompiledMethods which have both Pointers (header+literals) and Bytes.
type Object struct {
	SqClass *Object // class (resolved during install)
	Hash    int     // identity hash
	Format  int     // Squeak format code 0..15 (the JS _format)

	Pointers []Value  // inst vars + indexable oops (formats 0..4, and method header+lits)
	Words    []uint32 // indexable words (format 6)
	Bytes    []byte   // indexable bytes (formats 8..11, and method bytecodes)

	IsFloat bool
	Float   float64

	Oop        int     // identifies this object in a snapshot
	Mark       bool    // GC mark
	Dirty      bool    // may reference a new object
	NextObject *Object // linked list through old/young space

	// Decorations of well-known singletons, set by decorateKnownObjects.
	IsNil        bool
	IsTrue       bool
	IsFalse      bool
	IsFloatClass bool
	IsCompact    bool

	// classInt holds the raw class field (compact index or old address) between
	// initFromImage and installFromImage. Not used after install.
	classInt uint32
}

// --- instantiation -------------------------------------------------------

// initInstanceOf initializes a freshly allocated instance of aClass with the
// given number of indexable slots, mirroring Squeak.Object>>initInstanceOf.
func (o *Object) initInstanceOf(aClass *Object, indexableSize, hash int, nilObj *Object) {
	o.SqClass = aClass
	o.Hash = hash
	instSpec := aClass.asInt(aClass.Pointers[ClassFormat])
	instSize := ((instSpec >> 1) & 0x3F) + ((instSpec >> 10) & 0xC0) - 1
	o.Format = (instSpec >> 7) & 0xF

	if o.Format < 8 {
		if o.Format != 6 {
			if instSize+indexableSize > 0 {
				o.Pointers = fillPointers(instSize+indexableSize, nilObj)
			}
		} else { // Words
			if indexableSize > 0 {
				if aClass.IsFloatClass {
					o.IsFloat = true
					o.Float = 0.0
				} else {
					o.Words = make([]uint32, indexableSize)
				}
			}
		}
	} else { // Bytes
		if indexableSize > 0 {
			o.Bytes = make([]byte, indexableSize)
		}
	}
}

// initAsClone copies another object's body (Squeak.Object>>initAsClone).
func (o *Object) initAsClone(original *Object, hash int) {
	o.SqClass = original.SqClass
	o.Hash = hash
	o.Format = original.Format
	if original.IsFloat {
		o.IsFloat = true
		o.Float = original.Float
		return
	}
	if original.Pointers != nil {
		o.Pointers = append([]Value(nil), original.Pointers...)
	}
	if original.Words != nil {
		o.Words = append([]uint32(nil), original.Words...)
	}
	if original.Bytes != nil {
		o.Bytes = append([]byte(nil), original.Bytes...)
	}
}

func fillPointers(length int, filler *Object) []Value {
	a := make([]Value, length)
	for i := range a {
		a[i] = filler
	}
	return a
}

// --- loading -------------------------------------------------------------

// initFromImage records the unmapped header data read from the image.
func (o *Object) initFromImage(oop int, cls uint32, fmt, hash int) {
	o.Oop = oop
	o.classInt = cls
	o.Format = fmt
	o.Hash = hash
}

func readU32(b []byte, wordIndex int, littleEndian bool) uint32 {
	off := wordIndex * 4
	if littleEndian {
		return binary.LittleEndian.Uint32(b[off:])
	}
	return binary.BigEndian.Uint32(b[off:])
}

// installFromImage decodes this object's body from raw bits and rectifies
// pointers via oopMap. Classic (non-Spur) path only.
func (o *Object) installFromImage(oopMap map[uint32]*Object, bits []byte, ccArray []*Object, floatClass *Object, littleEndian bool) {
	// map compact classes
	if o.classInt > 0 && o.classInt < 32 {
		o.SqClass = ccArray[o.classInt-1]
	} else {
		o.SqClass = oopMap[o.classInt]
	}
	nWords := len(bits) / 4
	switch {
	case o.Format < 5:
		// Formats 0..4 -- pointer fields
		if nWords > 0 {
			o.Pointers = o.decodePointers(nWords, bits, oopMap, littleEndian)
		}
	case o.Format >= 12:
		// Formats 12..15 -- CompiledMethods, both pointers and bits
		methodHeader := readU32(bits, 0, littleEndian)
		numLits := int((methodHeader >> 10) & 255)
		o.Pointers = o.decodePointers(numLits+1, bits, oopMap, littleEndian) // header+lits
		o.Bytes = o.decodeBytes(nWords-(numLits+1), bits, numLits+1, o.Format&3)
	case o.Format >= 8:
		// Formats 8..11 -- ByteArrays and ByteStrings
		if nWords > 0 {
			o.Bytes = o.decodeBytes(nWords, bits, 0, o.Format&3)
		}
	case o.SqClass == floatClass:
		o.IsFloat = true
		o.Float = o.decodeFloat(bits, littleEndian)
	default:
		// format 6 -- indexable words
		if nWords > 0 {
			o.Words = o.decodeWords(nWords, bits, littleEndian)
		}
	}
	o.Mark = false
}

func (o *Object) decodePointers(nWords int, bits []byte, oopMap map[uint32]*Object, littleEndian bool) []Value {
	ptrs := make([]Value, nWords)
	for i := 0; i < nWords; i++ {
		oop := readU32(bits, i, littleEndian)
		if oop&1 == 1 { // SmallInteger
			ptrs[i] = int(int32(oop) >> 1)
		} else { // Object
			if obj := oopMap[oop]; obj != nil {
				ptrs[i] = obj
			} else {
				// Garbage beyond a context's stack pointer can reference a
				// missing oop; fill an arbitrary SmallInteger (never accessed).
				ptrs[i] = 42424242
			}
		}
	}
	return ptrs
}

func (o *Object) decodeWords(nWords int, bits []byte, littleEndian bool) []uint32 {
	words := make([]uint32, nWords)
	for i := 0; i < nWords; i++ {
		words[i] = readU32(bits, i, littleEndian)
	}
	return words
}

// decodeBytes copies indexable bytes in file order; fmtLowBits (0..3) trims the
// trailing pad bytes of the last word.
func (o *Object) decodeBytes(nWords int, bits []byte, wordOffset, fmtLowBits int) []byte {
	nBytes := nWords*4 - fmtLowBits
	if nBytes < 0 {
		nBytes = 0
	}
	out := make([]byte, nBytes)
	copy(out, bits[wordOffset*4:wordOffset*4+nBytes])
	return out
}

func (o *Object) decodeFloat(bits []byte, littleEndian bool) float64 {
	if !littleEndian {
		return math.Float64frombits(binary.BigEndian.Uint64(bits))
	}
	// Little-endian image: 64-bit floats are stored as two 32-bit words in
	// big-endian-of-words order; swap the words then read native little-endian.
	lo := binary.LittleEndian.Uint32(bits[0:])
	hi := binary.LittleEndian.Uint32(bits[4:])
	return math.Float64frombits(uint64(lo)<<32 | uint64(hi))
}

// --- testing -------------------------------------------------------------

func (o *Object) IsWords() bool        { return o.Format == 6 }
func (o *Object) IsBytes() bool        { return o.Format >= 8 && o.Format <= 11 }
func (o *Object) IsWordsOrBytes() bool { return o.Format == 6 || (o.Format >= 8 && o.Format <= 11) }
func (o *Object) IsPointers() bool     { return o.Format <= 4 }
func (o *Object) IsWeak() bool         { return o.Format == 4 }
func (o *Object) IsMethod() bool       { return o.Format >= 12 }

func sameFormats(a, b int) bool {
	if a < 8 {
		return a == b
	}
	return (a & 0xC) == (b & 0xC)
}

// SameFormatAs reports whether two objects have compatible storage formats.
func (o *Object) SameFormatAs(other *Object) bool { return sameFormats(o.Format, other.Format) }

// IndexableSize returns the number of indexable slots (or -1 if not indexable).
// allowBeyondSP is true for old-primitive images (contexts report full size).
func (o *Object) IndexableSize(allowBeyondSP, isContext bool) int {
	fmt := o.Format
	if fmt < 2 {
		return -1 // not indexable
	}
	if fmt == 3 && isContext && !allowBeyondSP {
		return asIntValue(o.Pointers[ContextStackPointer])
	}
	if fmt < 6 {
		return o.PointersSize() - o.InstSize()
	}
	if fmt < 8 {
		return o.WordsSize()
	}
	if fmt < 12 {
		return o.BytesSize()
	}
	return o.BytesSize() + 4*o.PointersSize()
}

// --- accessing -----------------------------------------------------------

func (o *Object) PointersSize() int {
	return len(o.Pointers)
}

func (o *Object) BytesSize() int {
	return len(o.Bytes)
}

func (o *Object) WordsSize() int {
	if o.IsFloat {
		return 2
	}
	return len(o.Words)
}

// InstSize returns the number of named instance variables (from the format,
// falling back to the class for indexable-pointer objects).
func (o *Object) InstSize() int {
	fmt := o.Format
	if fmt > 4 || fmt == 2 {
		return 0 // indexable fields only
	}
	if fmt < 2 {
		return o.PointersSize() // fixed fields only
	}
	return o.SqClass.classInstSize()
}

// --- as class ------------------------------------------------------------

func (o *Object) classInstFormat() int {
	return int((o.asInt(o.Pointers[ClassFormat]) >> 7) & 0xF)
}

func (o *Object) classInstSize() int {
	spec := o.asInt(o.Pointers[ClassFormat])
	return ((spec >> 10) & 0xC0) + ((spec >> 1) & 0x3F) - 1
}

func (o *Object) asInt(v Value) int {
	if n, ok := v.(int); ok {
		return n
	}
	return 0
}

func (o *Object) Superclass() *Object {
	if len(o.Pointers) == 0 {
		return nil
	}
	if s, ok := o.Pointers[ClassSuperclass].(*Object); ok {
		return s
	}
	return nil
}

// InstVarNames returns this class's own instance-variable names.
// The index moved from 4 to 3 in newer images, so both are tried.
func (o *Object) InstVarNames() []string {
	for index := 3; index <= 4; index++ {
		if index >= len(o.Pointers) {
			continue
		}
		vn, ok := o.Pointers[index].(*Object)
		if !ok || vn.Pointers == nil || len(vn.Pointers) == 0 {
			continue
		}
		if first, ok := vn.Pointers[0].(*Object); ok && first.Bytes != nil {
			names := make([]string, len(vn.Pointers))
			for i, each := range vn.Pointers {
				if eo, ok := each.(*Object); ok {
					names[i] = eo.BytesAsString()
				}
			}
			return names
		}
	}
	return nil
}

func (o *Object) AllInstVarNames() []string {
	super := o.Superclass()
	if super == nil || super.IsNil {
		return o.InstVarNames()
	}
	return append(super.AllInstVarNames(), o.InstVarNames()...)
}

// ClassName returns the name of this class object (or "X class" for metaclasses).
func (o *Object) ClassName() string {
	if o.Pointers == nil {
		return "_NOTACLASS_"
	}
	for nameIdx := 6; nameIdx <= 7; nameIdx++ {
		if nameIdx >= len(o.Pointers) {
			break
		}
		if name, ok := o.Pointers[nameIdx].(*Object); ok && name.Bytes != nil {
			return name.BytesAsString()
		}
	}
	// must be a metaclass: find its "thisClass" and append " class"
	for clsIndex := 5; clsIndex <= 6; clsIndex++ {
		if clsIndex >= len(o.Pointers) {
			break
		}
		cls, ok := o.Pointers[clsIndex].(*Object)
		if !ok || cls.Pointers == nil {
			continue
		}
		for nameIdx := 6; nameIdx <= 7; nameIdx++ {
			if nameIdx >= len(cls.Pointers) {
				break
			}
			if name, ok := cls.Pointers[nameIdx].(*Object); ok && name.Bytes != nil {
				return name.BytesAsString() + " class"
			}
		}
	}
	return "_SOMECLASS_"
}

// --- as method -----------------------------------------------------------

func (o *Object) methodHeader() int  { return o.asInt(o.Pointers[0]) }
func (o *Object) MethodNumLits() int { return (o.methodHeader() >> 9) & 0xFF }
func (o *Object) MethodNumArgs() int { return (o.methodHeader() >> 24) & 0xF }

func (o *Object) MethodPrimitiveIndex() int {
	primBits := o.methodHeader() & 0x300001FF
	if primBits > 0x1FF {
		return (primBits & 0x1FF) + (primBits >> 19)
	}
	return primBits
}

func (o *Object) MethodTempCount() int        { return (o.methodHeader() >> 18) & 63 }
func (o *Object) MethodNeedsLargeFrame() bool { return o.methodHeader()&0x20000 > 0 }

func (o *Object) MethodClassForSuper() *Object {
	assn := o.Pointers[o.MethodNumLits()].(*Object)
	if v, ok := assn.Pointers[AssnValue].(*Object); ok {
		return v
	}
	return nil
}

func (o *Object) MethodGetLiteral(zeroBasedIndex int) Value {
	return o.Pointers[1+zeroBasedIndex] // step over header
}

// --- as context ----------------------------------------------------------

func (o *Object) ContextIsBlock() bool {
	if BlockContextArgumentCount >= len(o.Pointers) {
		return false
	}
	_, isInt := o.Pointers[BlockContextArgumentCount].(int)
	return isInt
}

func (o *Object) ContextHome() *Object {
	if o.ContextIsBlock() {
		if h, ok := o.Pointers[BlockContextHome].(*Object); ok {
			return h
		}
		return nil
	}
	return o
}

func (o *Object) ContextMethod() *Object {
	home := o.ContextHome()
	if home == nil {
		return nil
	}
	if m, ok := home.Pointers[ContextMethod].(*Object); ok {
		return m
	}
	return nil
}

// --- printing ------------------------------------------------------------

func (o *Object) BytesAsString() string {
	if o.Bytes == nil {
		return ""
	}
	return string(o.Bytes)
}

// SqInstName returns a short human-readable description, for diagnostics.
func (o *Object) SqInstName() string {
	switch {
	case o.IsNil:
		return "nil"
	case o.IsTrue:
		return "true"
	case o.IsFalse:
		return "false"
	case o.IsFloat:
		return floatToString(o.Float)
	}
	if o.SqClass == nil {
		return "a ???"
	}
	className := o.SqClass.ClassName()
	if strings.Contains(className, " ") {
		return "the " + className
	}
	switch className {
	case "String", "ByteString":
		return "'" + o.BytesAsString() + "'"
	case "Symbol", "ByteSymbol":
		return "#" + o.BytesAsString()
	}
	if len(className) > 0 && strings.ContainsRune("AEIOUaeiou", rune(className[0])) {
		return "an" + className
	}
	return "a" + className
}

func floatToString(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsRune(s, '.') && !strings.ContainsAny(s, "eEnN") {
		s += ".0"
	}
	return s
}
