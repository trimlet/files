package main

import (
	"os/exec"
	"runtime"
)

// openWithSystem gives the file to the system, which opens it with its default app:
// the same as a double click in Finder or Explorer.
func openWithSystem(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}
