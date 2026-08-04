// Command squeakgui runs a Squeak image live in an interactive Ebitengine
// window (mouse + keyboard). This command links the GUI toolkit; for headless
// use (boot, snapshot, eval) use the squeak command instead.
//
//	squeakgui <image>              windowed
//	squeakgui -fullscreen <image>  fullscreen
package main

import (
	"flag"
	"fmt"
	"os"

	"squeakg/internal/display"
	"squeakg/internal/vm"
)

func main() {
	fullscreen := flag.Bool("fullscreen", false, "open the window fullscreen")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: squeakgui [-fullscreen] <image-file>")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}

	buf, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "read image:", err)
		os.Exit(1)
	}
	img, err := vm.ReadImage(flag.Arg(0), buf)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load image:", err)
		os.Exit(1)
	}
	interp, err := vm.NewInterpreter(img)
	if err != nil {
		fmt.Fprintln(os.Stderr, "interpreter init:", err)
		os.Exit(1)
	}

	fmt.Println("booting for interactive display...")
	interp.BootToIdle(2_000_000)
	fmt.Printf("booted in %d bytecodes; opening window\n", interp.ByteCodeCount)

	be := newVMBackend(interp, img, "Squeak-G — "+flag.Arg(0))
	if err := display.Run(be, *fullscreen); err != nil {
		fmt.Fprintln(os.Stderr, "display:", err)
		os.Exit(1)
	}
}
