// Command squeak is the headless Go Squeak VM (no GUI dependency).
//
//	squeak <image>                      load and print diagnostics
//	squeak -boot N <image>              run up to N bytecodes (0 = until idle)
//	squeak -boot 0 -snap out.png <img>  boot to idle, save the screen as PNG
//	squeak -eval 'EXPR' <image>         print-it: evaluate and print the result
//	squeak -doit 'EXPR' <image>         do-it: evaluate for effect
//
// For the interactive window, use the squeakgui command (make run).
package main

import (
	"flag"
	"fmt"
	"image/png"
	"os"
	"runtime/debug"
	"sort"

	"squeakg/internal/vm"
)

func main() {
	boot := flag.Int("boot", -1, "run up to N bytecodes (0 = run until idle); -1 = don't run")
	snap := flag.String("snap", "", "after booting, render the Display to this PNG file")
	profile := flag.Bool("profile", false, "print a backtrace and primitive histogram after booting")
	eval := flag.String("eval", "", "print-it: compile+run a Smalltalk expression and print its printString")
	doit := flag.String("doit", "", "do-it: compile+run a Smalltalk expression for its effect")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: squeak [flags] <image-file>")
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

	// Do-it / print-it: evaluate a Smalltalk expression and exit.
	if *eval != "" || *doit != "" {
		if *doit != "" {
			if _, err := interp.Evaluate(*doit); err != nil {
				fmt.Fprintln(os.Stderr, "doit:", err)
				os.Exit(1)
			}
		}
		if *eval != "" {
			out, err := interp.PrintIt(*eval)
			if err != nil {
				fmt.Fprintln(os.Stderr, "print-it:", err)
				os.Exit(1)
			}
			fmt.Println(out)
		}
		return
	}

	vm.PrintDiagnostics(img, os.Stdout)
	fmt.Println(interp.DescribeInitialContext())

	if *snap != "" && *boot < 0 {
		*boot = 0 // -snap implies boot-to-idle
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
