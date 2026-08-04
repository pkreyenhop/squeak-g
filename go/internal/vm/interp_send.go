package vm

import (
	"fmt"
	"math"
)

// --- sending & method lookup ---------------------------------------------

func (vm *Interpreter) send(selector *Object, argCount int, doSuper bool) {
	newRcvr := vm.stackValue(argCount)
	var lookupClass *Object
	if doSuper {
		lookupClass = vm.Method.MethodClassForSuper().Superclass()
	} else {
		lookupClass = vm.getClass(newRcvr)
	}
	entry := vm.findSelectorInClass(selector, argCount, lookupClass, argCount)
	vm.executeNewMethod(newRcvr, entry.method, entry.argCount, entry.primIndex, entry.mClass, selector)
}

func (vm *Interpreter) findSelectorInClass(selector *Object, trueArgCount int, startingClass *Object, argCount int) *methodCacheEntry {
	vm.currentSelector = selector
	cacheEntry := vm.findMethodCacheEntry(selector, startingClass)
	if cacheEntry.method != nil {
		return cacheEntry
	}
	currentClass := startingClass
	for currentClass != nil && !currentClass.IsNil {
		mDict := vm.asObj(currentClass.Pointers[ClassMdict])
		if mDict == nil || mDict.IsNil {
			// MethodDict is nil -- send #cannotInterpret:
			cantInterpSel := vm.SpecialObjects[SplObSelectorCannotInterpret].(*Object)
			cantInterpMsg := vm.createActualMessage(selector, trueArgCount, startingClass)
			vm.popNandPush(argCount+1, cantInterpMsg)
			return vm.findSelectorInClass(cantInterpSel, 1, currentClass.Superclass(), 1)
		}
		newMethod := vm.lookupSelectorInDict(mDict, selector)
		if newMethod != nil && !newMethod.IsNil {
			cacheEntry.method = newMethod
			if newMethod.IsMethod() {
				cacheEntry.primIndex = newMethod.MethodPrimitiveIndex()
				cacheEntry.argCount = newMethod.MethodNumArgs()
				if cacheEntry.primIndex != 0 {
					// note details for verification of at:/at:put: primitives
					vm.verifyAtSelector = selector
					vm.verifyAtClass = startingClass
				}
			} else {
				cacheEntry.primIndex = 576 // primitiveInvokeObjectAsMethod
				cacheEntry.argCount = trueArgCount
			}
			cacheEntry.mClass = currentClass
			return cacheEntry
		}
		currentClass = currentClass.Superclass()
	}
	// Not found -- send #doesNotUnderstand:
	dnuSel := vm.SpecialObjects[SplObSelectorDoesNotUnderstand].(*Object)
	if selector == dnuSel {
		panic("Recursive not understood error encountered")
	}
	if vm.debugDNU && selector.BytesAsString() == "asciiValue" && vm.dnuSeen == 0 {
		vm.dnuSeen++
		fmt.Println("=== backtrace at 'SmallInteger>>asciiValue' DNU ===")
		for _, l := range vm.Backtrace(15) {
			fmt.Println("  " + l)
		}
	}
	dnuMsg := vm.createActualMessage(selector, trueArgCount, startingClass)
	vm.popNandPush(argCount, dnuMsg)
	return vm.findSelectorInClass(dnuSel, 1, startingClass, 1)
}

func (vm *Interpreter) lookupSelectorInDict(mDict, selector *Object) *Object {
	dictSize := len(mDict.Pointers)
	mask := (dictSize - MethodDictSelectorStart) - 1
	index := (mask & selector.Hash) + MethodDictSelectorStart
	hasWrapped := false
	for {
		nextSelector := mDict.Pointers[index]
		if no, ok := nextSelector.(*Object); ok {
			if no == selector {
				methArray := mDict.Pointers[MethodDictArray].(*Object)
				return vm.asObj(methArray.Pointers[index-MethodDictSelectorStart])
			}
			if no.IsNil {
				return vm.NilObj
			}
		}
		index++
		if index == dictSize {
			if hasWrapped {
				return vm.NilObj
			}
			index = MethodDictSelectorStart
			hasWrapped = true
		}
	}
}

func (vm *Interpreter) findMethodCacheEntry(selector, lkupClass *Object) *methodCacheEntry {
	vm.methodCacheRand = (vm.methodCacheRand + 1) & 3
	firstProbe := (selector.Hash ^ lkupClass.Hash) & vm.methodCacheMask
	probe := firstProbe
	for i := 0; i < 4; i++ {
		entry := &vm.methodCache[probe]
		if entry.selector == selector && entry.lkupClass == lkupClass {
			return entry
		}
		if i == vm.methodCacheRand {
			firstProbe = probe
		}
		probe = (probe + selector.Hash) & vm.methodCacheMask
	}
	entry := &vm.methodCache[firstProbe]
	entry.lkupClass = lkupClass
	entry.selector = selector
	entry.method = nil
	return entry
}

func (vm *Interpreter) flushMethodCache() bool {
	for i := range vm.methodCache {
		vm.methodCache[i].selector = nil
		vm.methodCache[i].method = nil
	}
	return true
}

func (vm *Interpreter) executeNewMethod(newRcvr Value, newMethod *Object, argumentCount, primitiveIndex int, optClass, optSel *Object) {
	vm.SendCount++
	if primitiveIndex > 0 {
		if vm.tryPrimitive(primitiveIndex, argumentCount, newMethod) {
			return // primitive succeeded
		}
	}
	newContext := vm.allocateOrRecycleContext(newMethod.MethodNeedsLargeFrame())
	tempCount := newMethod.MethodTempCount()
	newPC := 0
	newSP := ContextTempFrameStart + tempCount - 1
	newContext.Pointers[ContextMethod] = newMethod
	newContext.Pointers[BlockContextInitialIP] = vm.NilObj
	newContext.Pointers[ContextSender] = vm.ActiveContext
	// copy receiver and args (contiguous) into the new frame
	vm.arrayCopy(vm.ActiveContext.Pointers, vm.SP-argumentCount, newContext.Pointers, ContextTempFrameStart-1, argumentCount+1)
	// nil out remaining temps
	for i := ContextTempFrameStart + argumentCount; i < ContextTempFrameStart+tempCount; i++ {
		newContext.Pointers[i] = vm.NilObj
	}
	vm.popN(argumentCount + 1)
	vm.reclaimableContextCount++
	vm.storeContextRegisters()
	// switch
	vm.ActiveContext = newContext
	vm.ActiveContext.Dirty = true
	vm.HomeContext = newContext
	vm.Method = newMethod
	vm.PC = newPC
	vm.SP = newSP
	vm.Receiver = newContext.Pointers[ContextReceiver]
	if vm.Receiver != newRcvr {
		panic("receivers don't match")
	}
	vm.interruptCheckCounter--
	if vm.interruptCheckCounter <= 0 {
		vm.checkForInterrupts()
	}
}

func (vm *Interpreter) doReturn(returnValue Value, targetContext *Object) {
	if targetContext == nil {
		ctx := vm.HomeContext
		if vm.HasClosures {
			for {
				closure := vm.asObj(ctx.Pointers[ContextClosureOrVirtualPC])
				if closure == nil || closure.IsNil {
					break
				}
				ctx = vm.asObj(closure.Pointers[ClosureOuterContext])
			}
		}
		targetContext = vm.asObj(ctx.Pointers[ContextSender])
	}
	if targetContext == nil || targetContext.IsNil || isNilValue(vm, targetContext.Pointers[ContextInstructionPointer]) {
		vm.cannotReturn(returnValue)
		return
	}
	// search up the stack for an unwind
	thisContext := vm.asObj(vm.ActiveContext.Pointers[ContextSender])
	for thisContext != targetContext {
		if thisContext == nil || thisContext.IsNil {
			vm.cannotReturn(returnValue)
			return
		}
		if vm.isUnwindMarked(thisContext) {
			vm.aboutToReturnThrough(returnValue, thisContext)
			return
		}
		thisContext = vm.asObj(thisContext.Pointers[ContextSender])
	}
	// peel back the stack
	thisContext = vm.ActiveContext
	for thisContext != targetContext {
		nextContext := vm.asObj(thisContext.Pointers[ContextSender])
		thisContext.Pointers[ContextSender] = vm.NilObj
		thisContext.Pointers[ContextInstructionPointer] = vm.NilObj
		if vm.reclaimableContextCount > 0 {
			vm.reclaimableContextCount--
			vm.recycleIfPossible(thisContext)
		}
		thisContext = nextContext
	}
	vm.ActiveContext = thisContext
	vm.ActiveContext.Dirty = true
	vm.fetchContextRegisters(vm.ActiveContext)
	vm.push(returnValue)
}

func (vm *Interpreter) aboutToReturnThrough(resultObj Value, aContext *Object) {
	vm.push(vm.exportThisContext())
	vm.push(resultObj)
	vm.push(aContext)
	vm.send(vm.spl(SplObSelectorAboutToReturn).(*Object), 2, false)
}

func (vm *Interpreter) cannotReturn(resultObj Value) {
	vm.push(vm.exportThisContext())
	vm.push(resultObj)
	vm.send(vm.SpecialObjects[SplObSelectorCannotReturn].(*Object), 1, false)
}

func (vm *Interpreter) tryPrimitive(primIndex, argCount int, newMethod *Object) bool {
	if primIndex > 255 && primIndex < 520 {
		if primIndex >= 264 { // return instvar
			vm.popNandPush(1, vm.asObj(vm.top()).Pointers[primIndex-264])
			return true
		}
		switch primIndex {
		case 256:
			return true // return self
		case 257:
			vm.popNandPush(1, vm.TrueObj)
			return true
		case 258:
			vm.popNandPush(1, vm.FalseObj)
			return true
		case 259:
			vm.popNandPush(1, vm.NilObj)
			return true
		}
		vm.popNandPush(1, primIndex-261) // return -1..2
		return true
	}
	return vm.prim.doPrimitive(primIndex, argCount, newMethod)
}

func (vm *Interpreter) createActualMessage(selector *Object, argCount int, cls *Object) *Object {
	message := vm.InstantiateClass(vm.SpecialObjects[SplObClassMessage].(*Object), 0)
	argArray := vm.InstantiateClass(vm.SpecialObjects[SplObClassArray].(*Object), argCount)
	vm.arrayCopy(vm.ActiveContext.Pointers, vm.SP-argCount+1, argArray.Pointers, 0, argCount)
	message.Pointers[MessageSelector] = selector
	message.Pointers[MessageArguments] = argArray
	if len(message.Pointers) > MessageLookupClass {
		message.Pointers[MessageLookupClass] = cls
	}
	return message
}

func (vm *Interpreter) primitivePerform(argCount int) bool {
	selector := vm.asObj(vm.stackValue(argCount - 1))
	rcvr := vm.stackValue(argCount)
	trueArgCount := argCount - 1
	entry := vm.findSelectorInClass(selector, trueArgCount, vm.getClass(rcvr), argCount)
	if entry.selector == selector {
		if entry.argCount != trueArgCount {
			return false
		}
		stack := vm.ActiveContext.Pointers
		selectorIndex := vm.SP - trueArgCount
		vm.arrayCopy(stack, selectorIndex+1, stack, selectorIndex, trueArgCount)
		vm.SP--
	} else {
		rcvr = vm.stackValue(entry.argCount)
	}
	vm.executeNewMethod(rcvr, entry.method, entry.argCount, entry.primIndex, entry.mClass, selector)
	return true
}

func (vm *Interpreter) primitivePerformWithArgs(argCount int, supered bool) bool {
	rcvrPos := 2
	if supered {
		rcvrPos = 3
	}
	rcvr := vm.stackValue(rcvrPos)
	selector := vm.asObj(vm.stackValue(rcvrPos - 1))
	args := vm.asObj(vm.stackValue(rcvrPos - 2))
	if args == nil || args.SqClass != vm.SpecialObjects[SplObClassArray] {
		return false
	}
	var lookupClass *Object
	if supered {
		lookupClass = vm.asObj(vm.top())
		cls := vm.getClass(rcvr)
		for cls != lookupClass {
			cls = cls.Superclass()
			if cls == nil || cls.IsNil {
				return false
			}
		}
	} else {
		lookupClass = vm.getClass(rcvr)
	}
	trueArgCount := len(args.Pointers)
	entry := vm.findSelectorInClass(selector, trueArgCount, lookupClass, argCount)
	if entry.selector == selector {
		if entry.argCount != trueArgCount {
			return false
		}
		stack := vm.ActiveContext.Pointers
		selectorIndex := vm.SP - (argCount - 1)
		stack[selectorIndex-1] = rcvr
		vm.arrayCopy(args.Pointers, 0, stack, selectorIndex, trueArgCount)
		vm.SP += trueArgCount - argCount
	} else {
		rcvr = vm.stackValue(entry.argCount)
	}
	vm.executeNewMethod(rcvr, entry.method, entry.argCount, entry.primIndex, entry.mClass, selector)
	return true
}

func (vm *Interpreter) primitiveInvokeObjectAsMethod(argCount int, method *Object) bool {
	// Invoked when method lookup finds a non-method: send run:with:in:.
	orgArgs := vm.InstantiateClass(vm.SpecialObjects[SplObClassArray].(*Object), argCount)
	for i := 0; i < argCount; i++ {
		orgArgs.Pointers[argCount-i-1] = vm.pop()
	}
	orgReceiver := vm.pop()
	orgSelector := vm.currentSelector
	runWithIn := vm.spl(SplObSelectorRunWithIn).(*Object)
	vm.push(method)
	vm.push(orgSelector)
	vm.push(orgArgs)
	vm.push(orgReceiver)
	vm.send(runWithIn, 3, false)
	return true
}

// --- contexts ------------------------------------------------------------

func (vm *Interpreter) isContext(obj *Object) bool {
	return obj.SqClass == vm.SpecialObjects[SplObClassMethodContext] ||
		obj.SqClass == vm.SpecialObjects[SplObClassBlockContext]
}

func (vm *Interpreter) isMethodContext(obj *Object) bool {
	return obj.SqClass == vm.SpecialObjects[SplObClassMethodContext]
}

func (vm *Interpreter) isUnwindMarked(ctx *Object) bool {
	if !vm.isMethodContext(ctx) {
		return false
	}
	method := vm.asObj(ctx.Pointers[ContextMethod])
	return method != nil && method.MethodPrimitiveIndex() == 198
}

func (vm *Interpreter) exportThisContext() *Object {
	vm.storeContextRegisters()
	vm.reclaimableContextCount = 0
	return vm.ActiveContext
}

func (vm *Interpreter) recycleIfPossible(ctx *Object) {
	if !vm.isMethodContext(ctx) {
		return
	}
	switch len(ctx.Pointers) {
	case ContextTempFrameStart + ContextSmallFrameSize:
		ctx.Pointers[0] = vm.freeContexts
		vm.freeContexts = ctx
	case ContextTempFrameStart + ContextLargeFrameSize:
		ctx.Pointers[0] = vm.freeLargeContexts
		vm.freeLargeContexts = ctx
	}
}

func (vm *Interpreter) allocateOrRecycleContext(needsLarge bool) *Object {
	if needsLarge {
		if !vm.freeLargeContexts.IsNil {
			freebie := vm.freeLargeContexts
			vm.freeLargeContexts = vm.asObj(freebie.Pointers[0])
			return freebie
		}
		return vm.InstantiateClass(vm.SpecialObjects[SplObClassMethodContext].(*Object), ContextLargeFrameSize)
	}
	if !vm.freeContexts.IsNil {
		freebie := vm.freeContexts
		vm.freeContexts = vm.asObj(freebie.Pointers[0])
		return freebie
	}
	return vm.InstantiateClass(vm.SpecialObjects[SplObClassMethodContext].(*Object), ContextSmallFrameSize)
}

// --- stack access --------------------------------------------------------

func (vm *Interpreter) pop() Value {
	v := vm.ActiveContext.Pointers[vm.SP]
	vm.SP--
	return v
}
func (vm *Interpreter) popN(n int) { vm.SP -= n }
func (vm *Interpreter) push(o Value) {
	vm.SP++
	vm.ActiveContext.Pointers[vm.SP] = o
}
func (vm *Interpreter) popNandPush(n int, o Value) {
	vm.SP -= n - 1
	vm.ActiveContext.Pointers[vm.SP] = o
}
func (vm *Interpreter) top() Value             { return vm.ActiveContext.Pointers[vm.SP] }
func (vm *Interpreter) stackTopPut(o Value)    { vm.ActiveContext.Pointers[vm.SP] = o }
func (vm *Interpreter) stackValue(d int) Value { return vm.ActiveContext.Pointers[vm.SP-d] }

func (vm *Interpreter) stackInteger(d int) int {
	return vm.checkSmallInt(vm.stackValue(d))
}

func (vm *Interpreter) stackIntOrFloat(d int) float64 {
	num := vm.stackValue(d)
	if n, ok := num.(int); ok {
		return float64(n)
	}
	o, ok := num.(*Object)
	if !ok || o == nil {
		vm.success = false
		return 0
	}
	if o.IsFloat {
		vm.resultIsFloat = true
		return o.Float
	}
	if o.Bytes != nil && len(o.Bytes) == 4 {
		value := 0.0
		for i := 3; i >= 0; i-- {
			value = value*256 + float64(o.Bytes[i])
		}
		if o.SqClass == vm.spl(SplObClassLargePositiveInteger) {
			return value
		}
		if o.SqClass == vm.spl(SplObClassLargeNegativeInteger) {
			return -value
		}
	}
	vm.success = false
	return 0
}

func (vm *Interpreter) pop2AndPushIntResult(intResult int) bool {
	if vm.success && vm.canBeSmallInt(intResult) {
		vm.popNandPush(2, intResult)
		return true
	}
	return false
}

func (vm *Interpreter) pop2AndPushNumResult(numResult float64) bool {
	if !vm.success {
		return false
	}
	if vm.resultIsFloat {
		vm.popNandPush(2, vm.prim.makeFloat(numResult))
		return true
	}
	if numResult >= MinSmallInt && numResult <= MaxSmallInt {
		vm.popNandPush(2, int(numResult))
		return true
	}
	if numResult >= -0xFFFFFFFF && numResult <= 0xFFFFFFFF {
		negative := numResult < 0
		unsigned := uint32(math.Abs(numResult))
		cls := SplObClassLargePositiveInteger
		if negative {
			cls = SplObClassLargeNegativeInteger
		}
		lgClass, ok := vm.spl(cls).(*Object)
		if !ok || lgClass.IsNil {
			return false // image has no such class; fall back to Smalltalk
		}
		lg := vm.InstantiateClass(lgClass, 4)
		lg.Bytes[0] = byte(unsigned)
		lg.Bytes[1] = byte(unsigned >> 8)
		lg.Bytes[2] = byte(unsigned >> 16)
		lg.Bytes[3] = byte(unsigned >> 24)
		vm.popNandPush(2, lg)
		return true
	}
	return false
}

func (vm *Interpreter) pop2AndPushBoolResult(boolResult bool) bool {
	if !vm.success {
		return false
	}
	if boolResult {
		vm.popNandPush(2, vm.TrueObj)
	} else {
		vm.popNandPush(2, vm.FalseObj)
	}
	return true
}

// --- numbers -------------------------------------------------------------

func (vm *Interpreter) getClass(obj Value) *Object {
	if _, ok := obj.(int); ok {
		return vm.SpecialObjects[SplObClassInteger].(*Object)
	}
	if o, ok := obj.(*Object); ok {
		return o.SqClass
	}
	return vm.NilObj
}

func (vm *Interpreter) canBeSmallInt(anInt int) bool {
	return anInt >= MinSmallInt && anInt <= MaxSmallInt
}

func (vm *Interpreter) isSmallInt(v Value) bool {
	_, ok := v.(int)
	return ok
}

func (vm *Interpreter) checkSmallInt(v Value) int {
	if n, ok := v.(int); ok {
		return n
	}
	vm.success = false
	return 1
}

func (vm *Interpreter) quickDivide(rcvr, arg int) int {
	if arg == 0 {
		return NonSmallInt
	}
	result := rcvr / arg
	if result*arg == rcvr {
		return result
	}
	return NonSmallInt
}

func (vm *Interpreter) div(rcvr, arg int) int {
	if arg == 0 {
		return NonSmallInt
	}
	return int(math.Floor(float64(rcvr) / float64(arg)))
}

func (vm *Interpreter) mod(rcvr, arg int) int {
	if arg == 0 {
		return NonSmallInt
	}
	return rcvr - int(math.Floor(float64(rcvr)/float64(arg)))*arg
}

func (vm *Interpreter) safeShift(smallInt, shiftCount int) int {
	if shiftCount < 0 {
		if shiftCount < -31 {
			if smallInt < 0 {
				return -1
			}
			return 0
		}
		return smallInt >> uint(-shiftCount)
	}
	if shiftCount > 31 {
		if smallInt == 0 {
			return 0
		}
		return NonSmallInt
	}
	shifted := smallInt << uint(shiftCount)
	if shifted>>uint(shiftCount) != smallInt {
		return NonSmallInt
	}
	return shifted
}

// --- utils ---------------------------------------------------------------

func (vm *Interpreter) InstantiateClass(aClass *Object, indexableSize int) *Object {
	return vm.Image.InstantiateClass(aClass, indexableSize)
}

func (vm *Interpreter) arrayCopy(src []Value, srcPos int, dest []Value, destPos, length int) {
	if length <= 0 {
		return
	}
	if &src[0] == &dest[0] && srcPos < destPos {
		for i := length - 1; i >= 0; i-- {
			dest[destPos+i] = src[srcPos+i]
		}
	} else {
		for i := 0; i < length; i++ {
			dest[destPos+i] = src[srcPos+i]
		}
	}
}

func isNilValue(vm *Interpreter, v Value) bool {
	o, ok := v.(*Object)
	return ok && o.IsNil
}

// --- interrupts (simplified) ---------------------------------------------

func (vm *Interpreter) forceInterruptCheck() { vm.interruptCheckCounter = -1000 }

func (vm *Interpreter) checkForInterrupts() {
	vm.interruptCheckCounter = vm.interruptCheckCounterFeedBackReset
	now := vm.prim.millisecondClockValue()
	if vm.signalLowSpace {
		vm.signalLowSpace = false
		if sema := vm.asObj(vm.SpecialObjects[SplObTheLowSpaceSemaphore]); sema != nil && !sema.IsNil {
			vm.prim.synchronousSignal(sema)
		}
	}
	if vm.nextWakeupTick != 0 && now >= vm.nextWakeupTick {
		vm.nextWakeupTick = 0
		if sema := vm.asObj(vm.SpecialObjects[SplObTheTimerSemaphore]); sema != nil && !sema.IsNil {
			vm.prim.synchronousSignal(sema)
		}
	}
	if len(vm.prim.semaphoresToSignal) > 0 {
		vm.prim.signalExternalSemaphores()
	}
}

// goIdle is called by the idle primitive (relinquishProcessor). It breaks out
// of the run loop so the host can wait for input/timers.
func (vm *Interpreter) goIdle() {
	vm.forceInterruptCheck()
	vm.checkForInterrupts()
	vm.breakOut = true
}

var _ = fmt.Sprintf
