package vm

import (
	"math"
	"time"
)

func (p *Primitives) asObj(v Value) *Object {
	if o, ok := v.(*Object); ok {
		return o
	}
	return nil
}

func vmIdentical(a, b Value) bool { return a == b }

func intQuo(a, b int) int {
	if b == 0 {
		return NonSmallInt
	}
	return a / b // Go truncates toward zero, like Smalltalk quo:
}

// --- stack coercions -----------------------------------------------------

func (p *Primitives) popNIfOK(n int) bool {
	if !p.success {
		return false
	}
	p.vm.popN(n)
	return true
}

func (p *Primitives) popNandPushIfOK(n int, v Value) bool {
	if !p.success || v == nil {
		return false
	}
	p.vm.popNandPush(n, v)
	return true
}

func (p *Primitives) popNandPushBoolIfOK(n int, b bool) bool {
	if !p.success {
		return false
	}
	if b {
		p.vm.popNandPush(n, p.vm.TrueObj)
	} else {
		p.vm.popNandPush(n, p.vm.FalseObj)
	}
	return true
}

func (p *Primitives) popNandPushIntIfOK(n, v int) bool {
	if !p.success || !p.vm.canBeSmallInt(v) {
		return false
	}
	p.vm.popNandPush(n, v)
	return true
}

func (p *Primitives) popNandPushFloatIfOK(n int, v float64) bool {
	if !p.success {
		return false
	}
	p.vm.popNandPush(n, p.makeFloat(v))
	return true
}

func (p *Primitives) stackInteger(d int) int {
	if n, ok := p.vm.stackValue(d).(int); ok {
		return n
	}
	p.success = false
	return 0
}

func (p *Primitives) stackNonInteger(d int) *Object {
	if o, ok := p.vm.stackValue(d).(*Object); ok {
		return o
	}
	p.success = false
	return p.vm.NilObj
}

func (p *Primitives) stackFloat(d int) float64 {
	return p.checkFloat(p.vm.stackValue(d))
}

func (p *Primitives) checkFloat(v Value) float64 {
	if o, ok := v.(*Object); ok && o.IsFloat {
		return o.Float
	}
	if n, ok := v.(int); ok {
		return float64(n)
	}
	p.success = false
	return 0
}

func (p *Primitives) stackPos32(d int) uint32 {
	return p.positive32BitValueOf(p.vm.stackValue(d))
}

func (p *Primitives) positive32BitValueOf(v Value) uint32 {
	if n, ok := v.(int); ok {
		if n >= 0 {
			return uint32(n)
		}
		p.success = false
		return 0
	}
	o, ok := v.(*Object)
	if !ok || o.SqClass != p.vm.SpecialObjects[SplObClassLargePositiveInteger] || len(o.Bytes) != 4 {
		p.success = false
		return 0
	}
	var value uint32
	for i := 0; i < 4; i++ {
		value |= uint32(o.Bytes[i]) << (8 * i)
	}
	return value
}

func (p *Primitives) pos32BitIntFor(v uint32) Value {
	if v <= MaxSmallInt {
		return int(v)
	}
	lg := p.vm.InstantiateClass(p.vm.SpecialObjects[SplObClassLargePositiveInteger].(*Object), 4)
	for i := 0; i < 4; i++ {
		lg.Bytes[i] = byte(v >> (8 * i))
	}
	return lg
}

// --- constructors --------------------------------------------------------

func (p *Primitives) makeFloat(v float64) *Object {
	f := p.vm.InstantiateClass(p.vm.SpecialObjects[SplObClassFloat].(*Object), 2)
	f.IsFloat = true
	f.Float = v
	return f
}

func (p *Primitives) makePointWithXandY(x, y Value) *Object {
	pt := p.vm.InstantiateClass(p.vm.SpecialObjects[SplObClassPoint].(*Object), 0)
	pt.Pointers[PointX] = x
	pt.Pointers[PointY] = y
	return pt
}

func (p *Primitives) makeStString(s string) *Object {
	str := p.vm.InstantiateClass(p.vm.SpecialObjects[SplObClassString].(*Object), len(s))
	copy(str.Bytes, s)
	return str
}

func (p *Primitives) primitiveMakePoint(argCount int, checkNumbers bool) bool {
	p.success = true
	x := p.vm.stackValue(1)
	y := p.vm.stackValue(0)
	if checkNumbers {
		p.checkFloat(x)
		p.checkFloat(y)
		if !p.success {
			return false
		}
	}
	p.vm.popNandPush(1+argCount, p.makePointWithXandY(x, y))
	return true
}

// --- numbers -------------------------------------------------------------

func (p *Primitives) doBitAnd() Value {
	r := p.stackPos32(1)
	a := p.stackPos32(0)
	if !p.success {
		return nil
	}
	return p.pos32BitIntFor(r & a)
}

func (p *Primitives) doBitOr() Value {
	r := p.stackPos32(1)
	a := p.stackPos32(0)
	if !p.success {
		return nil
	}
	return p.pos32BitIntFor(r | a)
}

func (p *Primitives) doBitXor() Value {
	r := p.stackPos32(1)
	a := p.stackPos32(0)
	if !p.success {
		return nil
	}
	return p.pos32BitIntFor(r ^ a)
}

func (p *Primitives) doBitShift() Value {
	rcvr := p.stackPos32(1)
	arg := p.stackInteger(0)
	if !p.success {
		return nil
	}
	var result uint32
	if arg < 0 {
		if arg < -31 {
			return p.pos32BitIntFor(0)
		}
		result = rcvr >> uint(-arg)
	} else {
		if arg > 31 {
			p.success = false
			return nil
		}
		result = rcvr << uint(arg)
		if result>>uint(arg) != rcvr {
			p.success = false
			return nil
		}
	}
	return p.pos32BitIntFor(result)
}

func (p *Primitives) safeFDiv(a, b float64) float64 {
	if b == 0 {
		p.success = false
		return 1
	}
	return a / b
}

func (p *Primitives) floatAsSmallInt(f float64) Value {
	var t float64
	if f >= 0 {
		t = math.Floor(f)
	} else {
		t = math.Ceil(f)
	}
	if t == float64(int(t)) && p.vm.canBeSmallInt(int(t)) {
		return int(t)
	}
	p.success = false
	return 0
}

// --- indexing ------------------------------------------------------------

// makeAtCacheInfo fills and returns an at-cache entry for array. It caches only
// for non-super sends of at:/at:put: to a stable (non-context) receiver class.
func (p *Primitives) makeAtCacheInfo(cache *[32]atCacheEntry, atOrPutSel Value, array *Object, convertChars, includeInstVars bool) *atCacheEntry {
	cacheable := p.vm.verifyAtSelector == atOrPutSel &&
		p.vm.verifyAtClass == array.SqClass &&
		!p.vm.isContext(array)
	var info *atCacheEntry
	if cacheable {
		info = &cache[array.Hash&31]
	} else {
		info = &p.nonCachedInfo
	}
	info.array = array
	info.convertChars = convertChars
	if includeInstVars {
		idx := array.IndexableSize(p.allowAccessBeyond, p.vm.isContext(array))
		if idx < 0 {
			idx = 0
		}
		info.size = array.InstSize() + idx
		info.ivarOffset = 0
	} else {
		info.size = array.IndexableSize(p.allowAccessBeyond, p.vm.isContext(array))
		if array.IsPointers() {
			info.ivarOffset = array.InstSize()
		} else {
			info.ivarOffset = 0
		}
	}
	return info
}

func (p *Primitives) atSelector() Value    { return p.vm.SpecialSelectors[32] }
func (p *Primitives) atPutSelector() Value { return p.vm.SpecialSelectors[34] }

func (p *Primitives) charFromInt(ascii int) *Object {
	if ct := p.asObj(p.vm.SpecialObjects[SplObCharacterTable]); ct != nil && ascii >= 0 && ascii < len(ct.Pointers) {
		if c := p.asObj(ct.Pointers[ascii]); c != nil {
			return c
		}
	}
	return p.vm.Image.GetCharacter(ascii)
}

func (p *Primitives) objectAt(cameFromBytecode, convertChars, includeInstVars bool) Value {
	array := p.stackNonInteger(1)
	index := int(p.stackPos32(0))
	if !p.success {
		return nil
	}
	var info *atCacheEntry
	if cameFromBytecode {
		info = &p.atCache[array.Hash&31]
		if info.array != array {
			p.success = false
			return nil
		}
	} else {
		info = p.makeAtCacheInfo(&p.atCache, p.atSelector(), array, convertChars, includeInstVars)
	}
	if index < 1 || index > info.size {
		p.success = false
		return nil
	}
	if includeInstVars {
		return array.Pointers[index-1]
	}
	if array.IsPointers() {
		return array.Pointers[index-1+info.ivarOffset]
	}
	if array.IsWords() {
		w := array.Words[index-1]
		if info.convertChars {
			return p.charFromInt(int(w & 0x3FFFFFFF))
		}
		return p.pos32BitIntFor(w)
	}
	if array.IsBytes() {
		b := int(array.Bytes[index-1]) & 0xFF
		if info.convertChars {
			return p.charFromInt(b)
		}
		return b
	}
	// CompiledMethod: bytes follow the pointer (header+literals) area
	offset := array.PointersSize() * 4
	if index-1-offset < 0 {
		p.success = false
		return nil
	}
	return int(array.Bytes[index-1-offset]) & 0xFF
}

func (p *Primitives) objectAtPut(cameFromBytecode, convertChars, includeInstVars bool) Value {
	array := p.stackNonInteger(2)
	index := int(p.stackPos32(1))
	if !p.success {
		return nil
	}
	var info *atCacheEntry
	if cameFromBytecode {
		info = &p.atPutCache[array.Hash&31]
		if info.array != array {
			p.success = false
			return nil
		}
	} else {
		info = p.makeAtCacheInfo(&p.atPutCache, p.atPutSelector(), array, convertChars, includeInstVars)
	}
	if index < 1 || index > info.size {
		p.success = false
		return nil
	}
	convertChars = info.convertChars
	objToPut := p.vm.stackValue(0)
	if includeInstVars {
		array.Dirty = true
		array.Pointers[index-1] = objToPut
		return objToPut
	}
	if array.IsPointers() {
		array.Dirty = true
		array.Pointers[index-1+info.ivarOffset] = objToPut
		return objToPut
	}
	var intToPut int
	if array.IsWords() {
		if convertChars {
			c := p.asObj(objToPut)
			if c == nil || c.SqClass != p.vm.SpecialObjects[SplObClassCharacter] {
				p.success = false
				return objToPut
			}
			intToPut = asIntValue(c.Pointers[0])
		} else {
			intToPut = int(p.positive32BitValueOf(objToPut))
		}
		if p.success {
			array.Words[index-1] = uint32(intToPut)
		}
		return objToPut
	}
	if convertChars {
		c := p.asObj(objToPut)
		if c == nil || c.SqClass != p.vm.SpecialObjects[SplObClassCharacter] {
			p.success = false
			return objToPut
		}
		intToPut = asIntValue(c.Pointers[0])
	} else {
		n, ok := objToPut.(int)
		if !ok {
			p.success = false
			return objToPut
		}
		intToPut = n
	}
	if intToPut < 0 || intToPut > 255 {
		p.success = false
		return objToPut
	}
	if array.IsBytes() {
		array.Bytes[index-1] = byte(intToPut)
		return objToPut
	}
	offset := array.PointersSize() * 4
	if index-1-offset < 0 {
		p.success = false
		return nil
	}
	array.Bytes[index-1-offset] = byte(intToPut)
	return objToPut
}

func (p *Primitives) objectSize(cameFromBytecode bool) Value {
	rcvr := p.asObj(p.vm.stackValue(0))
	size := -1
	if rcvr != nil {
		if cameFromBytecode {
			if rcvr.SqClass == p.vm.SpecialObjects[SplObClassArray] {
				size = rcvr.PointersSize()
			} else if rcvr.SqClass == p.vm.SpecialObjects[SplObClassString] {
				size = rcvr.BytesSize()
			}
		} else {
			size = rcvr.IndexableSize(p.allowAccessBeyond, p.vm.isContext(rcvr))
		}
	}
	if size == -1 {
		p.success = false
		return nil
	}
	return p.pos32BitIntFor(uint32(size))
}

func (p *Primitives) doStringReplace() Value {
	dst := p.stackNonInteger(4)
	dstPos := p.stackInteger(3) - 1
	count := p.stackInteger(2) - dstPos
	src := p.stackNonInteger(1)
	srcPos := p.stackInteger(0) - 1
	if !p.success {
		return dst
	}
	if !src.SameFormatAs(dst) {
		p.success = false
		return dst
	}
	if src.IsPointers() {
		srcPos += src.InstSize()
		if srcPos < 0 || srcPos+count > src.PointersSize() {
			p.success = false
			return dst
		}
		dstPos += dst.InstSize()
		if dstPos < 0 || dstPos+count > dst.PointersSize() {
			p.success = false
			return dst
		}
		for i := 0; i < count; i++ {
			dst.Pointers[dstPos+i] = src.Pointers[srcPos+i]
		}
		return dst
	}
	if src.IsWords() {
		if srcPos < 0 || srcPos+count > src.WordsSize() || dstPos < 0 || dstPos+count > dst.WordsSize() {
			p.success = false
			return dst
		}
		for i := 0; i < count; i++ {
			dst.Words[dstPos+i] = src.Words[srcPos+i]
		}
		return dst
	}
	// bytes
	if srcPos < 0 || srcPos+count > src.BytesSize() || dstPos < 0 || dstPos+count > dst.BytesSize() {
		p.success = false
		return dst
	}
	for i := 0; i < count; i++ {
		dst.Bytes[dstPos+i] = src.Bytes[srcPos+i]
	}
	return dst
}

// --- storage -------------------------------------------------------------

func (p *Primitives) instantiate(cls *Object, size int) Value {
	if cls == nil {
		p.success = false
		return nil
	}
	if size*4 > p.vm.Image.BytesLeft() {
		p.success = false
		p.vm.primFailCode = PrimErrNoMemory
		return nil
	}
	return p.vm.InstantiateClass(cls, size)
}

func (p *Primitives) identityHash(o *Object) Value {
	if o == nil {
		p.success = false
		return nil
	}
	return o.Hash
}

func (p *Primitives) someInstanceOf(cls *Object) Value {
	if inst := p.vm.Image.SomeInstanceOf(cls); inst != nil {
		return inst
	}
	p.success = false
	return nil
}

func (p *Primitives) nextInstanceAfter(o *Object) Value {
	if inst := p.vm.Image.NextInstanceAfter(o); inst != nil {
		return inst
	}
	p.success = false
	return nil
}

func (p *Primitives) nextObject(o Value) Value {
	obj := p.asObj(o)
	if obj == nil || obj.NextObject == nil {
		return 0
	}
	return obj.NextObject
}

func (p *Primitives) pointsTo(rcvr *Object, arg Value) bool {
	if rcvr == nil || rcvr.Pointers == nil {
		return false
	}
	for _, v := range rcvr.Pointers {
		if v == arg {
			return true
		}
	}
	return false
}

func (p *Primitives) primitiveNewMethod(argCount int) bool {
	header := p.stackInteger(0)
	bytecodeCount := p.stackInteger(1)
	if !p.success {
		return false
	}
	cls := p.asObj(p.vm.stackValue(2))
	method := p.vm.InstantiateClass(cls, bytecodeCount)
	method.Pointers = []Value{header}
	method.Format = 12 // CompiledMethod
	litCount := method.MethodNumLits()
	for i := 0; i < litCount; i++ {
		method.Pointers = append(method.Pointers, p.vm.NilObj)
	}
	p.vm.popNandPush(1+argCount, method)
	return true
}

// --- time ----------------------------------------------------------------

func (p *Primitives) millisecondClockValue() int {
	return int(time.Since(p.vm.startupTime).Milliseconds()) & MillisecondClockMask
}

var epoch1901 = time.Date(1901, 1, 1, 0, 0, 0, 0, time.UTC)

func (p *Primitives) secondClock() Value {
	return p.pos32BitIntFor(uint32(time.Since(epoch1901).Seconds()))
}

func (p *Primitives) primitiveSignalAtMilliseconds(argCount int) bool {
	msTime := p.stackInteger(0)
	sema := p.stackNonInteger(1)
	if !p.success {
		return false
	}
	if sema.SqClass == p.vm.SpecialObjects[SplObClassSemaphore] {
		p.vm.SpecialObjects[SplObTheTimerSemaphore] = sema
		p.vm.nextWakeupTick = msTime
	} else {
		p.vm.SpecialObjects[SplObTheTimerSemaphore] = p.vm.NilObj
		p.vm.nextWakeupTick = 0
	}
	p.vm.popN(argCount)
	return true
}

// --- misc ----------------------------------------------------------------

func (p *Primitives) setLowSpaceThreshold() Value {
	_ = p.stackInteger(0)
	return p.vm.stackValue(1)
}

func (p *Primitives) registerSemaphore(specialIndex int) Value {
	sema := p.asObj(p.vm.top())
	if sema != nil && sema.SqClass == p.vm.SpecialObjects[SplObClassSemaphore] {
		p.vm.SpecialObjects[specialIndex] = sema
	} else {
		p.vm.SpecialObjects[specialIndex] = p.vm.NilObj
	}
	return p.vm.stackValue(1)
}

func (p *Primitives) primitiveInputSemaphore(argCount int) bool {
	p.vm.popNandPush(argCount+1, p.vm.NilObj)
	return true
}

func (p *Primitives) primitiveQuit(argCount int) bool {
	p.quitFlag = true
	p.vm.breakOut = true
	return true
}

func (p *Primitives) primitiveRelinquishProcessor(argCount int) bool {
	p.vm.popN(argCount)
	p.vm.goIdle()
	return true
}

func (p *Primitives) primitiveImageName(argCount int) bool {
	return p.popNandPushIfOK(argCount+1, p.makeStString(p.vm.Image.Name))
}

func (p *Primitives) primitiveGetAttribute(argCount int) bool {
	attr := p.stackInteger(0)
	if !p.success {
		return false
	}
	switch attr {
	case 1001:
		return p.popNandPushIfOK(argCount+1, p.makeStString(PlatformName))
	case 1002:
		return p.popNandPushIfOK(argCount+1, p.makeStString("unknown"))
	}
	return false
}

func (p *Primitives) primitiveVMParameter(argCount int) bool {
	size := 44
	switch argCount {
	case 0:
		arr := p.vm.InstantiateClass(p.vm.SpecialObjects[SplObClassArray].(*Object), size)
		for i := range arr.Pointers {
			arr.Pointers[i] = 0
		}
		return p.popNandPushIfOK(1, arr)
	case 1:
		return p.popNandPushIfOK(2, 0)
	case 2:
		return p.popNandPushIfOK(3, 0)
	}
	return false
}

var _ = math.Sqrt
