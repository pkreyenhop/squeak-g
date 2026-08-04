package vm

import (
	"errors"
	"fmt"
	"time"
)

// Interpreter is the Go port of Squeak.Interpreter. So far it ports the image
// state loading and the initial-context setup; the bytecode dispatch loop and
// primitives are the next milestone (see Interpret).
type Interpreter struct {
	Image *Image

	SpecialObjects   []Value
	SpecialSelectors []Value
	NilObj           *Object
	TrueObj          *Object
	FalseObj         *Object
	HasClosures      bool

	// Execution registers (valid once a context is fetched).
	ActiveContext *Object
	HomeContext   *Object
	Method        *Object
	Receiver      Value
	PC            int // absolute bytecode index into Method.Bytes
	SP            int // absolute stack index into ActiveContext.Pointers

	ByteCodeCount int
	SendCount     int

	startupTime time.Time
}

// NewInterpreter builds an interpreter over a loaded image and prepares the
// initial context, mirroring Squeak.Interpreter>>initialize's load path
// (loadImageState + loadInitialContext).
func NewInterpreter(img *Image) (*Interpreter, error) {
	vm := &Interpreter{Image: img, startupTime: time.Now()}
	if err := vm.loadImageState(); err != nil {
		return nil, err
	}
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
	return nil
}

// loadInitialContext follows SchedulerAssociation -> activeProcess ->
// suspendedContext and fetches its registers.
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
	return nil
}

func (vm *Interpreter) fetchContextRegisters(ctx *Object) {
	meth := ctx.Pointers[ContextMethod]
	if _, isInt := meth.(int); isInt {
		// Integer method field => this is a block context; home holds the method.
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

func (vm *Interpreter) decodeSqueakPC(squeakPC int, method *Object) int {
	return squeakPC - len(method.Pointers)*4 - 1
}

func (vm *Interpreter) decodeSqueakSP(squeakSP int) int {
	return squeakSP + (ContextTempFrameStart - 1)
}

// DescribeInitialContext returns a one-line description of where execution would
// begin — useful to verify the pointer graph without a running interpreter.
func (vm *Interpreter) DescribeInitialContext() string {
	sel := "?"
	if vm.Method != nil && vm.Method.SqClass != nil {
		sel = vm.Method.SqClass.ClassName()
	}
	recv := "?"
	if r, ok := vm.Receiver.(*Object); ok {
		recv = r.SqInstName()
	} else if n, ok := vm.Receiver.(int); ok {
		recv = fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("initial context: method(%s) receiver=%s pc=%d sp=%d numLits=%d numArgs=%d prim=%d",
		sel, recv, vm.PC, vm.SP, vm.Method.MethodNumLits(), vm.Method.MethodNumArgs(), vm.Method.MethodPrimitiveIndex())
}

// Interpret runs the bytecode dispatch loop.
//
// NOT YET IMPLEMENTED. This is the next and largest porting milestone: the
// bytecode loop from vm.interpreter.js (~1900 lines) plus the primitive set
// from vm.primitives.js (~2200 lines) and BitBlt. The object model, image
// loader, and context setup that feed it are complete and verified.
func (vm *Interpreter) Interpret() error {
	return errors.New("bytecode interpreter not yet ported (see internal/vm/interp.go)")
}

func asIntValue(v Value) int {
	if n, ok := v.(int); ok {
		return n
	}
	return 0
}
