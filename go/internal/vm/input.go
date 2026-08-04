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
	vm.prim.mouseX = x
	vm.prim.mouseY = y
	vm.prim.buttons = buttons
}

// PushKey enqueues a keyboard code (modifiers<<8 | charCode).
func (vm *Interpreter) PushKey(code int) {
	vm.prim.keys = append(vm.prim.keys, code)
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
