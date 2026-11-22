package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/trimlet/gui"
)

// editField is a name being written inside its own row: R (rename) or N (new folder).
// Enter does it, Esc goes back without changing anything.
type editField struct {
	panel   int
	newDir  bool   // N; otherwise R
	old     string // R: the name being renamed
	text    []rune
	pos     int  // the text cursor
	all     bool // the whole text is selected: the first character typed replaces it (N)
	restore int  // N: the panel's cursor before the new row was added
}

const editHint = "type the name · ←→ move · enter %s · esc back"

// askRename starts R on the one selected file or folder: its panel becomes active, the cursor goes
// on it and its name becomes editable in its row, so it is always clear what is renamed.
func (m *model) askRename() {
	items, _ := m.selection()
	switch {
	case len(items) == 0:
		m.status = noSelection
		return
	case len(items) > 1:
		m.status = gui.Line{gui.Colored("Select only one file to rename", gui.Amber)}
		return
	}
	m.active = m.selPanel
	p := m.panels[m.active]
	p.find(items[0].name)
	name := []rune(items[0].name)
	m.edit = &editField{panel: m.active, old: items[0].name, text: name, pos: len(name)}
}

// askNewFolder adds a row right after ".." in the active panel, with the name editable.
func (m *model) askNewFolder() {
	p := m.panels[m.active]
	at := 0
	if len(p.entries) > 0 && p.entries[0].up {
		at = 1
	}
	f := &editField{panel: m.active, newDir: true, text: []rune("New folder"), restore: p.cursor, all: true}
	f.pos = len(f.text)
	p.entries = append(p.entries[:at], append([]entry{{dir: true}}, p.entries[at:]...)...)
	p.cursor = at
	m.edit = f
}

func (m *model) editKey(k string) {
	f := m.edit
	switch k {
	case gui.KeyLeft:
		f.all = false
		f.pos = max(f.pos-1, 0)
	case gui.KeyRight:
		f.all = false
		f.pos = min(f.pos+1, len(f.text))
	case gui.KeyHome:
		f.all, f.pos = false, 0
	case gui.KeyEnd:
		f.all, f.pos = false, len(f.text)
	case gui.KeyBackspace:
		if f.all {
			f.text, f.pos, f.all = nil, 0, false
		} else if f.pos > 0 {
			f.text = append(f.text[:f.pos-1], f.text[f.pos:]...)
			f.pos--
		}
	case gui.KeyEnter:
		m.commitEdit()
	case gui.KeyEsc:
		m.closeEdit()
	}
}

// insert writes text at the cursor (typed characters or a paste, on one line).
func (m *model) insert(text string) {
	f := m.edit
	text = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ").Replace(text)
	if f.all {
		f.text, f.pos, f.all = nil, 0, false
	}
	r := []rune(text)
	f.text = append(f.text[:f.pos], append(r, f.text[f.pos:]...)...)
	f.pos += len(r)
}

// closeEdit ends the editing without changes: N takes its row away, R keeps the selection.
func (m *model) closeEdit() {
	f := m.edit
	m.edit = nil
	if f.newDir {
		p := m.panels[f.panel]
		for i, e := range p.entries {
			if e.dir && e.name == "" && !e.up {
				p.entries = append(p.entries[:i], p.entries[i+1:]...)
				break
			}
		}
		p.cursor = min(f.restore, max(len(p.entries)-1, 0))
	}
}

// commitEdit renames or creates the folder. A name already used, an empty name or a name with a
// path separator does nothing and says why (the field closes, as decided).
func (m *model) commitEdit() {
	f := m.edit
	name := strings.TrimSpace(string(f.text))
	dir := m.panels[f.panel].path
	m.closeEdit()

	if !f.newDir && name == f.old {
		return // nothing changed
	}
	if msg := badName(name); msg != "" {
		m.status = gui.Line{gui.Colored(msg, gui.Amber)}
		return
	}
	target := filepath.Join(dir, name)
	// On Mac and Windows "Report.txt" and "report.txt" are the same name: changing only the case
	// of the file being renamed is allowed.
	sameFile := !f.newDir && strings.EqualFold(name, f.old)
	if _, err := os.Lstat(target); err == nil && !sameFile {
		m.status = gui.Line{gui.Colored(fmt.Sprintf("%q already exists", name), gui.Amber)}
		return
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		m.status = gui.Line{gui.Colored("✘ "+err.Error(), gui.Red)}
		return
	}

	var err error
	if f.newDir {
		err = os.Mkdir(target, 0o755)
	} else {
		err = os.Rename(filepath.Join(dir, f.old), target)
	}
	if err != nil {
		m.status = gui.Line{gui.Colored("✘ "+err.Error(), gui.Red)}
		return
	}
	if f.newDir {
		m.remember(&undoOp{kind: undoNewDir, dir: dir, what: name})
	} else {
		m.remember(&undoOp{kind: undoRename, dir: dir, what: name, back: f.old})
		m.clearSelection()
	}
	m.reload()
	m.panels[f.panel].find(name)
	if f.newDir {
		m.status = gui.Line{gui.Colored(fmt.Sprintf("✔ Folder %q created", name), gui.Amber)}
	} else {
		m.status = gui.Line{gui.Colored(fmt.Sprintf("✔ Renamed to %q", name), gui.Amber)}
	}
}

// badName says what is wrong with a name, or "" if it can be used.
func badName(name string) string {
	switch {
	case name == "":
		return "The name is empty"
	case name == "." || name == "..":
		return fmt.Sprintf("%q cannot be used as a name", name)
	case strings.ContainsAny(name, `/\`):
		return `A name cannot contain / or \`
	}
	return ""
}

// renderField draws the name being written, width characters wide, with a block on the text
// cursor; the whole text is marked when the first key will replace it.
func (f *editField) render(width int) line {
	text := f.text
	start := max(f.pos-(width-1), 0) // keep the cursor in view
	visible := text[start:min(len(text), start+width)]
	pos := f.pos - start

	marked := gui.Palettes["purple"][8]
	var l line
	if f.all {
		l = append(l, gui.Span{Text: string(visible), Color: gui.White, Bg: marked, Bold: true})
	} else {
		l = append(l, gui.Span{Text: string(visible[:pos]), Color: gui.White, Bold: true})
	}
	under := " "
	if pos < len(visible) && !f.all {
		under = string(visible[pos])
	}
	if !f.all || pos >= len(visible) {
		l = append(l, gui.Span{Text: under, Color: gui.Background, Bg: gui.Accent, Bold: true})
	}
	if !f.all && pos+1 < len(visible) {
		l = append(l, gui.Span{Text: string(visible[pos+1:]), Color: gui.White, Bold: true})
	}
	if n := gui.LineLen(l); n < width {
		l = append(l, gui.Plain(strings.Repeat(" ", width-n)))
	}
	return l
}
