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

// SchedulerReport summarizes the scheduler: the active process plus every
// process queued by priority, with the top of each suspended context, and the
// timer state. Used to diagnose why an image won't reach idle.
func (vm *Interpreter) SchedulerReport() string {
	names := vm.buildSelectorMap()
	frame := func(ctx *Object) string {
		if ctx == nil || ctx.IsNil {
			return "<nil ctx>"
		}
		m := vm.asObj(ctx.Pointers[ContextMethod])
		if _, isInt := ctx.Pointers[ContextMethod].(int); isInt {
			if h := vm.asObj(ctx.Pointers[BlockContextHome]); h != nil {
				m = vm.asObj(h.Pointers[ContextMethod])
			}
		}
		if m == nil {
			return "<?>"
		}
		if s, ok := names[m]; ok {
			return s
		}
		return "<unnamed>"
	}
	var b []byte
	out := func(s string) { b = append(b, s...) }
	sched := vm.prim.getScheduler()
	active := vm.asObj(sched.Pointers[ProcSchedActiveProcess])
	out("active process: prio " + itoa(asIntValue(active.Pointers[ProcPriority])) + " @ " + frame(vm.ActiveContext) + "\n")
	lists := vm.asObj(sched.Pointers[ProcSchedProcessLists])
	for i := len(lists.Pointers) - 1; i >= 0; i-- {
		l := vm.asObj(lists.Pointers[i])
		p := vm.asObj(l.Pointers[LinkedListFirstLink])
		for p != nil && !p.IsNil {
			out("  ready prio " + itoa(i+1) + ": suspended @ " + frame(vm.asObj(p.Pointers[ProcSuspendedContext])) + "\n")
			p = vm.asObj(p.Pointers[LinkNextLink])
		}
	}
	// Dump every Process instance's suspended stack (finds ones blocked on a
	// semaphore, which are not in the scheduler's ready lists).
	stack := func(ctx *Object, n int) string {
		s := ""
		for i := 0; i < n && ctx != nil && !ctx.IsNil; i++ {
			s += " " + frame(ctx)
			ctx = vm.asObj(ctx.Pointers[ContextSender])
		}
		return s
	}
	procClass := vm.asObj(vm.spl(SplObClassProcess))
	for p := vm.Image.SomeInstanceOf(procClass); p != nil; p = vm.Image.NextInstanceAfter(p) {
		tag := ""
		if p == active {
			tag = " [ACTIVE]"
		}
		out("process prio " + itoa(asIntValue(p.Pointers[ProcPriority])) + tag + ":" + stack(vm.asObj(p.Pointers[ProcSuspendedContext]), 5) + "\n")
	}
	out("nextWakeupTick=" + itoa(vm.nextWakeupTick) + " now=" + itoa(vm.prim.millisecondClockValue()) + "\n")
	timer := vm.asObj(vm.spl(SplObTheTimerSemaphore))
	out("timer sema set=" + boolStr(timer != nil && !timer.IsNil))
	return string(b)
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }
func boolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
