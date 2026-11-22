package main

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/trimlet/gui"
)

// model is the whole app: two panels, which one is active, and the one selection.
type model struct {
	panels [2]*panel
	active int

	// There is one selection at a time: the names selected in panel selPanel (-1 = none).
	selPanel int
	sel      map[string]bool

	hidden  bool        // hidden files and folders are shown (both panels)
	status  gui.Line    // message on the bottom line, cleared at the next key
	confirm *gui.Dialog // the question window, when open
	edit    *editField  // the name being written in its row (R, N), when open
	open    func(path string) error
	trash   func(path string) (string, error) // to the Trash; returns where it went ("" = unknown)
	last    *undoOp                           // the last operation, while it can still be undone
	quit    func()
	// ui runs f in the user interface and redraws; it can be called from any goroutine (nil in tests).
	ui func(f func())
	// onDirs, if set, is told the panels' folders whenever one changes (nil in tests).
	onDirs func(left, right string)

	flash    string // the key lit in the hint band, as written there ("tab", "c"…), for a moment
	flashSeq int
	cols     int
	rows     int
}

func newModel(left, right string, open func(string) error, quit func()) *model {
	m := &model{selPanel: -1, open: open, quit: quit, trash: moveToTrash}
	for i, path := range []string{left, right} {
		p, err := readPanel(path, false)
		if err != nil {
			p = &panel{path: path}
			m.status = gui.Line{gui.Colored("✘ "+err.Error(), gui.Red)}
		}
		m.panels[i] = p
	}
	return m
}

// Paste works only while a name is being written.
func (m *model) Paste(text string) {
	if m.edit != nil && m.confirm == nil {
		m.insert(text)
	}
}
func (m *model) CanPaste() bool { return m.edit != nil && m.confirm == nil }

func (m *model) Key(k string) {
	m.lightUp(keyNames[k])
	m.status = nil
	if m.confirm != nil {
		// The answer may open another window: close this one before giving it.
		c := m.confirm
		m.confirm = nil
		if !c.Key(k) {
			m.confirm = c
		}
		return
	}
	if m.edit != nil {
		m.editKey(k)
		return
	}
	p := m.panels[m.active]
	switch k {
	case gui.KeyUp:
		p.move(-1)
	case gui.KeyDown:
		p.move(1)
	case gui.KeyTab:
		m.active = 1 - m.active
	case gui.KeyShiftTab:
		m.sameFolder()
	case gui.KeyEnter:
		m.enter()
	case gui.KeyBackspace:
		m.goUp(m.active)
	case gui.KeyEsc:
		m.esc()
	}
}

// sameFolder is Shift + Tab: the other panel opens the folder of this one, with the cursor on the
// same row, and becomes the active panel.
func (m *model) sameFolder() {
	p := m.panels[m.active]
	focus := ""
	if e, ok := p.current(); ok && !e.up {
		focus = e.name
	}
	other := 1 - m.active
	m.changeDir(other, p.path, focus)
	if m.panels[other].path == p.path {
		m.active = other
	}
}

// esc removes the selection; without a selection it asks before closing the app.
func (m *model) esc() {
	if p := m.panels[m.active]; p.filter != 0 {
		name := ""
		if e, ok := p.current(); ok && !e.up {
			name = e.name
		}
		p.applyFilter(0)
		p.find(name)
		return
	}
	if m.selPanel != -1 {
		m.clearSelection()
		return
	}
	m.confirm = gui.QuitDialog("Trimlet Files", m.quit)
}

func (m *model) Rune(r rune) {
	if r == ' ' {
		m.lightUp("space")
	} else {
		m.lightUp(strings.ToLower(string(r)))
	}
	m.status = nil
	if m.confirm != nil {
		c := m.confirm
		m.confirm = nil
		if !c.Rune(r) {
			m.confirm = c
		}
		return
	}
	if m.edit != nil {
		m.insert(string(r))
		return
	}
	// Shift + letter filters; the commands are the lowercase letters.
	if unicode.IsUpper(r) {
		m.setFilter(unicode.ToLower(r))
		return
	}
	switch r {
	case ' ':
		m.toggle()
	case 'h':
		m.goHome()
	case '.':
		m.toggleHidden()
	case 'c':
		m.askTransfer(false)
	case 'm':
		m.askTransfer(true)
	case 'd':
		m.askDelete()
	case 'r':
		m.askRename()
	case 'n':
		m.askNewFolder()
	case 'u':
		m.askUndo()
	}
}

// setFilter shows in the active panel only the rows starting with r (one letter: a new one replaces
// the old). With a selection it does nothing, so a filter never hides selected files.
func (m *model) setFilter(r rune) {
	if m.selPanel != -1 {
		m.status = gui.Line{gui.Colored("Press esc to clear the selection first", gui.Amber)}
		return
	}
	p := m.panels[m.active]
	p.applyFilter(r)
	p.top = 0
	p.cursor = 0
	if len(p.entries) > 0 && p.entries[0].up {
		p.cursor = min(1, len(p.entries)-1)
	}
	if n := len(p.entries); n == 0 || (n == 1 && p.entries[0].up) {
		m.status = gui.Line{gui.Colored(fmt.Sprintf("No files starting with %q", string(r)), gui.Amber)}
	}
}

// goHome shows the home folder in the active panel.
func (m *model) goHome() {
	if home != "" && m.panels[m.active].path != home {
		m.changeDir(m.active, home, "")
	}
}

// toggleHidden shows or hides the hidden files and folders in both panels. With a selection it does
// nothing, so that the selection never changes by itself (a selected hidden file would disappear).
func (m *model) toggleHidden() {
	if m.selPanel != -1 {
		m.status = gui.Line{gui.Colored("Press esc to clear the selection first", gui.Amber)}
		return
	}
	m.hidden = !m.hidden
	m.reload()
}

// reload reads both panels again, after a change on disk or to the hidden files.
// The cursor stays on the same row, or near where it was if that row is gone.
func (m *model) reload() {
	for i, p := range m.panels {
		focus := ""
		if e, ok := p.current(); ok && !e.up {
			focus = e.name
		}
		n, err := readPanel(p.path, m.hidden)
		if errors.Is(err, fs.ErrNotExist) {
			// The folder is gone (deleted or moved from the other panel): show the nearest folder
			// above it that still exists, with the cursor where it was.
			if m.goToExisting(i) {
				continue
			}
		}
		if err != nil {
			m.status = gui.Line{gui.Colored("✘ "+err.Error(), gui.Red)}
			continue
		}
		n.applyFilter(p.filter) // same folder: the filter stays
		n.cursor = min(p.cursor, max(len(n.entries)-1, 0))
		n.top = p.top
		n.find(focus)
		m.panels[i] = n
	}
}

// enter opens the folder under the cursor, goes up on "..", and gives a file to the system.
func (m *model) enter() {
	p := m.panels[m.active]
	e, ok := p.current()
	switch {
	case !ok:
	case e.up:
		m.goUp(m.active)
	case e.dir:
		m.changeDir(m.active, filepath.Join(p.path, e.name), "")
	default:
		if err := m.open(filepath.Join(p.path, e.name)); err != nil {
			m.status = gui.Line{gui.Colored("✘ "+err.Error(), gui.Red)}
		}
	}
}

// goUp shows the parent folder, with the cursor on the folder we came from.
func (m *model) goUp(i int) {
	p := m.panels[i]
	if isRoot(p.path) {
		return
	}
	m.changeDir(i, filepath.Dir(p.path), filepath.Base(p.path))
}

// changeDir shows path in panel i, with the cursor on the row called focus (if any).
// If the folder cannot be read, the panel stays where it was. If panel i holds the
// selection, the selection disappears.
func (m *model) changeDir(i int, path, focus string) {
	p, err := readPanel(path, m.hidden)
	if err != nil {
		m.status = gui.Line{gui.Colored("✘ "+err.Error(), gui.Red)}
		return
	}
	p.find(focus)
	m.panels[i] = p
	if m.selPanel == i {
		m.clearSelection()
	}
	m.saveDirs()
}

// saveDirs keeps the folders of the two panels, so the app reopens where it was left.
func (m *model) saveDirs() {
	if m.onDirs != nil {
		m.onDirs(m.panels[0].path, m.panels[1].path)
	}
}

// toggle selects the row under the cursor (or removes it from the selection) and moves down.
// Selecting in one panel removes the selection in the other one. ".." cannot be selected.
func (m *model) toggle() {
	p := m.panels[m.active]
	e, ok := p.current()
	if ok && !e.up {
		if m.selPanel != m.active {
			m.sel = map[string]bool{}
			m.selPanel = m.active
		}
		if m.sel[e.name] {
			delete(m.sel, e.name)
		} else {
			m.sel[e.name] = true
		}
		if len(m.sel) == 0 {
			m.clearSelection()
		}
	}
	p.move(1)
}

func (m *model) clearSelection() {
	m.selPanel, m.sel = -1, nil
}

func (m *model) selected(i int, name string) bool {
	return m.selPanel == i && m.sel[name]
}

// keyNames are the keys as the hint band writes them. ↑ ↓ do not light up: the cursor moving is
// already seen (author, 2026-11-19).
var keyNames = map[string]string{
	gui.KeyLeft: "←→", gui.KeyRight: "←→",
	gui.KeyTab: "tab", gui.KeyShiftTab: "shift+tab", gui.KeyEnter: "enter", gui.KeyBackspace: "backspace", gui.KeyEsc: "esc",
}

// flashTime is how long a pressed key stays lit in the hint band.
const flashTime = 300 * time.Millisecond

// lightUp lights the key in the hint band for a moment, so you see what you pressed.
func (m *model) lightUp(key string) {
	m.flash = key
	m.flashSeq++
	if m.ui == nil || key == "" {
		return
	}
	seq := m.flashSeq
	time.AfterFunc(flashTime, func() {
		m.ui(func() {
			if m.flashSeq == seq {
				m.flash = ""
			}
		})
	})
}

// goToExisting moves panel i to the nearest folder above its own that can be read, with the cursor
// on the way back down. It returns false if none can (it keeps the panel as it is).
func (m *model) goToExisting(i int) bool {
	path := m.panels[i].path
	for !isRoot(path) {
		child := filepath.Base(path)
		path = filepath.Dir(path)
		if p, err := readPanel(path, m.hidden); err == nil {
			p.find(child)
			m.panels[i] = p
			if m.selPanel == i {
				m.clearSelection()
			}
			m.saveDirs()
			return true
		}
	}
	return false
}
