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
	_ = r.u32() // lastHash
	_ = r.u32() // savedWindowSize
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
	if o, ok := spl[SplObClassFloat].(*Object); ok {
		o.IsFloatClass = true
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
