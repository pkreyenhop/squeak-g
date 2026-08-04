// Command squeak loads a Squeak image and (eventually) runs it.
//
// This is the bootstrap MVP of the Go port. Today it can:
//   - load a classic-format image and print diagnostics (default), and
//   - open the no-cgo Ebitengine display shell with a demo backend (-display).
//
// The interpreter is being ported next; once it runs, -display will show the
// live Squeak screen instead of the demo pattern.
package main

import (
	"flag"
	"fmt"
	"os"

	"squeakg/internal/display"
	"squeakg/internal/vm"
)

func main() {
	showDisplay := flag.Bool("display", false, "open the Ebitengine display shell (demo backend for now)")
	width := flag.Int("w", 800, "display width (demo backend)")
	height := flag.Int("h", 600, "display height (demo backend)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: squeak [flags] <image-file>")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showDisplay {
		if err := display.Run(display.NewDemoBackend(*width, *height)); err != nil {
			fmt.Fprintln(os.Stderr, "display:", err)
			os.Exit(1)
		}
		return
	}

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}
	path := flag.Arg(0)
	buf, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read image:", err)
		os.Exit(1)
	}
	img, err := vm.ReadImage(path, buf)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load image:", err)
		os.Exit(1)
	}
	vm.PrintDiagnostics(img, os.Stdout)

	// Exercise the interpreter's init path (scheduler -> process -> context).
	if interp, err := vm.NewInterpreter(img); err != nil {
		fmt.Fprintln(os.Stderr, "interpreter init:", err)
	} else {
		fmt.Println(interp.DescribeInitialContext())
	}
}
