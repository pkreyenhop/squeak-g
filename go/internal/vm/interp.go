package vm

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// Interpreter is the Go port of Squeak.Interpreter: the bytecode engine.
// It supports the classic V3 (non-Sista) bytecode set used by mini.image.
type Interpreter struct {
	Image *Image
	prim  *Primitives

	SpecialObjects   []Value
	SpecialSelectors []Value
	NilObj           *Object
	TrueObj          *Object
	FalseObj         *Object
	HasClosures      bool

	// Execution registers.
	ActiveContext *Object
	HomeContext   *Object
	Method        *Object
	Receiver      Value
	PC            int // absolute index into Method.Bytes
	SP            int // absolute index into ActiveContext.Pointers

	// Arithmetic scratch flags (mirror the JS instance vars).
	success       bool
	resultIsFloat bool
	primFailCode  int

	// Context recycling free lists (nil object terminated).
	freeContexts            *Object
	freeLargeContexts       *Object
	reclaimableContextCount int

	// Method cache.
	methodCache     []methodCacheEntry
	methodCacheMask int
	methodCacheRand int

	// Interrupt / scheduling.
	interruptCheckCounter              int
	interruptCheckCounterFeedBackReset int
	nextWakeupTick                     int
	signalLowSpace                     bool

	currentSelector Value
	// at-cache validation: set when a primitive method is selected, so the
	// at:/at:put: bytecodes know whether the receiver's class converts to chars.
	verifyAtSelector *Object
	verifyAtClass    *Object

	ByteCodeCount int
	SendCount     int

	breakOut    bool
	isIdle      bool
	evalBooted  bool
	startupTime time.Time
}

type methodCacheEntry struct {
	lkupClass *Object
	selector  *Object
	method    *Object
	primIndex int
	argCount  int
	mClass    *Object
}

// NewInterpreter builds an interpreter over a loaded image and prepares the
// initial context.
func NewInterpreter(img *Image) (*Interpreter, error) {
	vm := &Interpreter{Image: img, startupTime: time.Now()}
	vm.methodCache = make([]methodCacheEntry, 1024)
	vm.methodCacheMask = 1023
	vm.interruptCheckCounterFeedBackReset = 1000
	vm.interruptCheckCounter = 1000
	if err := vm.loadImageState(); err != nil {
		return nil, err
	}
	vm.prim = NewPrimitives(vm)
	if err := vm.loadInitialContext(); err != nil {
		return nil, err
	}
	return vm, nil
}

func (vm *Interpreter) loadImageState() error {
	vm.SpecialObjects = vm.Image.SpecialObjectsArray.Pointers
	sel, ok := vm.SpecialObjects[SplObSpecialSelectors].(*Object)
	if !ok {
		return errors.New("special selectors array missing")
	}
	vm.SpecialSelectors = sel.Pointers
	vm.NilObj = vm.Image.SpecialObject(SplObNilObject)
	vm.TrueObj = vm.Image.SpecialObject(SplObTrueObject)
	vm.FalseObj = vm.Image.SpecialObject(SplObFalseObject)
	vm.HasClosures = vm.Image.HasClosures
	vm.freeContexts = vm.NilObj
	vm.freeLargeContexts = vm.NilObj
	return nil
}

func (vm *Interpreter) loadInitialContext() error {
	schedAssn, ok := vm.SpecialObjects[SplObSchedulerAssociation].(*Object)
	if !ok {
		return errors.New("scheduler association missing")
	}
	sched, ok := schedAssn.Pointers[AssnValue].(*Object)
	if !ok {
		return errors.New("scheduler missing")
	}
	proc, ok := sched.Pointers[ProcSchedActiveProcess].(*Object)
	if !ok {
		return errors.New("active process missing")
	}
	ctx, ok := proc.Pointers[ProcSuspendedContext].(*Object)
	if !ok {
		return errors.New("suspended context missing")
	}
	vm.ActiveContext = ctx
	ctx.Dirty = true
	vm.fetchContextRegisters(ctx)
	vm.reclaimableContextCount = 0
	return nil
}

// --- context register save/restore ---------------------------------------

func (vm *Interpreter) fetchContextRegisters(ctx *Object) {
	meth := ctx.Pointers[ContextMethod]
	if _, isInt := meth.(int); isInt {
		home := ctx.Pointers[BlockContextHome].(*Object)
		vm.HomeContext = home
		meth = home.Pointers[ContextMethod]
	} else {
		vm.HomeContext = ctx
	}
	vm.Receiver = vm.HomeContext.Pointers[ContextReceiver]
	vm.Method = meth.(*Object)
	vm.PC = vm.decodeSqueakPC(asIntValue(ctx.Pointers[ContextInstructionPointer]), vm.Method)
	vm.SP = vm.decodeSqueakSP(asIntValue(ctx.Pointers[ContextStackPointer]))
}

func (vm *Interpreter) storeContextRegisters() {
	vm.ActiveContext.Pointers[ContextInstructionPointer] = vm.encodeSqueakPC(vm.PC, vm.Method)
	vm.ActiveContext.Pointers[ContextStackPointer] = vm.encodeSqueakSP(vm.SP)
}

func (vm *Interpreter) encodeSqueakPC(intPC int, method *Object) int {
	return intPC + len(method.Pointers)*4 + 1
}
func (vm *Interpreter) decodeSqueakPC(squeakPC int, method *Object) int {
	return squeakPC - len(method.Pointers)*4 - 1
}
func (vm *Interpreter) encodeSqueakSP(intSP int) int {
	return intSP - (ContextTempFrameStart - 1)
}
func (vm *Interpreter) decodeSqueakSP(squeakSP int) int {
	return squeakSP + (ContextTempFrameStart - 1)
}

// --- run loop ------------------------------------------------------------

// Run executes bytecodes until maxBytecodes is reached (0 = unlimited) or the
// interpreter breaks out (idle). Returns the number of bytecodes executed.
func (vm *Interpreter) Run(maxBytecodes int) int {
	start := vm.ByteCodeCount
	vm.breakOut = false
	vm.isIdle = false
	for !vm.breakOut {
		vm.interpretOne()
		if maxBytecodes > 0 && vm.ByteCodeCount-start >= maxBytecodes {
			break
		}
	}
	return vm.ByteCodeCount - start
}

// Idle reports whether the last Run stopped because the image went idle (as
// opposed to a display/frame breakout or a bytecode cap).
func (vm *Interpreter) Idle() bool { return vm.isIdle }

// Quitting reports whether the image asked to quit (primitiveQuit).
func (vm *Interpreter) Quitting() bool { return vm.prim.quitFlag }

// BootToIdle repeatedly runs UI cycles until the image is genuinely idle or a
// safety cap is hit. Used for headless boot-to-snapshot.
func (vm *Interpreter) BootToIdle(perCycleCap int) {
	for i := 0; i < 200000; i++ {
		vm.Run(perCycleCap)
		if vm.isIdle {
			return
		}
	}
}

// Boot runs UI cycles until the image goes idle or maxTotal bytecodes have been
// executed. Some images (e.g. Squeak 1.1) never signal idle because their
// background process busy-loops, so a total budget is needed to bound startup.
func (vm *Interpreter) Boot(maxTotal int) {
	start := vm.ByteCodeCount
	for {
		vm.Run(2_000_000)
		if vm.isIdle || vm.ByteCodeCount-start >= maxTotal {
			return
		}
	}
}

func (vm *Interpreter) nextByte() int {
	b := int(vm.Method.Bytes[vm.PC])
	vm.PC++
	return b
}

func (vm *Interpreter) nono() {
	panic(fmt.Sprintf("Oh No! (unexpected bytecode at pc=%d)", vm.PC-1))
}

// --- main V3 dispatch loop -----------------------------------------------

func (vm *Interpreter) interpretOne() {
	vm.ByteCodeCount++
	b := vm.nextByte()
	switch {
	case b <= 0x0F: // push receiver variable
		vm.push(vm.Receiver.(*Object).Pointers[b&0xF])
	case b <= 0x1F: // push temporary variable
		vm.push(vm.HomeContext.Pointers[ContextTempFrameStart+(b&0xF)])
	case b <= 0x3F: // push literal constant
		vm.push(vm.Method.MethodGetLiteral(b & 0x1F))
	case b <= 0x5F: // push literal variable (Association value)
		vm.push(vm.Method.MethodGetLiteral(b & 0x1F).(*Object).Pointers[AssnValue])
	case b <= 0x67: // pop into receiver variable
		r := vm.Receiver.(*Object)
		r.Dirty = true
		r.Pointers[b&7] = vm.pop()
	case b <= 0x6F: // pop into temporary variable
		vm.HomeContext.Pointers[ContextTempFrameStart+(b&7)] = vm.pop()
	case b == 0x70:
		vm.push(vm.Receiver)
	case b == 0x71:
		vm.push(vm.TrueObj)
	case b == 0x72:
		vm.push(vm.FalseObj)
	case b == 0x73:
		vm.push(vm.NilObj)
	case b == 0x74:
		vm.push(-1)
	case b == 0x75:
		vm.push(0)
	case b == 0x76:
		vm.push(1)
	case b == 0x77:
		vm.push(2)
	case b == 0x78:
		vm.doReturn(vm.Receiver, nil)
	case b == 0x79:
		vm.doReturn(vm.TrueObj, nil)
	case b == 0x7A:
		vm.doReturn(vm.FalseObj, nil)
	case b == 0x7B:
		vm.doReturn(vm.NilObj, nil)
	case b == 0x7C:
		vm.doReturn(vm.pop(), nil)
	case b == 0x7D: // blockReturn
		vm.doReturn(vm.pop(), vm.asObj(vm.ActiveContext.Pointers[BlockContextCaller]))
	case b == 0x7E, b == 0x7F:
		vm.nono()
	case b == 0x80:
		vm.extendedPush(vm.nextByte())
	case b == 0x81:
		vm.extendedStore(vm.nextByte())
	case b == 0x82:
		vm.extendedStorePop(vm.nextByte())
	case b == 0x83:
		b2 := vm.nextByte()
		vm.send(vm.selectorObj(b2&31), b2>>5, false)
	case b == 0x84:
		vm.doubleExtendedDoAnything(vm.nextByte())
	case b == 0x85:
		b2 := vm.nextByte()
		vm.send(vm.selectorObj(b2&31), b2>>5, true)
	case b == 0x86:
		b2 := vm.nextByte()
		vm.send(vm.selectorObj(b2&63), b2>>6, false)
	case b == 0x87:
		vm.pop()
	case b == 0x88:
		vm.push(vm.top())
	case b == 0x89:
		vm.push(vm.exportThisContext())
	case b == 0x8A: // pushNewArray
		vm.pushNewArray(vm.nextByte())
	case b == 0x8B:
		vm.callPrimBytecode(0x81)
	case b == 0x8C: // remote push from temp vector
		b2 := vm.nextByte()
		vec := vm.HomeContext.Pointers[ContextTempFrameStart+vm.nextByte()].(*Object)
		vm.push(vec.Pointers[b2])
	case b == 0x8D: // remote store into temp vector
		b2 := vm.nextByte()
		vec := vm.HomeContext.Pointers[ContextTempFrameStart+vm.nextByte()].(*Object)
		vec.Pointers[b2] = vm.top()
		vec.Dirty = true
	case b == 0x8E: // remote store and pop into temp vector
		b2 := vm.nextByte()
		vec := vm.HomeContext.Pointers[ContextTempFrameStart+vm.nextByte()].(*Object)
		vec.Pointers[b2] = vm.pop()
		vec.Dirty = true
	case b == 0x8F:
		vm.pushClosureCopy()
	case b <= 0x97: // short unconditional jump
		vm.PC += (b & 7) + 1
	case b <= 0x9F: // short jump if false
		vm.jumpIfFalse((b & 7) + 1)
	case b <= 0xA7: // long jump forward/back
		b2 := vm.nextByte()
		vm.PC += ((b&7)-4)*256 + b2
		if (b & 7) < 4 {
			vm.interruptCheckCounter--
			if vm.interruptCheckCounter <= 0 {
				vm.checkForInterrupts()
			}
		}
	case b <= 0xAB: // long jump if true
		vm.jumpIfTrue((b&3)*256 + vm.nextByte())
	case b <= 0xAF: // long jump if false
		vm.jumpIfFalse((b&3)*256 + vm.nextByte())
	case b <= 0xBF: // arithmetic / special bytecode selectors
		vm.bytecodePrimArith(b)
	case b <= 0xCF: // at:, at:put:, size, ==, class, ...
		if !vm.prim.quickSendOther(vm.Receiver, b&0xF) {
			vm.sendSpecial((b & 0xF) + 16)
		}
	case b <= 0xDF: // send literal selector, 0 args
		vm.send(vm.selectorObj(b&0xF), 0, false)
	case b <= 0xEF: // send literal selector, 1 arg
		vm.send(vm.selectorObj(b&0xF), 1, false)
	default: // 0xF0..0xFF: send literal selector, 2 args
		vm.send(vm.selectorObj(b&0xF), 2, false)
	}
}

func (vm *Interpreter) selectorObj(index int) *Object {
	return vm.Method.MethodGetLiteral(index).(*Object)
}

// bytecodePrimArith handles bytecodes 0xB0..0xBF (arithmetic & special selectors).
func (vm *Interpreter) bytecodePrimArith(b int) {
	vm.success = true
	vm.resultIsFloat = false
	switch b {
	case 0xB0:
		if !vm.pop2AndPushNumResult(vm.stackIntOrFloat(1) + vm.stackIntOrFloat(0)) {
			vm.sendSpecial(b & 0xF)
		}
	case 0xB1:
		if !vm.pop2AndPushNumResult(vm.stackIntOrFloat(1) - vm.stackIntOrFloat(0)) {
			vm.sendSpecial(b & 0xF)
		}
	case 0xB2:
		if !vm.pop2AndPushBoolResult(vm.stackIntOrFloat(1) < vm.stackIntOrFloat(0)) {
			vm.sendSpecial(b & 0xF)
		}
	case 0xB3:
		if !vm.pop2AndPushBoolResult(vm.stackIntOrFloat(1) > vm.stackIntOrFloat(0)) {
			vm.sendSpecial(b & 0xF)
		}
	case 0xB4:
		if !vm.pop2AndPushBoolResult(vm.stackIntOrFloat(1) <= vm.stackIntOrFloat(0)) {
			vm.sendSpecial(b & 0xF)
		}
	case 0xB5:
		if !vm.pop2AndPushBoolResult(vm.stackIntOrFloat(1) >= vm.stackIntOrFloat(0)) {
			vm.sendSpecial(b & 0xF)
		}
	case 0xB6:
		if !vm.pop2AndPushBoolResult(vm.stackIntOrFloat(1) == vm.stackIntOrFloat(0)) {
			vm.sendSpecial(b & 0xF)
		}
	case 0xB7:
		if !vm.pop2AndPushBoolResult(vm.stackIntOrFloat(1) != vm.stackIntOrFloat(0)) {
			vm.sendSpecial(b & 0xF)
		}
	case 0xB8:
		if !vm.pop2AndPushNumResult(vm.stackIntOrFloat(1) * vm.stackIntOrFloat(0)) {
			vm.sendSpecial(b & 0xF)
		}
	case 0xB9:
		if !vm.pop2AndPushIntResult(vm.quickDivide(vm.stackInteger(1), vm.stackInteger(0))) {
			vm.sendSpecial(b & 0xF)
		}
	case 0xBA:
		if !vm.pop2AndPushIntResult(vm.mod(vm.stackInteger(1), vm.stackInteger(0))) {
			vm.sendSpecial(b & 0xF)
		}
	case 0xBB:
		if !vm.prim.primitiveMakePoint(1, true) {
			vm.sendSpecial(b & 0xF)
		}
	case 0xBC:
		if !vm.pop2AndPushIntResult(vm.safeShift(vm.stackInteger(1), vm.stackInteger(0))) {
			vm.sendSpecial(b & 0xF)
		}
	case 0xBD:
		if !vm.pop2AndPushIntResult(vm.div(vm.stackInteger(1), vm.stackInteger(0))) {
			vm.sendSpecial(b & 0xF)
		}
	case 0xBE:
		if !vm.pop2AndPushIntResult(vm.stackInteger(1) & vm.stackInteger(0)) {
			vm.sendSpecial(b & 0xF)
		}
	case 0xBF:
		if !vm.pop2AndPushIntResult(vm.stackInteger(1) | vm.stackInteger(0)) {
			vm.sendSpecial(b & 0xF)
		}
	}
}

// --- extended pushes/stores ----------------------------------------------

func (vm *Interpreter) extendedPush(nextByte int) {
	lobits := nextByte & 63
	switch nextByte >> 6 {
	case 0:
		vm.push(vm.Receiver.(*Object).Pointers[lobits])
	case 1:
		vm.push(vm.HomeContext.Pointers[ContextTempFrameStart+lobits])
	case 2:
		vm.push(vm.Method.MethodGetLiteral(lobits))
	case 3:
		vm.push(vm.Method.MethodGetLiteral(lobits).(*Object).Pointers[AssnValue])
	}
}

func (vm *Interpreter) extendedStore(nextByte int) {
	lobits := nextByte & 63
	switch nextByte >> 6 {
	case 0:
		r := vm.Receiver.(*Object)
		r.Dirty = true
		r.Pointers[lobits] = vm.top()
	case 1:
		vm.HomeContext.Pointers[ContextTempFrameStart+lobits] = vm.top()
	case 2:
		vm.nono()
	case 3:
		assoc := vm.Method.MethodGetLiteral(lobits).(*Object)
		assoc.Dirty = true
		assoc.Pointers[AssnValue] = vm.top()
	}
}

func (vm *Interpreter) extendedStorePop(nextByte int) {
	lobits := nextByte & 63
	switch nextByte >> 6 {
	case 0:
		r := vm.Receiver.(*Object)
		r.Dirty = true
		r.Pointers[lobits] = vm.pop()
	case 1:
		vm.HomeContext.Pointers[ContextTempFrameStart+lobits] = vm.pop()
	case 2:
		vm.nono()
	case 3:
		assoc := vm.Method.MethodGetLiteral(lobits).(*Object)
		assoc.Dirty = true
		assoc.Pointers[AssnValue] = vm.pop()
	}
}

func (vm *Interpreter) doubleExtendedDoAnything(byte2 int) {
	byte3 := vm.nextByte()
	switch byte2 >> 5 {
	case 0:
		vm.send(vm.selectorObj(byte3), byte2&31, false)
	case 1:
		vm.send(vm.selectorObj(byte3), byte2&31, true)
	case 2:
		vm.push(vm.Receiver.(*Object).Pointers[byte3])
	case 3:
		vm.push(vm.Method.MethodGetLiteral(byte3))
	case 4:
		vm.push(vm.Method.MethodGetLiteral(byte3).(*Object).Pointers[AssnValue])
	case 5:
		r := vm.Receiver.(*Object)
		r.Dirty = true
		r.Pointers[byte3] = vm.top()
	case 6:
		r := vm.Receiver.(*Object)
		r.Dirty = true
		r.Pointers[byte3] = vm.pop()
	case 7:
		assoc := vm.Method.MethodGetLiteral(byte3).(*Object)
		assoc.Dirty = true
		assoc.Pointers[AssnValue] = vm.top()
	}
}

func (vm *Interpreter) jumpIfTrue(delta int) {
	top := vm.pop()
	if to, ok := top.(*Object); ok {
		if to.IsTrue {
			vm.PC += delta
			return
		}
		if to.IsFalse {
			return
		}
	}
	vm.push(top)
	vm.send(vm.SpecialObjects[SplObSelectorMustBeBoolean].(*Object), 0, false)
}

func (vm *Interpreter) jumpIfFalse(delta int) {
	top := vm.pop()
	if to, ok := top.(*Object); ok {
		if to.IsFalse {
			vm.PC += delta
			return
		}
		if to.IsTrue {
			return
		}
	}
	vm.push(top)
	vm.send(vm.SpecialObjects[SplObSelectorMustBeBoolean].(*Object), 0, false)
}

func (vm *Interpreter) sendSpecial(lobits int) {
	sel := vm.SpecialSelectors[lobits*2].(*Object)
	argCount := asIntValue(vm.SpecialSelectors[lobits*2+1])
	vm.send(sel, argCount, false)
}

func (vm *Interpreter) callPrimBytecode(extendedStoreBytecode int) {
	vm.PC += 2 // skip over primitive number
	if vm.primFailCode != 0 {
		if int(vm.Method.Bytes[vm.PC]) == extendedStoreBytecode {
			vm.stackTopPut(vm.getErrorObjectFromPrimFailCode())
		}
		vm.primFailCode = 0
	}
}

func (vm *Interpreter) getErrorObjectFromPrimFailCode() Value {
	if t, ok := vm.SpecialObjects[SplObPrimErrTableIndex].(*Object); ok && t.Pointers != nil {
		if vm.primFailCode-1 < len(t.Pointers) {
			return t.Pointers[vm.primFailCode-1]
		}
	}
	return vm.primFailCode
}

// SplObPrimErrTableIndex is index 51 (kept local; not in the trimmed constant set).
const SplObPrimErrTableIndex = 51

// spl safely reads a special object. Old images have a short special-objects
// array; indices beyond it read as nil (matching JS's undefined -> falsy).
func (vm *Interpreter) spl(i int) Value {
	if i < len(vm.SpecialObjects) {
		return vm.SpecialObjects[i]
	}
	return vm.NilObj
}

func asIntValue(v Value) int {
	if n, ok := v.(int); ok {
		return n
	}
	return 0
}

func (vm *Interpreter) asObj(v Value) *Object {
	if o, ok := v.(*Object); ok {
		return o
	}
	return nil
}

// Interpret is retained for compatibility; it runs until idle.
func (vm *Interpreter) Interpret() error {
	vm.Run(0)
	return nil
}

// DescribeInitialContext returns a one-line description of where execution begins.
func (vm *Interpreter) DescribeInitialContext() string {
	recv := "?"
	if r, ok := vm.Receiver.(*Object); ok {
		recv = r.SqInstName()
	} else if n, ok := vm.Receiver.(int); ok {
		recv = fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("initial context: receiver=%s pc=%d sp=%d numLits=%d numArgs=%d prim=%d",
		recv, vm.PC, vm.SP, vm.Method.MethodNumLits(), vm.Method.MethodNumArgs(), vm.Method.MethodPrimitiveIndex())
}

var _ = math.Sqrt // keep math imported for helpers used across files

// BltStats returns BitBlt counters (for diagnostics).
func (vm *Interpreter) BltStats() (total, toDisplay int) { return vm.prim.BltStats() }

// PrimCounts exposes primitive call counts for profiling.
func (vm *Interpreter) PrimCounts() []int { return vm.prim.PrimCounts() }
