package vm

// A compact, pixel-based BitBlt. This is not the word-optimized generated
// plugin; it implements the classic Smalltalk-80 BitBlt semantics at the pixel
// level (get source pixel, AND with halftone, combine with dest via the
// combination rule, store). Correct for all depths and the boolean rules 0-15,
// which covers the 1-bit MVC display. Non-boolean rules fall back to store.

// BitBlt instance-variable indices.
const (
	bbDestForm     = 0
	bbSourceForm   = 1
	bbHalftoneForm = 2
	bbRule         = 3
	bbDestX        = 4
	bbDestY        = 5
	bbWidth        = 6
	bbHeight       = 7
	bbSourceX      = 8
	bbSourceY      = 9
	bbClipX        = 10
	bbClipY        = 11
	bbClipWidth    = 12
	bbClipHeight   = 13
	bbColorMap     = 14

	formBits   = 0
	formWidth  = 1
	formHeight = 2
	formDepth  = 3
)

type bltForm struct {
	bits   []uint32
	width  int
	height int
	depth  int
	ppw    int // pixels per word
	pitch  int // words per row
}

func newBltForm(o *Object) *bltForm {
	if o == nil || len(o.Pointers) < 4 {
		return nil
	}
	bitsObj, _ := o.Pointers[formBits].(*Object)
	if bitsObj == nil || bitsObj.Words == nil {
		return nil
	}
	depth := asIntValue(o.Pointers[formDepth])
	if depth <= 0 {
		return nil
	}
	w := asIntValue(o.Pointers[formWidth])
	h := asIntValue(o.Pointers[formHeight])
	ppw := 32 / depth
	return &bltForm{
		bits:   bitsObj.Words,
		width:  w,
		height: h,
		depth:  depth,
		ppw:    ppw,
		pitch:  (w*depth + 31) >> 5,
	}
}

func (f *bltForm) mask() uint32 { return (1 << uint(f.depth)) - 1 }

func (f *bltForm) pixelAt(x, y int) uint32 {
	idx := y*f.pitch + x/f.ppw
	if idx < 0 || idx >= len(f.bits) {
		return 0
	}
	shift := 32 - (x%f.ppw+1)*f.depth
	return (f.bits[idx] >> uint(shift)) & f.mask()
}

func (f *bltForm) pixelPut(x, y int, val uint32) {
	idx := y*f.pitch + x/f.ppw
	if idx < 0 || idx >= len(f.bits) {
		return
	}
	shift := uint(32 - (x%f.ppw+1)*f.depth)
	posMask := f.mask() << shift
	f.bits[idx] = (f.bits[idx] &^ posMask) | ((val & f.mask()) << shift)
}

// mergeRule applies boolean combination rules 0-15 at the pixel level.
func mergeRule(rule int, src, dst, mask uint32) uint32 {
	switch rule {
	case 0:
		return 0
	case 1:
		return src & dst
	case 2:
		return src &^ dst
	case 3:
		return src
	case 4:
		return (^src & mask) & dst
	case 5:
		return dst
	case 6:
		return src ^ dst
	case 7:
		return src | dst
	case 8:
		return ^(src | dst) & mask
	case 9:
		return ^(src ^ dst) & mask
	case 10:
		return ^dst & mask
	case 11:
		return src | (^dst & mask)
	case 12:
		return ^src & mask
	case 13:
		return (^src & mask) | dst
	case 14:
		return ^(src & dst) & mask
	case 15:
		return mask
	default:
		return src // non-boolean rules: approximate with store
	}
}

func fetchIntOr(o *Object, index, def int) int {
	if index >= len(o.Pointers) {
		return def
	}
	switch v := o.Pointers[index].(type) {
	case int:
		return v
	case *Object:
		if v.IsFloat {
			return int(v.Float)
		}
	}
	return def
}

// primitiveCopyBits implements primitive 96 (BitBlt>>copyBits).
func (p *Primitives) primitiveCopyBits(argCount int) bool {
	bb := p.asObj(p.vm.stackValue(argCount))
	if bb == nil || len(bb.Pointers) < 15 {
		return false
	}
	if !p.copyBits(bb) {
		return false
	}
	p.vm.popN(argCount) // leave receiver (self) on the stack
	return true
}

func (p *Primitives) copyBits(bb *Object) bool {
	rule := fetchIntOr(bb, bbRule, 0)

	destObj := p.asObj(bb.Pointers[bbDestForm])
	dest := newBltForm(destObj)
	if dest == nil {
		return false
	}
	sourceObj := p.asObj(bb.Pointers[bbSourceForm])
	noSource := sourceObj == nil || sourceObj.IsNil
	var src *bltForm
	if !noSource {
		src = newBltForm(sourceObj)
		if src == nil {
			return false
		}
	}

	// Halftone: a Bitmap (words) or a Form; rows tile vertically.
	var htWords []uint32
	if ht := p.asObj(bb.Pointers[bbHalftoneForm]); ht != nil && !ht.IsNil {
		if ht.Words != nil {
			htWords = ht.Words
		} else if hf := newBltForm(ht); hf != nil {
			htWords = hf.bits
		}
	}

	// Color map (source pixel value -> dest pixel value).
	var colorMap []uint32
	if cm := p.asObj(bb.Pointers[bbColorMap]); cm != nil && !cm.IsNil && cm.Words != nil {
		colorMap = cm.Words
	}

	destX := fetchIntOr(bb, bbDestX, 0)
	destY := fetchIntOr(bb, bbDestY, 0)
	w := fetchIntOr(bb, bbWidth, dest.width)
	h := fetchIntOr(bb, bbHeight, dest.height)
	sourceX := fetchIntOr(bb, bbSourceX, 0)
	sourceY := fetchIntOr(bb, bbSourceY, 0)
	clipX := fetchIntOr(bb, bbClipX, 0)
	clipY := fetchIntOr(bb, bbClipY, 0)
	clipW := fetchIntOr(bb, bbClipWidth, dest.width)
	clipH := fetchIntOr(bb, bbClipHeight, dest.height)

	// Clamp clip rect to dest bounds.
	if clipX < 0 {
		clipW += clipX
		clipX = 0
	}
	if clipY < 0 {
		clipH += clipY
		clipY = 0
	}
	if clipX+clipW > dest.width {
		clipW = dest.width - clipX
	}
	if clipY+clipH > dest.height {
		clipH = dest.height - clipY
	}

	// Clip the blit rectangle against the clip rect, adjusting source origin.
	if destX < clipX {
		d := clipX - destX
		w -= d
		destX += d
		sourceX += d
	}
	if destY < clipY {
		d := clipY - destY
		h -= d
		destY += d
		sourceY += d
	}
	if destX+w > clipX+clipW {
		w = clipX + clipW - destX
	}
	if destY+h > clipY+clipH {
		h = clipY + clipH - destY
	}
	// Clip against source bounds.
	if !noSource {
		if sourceX < 0 {
			w += sourceX
			destX -= sourceX
			sourceX = 0
		}
		if sourceY < 0 {
			h += sourceY
			destY -= sourceY
			sourceY = 0
		}
		if sourceX+w > src.width {
			w = src.width - sourceX
		}
		if sourceY+h > src.height {
			h = src.height - sourceY
		}
	}
	p.bltCount++
	if destObj == p.vm.Image.SpecialObject(SplObTheDisplay) {
		p.bltToDisplay++
	}
	if w <= 0 || h <= 0 {
		return true // nothing to draw, but still a success
	}

	mask := dest.mask()
	overlap := !noSource && sourceObj == destObj

	// Direction for overlapping same-form copies (scrolling).
	vStart, vEnd, vStep := 0, h, 1
	if overlap && destY > sourceY {
		vStart, vEnd, vStep = h-1, -1, -1
	}
	hStart, hEnd, hStep := 0, w, 1
	if overlap && destX > sourceX {
		hStart, hEnd, hStep = w-1, -1, -1
	}

	for yy := vStart; yy != vEnd; yy += vStep {
		dy := destY + yy
		var htPixelRow uint32
		haveHT := len(htWords) > 0
		if haveHT {
			htPixelRow = htWords[dy%len(htWords)]
		}
		for xx := hStart; xx != hEnd; xx += hStep {
			dx := destX + xx
			var srcVal uint32
			if noSource {
				srcVal = mask
			} else {
				srcVal = src.pixelAt(sourceX+xx, sourceY+yy)
				if colorMap != nil && int(srcVal) < len(colorMap) {
					srcVal = colorMap[srcVal] & mask
				}
			}
			if haveHT {
				// Extract the dest-depth halftone pixel for this column.
				ht := (htPixelRow >> uint(32-(dx%dest.ppw+1)*dest.depth)) & mask
				srcVal &= ht
			}
			dstVal := dest.pixelAt(dx, dy)
			dest.pixelPut(dx, dy, mergeRule(rule, srcVal, dstVal, mask)&mask)
		}
	}
	return true
}

// BltStats returns how many BitBlts ran and how many targeted the Display.
func (p *Primitives) BltStats() (total, toDisplay int) { return p.bltCount, p.bltToDisplay }

// PrimCounts returns the per-primitive call counts (index -> count).
func (p *Primitives) PrimCounts() []int { return p.primCounts }
