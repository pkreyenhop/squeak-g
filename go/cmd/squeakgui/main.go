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
	scale := flag.Int("scale", 1, "integer UI magnification (2 = double-size fonts, crisp)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: squeakgui [-fullscreen] [-scale N] <image-file>")
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

	if *scale < 1 {
		*scale = 1
	}
	// Adopt the monitor's resolution (divided by the scale factor) before
	// booting: the image queries the screen size on start-up
	// (primitiveScreenSize) and sizes its Display to it, so this must be set
	// before BootToIdle. A scale of 2 gives a 2x-magnified, crisp UI.
	if mw, mh := display.MonitorSize(); mw > 0 && mh > 0 {
		dw, dh := mw / *scale, mh / *scale
		interp.SetScreenSize(dw, dh)
		fmt.Printf("host screen: %dx%d, display: %dx%d (scale %dx)\n", mw, mh, dw, dh, *scale)
	}

	fmt.Println("booting for interactive display...")
	interp.Boot(60_000_000)
	fmt.Printf("booted in %d bytecodes; opening window\n", interp.ByteCodeCount)

	be := newVMBackend(interp, img, "Squeak-G — "+flag.Arg(0))
	if err := display.Run(be, *fullscreen, *scale); err != nil {
		fmt.Fprintln(os.Stderr, "display:", err)
		os.Exit(1)
	}
}
