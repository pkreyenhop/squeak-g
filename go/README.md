# SqueakG — a Go port of SqueakJS

A work-in-progress port of the [SqueakJS](https://squeak.js.org/) Smalltalk
virtual machine (in the parent directory) to Go, with a native
[Ebitengine](https://ebitengine.org/) display.

This is a **bootstrap MVP**: the foundational, exacting layers (object model +
image loader) are ported and verified against a real image, and the
window/input shell is in place. The bytecode interpreter and primitives — the
largest remaining piece — are next.

## Status

| Layer | JS source | Go | State |
|---|---|---|---|
| Constants / layouts | `vm.js` | `internal/vm/constants.go` | ✅ done |
| Object model (classic) | `vm.object.js` | `internal/vm/object.go` | ✅ done |
| Image loader (classic 6502) | `vm.image.js` | `internal/vm/image.go` | ✅ done & verified |
| Interpreter init path | `vm.interpreter.js` | `internal/vm/interp.go` | ✅ done (scheduler→process→context) |
| Bytecode dispatch loop | `vm.interpreter.js` | `internal/vm/interp.go` | ⛔ **not yet ported** |
| Primitives | `vm.primitives.js` | — | ⛔ not started |
| BitBlt | `plugins/BitBltPlugin.js` | — | ⛔ not started |
| Display + input shell | `vm.display.browser.js` | `internal/display/` | ✅ shell done (demo backend) |
| Spur / 64-bit image format | `vm.object.spur.js` | — | ⛔ classic 32-bit only so far |

"Verified" means: loading `demo/mini.image` decodes 15,893 objects, all class
names resolve, and the scheduler → active process → suspended context → method
→ receiver chain resolves to `aSystemDictionary` — i.e. the whole pointer graph
is correctly rectified. See `internal/vm/image_test.go`.

## Build & run

Requires Go 1.24+.

Load an image and print diagnostics (no GUI, no cgo needed):

```bash
cd go
go run ./cmd/squeak ../demo/mini.image
```

Open the display shell (currently a demo backend until the interpreter runs):

```bash
cd go
go run ./cmd/squeak -display
```

Run the tests:

```bash
cd go
go test ./...
```

## The cgo situation (important)

The goal was a native Go GUI with **no cgo**. Reality on desktop:

- **Ebitengine needs cgo on macOS and Linux**; only **Windows** is cgo-free
  (its README says so explicitly, and the macOS backend compiles Objective-C
  through cgo). Fyne and Gio have the same macOS constraint.
- A real native window on macOS *must* call Cocoa — via cgo, or via hand-rolled
  `purego` calls. Truly zero-cgo-everywhere would mean a browser-canvas or
  terminal display instead of a native window.

We chose **Ebitengine, accepting cgo on macOS/Linux** for a real native window.
Ebitengine is the right fit because Squeak's display is just a framebuffer:
BitBlt renders one bitmap, and the shell blits that RGBA buffer to the window
and feeds mouse/keyboard events back — exactly Ebitengine's model.

Build for Windows with no cgo:

```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./cmd/squeak
```

## Architecture

```
cmd/squeak         CLI entry point (diagnostics today; boots the VM later)
internal/vm        the virtual machine
  constants.go     header codes, special-object indices, class layouts
  object.go        Object struct + format decoding + class/method/context accessors
  image.go         classic image reader (header, oopMap, install, fixups)
  interp.go        Interpreter state + init path (bytecode loop is the TODO)
  diagnostics.go   human-readable image summary
internal/display   Ebitengine window/input shell
  shell.go         Backend interface + ebiten.Game adapter
  input.go         mouse/keyboard polling -> Backend
  demo.go          placeholder backend until the VM produces frames
```

A Smalltalk value is modeled as Go `any`, holding either an `int`
(a SmallInteger) or an `*Object` — mirroring SqueakJS, where SmallIntegers are
plain numbers and everything else is an object.

The VM will drive the window by implementing `display.Backend`: `Frame()`
returns the Squeak Display form as RGBA, `Step()` runs the VM for a frame, and
`Mouse`/`Key` push input events into the VM's event queue.

## Roadmap (next steps)

1. **Bytecode interpreter loop** — port `vm.interpreter.js`: `interpret`,
   `fetchNextBytecode`, sends, returns, closures, context switching.
2. **Core primitives** — port `vm.primitives.js`: arithmetic, `at:`/`at:put:`,
   object allocation, `perform:`, process/semaphore, time.
3. **BitBlt** — port `BitBltPlugin.js` so the Display form gets pixels.
4. **Wire the real display Backend** — replace the demo backend with one that
   reads the Squeak Display form and forwards the event queue.
5. **Spur + 64-bit image format** — port `vm.object.spur.js` and the Spur branch
   of the loader to run modern images (`headless.image`, Cuis, etc.).
6. Later: image saving, more plugins (Sockets, FFI, sound), optional JIT.
```
