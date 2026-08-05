package vm

// Interactive input support for old MVC images, which poll the Sensor via
// primitiveMousePoint/MouseButtons/KeyboardNext/KeyboardPeek. The host display
// backend feeds state in through the Interpreter setters below.

// Squeak mouse/modifier bit constants (from vm.input.js).
const (
	MouseBlue      = 1
	MouseYellow    = 2
	MouseRed       = 4
	MouseAll       = 7
	KeyboardShift  = 8
	KeyboardCtrl   = 16
	KeyboardOption = 32
	KeyboardCmd    = 64
	KeyboardAll    = 120
)

// SetMouse records the pointer position (in Display pixels) and the full button
// bitmask (mouse buttons OR modifier bits).
func (vm *Interpreter) SetMouse(x, y, buttons int) {
	if vm.prim.mouseX != x || vm.prim.mouseY != y || vm.prim.buttons != buttons {
		vm.prim.displayIdle = 0
		vm.isIdle = false
		btnMask := buttons & MouseAll
		modMask := buttons & KeyboardAll
		vm.prim.enqueueEvent([8]int{1, vm.prim.millisecondClockValue(), x, y, btnMask, modMask, 0, 0})
	}
	vm.prim.mouseX = x
	vm.prim.mouseY = y
	vm.prim.buttons = buttons
}

// PushKey enqueues a keyboard code (modifiers<<8 | charCode).
func (vm *Interpreter) PushKey(code int) {
	vm.prim.displayIdle = 0
	vm.isIdle = false
	vm.prim.keys = append(vm.prim.keys, code)
	charCode := code & 0xFF
	modifiers := (code >> 8) & 0xFF
	vm.prim.enqueueEvent([8]int{2, vm.prim.millisecondClockValue(), charCode, 0, modifiers, charCode, 0, 0})
}

// enqueueEvent records an event for the event-driven (primitiveGetNextEvent)
// input model and signals the input semaphore. It only queues when the image
// has actually registered an input semaphore, so polling-only images (which
// never drain the queue via getNextEvent) don't leak memory. The queue is also
// capped as a safety net.
func (p *Primitives) enqueueEvent(evt [8]int) {
	if p.inputSemaphoreIdx <= 0 {
		return
	}
	if len(p.eventQueue) >= 256 {
		p.eventQueue = p.eventQueue[1:]
	}
	p.eventQueue = append(p.eventQueue, evt)
	p.vm.SignalSemaphoreWithIndex(p.inputSemaphoreIdx)
}

// ResetIdle resets the idle counter and wakes the interpreter.
func (vm *Interpreter) ResetIdle() {
	vm.prim.displayIdle = 0
	vm.isIdle = false
}

// SetScreenSize records the host screen size answered by primitiveScreenSize.
func (vm *Interpreter) SetScreenSize(w, h int) {
	vm.prim.screenW = w
	vm.prim.screenH = h
}

// DisplayModified reports whether the Display changed since the last call, so
// the host can skip re-blitting an unchanged frame.
func (vm *Interpreter) DisplayModified() bool {
	m := vm.prim.displayDirty
	vm.prim.displayDirty = false
	return m
}

// primitiveClipboardText (141) bridges the Squeak clipboard to the host OS
// clipboard: 0 args reads it (answers a String), 1 arg writes it.
func (p *Primitives) primitiveClipboardText(argCount int) bool {
	switch argCount {
	case 0:
		p.vm.popNandPush(1, p.makeStString(hostClipboardRead()))
		return true
	case 1:
		if s := p.asObj(p.vm.top()); s != nil && s.Bytes != nil {
			hostClipboardWrite(s.BytesAsString())
		}
		p.vm.pop() // answer the receiver
		return true
	}
	return false
}

func (p *Primitives) primitiveMousePoint(argCount int) bool {
	return p.popNandPushIfOK(argCount+1, p.makePointWithXandY(p.mouseX, p.mouseY))
}

func (p *Primitives) primitiveMouseButtons(argCount int) bool {
	p.popNandPushIfOK(argCount+1, p.buttons)
	// The image polls buttons when done displaying; break out so the host can
	// render promptly, and go fully idle after many idle polls.
	p.vm.breakOut = true
	p.displayIdle++
	if p.displayIdle > 20 {
		p.vm.goIdle()
	}
	return true
}

func (p *Primitives) primitiveKeyboardNext(argCount int) bool {
	if len(p.keys) == 0 {
		return false // no key: fail, Smalltalk answers nil
	}
	k := p.keys[0]
	p.keys = p.keys[1:]
	return p.popNandPushIfOK(argCount+1, k)
}

func (p *Primitives) primitiveKeyboardPeek(argCount int) bool {
	if len(p.keys) == 0 {
		return p.popNandPushIfOK(argCount+1, p.vm.NilObj)
	}
	return p.popNandPushIfOK(argCount+1, p.keys[0])
}

func (p *Primitives) primitiveScreenSize(argCount int) bool {
	if p.screenW <= 0 || p.screenH <= 0 {
		return false
	}
	return p.popNandPushIfOK(argCount+1, p.makePointWithXandY(p.screenW, p.screenH))
}

func (p *Primitives) primitiveInputSemaphore(argCount int) bool {
	if argCount != 1 {
		return false
	}
	idx := p.stackInteger(0)
	if !p.success {
		return false
	}
	p.inputSemaphoreIdx = idx
	p.vm.popN(argCount)
	return true
}

func (p *Primitives) primitiveGetNextEvent(argCount int) bool {
	if argCount != 1 {
		return false
	}
	evtBuf := p.asObj(p.vm.stackValue(0))
	if evtBuf == nil || !evtBuf.IsPointers() || len(evtBuf.Pointers) < 8 {
		return false
	}
	p.displayIdle++
	if len(p.eventQueue) == 0 {
		for i := 0; i < 8; i++ {
			evtBuf.Pointers[i] = 0
		}
	} else {
		evt := p.eventQueue[0]
		p.eventQueue = p.eventQueue[1:]
		for i := 0; i < 8; i++ {
			evtBuf.Pointers[i] = evt[i]
		}
	}
	p.vm.popN(argCount)
	return true
}
