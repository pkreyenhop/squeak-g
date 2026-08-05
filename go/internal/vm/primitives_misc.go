package vm

import "math"

// MiscPrimitivePlugin, FloatArrayPlugin, and Matrix2x3Plugin implementations for SqueakG.

func (p *Primitives) dispatchMiscPrimitive(fn string, argCount int) bool {
	switch fn {
	case "primitiveDecompressFromByteArray":
		return p.primitiveDecompressFromByteArray(argCount)
	case "primitiveCompareString":
		return p.primitiveCompareString(argCount)
	case "primitiveFindSubstring":
		return p.primitiveFindSubstring(argCount)
	case "primitiveFindFirstInString":
		return p.primitiveFindFirstInString(argCount)
	case "primitiveIndexOfAsciiInString":
		return p.primitiveIndexOfAsciiInString(argCount)
	case "primitiveTranslateStringWithTable":
		return p.primitiveTranslateStringWithTable(argCount)
	case "primitiveStringHash":
		return p.primitiveStringHash(argCount)
	case "primitiveCompressToByteArray":
		return p.primitiveCompressToByteArray(argCount)
	case "primitiveConvert8BitSigned":
		return p.primitiveConvert8BitSigned(argCount)
	}
	return false
}

func (p *Primitives) primitiveStoreStackp(argCount int) bool {
	if argCount != 1 {
		return false
	}
	ctxt := p.stackNonInteger(1)
	newStackp := p.stackInteger(0)
	if !p.success || ctxt == nil || newStackp < 0 {
		return false
	}
	spIndex := ContextTempFrameStart + newStackp - 1
	if spIndex >= len(ctxt.Pointers) {
		return false
	}
	oldStackp, ok := ctxt.Pointers[ContextStackPointer].(int)
	if ok {
		for s := oldStackp + 1; s <= newStackp; s++ {
			idx := ContextTempFrameStart + s - 1
			if idx < len(ctxt.Pointers) {
				ctxt.Pointers[idx] = p.vm.NilObj
			}
		}
	}
	ctxt.Pointers[ContextStackPointer] = newStackp
	ctxt.Dirty = true
	p.vm.popN(argCount)
	return true
}

func (p *Primitives) primitiveConstantFill(argCount int) bool {
	if argCount != 1 {
		return false
	}
	rcvr := p.stackNonInteger(1)
	val := p.stackPos32(0)
	if !p.success || rcvr == nil || !rcvr.IsWordsOrBytes() {
		return false
	}
	if rcvr.IsWords() {
		for i := range rcvr.Words {
			rcvr.Words[i] = val
		}
	} else if rcvr.IsBytes() {
		if val > 255 {
			return false
		}
		for i := range rcvr.Bytes {
			rcvr.Bytes[i] = byte(val)
		}
	}
	rcvr.Dirty = true
	p.vm.popN(argCount)
	return true
}

func (p *Primitives) primitiveShortAtAndPut(argCount int) bool {
	rcvr := p.stackNonInteger(argCount)
	index := p.stackInteger(argCount-1) - 1
	if !p.success || rcvr == nil || !rcvr.IsWords() || index < 0 || index >= len(rcvr.Words)*2 {
		return false
	}
	wordIdx := index / 2
	shift := uint(16 * (1 - (index % 2)))
	if p.vm.Image.LittleEndian {
		shift = uint(16 * (index % 2))
	}
	if argCount < 2 { // shortAt:
		val := int16((rcvr.Words[wordIdx] >> shift) & 0xFFFF)
		return p.popNandPushIntIfOK(argCount+1, int(val))
	}
	// shortAt:put:
	val := p.stackInteger(0)
	if val < -32768 || val > 32767 {
		return false
	}
	mask := uint32(0xFFFF) << shift
	rcvr.Words[wordIdx] = (rcvr.Words[wordIdx] &^ mask) | ((uint32(val) & 0xFFFF) << shift)
	rcvr.Dirty = true
	return p.popNandPushIntIfOK(argCount+1, val)
}

func (p *Primitives) primitiveHashMultiply(argCount int) bool {
	if argCount != 0 {
		return false
	}
	rcvr := p.stackInteger(0)
	if !p.success {
		return false
	}
	val := (rcvr * 1664525) & 0x0FFFFFFF
	return p.popNandPushIntIfOK(1, val)
}

func (p *Primitives) dispatchFloatArrayPrimitive(fn string, argCount int) bool {
	switch fn {
	case "primitiveAt":
		return p.primitiveFloatArrayAt(argCount)
	case "primitiveAtPut":
		return p.primitiveFloatArrayAtPut(argCount)
	}
	return false
}

func (p *Primitives) dispatchMatrix2x3Primitive(fn string, argCount int) bool {
	switch fn {
	case "primitiveTransformPoint":
		return p.primitiveTransformPoint(argCount)
	case "primitiveInvertPoint":
		return p.primitiveInvertPoint(argCount)
	case "primitiveIsIdentity":
		return p.primitiveIsIdentity(argCount)
	case "primitiveIsPureTranslation":
		return p.primitiveIsPureTranslation(argCount)
	case "primitiveComposeMatrix":
		return p.primitiveComposeMatrix(argCount)
	}
	return false
}

func (p *Primitives) dispatchB2DPrimitive(fn string, argCount int) bool {
	switch fn {
	case "primitiveInitializeBuffer":
		return p.primitiveInitializeBuffer(argCount)
	case "primitiveGetAALevel":
		return p.popNandPushIntIfOK(argCount+1, 1)
	case "primitiveGetFailureReason", "primitiveGetDepth":
		return p.popNandPushIntIfOK(argCount+1, 0)
	case "primitiveRegisterExternalFill", "primitiveRegisterExternalEdge",
		"primitiveAddGradientFill", "primitiveAddBitmapFill",
		"primitiveAddOval", "primitiveAddRect", "primitiveAddPolygon",
		"primitiveAddBezier", "primitiveAddLine", "primitiveAddActiveEdge":
		return p.popNandPushIntIfOK(argCount+1, 1)
	case "primitiveFinishedProcessing":
		return p.popNandPushBoolIfOK(argCount+1, true)
	case "primitiveGetClipRect":
		return p.primitiveGetClipRect(argCount)
	default:
		p.vm.popN(argCount)
		return true
	}
}

func (p *Primitives) primitiveGetClipRect(argCount int) bool {
	if argCount != 1 {
		return false
	}
	rectObj := p.asObj(p.vm.stackValue(0))
	if rectObj == nil || len(rectObj.Pointers) < 2 {
		return false
	}
	rectObj.Pointers[0] = p.makePointWithXandY(0, 0)
	rectObj.Pointers[1] = p.makePointWithXandY(800, 600)
	rectObj.Dirty = true
	return p.popNandPushIfOK(argCount+1, rectObj)
}

func (p *Primitives) primitiveInitializeBuffer(argCount int) bool {
	if argCount != 1 {
		return false
	}
	wbOop := p.asObj(p.vm.stackValue(0))
	if !p.success || wbOop == nil || wbOop.Words == nil || len(wbOop.Words) < 32 {
		return false
	}
	words := wbOop.Words
	size := len(words)
	words[0] = 0x41414141 // GWMagicNumber
	words[1] = uint32(size)
	words[2] = uint32(size)
	words[3] = 1 // GEStateUnlocked
	words[4] = 8 // GWHeaderSize
	words[5] = 4 // objUsed
	wbOop.Dirty = true
	return p.popNandPushIfOK(argCount+1, wbOop)
}

// --- FloatArray primitives ---

func (p *Primitives) primitiveFloatArrayAt(argCount int) bool {
	if argCount != 1 {
		return false
	}
	index := p.stackInteger(0)
	rcvr := p.asObj(p.vm.stackValue(1))
	if !p.success || rcvr == nil || rcvr.Words == nil || index < 1 || index > len(rcvr.Words) {
		return false
	}
	bits := rcvr.Words[index-1]
	val := float64(math.Float32frombits(bits))
	return p.popNandPushFloatIfOK(argCount+1, val)
}

func (p *Primitives) primitiveFloatArrayAtPut(argCount int) bool {
	if argCount != 2 {
		return false
	}
	valObj := p.vm.stackValue(0)
	index := p.stackInteger(1)
	rcvr := p.asObj(p.vm.stackValue(2))
	if !p.success || rcvr == nil || rcvr.Words == nil || index < 1 || index > len(rcvr.Words) {
		return false
	}
	var fval float64
	if n, ok := valObj.(int); ok {
		fval = float64(n)
	} else {
		fval = p.stackFloat(0)
	}
	if !p.success {
		return false
	}
	bits := math.Float32bits(float32(fval))
	rcvr.Words[index-1] = bits
	rcvr.Dirty = true
	return p.popNandPushIfOK(argCount+1, valObj)
}

// --- Matrix2x3 primitives ---

func (p *Primitives) getFloatArray6(obj *Object) ([]float32, bool) {
	if obj == nil || obj.Words == nil || len(obj.Words) < 6 {
		return nil, false
	}
	res := make([]float32, 6)
	for i := 0; i < 6; i++ {
		res[i] = math.Float32frombits(obj.Words[i])
	}
	return res, true
}

func (p *Primitives) getPointXY(obj Value) (float64, float64, bool) {
	pt := p.asObj(obj)
	if pt == nil || pt.SqClass != p.vm.SpecialObjects[SplObClassPoint] || len(pt.Pointers) < 2 {
		return 0, 0, false
	}
	return p.floatValueOf(pt.Pointers[0]), p.floatValueOf(pt.Pointers[1]), true
}

func (p *Primitives) floatValueOf(obj Value) float64 {
	if n, ok := obj.(int); ok {
		return float64(n)
	}
	if o := p.asObj(obj); o != nil && (o.IsFloat || o.SqClass == p.vm.SpecialObjects[SplObClassFloat]) {
		return o.Float
	}
	return 0
}

func (p *Primitives) primitiveTransformPoint(argCount int) bool {
	if argCount != 1 {
		return false
	}
	ptObj := p.vm.stackValue(0)
	matrixObj := p.asObj(p.vm.stackValue(1))
	m, okM := p.getFloatArray6(matrixObj)
	px, py, okP := p.getPointXY(ptObj)
	if !okM || !okP {
		return false
	}
	rx := float64(m[0])*px + float64(m[1])*py + float64(m[2])
	ry := float64(m[3])*px + float64(m[4])*py + float64(m[5])
	ix := int(math.Floor(rx + 0.5))
	iy := int(math.Floor(ry + 0.5))
	return p.popNandPushIfOK(argCount+1, p.makePointWithXandY(ix, iy))
}

func (p *Primitives) primitiveInvertPoint(argCount int) bool {
	if argCount != 1 {
		return false
	}
	ptObj := p.vm.stackValue(0)
	matrixObj := p.asObj(p.vm.stackValue(1))
	m, okM := p.getFloatArray6(matrixObj)
	px, py, okP := p.getPointXY(ptObj)
	if !okM || !okP {
		return false
	}
	x := px - float64(m[2])
	y := py - float64(m[5])
	det := float64(m[0])*float64(m[4]) - float64(m[1])*float64(m[3])
	if det == 0 {
		return false
	}
	det = 1.0 / det
	rx := (x*float64(m[4]) - float64(m[1])*y) * det
	ry := (float64(m[0])*y - x*float64(m[3])) * det
	ix := int(math.Floor(rx + 0.5))
	iy := int(math.Floor(ry + 0.5))
	return p.popNandPushIfOK(argCount+1, p.makePointWithXandY(ix, iy))
}

func (p *Primitives) primitiveIsIdentity(argCount int) bool {
	if argCount != 0 {
		return false
	}
	matrixObj := p.asObj(p.vm.stackValue(0))
	m, okM := p.getFloatArray6(matrixObj)
	if !okM {
		return false
	}
	isId := m[0] == 1.0 && m[1] == 0.0 && m[2] == 0.0 && m[3] == 0.0 && m[4] == 1.0 && m[5] == 0.0
	return p.popNandPushBoolIfOK(argCount+1, isId)
}

func (p *Primitives) primitiveIsPureTranslation(argCount int) bool {
	if argCount != 0 {
		return false
	}
	matrixObj := p.asObj(p.vm.stackValue(0))
	m, okM := p.getFloatArray6(matrixObj)
	if !okM {
		return false
	}
	isTrans := m[0] == 1.0 && m[1] == 0.0 && m[3] == 0.0 && m[4] == 1.0
	return p.popNandPushBoolIfOK(argCount+1, isTrans)
}

func (p *Primitives) primitiveComposeMatrix(argCount int) bool {
	if argCount != 2 {
		return false
	}
	m3Obj := p.asObj(p.vm.stackValue(0))
	m2Obj := p.asObj(p.vm.stackValue(1))
	m1Obj := p.asObj(p.vm.stackValue(2))
	m1, ok1 := p.getFloatArray6(m1Obj)
	m2, ok2 := p.getFloatArray6(m2Obj)
	if !ok1 || !ok2 || m3Obj == nil || m3Obj.Words == nil || len(m3Obj.Words) < 6 {
		return false
	}
	a11 := m1[0]*m2[0] + m1[1]*m2[3]
	a12 := m1[0]*m2[1] + m1[1]*m2[4]
	a13 := m1[0]*m2[2] + m1[1]*m2[5] + m1[2]
	a21 := m1[3]*m2[0] + m1[4]*m2[3]
	a22 := m1[3]*m2[1] + m1[4]*m2[4]
	a23 := m1[3]*m2[2] + m1[4]*m2[5] + m1[5]

	m3Obj.Words[0] = math.Float32bits(a11)
	m3Obj.Words[1] = math.Float32bits(a12)
	m3Obj.Words[2] = math.Float32bits(a13)
	m3Obj.Words[3] = math.Float32bits(a21)
	m3Obj.Words[4] = math.Float32bits(a22)
	m3Obj.Words[5] = math.Float32bits(a23)
	m3Obj.Dirty = true
	return p.popNandPushIfOK(argCount+1, m3Obj)
}

// --- MiscPrimitives ---

func (p *Primitives) primitiveDecompressFromByteArray(argCount int) bool {
	if argCount != 3 {
		return false
	}
	bmObj := p.asObj(p.vm.stackValue(2))
	baObj := p.asObj(p.vm.stackValue(1))
	startIndex := p.stackInteger(0)
	if !p.success || bmObj == nil || baObj == nil || bmObj.Words == nil || baObj.Bytes == nil {
		return false
	}
	bm := bmObj.Words
	ba := baObj.Bytes
	i := startIndex - 1 // 1-based index in Squeak
	end := len(ba)
	k := 0
	pastEnd := len(bm)

	for i < end {
		if i >= len(ba) {
			return false
		}
		anInt := int(ba[i])
		i++
		if anInt > 223 {
			if anInt <= 254 {
				if i >= len(ba) {
					return false
				}
				anInt = (anInt-224)*256 + int(ba[i])
				i++
			} else {
				anInt = 0
				for j := 0; j < 4; j++ {
					if i >= len(ba) {
						return false
					}
					anInt = (anInt << 8) + int(ba[i])
					i++
				}
			}
		}
		n := anInt >> 2
		if k+n > pastEnd {
			return false
		}
		code := anInt & 3
		switch code {
		case 0:
			// skip
		case 1:
			if i >= len(ba) {
				return false
			}
			b := uint32(ba[i])
			i++
			data := b | (b << 8) | (b << 16) | (b << 24)
			for j := 0; j < n; j++ {
				bm[k] = data
				k++
			}
		case 2:
			var data uint32
			for j := 0; j < 4; j++ {
				if i >= len(ba) {
					return false
				}
				data = (data << 8) | uint32(ba[i])
				i++
			}
			for j := 0; j < n; j++ {
				bm[k] = data
				k++
			}
		case 3:
			for m := 0; m < n; m++ {
				var data uint32
				for j := 0; j < 4; j++ {
					if i >= len(ba) {
						return false
					}
					data = (data << 8) | uint32(ba[i])
					i++
				}
				bm[k] = data
				k++
			}
		}
	}
	p.vm.popN(argCount)
	return true
}

func (p *Primitives) primitiveCompareString(argCount int) bool {
	if argCount != 3 {
		return false
	}
	s1Obj := p.asObj(p.vm.stackValue(2))
	s2Obj := p.asObj(p.vm.stackValue(1))
	orderObj := p.asObj(p.vm.stackValue(0))
	if !p.success || s1Obj == nil || s2Obj == nil || orderObj == nil || s1Obj.Bytes == nil || s2Obj.Bytes == nil || orderObj.Bytes == nil {
		return false
	}
	s1 := s1Obj.Bytes
	s2 := s2Obj.Bytes
	order := orderObj.Bytes
	len1 := len(s1)
	len2 := len(s2)
	minLen := len1
	if len2 < minLen {
		minLen = len2
	}
	for i := 0; i < minLen; i++ {
		b1 := s1[i]
		b2 := s2[i]
		var c1, c2 int
		if int(b1) < len(order) {
			c1 = int(order[b1])
		} else {
			c1 = int(b1)
		}
		if int(b2) < len(order) {
			c2 = int(order[b2])
		} else {
			c2 = int(b2)
		}
		if c1 != c2 {
			if c1 < c2 {
				return p.popNandPushIntIfOK(argCount+1, 1)
			}
			return p.popNandPushIntIfOK(argCount+1, 3)
		}
	}
	if len1 == len2 {
		return p.popNandPushIntIfOK(argCount+1, 2)
	}
	if len1 < len2 {
		return p.popNandPushIntIfOK(argCount+1, 1)
	}
	return p.popNandPushIntIfOK(argCount+1, 3)
}

func (p *Primitives) primitiveFindSubstring(argCount int) bool {
	if argCount != 4 {
		return false
	}
	keyObj := p.asObj(p.vm.stackValue(3))
	bodyObj := p.asObj(p.vm.stackValue(2))
	start := p.stackInteger(1)
	matchTableObj := p.asObj(p.vm.stackValue(0))
	if !p.success || keyObj == nil || bodyObj == nil || matchTableObj == nil || keyObj.Bytes == nil || bodyObj.Bytes == nil || matchTableObj.Bytes == nil {
		return false
	}
	key := keyObj.Bytes
	body := bodyObj.Bytes
	table := matchTableObj.Bytes
	if len(key) == 0 {
		return p.popNandPushIntIfOK(argCount+1, 0)
	}
	for startIndex := start; startIndex <= len(body)-len(key)+1; startIndex++ {
		match := true
		for index := 1; index <= len(key); index++ {
			bBody := body[startIndex+index-2]
			bKey := key[index-1]
			var cBody, cKey byte
			if int(bBody) < len(table) {
				cBody = table[bBody]
			} else {
				cBody = bBody
			}
			if int(bKey) < len(table) {
				cKey = table[bKey]
			} else {
				cKey = bKey
			}
			if cBody != cKey {
				match = false
				break
			}
		}
		if match {
			return p.popNandPushIntIfOK(argCount+1, startIndex)
		}
	}
	return p.popNandPushIntIfOK(argCount+1, 0)
}

func (p *Primitives) primitiveFindFirstInString(argCount int) bool {
	if argCount != 3 {
		return false
	}
	strObj := p.asObj(p.vm.stackValue(2))
	mapObj := p.asObj(p.vm.stackValue(1))
	start := p.stackInteger(0)
	if !p.success || strObj == nil || mapObj == nil || strObj.Bytes == nil || mapObj.Bytes == nil || len(mapObj.Bytes) != 256 {
		return p.popNandPushIntIfOK(argCount+1, 0)
	}
	str := strObj.Bytes
	incMap := mapObj.Bytes
	for i := start; i <= len(str); i++ {
		if incMap[str[i-1]] != 0 {
			return p.popNandPushIntIfOK(argCount+1, i)
		}
	}
	return p.popNandPushIntIfOK(argCount+1, 0)
}

func (p *Primitives) primitiveIndexOfAsciiInString(argCount int) bool {
	if argCount != 3 {
		return false
	}
	anInt := p.stackInteger(2)
	strObj := p.asObj(p.vm.stackValue(1))
	start := p.stackInteger(0)
	if !p.success || strObj == nil || strObj.Bytes == nil {
		return false
	}
	str := strObj.Bytes
	target := byte(anInt)
	for pos := start; pos <= len(str); pos++ {
		if str[pos-1] == target {
			return p.popNandPushIntIfOK(argCount+1, pos)
		}
	}
	return p.popNandPushIntIfOK(argCount+1, 0)
}

func (p *Primitives) primitiveTranslateStringWithTable(argCount int) bool {
	if argCount != 4 {
		return false
	}
	strObj := p.asObj(p.vm.stackValue(3))
	start := p.stackInteger(2)
	stop := p.stackInteger(1)
	tableObj := p.asObj(p.vm.stackValue(0))
	if !p.success || strObj == nil || tableObj == nil || strObj.Bytes == nil || tableObj.Bytes == nil {
		return false
	}
	str := strObj.Bytes
	table := tableObj.Bytes
	if start < 1 {
		start = 1
	}
	if stop > len(str) {
		stop = len(str)
	}
	for i := start; i <= stop; i++ {
		b := str[i-1]
		if int(b) < len(table) {
			str[i-1] = table[b]
		}
	}
	p.vm.popN(argCount)
	return true
}

func (p *Primitives) primitiveStringHash(argCount int) bool {
	if argCount != 2 {
		return false
	}
	baObj := p.asObj(p.vm.stackValue(1))
	speciesHash := p.stackInteger(0)
	if !p.success || baObj == nil || baObj.Bytes == nil {
		return false
	}
	ba := baObj.Bytes
	hash := speciesHash & 0x0FFFFFFF
	for _, b := range ba {
		hash += int(b)
		low := hash & 16383
		hash = ((9741 * low) + ((((9741 * (hash >> 14)) + (101 * low)) & 16383) * 16384)) & 0x0FFFFFFF
	}
	return p.popNandPushIntIfOK(argCount+1, hash)
}

func (p *Primitives) primitiveCompressToByteArray(argCount int) bool {
	return false
}

func (p *Primitives) primitiveConvert8BitSigned(argCount int) bool {
	return false
}
