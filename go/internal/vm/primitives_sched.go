package vm

// Process scheduling, semaphores, and block activation primitives.

func (vm *Interpreter) newActiveContext(newContext *Object) {
	vm.storeContextRegisters()
	vm.ActiveContext = newContext
	vm.ActiveContext.Dirty = true
	vm.fetchContextRegisters(newContext)
}

// --- scheduler access ----------------------------------------------------

func (p *Primitives) getScheduler() *Object {
	assn := p.asObj(p.vm.SpecialObjects[SplObSchedulerAssociation])
	return p.asObj(assn.Pointers[AssnValue])
}

func (p *Primitives) activeProcess() *Object {
	return p.asObj(p.getScheduler().Pointers[ProcSchedActiveProcess])
}

func (p *Primitives) resume(newProc *Object) {
	activeProc := p.activeProcess()
	activePriority := asIntValue(activeProc.Pointers[ProcPriority])
	newPriority := asIntValue(newProc.Pointers[ProcPriority])
	if newPriority > activePriority {
		p.putToSleep(activeProc)
		p.transferTo(newProc)
	} else {
		p.putToSleep(newProc)
	}
}

func (p *Primitives) putToSleep(aProcess *Object) {
	priority := asIntValue(aProcess.Pointers[ProcPriority])
	processLists := p.asObj(p.getScheduler().Pointers[ProcSchedProcessLists])
	processList := p.asObj(processLists.Pointers[priority-1])
	p.linkProcessToList(aProcess, processList)
}

func (p *Primitives) transferTo(newProc *Object) {
	sched := p.getScheduler()
	oldProc := p.asObj(sched.Pointers[ProcSchedActiveProcess])
	sched.Pointers[ProcSchedActiveProcess] = newProc
	sched.Dirty = true
	oldProc.Pointers[ProcSuspendedContext] = p.vm.ActiveContext
	oldProc.Dirty = true
	p.vm.newActiveContext(p.asObj(newProc.Pointers[ProcSuspendedContext]))
	newProc.Pointers[ProcSuspendedContext] = p.vm.NilObj
	p.vm.reclaimableContextCount = 0
}

func (p *Primitives) wakeHighestPriority() *Object {
	schedLists := p.asObj(p.getScheduler().Pointers[ProcSchedProcessLists])
	idx := len(schedLists.Pointers) - 1
	var processList *Object
	for {
		if idx < 0 {
			panic("scheduler could not find a runnable process")
		}
		processList = p.asObj(schedLists.Pointers[idx])
		idx--
		if !p.isEmptyList(processList) {
			break
		}
	}
	return p.removeFirstLinkOfList(processList)
}

func (p *Primitives) linkProcessToList(proc, aList *Object) {
	if p.isEmptyList(aList) {
		aList.Pointers[LinkedListFirstLink] = proc
	} else {
		lastLink := p.asObj(aList.Pointers[LinkedListLastLink])
		lastLink.Pointers[LinkNextLink] = proc
		lastLink.Dirty = true
	}
	aList.Pointers[LinkedListLastLink] = proc
	aList.Dirty = true
	proc.Pointers[ProcMyList] = aList
	proc.Dirty = true
}

func (p *Primitives) isEmptyList(aList *Object) bool {
	return isNilValue(p.vm, aList.Pointers[LinkedListFirstLink])
}

func (p *Primitives) removeFirstLinkOfList(aList *Object) *Object {
	first := p.asObj(aList.Pointers[LinkedListFirstLink])
	last := p.asObj(aList.Pointers[LinkedListLastLink])
	if first == last {
		aList.Pointers[LinkedListFirstLink] = p.vm.NilObj
		aList.Pointers[LinkedListLastLink] = p.vm.NilObj
	} else {
		next := first.Pointers[LinkNextLink]
		aList.Pointers[LinkedListFirstLink] = next
		aList.Dirty = true
	}
	first.Pointers[LinkNextLink] = p.vm.NilObj
	return first
}

func (p *Primitives) removeProcessFromList(process, list *Object) {
	first := p.asObj(list.Pointers[LinkedListFirstLink])
	last := p.asObj(list.Pointers[LinkedListLastLink])
	if process == first {
		next := process.Pointers[LinkNextLink]
		list.Pointers[LinkedListFirstLink] = next
		if process == last {
			list.Pointers[LinkedListLastLink] = p.vm.NilObj
		}
	} else {
		temp := first
		for {
			if temp == nil || temp.IsNil {
				if p.oldPrims {
					p.success = false
				}
				return
			}
			next := p.asObj(temp.Pointers[LinkNextLink])
			if next == process {
				break
			}
			temp = next
		}
		temp.Pointers[LinkNextLink] = process.Pointers[LinkNextLink]
		if process == last {
			list.Pointers[LinkedListLastLink] = temp
		}
	}
	process.Pointers[LinkNextLink] = p.vm.NilObj
}

// --- semaphore/process primitives ----------------------------------------

func (p *Primitives) primitiveResume() bool {
	p.resume(p.asObj(p.vm.top()))
	return true
}

func (p *Primitives) primitiveSuspend() bool {
	process := p.asObj(p.vm.top())
	if process == p.activeProcess() {
		p.vm.popNandPush(1, p.vm.NilObj)
		p.transferTo(p.wakeHighestPriority())
	} else {
		oldList := p.asObj(process.Pointers[ProcMyList])
		if oldList == nil || oldList.IsNil {
			return false
		}
		p.removeProcessFromList(process, oldList)
		if !p.success {
			return false
		}
		process.Pointers[ProcMyList] = p.vm.NilObj
		p.vm.popNandPush(1, oldList)
	}
	return true
}

func (p *Primitives) primitiveWait() bool {
	sema := p.asObj(p.vm.top())
	if sema == nil || sema.SqClass != p.vm.SpecialObjects[SplObClassSemaphore] {
		return false
	}
	excess := asIntValue(sema.Pointers[SemaphoreExcessSignals])
	if excess > 0 {
		sema.Pointers[SemaphoreExcessSignals] = excess - 1
	} else {
		p.linkProcessToList(p.activeProcess(), sema)
		p.transferTo(p.wakeHighestPriority())
	}
	return true
}

func (p *Primitives) primitiveSignal() bool {
	sema := p.asObj(p.vm.top())
	if sema == nil || sema.SqClass != p.vm.SpecialObjects[SplObClassSemaphore] {
		return false
	}
	p.synchronousSignal(sema)
	return true
}

func (p *Primitives) synchronousSignal(sema *Object) {
	if p.isEmptyList(sema) {
		sema.Pointers[SemaphoreExcessSignals] = asIntValue(sema.Pointers[SemaphoreExcessSignals]) + 1
	} else {
		p.resume(p.removeFirstLinkOfList(sema))
	}
}

func (p *Primitives) signalExternalSemaphores() {
	semaphores := p.asObj(p.vm.SpecialObjects[SplObExternalObjectsArray])
	semaClass := p.vm.SpecialObjects[SplObClassSemaphore]
	for len(p.semaphoresToSignal) > 0 {
		idx := p.semaphoresToSignal[0]
		p.semaphoresToSignal = p.semaphoresToSignal[1:]
		if idx-1 < len(semaphores.Pointers) {
			if sema := p.asObj(semaphores.Pointers[idx-1]); sema != nil && sema.SqClass == semaClass {
				p.synchronousSignal(sema)
			}
		}
	}
}

func (vm *Interpreter) SignalSemaphoreWithIndex(index int) {
	if index > 0 {
		vm.prim.semaphoresToSignal = append(vm.prim.semaphoresToSignal, index)
	}
}

// --- blocks (old BlockContext style) -------------------------------------

func (p *Primitives) doBlockCopy() Value {
	rcvr := p.asObj(p.vm.stackValue(1))
	sqArgCount := p.stackInteger(0)
	homeCtxt := rcvr
	if rcvr == nil || !p.vm.isContext(rcvr) {
		p.success = false
	}
	if !p.success {
		return rcvr
	}
	if _, isInt := homeCtxt.Pointers[ContextMethod].(int); isInt {
		homeCtxt = p.asObj(homeCtxt.Pointers[BlockContextHome])
	}
	blockSize := homeCtxt.PointersSize() - homeCtxt.InstSize()
	newBlock := p.vm.InstantiateClass(p.vm.SpecialObjects[SplObClassBlockContext].(*Object), blockSize)
	initialPC := p.vm.encodeSqueakPC(p.vm.PC+2, p.vm.Method)
	newBlock.Pointers[BlockContextInitialIP] = initialPC
	newBlock.Pointers[ContextInstructionPointer] = initialPC
	newBlock.Pointers[ContextStackPointer] = 0
	newBlock.Pointers[BlockContextArgumentCount] = sqArgCount
	newBlock.Pointers[BlockContextHome] = homeCtxt
	newBlock.Pointers[ContextSender] = p.vm.NilObj
	return newBlock
}

func (p *Primitives) primitiveBlockValue(argCount int) bool {
	rcvr := p.asObj(p.vm.stackValue(argCount))
	if rcvr == nil || rcvr.SqClass != p.vm.SpecialObjects[SplObClassBlockContext] {
		return false
	}
	block := rcvr
	blockArgCount, ok := block.Pointers[BlockContextArgumentCount].(int)
	if !ok || blockArgCount != argCount {
		return false
	}
	if !isNilValue(p.vm, block.Pointers[BlockContextCaller]) {
		return false
	}
	p.vm.arrayCopy(p.vm.ActiveContext.Pointers, p.vm.SP-argCount+1, block.Pointers, ContextTempFrameStart, argCount)
	block.Pointers[ContextInstructionPointer] = block.Pointers[BlockContextInitialIP]
	block.Pointers[ContextStackPointer] = argCount
	block.Pointers[BlockContextCaller] = p.vm.ActiveContext
	p.vm.popN(argCount + 1)
	p.vm.newActiveContext(block)
	p.vm.interruptCheckCounter--
	if p.vm.interruptCheckCounter <= 0 {
		p.vm.checkForInterrupts()
	}
	return true
}

func (p *Primitives) primitiveBlockValueWithArgs(argCount int) bool {
	block := p.asObj(p.vm.stackValue(1))
	array := p.asObj(p.vm.stackValue(0))
	if block == nil || block.SqClass != p.vm.SpecialObjects[SplObClassBlockContext] {
		return false
	}
	if array == nil || array.SqClass != p.vm.SpecialObjects[SplObClassArray] {
		return false
	}
	blockArgCount, ok := block.Pointers[BlockContextArgumentCount].(int)
	if !ok || blockArgCount != array.PointersSize() {
		return false
	}
	if !isNilValue(p.vm, block.Pointers[BlockContextCaller]) {
		return false
	}
	p.vm.arrayCopy(array.Pointers, 0, block.Pointers, ContextTempFrameStart, blockArgCount)
	block.Pointers[ContextInstructionPointer] = block.Pointers[BlockContextInitialIP]
	block.Pointers[ContextStackPointer] = blockArgCount
	block.Pointers[BlockContextCaller] = p.vm.ActiveContext
	p.vm.popN(argCount + 1)
	p.vm.newActiveContext(block)
	p.vm.interruptCheckCounter--
	if p.vm.interruptCheckCounter <= 0 {
		p.vm.checkForInterrupts()
	}
	return true
}
