// Package vm is a Go port of the SqueakJS Smalltalk virtual machine.
//
// This file ports the constant tables from vm.js: object header type codes,
// special-object array indices, well-known instance-variable layouts, numeric
// limits and primitive error codes. Values are kept identical to the JS source
// so image files load bit-for-bit compatibly.
package vm

// Version / platform strings (from vm.js "version").
const (
	VMVersion            = "SqueakG 0.1.0 (SqueakJS 1.3.3 port)"
	VMMakerVersion       = "[VMMakerJS-bf.17 VMMaker-bf.353]"
	VMInterpreterVersion = "JSInterpreter VMMaker.js-codefrau.1"
	PlatformName         = "Go"
)

// Object header type codes (low 2 bits of a classic header word).
const (
	HeaderTypeMask         = 3
	HeaderTypeSizeAndClass = 0 // 3-word header
	HeaderTypeClass        = 1 // 2-word header
	HeaderTypeFree         = 2 // free block
	HeaderTypeShort        = 3 // 1-word header
)

// Indices into the SpecialObjects array.
const (
	SplObNilObject                 = 0
	SplObFalseObject               = 1
	SplObTrueObject                = 2
	SplObSchedulerAssociation      = 3
	SplObClassBitmap               = 4
	SplObClassInteger              = 5
	SplObClassString               = 6
	SplObClassArray                = 7
	SplObSmalltalkDictionary       = 8
	SplObClassFloat                = 9
	SplObClassMethodContext        = 10
	SplObClassBlockContext         = 11
	SplObClassPoint                = 12
	SplObClassLargePositiveInteger = 13
	SplObTheDisplay                = 14
	SplObClassMessage              = 15
	SplObClassCompiledMethod       = 16
	SplObTheLowSpaceSemaphore      = 17
	SplObClassSemaphore            = 18
	SplObClassCharacter            = 19
	SplObSelectorDoesNotUnderstand = 20
	SplObSelectorCannotReturn      = 21
	SplObProcessSignalingLowSpace  = 22
	SplObSpecialSelectors          = 23
	SplObCharacterTable            = 24
	SplObSelectorMustBeBoolean     = 25
	SplObClassByteArray            = 26
	SplObClassProcess              = 27
	SplObCompactClasses            = 28
	SplObTheTimerSemaphore         = 29
	SplObTheInterruptSemaphore     = 30
	SplObFloatProto                = 31
	SplObSelectorCannotInterpret   = 34
	SplObMethodContextProto        = 35
	SplObClassBlockClosure         = 36
	SplObClassFullBlockClosure     = 37
	SplObExternalObjectsArray      = 38
	SplObClassPseudoContext        = 39
	SplObClassTranslatedMethod     = 40
	SplObTheFinalizationSemaphore  = 41
	SplObClassLargeNegativeInteger = 42
	SplObSelectorAboutToReturn     = 48
	SplObSelectorRunWithIn         = 49
)

// Well-known instance-variable layouts (from vm.js "known classes").
const (
	ClassSuperclass = 0
	ClassMdict      = 1
	ClassFormat     = 2
	ClassName       = 6

	ClassBindingValue = 1

	ContextSender             = 0
	ContextInstructionPointer = 1
	ContextStackPointer       = 2
	ContextMethod             = 3
	ContextClosureOrVirtualPC = 4
	ContextReceiver           = 5
	ContextTempFrameStart     = 6
	ContextSmallFrameSize     = 16
	ContextLargeFrameSize     = 56
	BlockContextCaller        = 0
	BlockContextArgumentCount = 3
	BlockContextInitialIP     = 4
	BlockContextHome          = 5

	ClosureOuterContext     = 0
	ClosureStartPC          = 1
	ClosureNumArgs          = 2
	ClosureFirstCopiedValue = 3
	ClosureFullMethod       = 1
	ClosureFullReceiver     = 3
	ClosureFullFirstCopied  = 4

	StreamArray    = 0
	StreamPosition = 1
	StreamLimit    = 2

	ProcSchedProcessLists  = 0
	ProcSchedActiveProcess = 1

	LinkNextLink        = 0
	LinkedListFirstLink = 0
	LinkedListLastLink  = 1

	SemaphoreExcessSignals = 2
	MutexOwner             = 2

	ProcSuspendedContext = 1
	ProcPriority         = 2
	ProcMyList           = 3

	AssnKey   = 0
	AssnValue = 1

	MethodDictArray         = 1
	MethodDictSelectorStart = 2

	MessageSelector    = 0
	MessageArguments   = 1
	MessageLookupClass = 2

	PointX = 0
	PointY = 1

	LargeIntegerBytes = 0
	LargeIntegerNeg   = 1
)

// Numeric limits (from vm.js "constants").
const (
	MinSmallInt          = -0x40000000
	MaxSmallInt          = 0x3FFFFFFF
	NonSmallInt          = -0x50000000
	MillisecondClockMask = 0x1FFFFFFF
)

// Primitive error codes (from vm.js "error codes").
const (
	PrimNoErr              = 0
	PrimErrGenericFailure  = 1
	PrimErrBadReceiver     = 2
	PrimErrBadArgument     = 3
	PrimErrBadIndex        = 4
	PrimErrBadNumArgs      = 5
	PrimErrInappropriate   = 6
	PrimErrUnsupported     = 7
	PrimErrNoModification  = 8
	PrimErrNoMemory        = 9
	PrimErrNoCMemory       = 10
	PrimErrNotFound        = 11
	PrimErrBadMethod       = 12
	PrimErrNamedInternal   = 13
	PrimErrObjectMayMove   = 14
	PrimErrLimitExceeded   = 15
	PrimErrObjectIsPinned  = 16
	PrimErrWritePastObject = 17
)
