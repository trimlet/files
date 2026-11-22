package main

import (
	"fmt"
	"path/filepath"

	"github.com/trimlet/gui"
)

// noSelection is the message of C, M, D and R when nothing is selected: they act only on the selection.
var noSelection = gui.Line{gui.Colored("No files selected", gui.Amber)}

// selection gives the selected rows, in the order of their panel, and the panel's folder.
func (m *model) selection() ([]entry, string) {
	if m.selPanel == -1 {
		return nil, ""
	}
	p := m.panels[m.selPanel]
	var out []entry
	for _, e := range p.entries {
		if m.sel[e.name] {
			out = append(out, e)
		}
	}
	return out, p.path
}

// describe counts the items: "1 file", "2 folders", "1 folder and 2 files".
func describe(items []entry) string {
	dirs, files := 0, 0
	for _, e := range items {
		if e.dir {
			dirs++
		} else {
			files++
		}
	}
	count := func(n int, one, many string) string {
		if n == 1 {
			return "1 " + one
		}
		return fmt.Sprintf("%d %s", n, many)
	}
	switch {
	case dirs == 0:
		return count(files, "file", "files")
	case files == 0:
		return count(dirs, "folder", "folders")
	default:
		return count(dirs, "folder", "folders") + " and " + count(files, "file", "files")
	}
}

// askDelete asks before moving the selection to the Trash. The question names the folder,
// because the selection may be in the other panel.
func (m *model) askDelete() {
	items, dir := m.selection()
	if len(items) == 0 {
		m.status = noSelection
		return
	}
	where := truncLeft(dir, max(m.cols-50, 20))
	m.confirm = gui.YesNo(fmt.Sprintf("Move %s in %s to Trash?", describe(items), where), func() { m.delete(items, dir) }, false)
}

// delete moves the items to the Trash, stopping at the first error, then reads the panels again.
// The selection disappears, since the operation was done (at least in part).
func (m *model) delete(items []entry, dir string) {
	done := 0
	var failed error
	op := &undoOp{kind: undoTrash, what: describe(items)}
	for _, e := range items {
		where, err := m.trash(filepath.Join(dir, e.name))
		if err != nil {
			failed = fmt.Errorf("%s: %w", e.name, err)
			break
		}
		op.pairs = append(op.pairs, [2]string{filepath.Join(dir, e.name), where})
		done++
	}
	m.remember(op)
	m.clearSelection()
	m.reload()
	if failed != nil {
		m.status = gui.Line{gui.Colored(fmt.Sprintf("✘ %d of %d moved to Trash. %v", done, len(items), failed), gui.Red)}
		return
	}
	m.status = gui.Line{gui.Colored("✔ "+describe(items)+" moved to Trash", gui.Amber)}
}
