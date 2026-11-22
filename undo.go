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

// u undoes the last operation only (not a list), after a confirmation. (Author, 2026-11-19.)

type undoKind int

const (
	undoRename undoKind = iota + 1
	undoNewDir
	undoTrash
	undoMove
	undoCopy
)

// undoOp is what is needed to undo the last operation.
type undoOp struct {
	kind  undoKind
	dir   string      // rename / new folder: the folder; copy / move: where the files went
	back  string      // rename: the old name; copy / move: where the files came from
	what  string      // rename / new folder: the (new) name; otherwise "3 files"…
	pairs [][2]string // trash: original → in the Trash; move: from → to; copy: what was created
	lost  int         // files replaced with Overwrite all: their old content cannot come back
}

// remember keeps op as the operation u undoes, if there is anything to undo.
func (m *model) remember(op *undoOp) {
	if (op.kind == undoTrash || op.kind == undoMove || op.kind == undoCopy) && len(op.pairs) == 0 {
		m.last = nil
		return
	}
	m.last = op
}

// undoLabel is the word after "undo" in the hint line.
func (op *undoOp) label() string {
	return map[undoKind]string{undoRename: "rename", undoNewDir: "new folder", undoTrash: "trash", undoMove: "move", undoCopy: "copy"}[op.kind]
}

// askUndo asks before undoing the last operation.
func (m *model) askUndo() {
	op := m.last
	if op == nil {
		return
	}
	var q string
	switch op.kind {
	case undoRename:
		q = fmt.Sprintf("Undo: rename %q back to %q?", op.what, op.back)
	case undoNewDir:
		q = fmt.Sprintf("Undo: remove the new folder %q?", op.what)
	case undoTrash:
		for _, p := range op.pairs {
			if p[1] == "" {
				m.status = gui.Line{gui.Colored("Cannot undo: where the files went in the Trash is not known on this system", gui.Amber)}
				return
			}
		}
		q = fmt.Sprintf("Undo: put back %s from the Trash?", op.what)
	case undoMove:
		q = fmt.Sprintf("Undo: move %s back to %s?", op.what, cutPath(shortPath(op.back), max(m.cols-50, 20)))
	case undoCopy:
		q = fmt.Sprintf("Undo: move the copies of %s in %s to the Trash?", op.what, cutPath(shortPath(op.dir), max(m.cols-60, 20)))
	}
	if op.lost > 0 {
		q += fmt.Sprintf(" (%d replaced %s cannot come back)", op.lost, plural(op.lost, "file", "files"))
	}
	m.confirm = gui.YesNo(q, m.undo, true)
}

// undo does it. What can no longer be undone (a file put back in the way, a folder no longer empty)
// stops it with a message.
func (m *model) undo() {
	op := m.last
	m.last = nil
	err := op.run(m.trash)
	m.reload()
	if err != nil {
		m.status = gui.Line{gui.Colored("✘ Undo stopped: "+err.Error(), gui.Red)}
		return
	}
	if op.kind == undoRename {
		for i := range m.panels {
			if m.panels[i].path == op.dir {
				m.panels[i].find(op.back)
			}
		}
	}
	m.status = gui.Line{gui.Colored("✔ Undone", gui.Amber)}
}

func (op *undoOp) run(trash func(string) (string, error)) error {
	free := func(path string) error {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("%s already exists", filepath.Base(path))
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	switch op.kind {
	case undoRename:
		from, to := filepath.Join(op.dir, op.what), filepath.Join(op.dir, op.back)
		if !equalFold(op.what, op.back) {
			if err := free(to); err != nil {
				return err
			}
		}
		return os.Rename(from, to)
	case undoNewDir:
		if err := os.Remove(filepath.Join(op.dir, op.what)); err != nil {
			return fmt.Errorf("%s is not empty any more", op.what)
		}
	case undoTrash:
		for _, p := range op.pairs {
			if err := free(p[0]); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(p[0]), 0o755); err != nil {
				return err
			}
			if err := os.Rename(p[1], p[0]); err != nil {
				return err
			}
		}
	case undoMove:
		for i := len(op.pairs) - 1; i >= 0; i-- {
			from, to := op.pairs[i][0], op.pairs[i][1]
			if err := free(from); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(from), 0o755); err != nil {
				return err
			}
			t := &transferer{}
			if err := t.move(to, from); err != nil {
				return err
			}
		}
	case undoCopy:
		for _, p := range op.pairs {
			if _, err := trash(p[0]); err != nil {
				return fmt.Errorf("%s: %w", filepath.Base(p[0]), err)
			}
		}
	}
	return nil
}

func equalFold(a, b string) bool { return strings.EqualFold(a, b) }
