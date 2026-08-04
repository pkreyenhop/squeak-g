// Command squeak loads and runs a Squeak image with the Go VM.
//
//	squeak <image>                 load and print diagnostics
//	squeak -boot N <image>         run up to N bytecodes (0 = until idle)
//	squeak -boot 0 -snap out.png <image>   boot to idle, save the screen as PNG
//	squeak -display                open the Ebitengine window (demo backend)
//	squeak -boot 0 -profile <image>        add a backtrace + primitive histogram
package main

import (
	"flag"
	"fmt"
	"image/png"
	"os"
	"runtime/debug"
	"sort"

	"squeakg/internal/display"
	"squeakg/internal/vm"
)

func main() {
	showDisplay := flag.Bool("display", false, "open the Ebitengine display shell (demo backend)")
	width := flag.Int("w", 800, "display width (demo backend)")
	height := flag.Int("h", 600, "display height (demo backend)")
	boot := flag.Int("boot", -1, "run up to N bytecodes (0 = run until idle); -1 = don't run")
	snap := flag.String("snap", "", "after booting, render the Display to this PNG file")
	profile := flag.Bool("profile", false, "print a backtrace and primitive histogram after booting")
	run := flag.Bool("run", false, "run the image live in an interactive window")
	fullscreen := flag.Bool("fullscreen", false, "open the interactive window fullscreen")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: squeak [flags] <image-file>")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showDisplay {
		if err := display.Run(display.NewDemoBackend(*width, *height), *fullscreen); err != nil {
			fmt.Fprintln(os.Stderr, "display:", err)
			os.Exit(1)
		}
		return
	}

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
	vm.PrintDiagnostics(img, os.Stdout)

	interp, err := vm.NewInterpreter(img)
	if err != nil {
		fmt.Fprintln(os.Stderr, "interpreter init:", err)
		return
	}
	fmt.Println(interp.DescribeInitialContext())

	// Interactive: run the image live in a window (implies no headless boot).
	if *run {
		fmt.Println("booting for interactive display...")
		interp.BootToIdle(2_000_000)
		fmt.Printf("booted in %d bytecodes; opening window\n", interp.ByteCodeCount)
		be := newVMBackend(interp, img, "Squeak-G — "+flag.Arg(0))
		if err := display.Run(be, *fullscreen); err != nil {
			fmt.Fprintln(os.Stderr, "display:", err)
			os.Exit(1)
		}
		return
	}

	// -snap implies booting until idle unless a bytecode budget was given.
	if *snap != "" && *boot < 0 {
		*boot = 0
	}

	if *boot >= 0 {
		if *boot == 0 {
			fmt.Println("booting: running until idle...")
		} else {
			fmt.Printf("booting: running up to %d bytecodes...\n", *boot)
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Fprintf(os.Stderr, "PANIC after %d bytecodes: %v\n%s\n",
						interp.ByteCodeCount, r, debug.Stack())
				}
			}()
			if *boot == 0 {
				interp.BootToIdle(2_000_000)
			} else {
				interp.Run(*boot)
			}
		}()
		fmt.Printf("executed %d bytecodes, %d sends\n", interp.ByteCodeCount, interp.SendCount)
		bt, bd := interp.BltStats()
		fmt.Printf("bitblt: %d total, %d to Display\n", bt, bd)
		if *profile {
			printProfile(interp)
		}
	}

	if *snap != "" {
		rgba, info, err := img.RenderDisplay()
		fmt.Printf("display: %dx%d depth=%d\n", info.Width, info.Height, info.Depth)
		if err != nil {
			fmt.Fprintln(os.Stderr, "render display:", err)
			os.Exit(1)
		}
		f, err := os.Create(*snap)
		if err != nil {
			fmt.Fprintln(os.Stderr, "create png:", err)
			os.Exit(1)
		}
		defer f.Close()
		if err := png.Encode(f, rgba); err != nil {
			fmt.Fprintln(os.Stderr, "encode png:", err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", *snap)
	}
}

func printProfile(interp *vm.Interpreter) {
	fmt.Println("backtrace at stop:")
	for _, l := range interp.Backtrace(30) {
		fmt.Println("  " + l)
	}
	type pk struct{ i, n int }
	var pks []pk
	for i, n := range interp.PrimCounts() {
		if n > 0 {
			pks = append(pks, pk{i, n})
		}
	}
	sort.Slice(pks, func(a, b int) bool { return pks[a].n > pks[b].n })
	fmt.Println("top primitives:")
	for k := 0; k < len(pks) && k < 15; k++ {
		fmt.Printf("  prim %d: %d\n", pks[k].i, pks[k].n)
	}
}
