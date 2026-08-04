package vm

// Closure bytecode support. mini.image (V3, non-closure) does not use these,
// but they are implemented for images that do.

func (vm *Interpreter) pushNewArray(nextByte int) {
	popValues := nextByte > 127
	count := nextByte & 127
	array := vm.InstantiateClass(vm.SpecialObjects[SplObClassArray].(*Object), count)
	if popValues {
		for i := 0; i < count; i++ {
			array.Pointers[i] = vm.stackValue(count - i - 1)
		}
		vm.popN(count)
	}
	vm.push(array)
}

func (vm *Interpreter) newClosure(numArgs, initialPC, numCopied int) *Object {
	closure := vm.InstantiateClass(vm.SpecialObjects[SplObClassBlockClosure].(*Object), numCopied)
	closure.Pointers[ClosureStartPC] = initialPC
	closure.Pointers[ClosureNumArgs] = numArgs
	return closure
}

func (vm *Interpreter) pushClosureCopy() {
	numArgsNumCopied := vm.nextByte()
	numArgs := numArgsNumCopied & 0xF
	numCopied := numArgsNumCopied >> 4
	blockSizeHigh := vm.nextByte()
	blockSize := blockSizeHigh*256 + vm.nextByte()
	initialPC := vm.encodeSqueakPC(vm.PC, vm.Method)
	closure := vm.newClosure(numArgs, initialPC, numCopied)
	closure.Pointers[ClosureOuterContext] = vm.ActiveContext
	vm.reclaimableContextCount = 0
	if numCopied > 0 {
		for i := 0; i < numCopied; i++ {
			closure.Pointers[ClosureFirstCopiedValue+i] = vm.stackValue(numCopied - i - 1)
		}
		vm.popN(numCopied)
	}
	vm.PC += blockSize
	vm.push(closure)
}
