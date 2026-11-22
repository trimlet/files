// Trimlet Files: a simple file manager with two panels, driven by the keyboard.
// See docs/apps/files.md.
package main

import (
	"github.com/trimlet/gui"
)

func main() {
	a, w := gui.NewWindow("local.trimletfiles", "Trimlet Files")
	gui.UsePalette("teal") // Files' color
	left, right := loadDirs()
	m := newModel(left, right, openWithSystem, a.Quit)
	m.onDirs = saveDirsFile
	var screen *gui.Screen
	m.ui = func(f func()) { screen.Do(f) }
	screen = gui.NewScreen(a, w, m)
	gui.Run(a, w)
}
