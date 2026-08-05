package vm

import (
	"os/exec"
	"runtime"
	"strings"
)

// Host system clipboard access without cgo, by shelling out to the platform's
// clipboard tools. Used by primitiveClipboardText (141) so Cmd-C / Cmd-V in the
// Squeak editor round-trip through the OS clipboard.

func hostClipboardRead() string {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbpaste")
	case "linux":
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard", "-o")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--output")
		}
	case "windows":
		cmd = exec.Command("powershell", "-NoProfile", "-Command", "Get-Clipboard")
	}
	if cmd == nil {
		return ""
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	s := string(out)
	if runtime.GOOS == "windows" {
		s = strings.TrimRight(s, "\r\n") // PowerShell appends a newline
	}
	return s
}

func hostClipboardWrite(s string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		}
	case "windows":
		cmd = exec.Command("clip")
	}
	if cmd == nil {
		return
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		return
	}
	_, _ = stdin.Write([]byte(s))
	_ = stdin.Close()
	_ = cmd.Wait()
}
