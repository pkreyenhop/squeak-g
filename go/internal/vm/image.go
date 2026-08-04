package vm

import (
	"encoding/binary"
	"fmt"
)

// Image is a loaded Squeak object memory (the Go equivalent of Squeak.Image).
// Only the classic (non-Spur) 32-bit format is supported so far.
type Image struct {
	Name         string
	Version      uint32
	IsSpur       bool
	HasClosures  bool
	LittleEndian bool

	OldBaseAddr         uint32
	SpecialObjectsArray *Object
	FirstOldObject      *Object
	LastOldObject       *Object

	OldSpaceCount  int
	OldSpaceBytes  int
	HeaderFlags    uint32
	CompactClasses []*Object // 31 entries; may hold nil for unused slots

	// Allocation state (Squeak.Image): new objects get negative oops and a
	// hash derived from lastHash. No explicit GC — Go's collector reclaims
	// unreachable Squeak objects, and contexts are recycled by the interpreter.
	newSpaceCount int
	lastHash      uint32
	nilObj        *Object

	// character cache for getCharacter (classic images: Character instances).
	characterTable map[int]*Object
	characterClass *Object
}

// classic base-version identification (from vm.image.js).
var baseVersions = map[uint32]bool{6501: true, 6502: true, 6504: true, 68000: true, 68002: true, 68004: true}

const baseVersionMask = 0x119EE

// reader is a big/little-endian word cursor over the image bytes.
type reader struct {
	buf []byte
	pos int
	le  bool
}

func (r *reader) u32() uint32 {
	var v uint32
	if r.le {
		v = binary.LittleEndian.Uint32(r.buf[r.pos:])
	} else {
		v = binary.BigEndian.Uint32(r.buf[r.pos:])
	}
	r.pos += 4
	return v
}

// ReadImage parses a Squeak image from raw bytes.
func ReadImage(name string, buf []byte) (*Image, error) {
	img := &Image{Name: name}
	r := &reader{buf: buf}

	// Determine endianness and file-header offset by probing known versions.
	fileHeaderSize := 0
	var version uint32
	le := false
	for {
		le = !le
		r.le = le
		r.pos = fileHeaderSize
		if r.pos+4 > len(buf) {
			return nil, fmt.Errorf("bad image version: file too small")
		}
		version = r.u32()
		if baseVersions[version&baseVersionMask] {
			break
		}
		if !le {
			fileHeaderSize += 512
		}
		if fileHeaderSize > 512 {
			return nil, fmt.Errorf("bad image version: tried all endian/header combos")
		}
	}
	img.Version = version
	img.LittleEndian = le
	img.HasClosures = !(version == 6501 || version == 6502 || version == 68000)
	img.IsSpur = version&16 != 0
	is64Bit := version >= 68000

	if img.IsSpur {
		return nil, fmt.Errorf("Spur image format (version %d) not yet supported by the Go port; classic 32-bit only", version)
	}
	if is64Bit {
		return nil, fmt.Errorf("64-bit images (version %d) not yet supported by the Go port", version)
	}

	// Image header (classic 32-bit: all words are 32 bits).
	imageHeaderSize := int(r.u32())
	objectMemorySize := int(r.u32())
	oldBaseAddr := r.u32()
	specialObjectsOopInt := r.u32()
	img.lastHash = r.u32() // lastHash, seeds the allocation hash sequence
	_ = r.u32()            // savedWindowSize
	img.HeaderFlags = r.u32()
	for i := 0; i < 4; i++ {
		_ = r.u32() // savedHeaderWords tail
	}
	_ = r.u32() // firstSegSize (Spur only)

	img.OldBaseAddr = oldBaseAddr
	headerSize := fileHeaderSize + imageHeaderSize
	r.pos = headerSize

	oopMap := make(map[uint32]*Object) // absolute address -> object
	rawBits := make(map[int][]byte)    // relative oop -> body bytes

	var prevObj, firstObj, object *Object
	memEnd := headerSize + objectMemorySize
	for r.pos < memEnd {
		var nWords int
		var classInt uint32
		header := r.u32()
		switch header & HeaderTypeMask {
		case HeaderTypeSizeAndClass: // 3-word header
			nWords = int(header >> 2)
			classInt = r.u32()
			header = r.u32()
		case HeaderTypeClass: // 2-word header
			classInt = header - HeaderTypeClass
			header = r.u32()
			nWords = int((header >> 2) & 63)
		case HeaderTypeShort: // 1-word header
			nWords = int((header >> 2) & 63)
			classInt = (header >> 12) & 31 // compact class index
		case HeaderTypeFree:
			return nil, fmt.Errorf("unexpected free block at pos %d", r.pos-4)
		}
		nWords-- // length includes the base header we already read

		oop := r.pos - 4 - headerSize // 0-rel byte oop of base header
		format := int((header >> 8) & 15)
		hash := int((header >> 17) & 4095)

		// Read body bits in file order; decoding happens during install.
		bits := make([]byte, nWords*4)
		copy(bits, buf[r.pos:r.pos+nWords*4])
		r.pos += nWords * 4

		object = &Object{}
		object.initFromImage(oop, classInt, format, hash)
		if classInt < 32 {
			object.Hash |= 0x10000000 // compact-class marker, cleared in fixCompactOops
		}
		if prevObj != nil {
			prevObj.NextObject = object
		}
		if firstObj == nil {
			firstObj = object
		}
		img.OldSpaceCount++
		prevObj = object
		oopMap[oldBaseAddr+uint32(oop)] = object
		rawBits[oop] = bits
	}
	img.FirstOldObject = firstObj
	img.LastOldObject = object
	if object != nil {
		object.NextObject = nil
	}
	img.OldSpaceBytes = objectMemorySize

	// Resolve special objects, compact-class table, and Float class from raw bits.
	splObs := oopMap[specialObjectsOopInt]
	if splObs == nil {
		return nil, fmt.Errorf("special objects array not found at %d", specialObjectsOopInt)
	}
	splRaw := rawBits[splObs.Oop]
	ccArrayObj := oopMap[readU32(splRaw, SplObCompactClasses, le)]
	if ccArrayObj == nil {
		return nil, fmt.Errorf("compact classes array not found")
	}
	ccRaw := rawBits[ccArrayObj.Oop]
	ccCount := len(ccRaw) / 4
	compactClasses := make([]*Object, ccCount)
	for i := 0; i < ccCount; i++ {
		compactClasses[i] = oopMap[readU32(ccRaw, i, le)]
	}
	floatClass := oopMap[readU32(splRaw, SplObClassFloat, le)]

	// Install (decode + rectify pointers of) every object.
	for obj := img.FirstOldObject; obj != nil; obj = obj.NextObject {
		obj.installFromImage(oopMap, rawBits[obj.Oop], compactClasses, floatClass, le)
	}

	img.SpecialObjectsArray = splObs
	img.decorateKnownObjects()
	img.fixCompiledMethods()
	img.fixCompactOops()
	return img, nil
}

// decorateKnownObjects marks the well-known singletons and compact classes.
func (img *Image) decorateKnownObjects() {
	spl := img.SpecialObjectsArray.Pointers
	if nilObj, ok := spl[SplObNilObject].(*Object); ok {
		nilObj.IsNil = true
	}
	if o, ok := spl[SplObTrueObject].(*Object); ok {
		o.IsTrue = true
	}
	if o, ok := spl[SplObFalseObject].(*Object); ok {
		o.IsFalse = true
	}
	if o, ok := spl[SplObNilObject].(*Object); ok {
		img.nilObj = o
	}
	if o, ok := spl[SplObClassFloat].(*Object); ok {
		o.IsFloatClass = true
	}
	if o, ok := spl[SplObClassCharacter].(*Object); ok {
		img.characterClass = o
	}
	if ccArray, ok := spl[SplObCompactClasses].(*Object); ok {
		img.CompactClasses = make([]*Object, len(ccArray.Pointers))
		for i, cc := range ccArray.Pointers {
			if co, ok := cc.(*Object); ok {
				img.CompactClasses[i] = co
				if !co.IsNil {
					co.IsCompact = true
				}
			}
		}
	}
}

// fixCompiledMethods repairs method classes in pre-6502 images.
func (img *Image) fixCompiledMethods() {
	if img.Version >= 6502 {
		return
	}
	cm, _ := img.SpecialObjectsArray.Pointers[SplObClassCompiledMethod].(*Object)
	for obj := img.FirstOldObject; obj != nil; obj = obj.NextObject {
		if obj.IsMethod() {
			obj.SqClass = cm
		}
	}
}

// fixCompactOops clears the temporary compact-class hash marker set during load.
//
// SqueakJS additionally adjusts each object's snapshot oop here so that saving
// the image reproduces the original layout. The running interpreter uses object
// identity, not oops, so that adjustment is deferred until image saving is
// ported; only the hash fix-up (which identityHash depends on) is done now.
func (img *Image) fixCompactOops() {
	for obj := img.FirstOldObject; obj != nil; obj = obj.NextObject {
		obj.Hash &= 0x0FFFFFFF
	}
}

// SpecialObject returns the object at the given SplOb* index.
func (img *Image) SpecialObject(index int) *Object {
	if o, ok := img.SpecialObjectsArray.Pointers[index].(*Object); ok {
		return o
	}
	return nil
}

// registerObject assigns a temporary (negative) oop and an identity hash to a
// newly allocated object (Squeak.Image>>registerObject).
func (img *Image) registerObject(o *Object) int {
	img.newSpaceCount++
	o.Oop = -img.newSpaceCount
	img.lastHash = (13849 + 27181*img.lastHash) & 0xFFFFFFFF
	return int(img.lastHash & 0xFFF)
}

// InstantiateClass allocates a new instance of aClass with indexableSize
// indexable slots (Squeak.Image>>instantiateClass).
func (img *Image) InstantiateClass(aClass *Object, indexableSize int) *Object {
	o := &Object{}
	hash := img.registerObject(o)
	o.initInstanceOf(aClass, indexableSize, hash, img.nilObj)
	return o
}

// Clone shallow-copies an object (Squeak.Image>>clone).
func (img *Image) Clone(object *Object) *Object {
	o := &Object{}
	hash := img.registerObject(o)
	o.initAsClone(object, hash)
	return o
}

// BytesLeft reports available memory. We don't track a real heap limit (Go's
// GC manages memory), so we report a large constant to allow allocation while
// still letting the image's runaway-allocation guard trip on absurd sizes.
func (img *Image) BytesLeft() int { return 100000000 }

// SomeInstanceOf returns the first old-space instance of a class, or nil.
func (img *Image) SomeInstanceOf(cls *Object) *Object {
	for obj := img.FirstOldObject; obj != nil; obj = obj.NextObject {
		if obj.SqClass == cls {
			return obj
		}
	}
	return nil
}

// NextInstanceAfter returns the next old-space instance of obj's class, or nil.
func (img *Image) NextInstanceAfter(obj *Object) *Object {
	cls := obj.SqClass
	for o := obj.NextObject; o != nil; o = o.NextObject {
		if o.SqClass == cls {
			return o
		}
	}
	return nil
}

// BulkBecome swaps the identities of the objects in from[] with to[]
// (Squeak.Image>>bulkBecome). twoWay makes it symmetric; copyHash swaps hashes
// so identity hash stays with the reference. Returns false if inputs are
// invalid. All old-space references are rewritten; Go's GC handles the rest.
func (img *Image) BulkBecome(from, to []Value, twoWay, copyHash bool) bool {
	if from == nil {
		return to == nil
	}
	n := len(from)
	if n != len(to) {
		return false
	}
	mutations := make(map[*Object]*Object, n*2)
	for i := 0; i < n; i++ {
		f, ok := from[i].(*Object)
		if !ok || f == nil {
			return false
		}
		if _, dup := mutations[f]; dup {
			return false
		}
		if _, ok := to[i].(*Object); !ok {
			return false
		}
		mutations[f] = to[i].(*Object)
	}
	if twoWay {
		for i := 0; i < n; i++ {
			t, ok := to[i].(*Object)
			if !ok || t == nil {
				return false
			}
			if _, dup := mutations[t]; dup {
				return false
			}
			mutations[t] = from[i].(*Object)
		}
	}
	if copyHash {
		for i := 0; i < n; i++ {
			f := from[i].(*Object)
			t := to[i].(*Object)
			f.Hash, t.Hash = t.Hash, f.Hash
		}
	}
	// Rewrite every reference in old space (class pointer + body pointers).
	for obj := img.FirstOldObject; obj != nil; obj = obj.NextObject {
		if mut := mutations[obj.SqClass]; mut != nil {
			obj.SqClass = mut
		}
		for j, p := range obj.Pointers {
			if po, ok := p.(*Object); ok {
				if mut := mutations[po]; mut != nil {
					obj.Pointers[j] = mut
				}
			}
		}
	}
	return true
}

// GetCharacter returns the (cached) Character instance for a code point.
// Classic images store the value in the Character's first inst var.
func (img *Image) GetCharacter(unicode int) *Object {
	if img.characterTable == nil {
		img.characterTable = map[int]*Object{}
	}
	if c := img.characterTable[unicode]; c != nil {
		return c
	}
	c := img.InstantiateClass(img.characterClass, 0)
	if len(c.Pointers) > 0 {
		c.Pointers[0] = unicode
	}
	img.characterTable[unicode] = c
	return c
}
