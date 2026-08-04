package vm

import "fmt"

// buildSelectorMap scans all MethodDictionaries in old space and maps each
// CompiledMethod to its selector string (for backtraces).
func (vm *Interpreter) buildSelectorMap() map[*Object]string {
	names := map[*Object]string{}
	mdClass := vm.asObj(vm.SpecialObjects[SplObClassMethodContext]) // unused guard
	_ = mdClass
	for obj := vm.Image.FirstOldObject; obj != nil; obj = obj.NextObject {
		// A MethodDict: pointers[1] is the method Array, pointers[2..] selectors.
		if len(obj.Pointers) < 2 {
			continue
		}
		arr := vm.asObj(obj.Pointers[MethodDictArray])
		if arr == nil || arr.Pointers == nil {
			continue
		}
		if len(arr.Pointers)+MethodDictSelectorStart != len(obj.Pointers) {
			continue
		}
		for j := 0; j < len(arr.Pointers); j++ {
			m := vm.asObj(arr.Pointers[j])
			sel := vm.asObj(obj.Pointers[MethodDictSelectorStart+j])
			if m != nil && m.IsMethod() && sel != nil && sel.Bytes != nil {
				if _, ok := names[m]; !ok {
					names[m] = sel.BytesAsString()
				}
			}
		}
	}
	return names
}

// Backtrace returns up to limit frames of the current sender chain, as
// "ReceiverClass>>selector" strings (best-effort).
func (vm *Interpreter) Backtrace(limit int) []string {
	names := vm.buildSelectorMap()
	var out []string
	ctx := vm.ActiveContext
	for ctx != nil && !ctx.IsNil && len(out) < limit {
		var method *Object
		block := ""
		if _, isInt := ctx.Pointers[ContextMethod].(int); isInt {
			home := vm.asObj(ctx.Pointers[BlockContextHome])
			if home != nil {
				method = vm.asObj(home.Pointers[ContextMethod])
			}
			block = "[] in "
		} else {
			method = vm.asObj(ctx.Pointers[ContextMethod])
		}
		recvClass := "?"
		if r, ok := ctx.Pointers[ContextReceiver].(*Object); ok && r.SqClass != nil {
			recvClass = r.SqClass.ClassName()
		} else if _, ok := ctx.Pointers[ContextReceiver].(int); ok {
			recvClass = "SmallInteger"
		}
		sel := "?"
		if method != nil {
			if s, ok := names[method]; ok {
				sel = s
			} else {
				sel = fmt.Sprintf("<prim %d>", method.MethodPrimitiveIndex())
			}
		}
		out = append(out, fmt.Sprintf("%s%s>>%s", block, recvClass, sel))
		ctx = vm.asObj(ctx.Pointers[ContextSender])
	}
	return out
}
