package vm

import "fmt"

// globalNamed looks up a global (e.g. a class) by name in the Smalltalk system
// dictionary. Handles the classic SystemDictionary layout used by old images.
func (vm *Interpreter) globalNamed(name string) *Object {
	smalltalk := vm.asObj(vm.SpecialObjects[SplObSmalltalkDictionary])
	if smalltalk == nil {
		return nil
	}
	if smalltalk.SqClass != nil && smalltalk.SqClass.ClassName() == "Association" {
		smalltalk = vm.asObj(smalltalk.Pointers[1])
	}
	if smalltalk == nil || len(smalltalk.Pointers) < 2 {
		return nil
	}
	// SystemDictionary is a HashedCollection: inst var 1 is the association array.
	arr := vm.asObj(smalltalk.Pointers[1])
	if arr == nil {
		return nil
	}
	for _, e := range arr.Pointers {
		assn := vm.asObj(e)
		if assn == nil || assn.IsNil || len(assn.Pointers) < 2 {
			continue
		}
		key := vm.asObj(assn.Pointers[0])
		if key != nil && key.Bytes != nil && key.BytesAsString() == name {
			return vm.asObj(assn.Pointers[1])
		}
	}
	return nil
}

// selectorNamed finds the unique Symbol for a selector string by scanning
// method dictionaries (symbols are interned, so identity is what matters).
func (vm *Interpreter) selectorNamed(name string) *Object {
	for obj := vm.Image.FirstOldObject; obj != nil; obj = obj.NextObject {
		if len(obj.Pointers) < MethodDictSelectorStart+1 {
			continue
		}
		arr := vm.asObj(obj.Pointers[MethodDictArray])
		if arr == nil || arr.Pointers == nil {
			continue
		}
		if len(arr.Pointers)+MethodDictSelectorStart != len(obj.Pointers) {
			continue
		}
		for i := MethodDictSelectorStart; i < len(obj.Pointers); i++ {
			sel := vm.asObj(obj.Pointers[i])
			if sel != nil && sel.Bytes != nil && sel.BytesAsString() == name {
				return sel
			}
		}
	}
	return nil
}

// sendAndRun injects a message send from the current context and runs the
// interpreter until it returns, answering the result. Timer interrupts are
// suppressed so the active process isn't switched out mid-evaluation.
func (vm *Interpreter) sendAndRun(rcvr Value, selector *Object, args ...Value) (Value, error) {
	savedTick := vm.nextWakeupTick
	vm.nextWakeupTick = 0
	defer func() { vm.nextWakeupTick = savedTick }()

	startCtx := vm.ActiveContext
	startSP := vm.SP
	vm.push(rcvr)
	for _, a := range args {
		vm.push(a)
	}
	vm.send(selector, len(args), false)

	const maxSteps = 200_000_000
	for steps := 0; ; steps++ {
		if vm.ActiveContext == startCtx && vm.SP == startSP+1 {
			break
		}
		if steps >= maxSteps {
			return nil, fmt.Errorf("evaluation did not return after %d bytecodes", maxSteps)
		}
		vm.interpretOne()
	}
	res := vm.top()
	vm.SP = startSP // drop the result, restoring the caller's stack
	return res, nil
}

// Evaluate compiles and runs a Smalltalk expression ("do it") via the image's
// Compiler and answers the result object. The image is first booted to a
// quiescent (idle) state so injecting the send doesn't race the scheduler.
func (vm *Interpreter) Evaluate(source string) (result Value, err error) {
	// A do-it runs in the idle process here; an expression that blocks on a
	// semaphore or forces a process switch can strand it. Recover so the CLI
	// reports an error rather than crashing.
	defer func() {
		if r := recover(); r != nil {
			result, err = nil, fmt.Errorf("evaluation failed: %v", r)
		}
	}()
	if !vm.evalBooted {
		vm.BootToIdle(2_000_000)
		vm.evalBooted = true
	}
	compiler := vm.globalNamed("Compiler")
	if compiler == nil {
		return nil, fmt.Errorf("no Compiler class in image")
	}
	evalSel := vm.selectorNamed("evaluate:")
	if evalSel == nil {
		return nil, fmt.Errorf("no #evaluate: selector in image")
	}
	src := vm.prim.makeStString(source)
	return vm.sendAndRun(compiler, evalSel, src)
}

// PrintIt evaluates an expression and answers its printString ("print it").
func (vm *Interpreter) PrintIt(source string) (string, error) {
	result, err := vm.Evaluate(source)
	if err != nil {
		return "", err
	}
	printSel := vm.selectorNamed("printString")
	if printSel == nil {
		// Fall back to the VM's own rendering if the image lacks #printString.
		if o := vm.asObj(result); o != nil {
			return o.SqInstName(), nil
		}
		return fmt.Sprintf("%v", result), nil
	}
	printed, err := vm.sendAndRun(result, printSel)
	if err != nil {
		return "", err
	}
	if o := vm.asObj(printed); o != nil && o.Bytes != nil {
		return o.BytesAsString(), nil
	}
	if o := vm.asObj(result); o != nil {
		return o.SqInstName(), nil
	}
	return fmt.Sprintf("%v", result), nil
}
