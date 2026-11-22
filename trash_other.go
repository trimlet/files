//go:build !darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// moveToTrash moves the file or folder to the Recycle Bin (Windows) or the Trash (Linux).
// Not tried yet on either system. Where it went is not known here, so it cannot be undone ("").
func moveToTrash(path string) (string, error) {
	return "", trashItem(path)
}

func trashItem(path string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// The path goes in an environment variable, so quotes in the name need no escaping.
		script := `Add-Type -AssemblyName Microsoft.VisualBasic; ` +
			`$p = $env:TRIMLET_TRASH; ` +
			`if (Test-Path -LiteralPath $p -PathType Container) { [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteDirectory($p, 'OnlyErrorDialogs', 'SendToRecycleBin') } ` +
			`else { [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($p, 'OnlyErrorDialogs', 'SendToRecycleBin') }`
		cmd = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
		cmd.Env = append(os.Environ(), "TRIMLET_TRASH="+path)
	} else {
		cmd = exec.Command("gio", "trash", "--", path)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return err
	}
	return nil
}
