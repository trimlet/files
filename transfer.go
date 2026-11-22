package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/trimlet/gui"
)

// C and M: copy or move the selection into the folder of the other panel (the panel without the
// selection). Folders with the same name always merge; files with the same name, also inside folders,
// get one question for the whole operation, before anything is done.

// askTransfer starts C (move false) or M (move true).
func (m *model) askTransfer(move bool) {
	items, src := m.selection()
	if len(items) == 0 {
		m.status = noSelection
		return
	}
	dst := m.panels[1-m.selPanel].path
	verb := "Copy"
	if move {
		verb = "Move"
	}
	if src == dst {
		m.status = gui.Line{gui.Colored("Both panels show the same folder: open another one in the other panel", gui.Amber)}
		return
	}
	for _, e := range items {
		if e.dir && inside(dst, filepath.Join(src, e.name)) {
			m.status = gui.Line{gui.Colored(fmt.Sprintf("%s: cannot %s a folder into itself", e.name, strings.ToLower(verb)), gui.Amber)}
			return
		}
	}

	files, clashes, err := countClashes(src, dst, items)
	if err != nil {
		m.status = gui.Line{gui.Colored("✘ "+err.Error(), gui.Red)}
		return
	}
	run := func(overwrite bool) func() {
		return func() { m.transfer(items, src, dst, move, overwrite) }
	}
	body := m.transferBody(items)
	if clashes == 0 {
		m.confirm = gui.YesNo(fmt.Sprintf("%s %s?", verb, describe(items)), run(false), true)
		m.confirm.Body = body
		return
	}
	m.confirm = &gui.Dialog{
		Question: fmt.Sprintf("%d of %d %s already exist where they go", clashes, files, plural(files, "item", "items")),
		Body:     body,
		Answers: []gui.Answer{
			{Key: 'o', Label: "Overwrite all", Do: run(true)},
			{Key: 'k', Label: "Keep old files", Do: run(false)},
			{Key: 'b', Label: "Back"},
		},
		Chosen: 2,
		Esc:    2,
	}
}

// inside says whether path is dir itself or a folder inside it.
func inside(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// countClashes counts the files that would be copied (also inside the folders) and how many of them
// already exist at the destination. Folders are not counted: they merge.
func countClashes(src, dst string, items []entry) (files, clashes int, err error) {
	var walk func(from, to string) error
	walk = func(from, to string) error {
		fi, err := os.Lstat(from)
		if err != nil {
			return err
		}
		ti, terr := os.Lstat(to)
		if fi.IsDir() {
			if terr == nil && !ti.IsDir() {
				clashes++ // a file where the folder would go: counted as one clash
				files++
				return nil
			}
			list, err := os.ReadDir(from)
			if err != nil {
				return err
			}
			for _, d := range list {
				if err := walk(filepath.Join(from, d.Name()), filepath.Join(to, d.Name())); err != nil {
					return err
				}
			}
			return nil
		}
		files++
		if terr == nil {
			clashes++
		}
		return nil
	}
	for _, e := range items {
		if err := walk(filepath.Join(src, e.name), filepath.Join(dst, e.name)); err != nil {
			return 0, 0, err
		}
	}
	return files, clashes, nil
}

// transfer copies or moves the items, stopping at the first error. With overwrite false the files
// that already exist are left as they are. Then the selection disappears and the panels are read again.
func (m *model) transfer(items []entry, src, dst string, move, overwrite bool) {
	t := &transferer{overwrite: overwrite}
	var err error
	for _, e := range items {
		from, to := filepath.Join(src, e.name), filepath.Join(dst, e.name)
		if move {
			err = t.move(from, to)
		} else {
			err = t.copy(from, to)
		}
		if err != nil {
			err = fmt.Errorf("%s: %w", e.name, err)
			break
		}
	}
	op := &undoOp{kind: undoCopy, what: describe(items), dir: dst, back: src, lost: t.replaced}
	if move {
		op.kind = undoMove
		op.pairs = t.moved
	} else {
		for _, p := range t.created {
			op.pairs = append(op.pairs, [2]string{p, ""})
		}
	}
	m.remember(op)
	m.clearSelection()
	m.reload()

	verb := "copied"
	if move {
		verb = "moved"
	}
	where := truncLeft(dst, max(m.cols-60, 20))
	if err != nil {
		m.status = gui.Line{gui.Colored("✘ Stopped: "+err.Error(), gui.Red)}
		return
	}
	msg := fmt.Sprintf("✔ %s %s to %s", describe(items), verb, where)
	if t.kept > 0 {
		msg += fmt.Sprintf(" (%d old %s kept)", t.kept, plural(t.kept, "file", "files"))
	}
	m.status = gui.Line{gui.Colored(msg, gui.Amber)}
}

type transferer struct {
	overwrite bool
	kept      int         // files left as they were because they already existed
	replaced  int         // files that replaced old ones (Overwrite all): those old ones are lost
	created   []string    // C: what was created at the top (a new folder counts once), for undo
	moved     [][2]string // M: each rename done, from → to, for undo
	inside    bool        // copying inside a folder just created: its content is not listed again
}

// copy copies a file, a link or a whole folder. A folder merges with one of the same name.
func (t *transferer) copy(from, to string) error {
	fi, err := os.Lstat(from)
	if err != nil {
		return err
	}
	ti, terr := os.Lstat(to)
	exists := terr == nil
	if terr != nil && !errors.Is(terr, fs.ErrNotExist) {
		return terr
	}

	if fi.IsDir() {
		if exists && !ti.IsDir() {
			if !t.overwrite {
				t.kept++
				return nil
			}
			if err := os.Remove(to); err != nil {
				return err
			}
			exists = false
		}
		inside := t.inside
		if !exists {
			if err := os.Mkdir(to, fi.Mode().Perm()); err != nil {
				return err
			}
			if !t.inside {
				t.created = append(t.created, to)
			}
			t.inside = true
		}
		defer func() { t.inside = inside }()
		list, err := os.ReadDir(from)
		if err != nil {
			return err
		}
		for _, d := range list {
			if err := t.copy(filepath.Join(from, d.Name()), filepath.Join(to, d.Name())); err != nil {
				return err
			}
		}
		return nil
	}

	if exists {
		if !t.overwrite {
			t.kept++
			return nil
		}
		if err := os.RemoveAll(to); err != nil { // also a folder where the file goes
			return err
		}
		t.replaced++
	} else if !t.inside {
		t.created = append(t.created, to)
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Readlink(from)
		if err != nil {
			return err
		}
		return os.Symlink(target, to)
	}
	return copyFile(from, to, fi)
}

// copyFile copies the content, the permissions and the modification time.
func copyFile(from, to string, fi fs.FileInfo) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fi.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(to)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chtimes(to, fi.ModTime(), fi.ModTime())
}

// move moves a file or folder. When nothing is in the way it is a rename (instant on the same disk;
// on another disk it is copied, then removed). A folder with the same name merges: what is inside is
// moved one by one, and the old folder is removed if nothing was left in it.
func (t *transferer) move(from, to string) error {
	fi, err := os.Lstat(from)
	if err != nil {
		return err
	}
	ti, terr := os.Lstat(to)
	if terr != nil && !errors.Is(terr, fs.ErrNotExist) {
		return terr
	}
	exists := terr == nil

	if exists && fi.IsDir() && ti.IsDir() {
		list, err := os.ReadDir(from)
		if err != nil {
			return err
		}
		for _, d := range list {
			if err := t.move(filepath.Join(from, d.Name()), filepath.Join(to, d.Name())); err != nil {
				return err
			}
		}
		os.Remove(from) // fails, and stays, if old files were kept inside
		return nil
	}
	if exists {
		if !t.overwrite {
			t.kept++
			return nil
		}
		if err := os.RemoveAll(to); err != nil {
			return err
		}
		t.replaced++
	}
	err = os.Rename(from, to)
	if errors.Is(err, syscall.EXDEV) { // another disk
		c := &transferer{overwrite: t.overwrite}
		if err := c.copy(from, to); err != nil {
			return err
		}
		err = os.RemoveAll(from)
	}
	if err == nil {
		t.moved = append(t.moved, [2]string{from, to})
	}
	return err
}

// transferBody is what the C / M window shows under its question: the two folders placed like the
// panels, with an arrow from where the files are to where they go, then the names (up to 3).
// (Author, 2026-11-19.)
func (m *model) transferBody(items []entry) []line {
	half := max((m.cols-20)/2, 12)
	left, right := cutPath(shortPath(m.panels[0].path), half), cutPath(shortPath(m.panels[1].path), half)
	arrow := "   ->   "
	if m.selPanel == 1 {
		arrow = "   <-   "
	}
	body := []line{
		{gui.Colored(left, gui.White), gui.Strong(arrow, gui.Accent), gui.Colored(right, gui.White)},
		nil,
	}
	const shown = 3
	for i, e := range items {
		if i == shown {
			body = append(body, line{gui.Colored(fmt.Sprintf("  and %d more", len(items)-shown), gui.Muted)})
			break
		}
		name := e.name
		if e.dir {
			name += "/"
		}
		body = append(body, line{gui.Plain("  " + gui.Truncate(name, half*2))})
	}
	return body
}

// cutPath shortens a path to n characters from the left, keeping whole folders at the end:
// "…/projects/trimlet". The last folder is cut only if it alone does not fit.
func cutPath(path string, n int) string {
	if len([]rune(path)) <= n {
		return path
	}
	parts := strings.Split(path, string(filepath.Separator))
	out := ""
	for i := len(parts) - 1; i > 0; i-- {
		try := parts[i]
		if out != "" {
			try += string(filepath.Separator) + out
		}
		if len([]rune("…"+string(filepath.Separator)+try)) > n {
			break
		}
		out = try
	}
	if out == "" {
		return truncLeft(path, n)
	}
	return "…" + string(filepath.Separator) + out
}
