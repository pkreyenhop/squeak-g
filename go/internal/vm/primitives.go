package vm

import (
	"fmt"
	"math"
	"os"
	"time"
)

// Primitives is the Go port of Squeak.Primitives. It implements the numbered
// primitive operations. Display/input primitives use a headless policy (most
// fail so the image's Smalltalk fallback runs); a real display can be layered
// on later. Unimplemented primitives warn once and fail.
type Primitives struct {
	vm      *Interpreter
	success bool

	oldPrims           bool
	allowAccessBeyond  bool
	deferDisplay       bool
	semaphoresToSignal []int
	quitFlag           bool

	warned       map[string]bool
	bltCount     int
	bltToDisplay int
	primCounts   []int

	// at-cache (correctness-critical: tells the at:/at:put: bytecodes whether a
	// given receiver class converts elements to Characters).
	atCache       [32]atCacheEntry
	atPutCache    [32]atCacheEntry
	nonCachedInfo atCacheEntry
}

type atCacheEntry struct {
	array        *Object
	convertChars bool
	size         int
	ivarOffset   int
}

// NewPrimitives creates the primitive handler for an interpreter.
func NewPrimitives(vm *Interpreter) *Primitives {
	return &Primitives{
		vm:                vm,
		oldPrims:          !vm.Image.HasClosures,
		allowAccessBeyond: !vm.Image.HasClosures,
		warned:            map[string]bool{},
		primCounts:        make([]int, 700),
	}
}

func (p *Primitives) warnOnce(msg string) {
	if !p.warned[msg] {
		p.warned[msg] = true
		fmt.Fprintln(os.Stderr, "squeak: "+msg)
	}
}

// --- dispatch ------------------------------------------------------------

// quickSendOther handles the 0xC0..0xCF "special selector" bytecodes.
func (p *Primitives) quickSendOther(rcvr Value, lobits int) bool {
	p.success = true
	switch lobits {
	case 0x0:
		return p.popNandPushIfOK(2, p.objectAt(true, true, false))
	case 0x1:
		return p.popNandPushIfOK(3, p.objectAtPut(true, true, false))
	case 0x2:
		return p.popNandPushIfOK(1, p.objectSize(true))
	case 0x6:
		return p.popNandPushBoolIfOK(2, vmIdentical(p.vm.stackValue(1), p.vm.stackValue(0)))
	case 0x7:
		return p.popNandPushIfOK(1, p.vm.getClass(p.vm.top()))
	case 0x8:
		return p.popNandPushIfOK(2, p.doBlockCopy())
	case 0x9:
		return p.primitiveBlockValue(0)
	case 0xA:
		return p.primitiveBlockValue(1)
	}
	return false
}

// doPrimitive dispatches a numbered primitive. Returns true on success.
func (p *Primitives) doPrimitive(index, argCount int, primMethod *Object) bool {
	p.success = true
	if p.primCounts != nil && index < len(p.primCounts) {
		p.primCounts[index]++
	}
	switch index {
	// Integer arithmetic (1-19)
	case 1:
		return p.popNandPushIntIfOK(argCount+1, p.stackInteger(1)+p.stackInteger(0))
	case 2:
		return p.popNandPushIntIfOK(argCount+1, p.stackInteger(1)-p.stackInteger(0))
	case 3:
		return p.popNandPushBoolIfOK(argCount+1, p.stackInteger(1) < p.stackInteger(0))
	case 4:
		return p.popNandPushBoolIfOK(argCount+1, p.stackInteger(1) > p.stackInteger(0))
	case 5:
		return p.popNandPushBoolIfOK(argCount+1, p.stackInteger(1) <= p.stackInteger(0))
	case 6:
		return p.popNandPushBoolIfOK(argCount+1, p.stackInteger(1) >= p.stackInteger(0))
	case 7:
		return p.popNandPushBoolIfOK(argCount+1, p.stackInteger(1) == p.stackInteger(0))
	case 8:
		return p.popNandPushBoolIfOK(argCount+1, p.stackInteger(1) != p.stackInteger(0))
	case 9:
		return p.popNandPushIntIfOK(argCount+1, p.stackInteger(1)*p.stackInteger(0))
	case 10:
		return p.popNandPushIntIfOK(argCount+1, p.vm.quickDivide(p.stackInteger(1), p.stackInteger(0)))
	case 11:
		return p.popNandPushIntIfOK(argCount+1, p.vm.mod(p.stackInteger(1), p.stackInteger(0)))
	case 12:
		return p.popNandPushIntIfOK(argCount+1, p.vm.div(p.stackInteger(1), p.stackInteger(0)))
	case 13:
		return p.popNandPushIntIfOK(argCount+1, intQuo(p.stackInteger(1), p.stackInteger(0)))
	case 14:
		return p.popNandPushIfOK(argCount+1, p.doBitAnd())
	case 15:
		return p.popNandPushIfOK(argCount+1, p.doBitOr())
	case 16:
		return p.popNandPushIfOK(argCount+1, p.doBitXor())
	case 17:
		return p.popNandPushIfOK(argCount+1, p.doBitShift())
	case 18:
		return p.primitiveMakePoint(argCount, false)
	case 19:
		return false // guard primitive, must fail

	// Float arithmetic (40-59)
	case 40:
		return p.popNandPushFloatIfOK(argCount+1, float64(p.stackInteger(0)))
	case 41:
		return p.popNandPushFloatIfOK(argCount+1, p.stackFloat(1)+p.stackFloat(0))
	case 42:
		return p.popNandPushFloatIfOK(argCount+1, p.stackFloat(1)-p.stackFloat(0))
	case 43:
		return p.popNandPushBoolIfOK(argCount+1, p.stackFloat(1) < p.stackFloat(0))
	case 44:
		return p.popNandPushBoolIfOK(argCount+1, p.stackFloat(1) > p.stackFloat(0))
	case 45:
		return p.popNandPushBoolIfOK(argCount+1, p.stackFloat(1) <= p.stackFloat(0))
	case 46:
		return p.popNandPushBoolIfOK(argCount+1, p.stackFloat(1) >= p.stackFloat(0))
	case 47:
		return p.popNandPushBoolIfOK(argCount+1, p.stackFloat(1) == p.stackFloat(0))
	case 48:
		return p.popNandPushBoolIfOK(argCount+1, p.stackFloat(1) != p.stackFloat(0))
	case 49:
		return p.popNandPushFloatIfOK(argCount+1, p.stackFloat(1)*p.stackFloat(0))
	case 50:
		return p.popNandPushFloatIfOK(argCount+1, p.safeFDiv(p.stackFloat(1), p.stackFloat(0)))
	case 51:
		return p.popNandPushIfOK(argCount+1, p.floatAsSmallInt(p.stackFloat(0)))
	case 55:
		return p.popNandPushFloatIfOK(argCount+1, math.Sqrt(p.stackFloat(0)))
	case 56:
		return p.popNandPushFloatIfOK(argCount+1, math.Sin(p.stackFloat(0)))
	case 57:
		return p.popNandPushFloatIfOK(argCount+1, math.Atan(p.stackFloat(0)))
	case 58:
		return p.popNandPushFloatIfOK(argCount+1, math.Log(p.stackFloat(0)))
	case 59:
		return p.popNandPushFloatIfOK(argCount+1, math.Exp(p.stackFloat(0)))

	// Subscript & storage (60-79)
	case 60:
		return p.popNandPushIfOK(argCount+1, p.objectAt(false, false, false))
	case 61:
		return p.popNandPushIfOK(argCount+1, p.objectAtPut(false, false, false))
	case 62:
		return p.popNandPushIfOK(argCount+1, p.objectSize(false))
	case 63:
		return p.popNandPushIfOK(argCount+1, p.objectAt(false, true, false))
	case 64:
		return p.popNandPushIfOK(argCount+1, p.objectAtPut(false, true, false))
	case 68, 73:
		return p.popNandPushIfOK(argCount+1, p.objectAt(false, false, true))
	case 69, 74:
		return p.popNandPushIfOK(argCount+1, p.objectAtPut(false, false, true))
	case 70:
		return p.popNandPushIfOK(argCount+1, p.instantiate(p.stackNonInteger(0), 0))
	case 71:
		return p.popNandPushIfOK(argCount+1, p.instantiate(p.stackNonInteger(1), int(p.stackPos32(0))))
	case 75:
		return p.popNandPushIfOK(argCount+1, p.identityHash(p.stackNonInteger(0)))
	case 77:
		return p.popNandPushIfOK(argCount+1, p.someInstanceOf(p.stackNonInteger(0)))
	case 78:
		return p.popNandPushIfOK(argCount+1, p.nextInstanceAfter(p.stackNonInteger(0)))
	case 79:
		return p.primitiveNewMethod(argCount)

	// Control (80-89)
	case 80:
		return p.popNandPushIfOK(argCount+1, p.doBlockCopy())
	case 81:
		return p.primitiveBlockValue(argCount)
	case 82:
		return p.primitiveBlockValueWithArgs(argCount)
	case 83:
		return p.vm.primitivePerform(argCount)
	case 84:
		return p.vm.primitivePerformWithArgs(argCount, false)
	case 85:
		return p.primitiveSignal()
	case 86:
		return p.primitiveWait()
	case 87:
		return p.primitiveResume()
	case 88:
		return p.primitiveSuspend()
	case 89:
		return p.vm.flushMethodCache()

	// I/O (90-109) -- headless policy
	case 90:
		return false // mousePoint
	case 91:
		return false // testDisplayDepth
	case 93:
		return p.primitiveInputSemaphore(argCount)
	case 94:
		return false // getNextEvent
	case 95:
		return false // inputWord
	case 96:
		return p.namedBitBltCopyBits(argCount)
	case 100:
		return p.vm.primitivePerformWithArgs(argCount, true)
	case 101:
		p.vm.popN(argCount) // beCursor -> self
		return true
	case 102: // beDisplay: register the receiver as TheDisplay (special obj #14)
		if displayObj := p.asObj(p.vm.stackValue(0)); displayObj != nil {
			p.vm.SpecialObjects[SplObTheDisplay] = displayObj
		}
		p.vm.popN(argCount)
		return true
	case 105:
		return p.popNandPushIfOK(argCount+1, p.doStringReplace())
	case 106:
		return false // screenSize
	case 107:
		return false // mouseButtons
	case 108:
		return false // kbdNext
	case 109:
		return false // kbdPeek

	// System (110-119)
	case 110:
		return p.popNandPushBoolIfOK(argCount+1, vmIdentical(p.vm.stackValue(1), p.vm.stackValue(0)))
	case 111:
		return p.popNandPushIfOK(argCount+1, p.vm.getClass(p.vm.top()))
	case 112:
		return p.popNandPushIfOK(argCount+1, p.vm.Image.BytesLeft())
	case 113:
		return p.primitiveQuit(argCount)
	case 116, 119:
		return p.vm.flushMethodCache()
	case 117:
		return p.doNamedPrimitive(argCount, primMethod)

	// Misc (120-149)
	case 121:
		return p.primitiveImageName(argCount)
	case 122:
		return false // reverseDisplay
	case 124:
		return p.popNandPushIfOK(argCount+1, p.registerSemaphore(SplObTheLowSpaceSemaphore))
	case 125:
		return p.popNandPushIfOK(argCount+1, p.setLowSpaceThreshold())
	case 126:
		return false // deferDisplayUpdates
	case 127:
		return p.popNIfOK(argCount) // showDisplayRect (no-op flush)
	case 129:
		return p.popNandPushIfOK(argCount+1, p.vm.Image.SpecialObjectsArray)
	case 130:
		return p.popNandPushIfOK(argCount+1, p.vm.Image.BytesLeft()) // fullGC -> bytesLeft
	case 131:
		return p.popNandPushIfOK(argCount+1, p.vm.Image.BytesLeft()) // partialGC -> bytesLeft
	case 132:
		return p.popNandPushBoolIfOK(argCount+1, p.pointsTo(p.stackNonInteger(1), p.vm.top()))
	case 133:
		return p.popNIfOK(argCount) // setInterruptKey
	case 134:
		return p.popNandPushIfOK(argCount+1, p.registerSemaphore(SplObTheInterruptSemaphore))
	case 135:
		return p.popNandPushIfOK(argCount+1, p.millisecondClockValue())
	case 136:
		return p.primitiveSignalAtMilliseconds(argCount)
	case 137:
		return p.popNandPushIfOK(argCount+1, p.secondClock())
	case 138:
		return p.popNandPushIfOK(argCount+1, p.vm.Image.FirstOldObject)
	case 139:
		return p.popNandPushIfOK(argCount+1, p.nextObject(p.vm.top()))
	case 140:
		return false // beep
	case 141:
		return false // clipboard
	case 142:
		return p.popNandPushIfOK(argCount+1, p.makeStString("/SqueakG"))
	case 148:
		return p.popNandPushIfOK(argCount+1, p.vm.Image.Clone(p.asObj(p.vm.top())))
	case 149:
		return p.primitiveGetAttribute(argCount)
	case 161: // primitiveDirectoryDelimitor (old images): answer $/
		if p.oldPrims {
			return p.popNandPushIfOK(argCount+1, p.charFromInt('/'))
		}
		return false

	// Other (230-254)
	case 230:
		return p.primitiveRelinquishProcessor(argCount)
	case 231:
		return true // forceDisplayUpdate (no-op)
	case 233:
		return false // setFullScreen
	case 240:
		return p.popNandPushIfOK(argCount+1, p.millisecondClockValue()*1000) // microsecondClockUTC approx
	case 241:
		return p.popNandPushIfOK(argCount+1, p.millisecondClockValue()*1000)
	case 254:
		return p.primitiveVMParameter(argCount)

	// Named-primitive / plugin numbers we route or fail.
	case 576:
		return p.vm.primitiveInvokeObjectAsMethod(argCount, primMethod)
	}
	p.warnOnce(fmt.Sprintf("missing primitive: %d", index))
	return false
}

func (p *Primitives) namedBitBltCopyBits(argCount int) bool {
	return p.primitiveCopyBits(argCount)
}

func (p *Primitives) doNamedPrimitive(argCount int, primMethod *Object) bool {
	if primMethod == nil || len(primMethod.Pointers) < 2 {
		return false
	}
	first := p.asObj(primMethod.Pointers[1])
	if first == nil || len(first.Pointers) != 4 {
		return false
	}
	module := p.asObj(first.Pointers[0]).BytesAsString()
	fn := p.asObj(first.Pointers[1]).BytesAsString()
	p.warnOnce("missing named primitive: " + module + "." + fn)
	return false
}

var _ = time.Now
