package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trimlet/gui"
)

// testDirs makes  root/{a/, b/, B.txt, c.txt, .hidden}  and returns root.
func testDirs(t *testing.T) string {
	root := t.TempDir()
	for _, d := range []string{"b", "a"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"c.txt", "B.txt", ".hidden"} {
		if err := os.WriteFile(filepath.Join(root, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func names(p *panel) []string {
	var out []string
	for _, e := range p.entries {
		out = append(out, e.name)
	}
	return out
}

func TestOrder(t *testing.T) {
	root := testDirs(t)
	for _, c := range []struct {
		hidden bool
		want   []string
	}{
		{true, []string{"..", "a", "b", ".hidden", "B.txt", "c.txt"}},
		{false, []string{"..", "a", "b", "B.txt", "c.txt"}},
	} {
		p, err := readPanel(root, c.hidden)
		if err != nil {
			t.Fatal(err)
		}
		if got := names(p); strings.Join(got, " ") != strings.Join(c.want, " ") {
			t.Fatalf("hidden %v: got %v, want %v", c.hidden, got, c.want)
		}
	}
}

func TestEnterAndUp(t *testing.T) {
	root := testDirs(t)
	m := newModel(root, root, func(string) error { return nil }, func() {})
	m.Key(gui.KeyDown) // a
	m.Key(gui.KeyEnter)
	if m.panels[0].path != filepath.Join(root, "a") {
		t.Fatalf("enter: path %s", m.panels[0].path)
	}
	m.Key(gui.KeyBackspace)
	if m.panels[0].path != filepath.Clean(root) {
		t.Fatalf("backspace: path %s", m.panels[0].path)
	}
	if e, _ := m.panels[0].current(); e.name != "a" {
		t.Fatalf("cursor after going up on %q, want a", e.name)
	}
	m.Key(gui.KeyUp) // ..
	m.Key(gui.KeyEnter)
	if m.panels[0].path != filepath.Dir(filepath.Clean(root)) {
		t.Fatalf("enter on ..: path %s", m.panels[0].path)
	}
}

func TestEnterOnFileOpensIt(t *testing.T) {
	root := testDirs(t)
	var opened string
	m := newModel(root, root, func(p string) error { opened = p; return nil }, func() {})
	m.Rune('.')   // show .hidden, as the steps below count it
	for range 5 { // .. a b .hidden B.txt c.txt
		m.Key(gui.KeyDown)
	}
	m.Key(gui.KeyEnter)
	if opened != filepath.Join(root, "c.txt") {
		t.Fatalf("opened %q", opened)
	}
}

func TestSelection(t *testing.T) {
	root := testDirs(t)
	m := newModel(root, root, func(string) error { return nil }, func() {})
	m.Rune('.') // show .hidden, as the steps below count it

	m.Rune(' ') // on "..": not selected, moves down
	if m.selPanel != -1 {
		t.Fatal(".. was selected")
	}
	m.Rune(' ') // a
	m.Rune(' ') // b
	if !m.selected(0, "a") || !m.selected(0, "b") {
		t.Fatal("a and b should be selected")
	}
	if e, _ := m.panels[0].current(); e.name != ".hidden" {
		t.Fatalf("cursor on %q after two selections", e.name)
	}

	// Tab and the arrows keep it.
	m.Key(gui.KeyTab)
	m.Key(gui.KeyDown)
	if !m.selected(0, "a") {
		t.Fatal("Tab or arrows removed the selection")
	}
	// The other panel changing folder keeps it.
	m.Key(gui.KeyEnter) // right panel: a
	if !m.selected(0, "a") {
		t.Fatal("the other panel's change of folder removed the selection")
	}
	// Selecting in the other panel removes it.
	m.Key(gui.KeyBackspace) // right: back on a
	m.Key(gui.KeyDown)      // right: b
	m.Rune(' ')
	if m.selected(0, "a") || !m.selected(1, "b") {
		t.Fatal("one selection at a time")
	}
	// Space again removes it from the selection.
	m.Key(gui.KeyUp)
	m.Rune(' ')
	if m.selPanel != -1 {
		t.Fatal("selection should be empty")
	}
	// Its own panel changing folder removes it.
	m.Rune(' ') // .hidden
	m.Key(gui.KeyBackspace)
	if m.selPanel != -1 {
		t.Fatal("changing folder should remove the selection")
	}
}

func TestRenderFits(t *testing.T) {
	root := testDirs(t)
	m := newModel(root, root, func(string) error { return nil }, func() {})
	for _, cols := range []int{60, 150} {
		lines := m.Render(cols, 20)
		if len(lines) != 19 {
			t.Fatalf("cols %d: %d lines, want 19 (rows less the top margin)", cols, len(lines))
		}
		for i, l := range lines[2 : len(lines)-3] {
			n := 0
			for _, s := range l {
				n += len([]rune(s.Text))
			}
			if n > cols {
				t.Fatalf("cols %d: line %d is %d wide", cols, i, n)
			}
		}
	}
}

func TestEsc(t *testing.T) {
	root := testDirs(t)
	quits := 0
	m := newModel(root, root, func(string) error { return nil }, func() { quits++ })

	// With a selection, Esc removes it and does not ask anything.
	m.Key(gui.KeyDown)
	m.Rune(' ')
	m.Key(gui.KeyEsc)
	if m.selPanel != -1 || m.confirm != nil {
		t.Fatal("Esc should only remove the selection")
	}

	// Without one, it asks with Yes chosen; Esc and N answer No, other keys are ignored.
	m.Key(gui.KeyEsc)
	if m.confirm == nil || m.confirm.ChosenKey() != 'y' {
		t.Fatal("Esc should ask, with Yes chosen")
	}
	m.Rune('q')
	m.Key(gui.KeyDown)
	if m.confirm == nil {
		t.Fatal("other keys should be ignored")
	}
	m.Key(gui.KeyEsc)
	m.Key(gui.KeyEsc)
	m.Rune('n')
	if quits != 0 || m.confirm != nil {
		t.Fatalf("quit %d times, window open %v", quits, m.confirm != nil)
	}

	// Y quits; so does Enter, Yes being chosen.
	m.Key(gui.KeyEsc)
	m.Rune('y')
	m.Key(gui.KeyEsc)
	m.Key(gui.KeyEnter)
	if quits != 2 {
		t.Fatalf("quit %d times, want 2", quits)
	}
}

func TestConfirmBoxFits(t *testing.T) {
	root := testDirs(t)
	m := newModel(root, root, func(string) error { return nil }, func() {})
	m.Key(gui.KeyEsc)
	for _, cols := range []int{60, 150} {
		for i, l := range m.Render(cols, 20) {
			if n := gui.LineLen(l); n > cols {
				t.Fatalf("cols %d: line %d is %d wide", cols, i, n)
			}
		}
	}
}

func TestHidden(t *testing.T) {
	root := testDirs(t)
	m := newModel(root, root, func(string) error { return nil }, func() {})
	if strings.Contains(strings.Join(names(m.panels[1]), " "), ".hidden") {
		t.Fatal("hidden files should not be shown at the start")
	}
	m.Rune('.')
	for i := range m.panels {
		if !strings.Contains(strings.Join(names(m.panels[i]), " "), ".hidden") {
			t.Fatalf("panel %d: H should show the hidden files in both panels", i)
		}
	}
	// With a selection, H does nothing and says why.
	m.Key(gui.KeyDown)
	m.Rune(' ')
	m.Rune('.')
	if !m.hidden || m.status == nil || !m.selected(0, "a") {
		t.Fatal("H with a selection should do nothing but show a message")
	}
	m.Key(gui.KeyEsc)
	m.Rune('.')
	if m.hidden {
		t.Fatal("H should hide them again")
	}
}

func TestDelete(t *testing.T) {
	root := testDirs(t)
	var trashed []string
	m := newModel(root, root, func(string) error { return nil }, func() {})
	m.trash = func(p string) (string, error) { trashed = append(trashed, p); return "", os.RemoveAll(p) }
	m.Render(150, 20)

	// Without a selection D does nothing.
	m.Rune('d')
	if m.confirm != nil || m.status == nil {
		t.Fatal("D without selection should only show a message")
	}

	// a/ and B.txt selected; select from the right panel, then press D from the left one.
	m.Key(gui.KeyTab)
	m.Key(gui.KeyDown) // a
	m.Rune(' ')        // a, cursor on b
	m.Key(gui.KeyDown) // B.txt
	m.Rune(' ')
	m.Key(gui.KeyTab)
	m.Rune('d')
	if m.confirm == nil || m.confirm.ChosenKey() != 'n' {
		t.Fatal("D should ask, with No chosen")
	}
	if want := "Move 1 folder and 1 file in"; !strings.HasPrefix(m.confirm.Question, want) {
		t.Fatalf("question %q", m.confirm.Question)
	}
	// No keeps everything, selection included.
	m.Rune('n')
	if len(trashed) != 0 || !m.selected(1, "a") {
		t.Fatal("No should change nothing")
	}
	m.Rune('d')
	m.Rune('y')
	if strings.Join(trashed, " ") != filepath.Join(root, "a")+" "+filepath.Join(root, "B.txt") {
		t.Fatalf("trashed %v", trashed)
	}
	if m.selPanel != -1 {
		t.Fatal("the selection should disappear after the delete")
	}
	for i := range m.panels {
		if got := strings.Join(names(m.panels[i]), " "); got != ".. b c.txt" {
			t.Fatalf("panel %d after delete: %s", i, got)
		}
	}
}

func TestNewFolder(t *testing.T) {
	root := testDirs(t)
	m := newModel(root, root, func(string) error { return nil }, func() {})
	m.Render(150, 20)

	// The first key replaces "New folder"; Enter creates it, the cursor stays on it.
	m.Rune('n')
	for _, r := range "photos" {
		m.Rune(r)
	}
	m.Key(gui.KeyEnter)
	if fi, err := os.Stat(filepath.Join(root, "photos")); err != nil || !fi.IsDir() {
		t.Fatalf("photos not created: %v", err)
	}
	if e, _ := m.panels[0].current(); e.name != "photos" || m.edit != nil {
		t.Fatalf("cursor on %q", e.name)
	}

	// Esc takes the new row away.
	before := strings.Join(names(m.panels[0]), " ")
	m.Rune('n')
	m.Key(gui.KeyEsc)
	if got := strings.Join(names(m.panels[0]), " "); got != before || m.edit != nil {
		t.Fatalf("after esc: %s", got)
	}

	// A name already used does nothing and says so.
	m.Rune('n')
	m.Rune('a')
	m.Key(gui.KeyEnter)
	if m.status == nil || !strings.Contains(m.status[0].Text, `"a" already exists`) {
		t.Fatalf("status %v", m.status)
	}
}

func TestRename(t *testing.T) {
	root := testDirs(t)
	m := newModel(root, root, func(string) error { return nil }, func() {})
	m.Render(150, 20)

	m.Rune('r')
	if m.edit != nil || m.status == nil {
		t.Fatal("R without selection should only show a message")
	}

	// Two selected: nothing.
	m.Key(gui.KeyDown)
	m.Rune(' ') // a
	m.Rune(' ') // b
	m.Rune('r')
	if m.edit != nil || !strings.Contains(m.status[0].Text, "only one") {
		t.Fatal("R with two selected should only show a message")
	}
	m.Key(gui.KeyEsc)

	// c.txt selected on the left, R pressed from the right: the left panel becomes active.
	m.Key(gui.KeyDown) // B.txt (cursor was on B.txt after selecting a, b)
	m.Key(gui.KeyDown) // c.txt
	m.Rune(' ')
	m.Key(gui.KeyTab)
	m.Rune('r')
	if m.edit == nil || m.active != 0 {
		t.Fatal("R should edit in the panel of the selection")
	}
	if e, _ := m.panels[0].current(); e.name != "c.txt" {
		t.Fatalf("cursor on %q", e.name)
	}
	// Esc keeps the selection.
	m.Key(gui.KeyEsc)
	if !m.selected(0, "c.txt") {
		t.Fatal("esc should keep the selection")
	}
	// A name already used: nothing, selection kept.
	m.Rune('r')
	for range 5 {
		m.Key(gui.KeyBackspace)
	}
	for _, r := range "B.txt" {
		m.Rune(r)
	}
	m.Key(gui.KeyEnter)
	if !m.selected(0, "c.txt") || !strings.Contains(m.status[0].Text, "already exists") {
		t.Fatalf("existing name: status %v", m.status)
	}
	// Rename for real.
	m.Rune('r')
	m.Key(gui.KeyLeft)
	m.Key(gui.KeyLeft)
	m.Key(gui.KeyLeft)
	m.Key(gui.KeyLeft) // before ".txt"
	m.Paste("-new")
	m.Key(gui.KeyEnter)
	if _, err := os.Stat(filepath.Join(root, "c-new.txt")); err != nil {
		t.Fatalf("not renamed: %v", err)
	}
	if m.selPanel != -1 {
		t.Fatal("the selection should disappear after renaming")
	}
	if e, _ := m.panels[0].current(); e.name != "c-new.txt" {
		t.Fatalf("cursor on %q", e.name)
	}
	// Changing only the case is allowed.
	m.Rune(' ')
	m.Rune('r')
	m.Key(gui.KeyHome)
	m.Key(gui.KeyRight)
	m.Key(gui.KeyBackspace)
	m.Rune('C')
	m.Key(gui.KeyEnter)
	if got := strings.Join(names(m.panels[0]), " "); !strings.Contains(got, "C-new.txt") {
		t.Fatalf("case change: %s (status %v)", got, m.status)
	}
}

func TestEditFits(t *testing.T) {
	root := testDirs(t)
	m := newModel(root, root, func(string) error { return nil }, func() {})
	m.Render(60, 20)
	m.Rune('n')
	for range 80 {
		m.Rune('x')
	}
	for _, cols := range []int{60, 150} {
		for i, l := range m.Render(cols, 20)[:17] {
			if n := gui.LineLen(l); n > cols {
				t.Fatalf("cols %d: line %d is %d wide", cols, i, n)
			}
		}
	}
}

// twoDirs makes  root/left/{a.txt, b.txt, photos/{x.jpg, y.jpg}}  and  root/right/{b.txt, photos/{y.jpg, z.jpg}},
// with different contents for the files that are in both, and a model with left and right open.
func twoDirs(t *testing.T) (string, *model) {
	root := t.TempDir()
	write := func(path, text string) {
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "left", "a.txt"), "new a")
	write(filepath.Join(root, "left", "b.txt"), "new b")
	write(filepath.Join(root, "left", "photos", "x.jpg"), "new x")
	write(filepath.Join(root, "left", "photos", "y.jpg"), "new y")
	write(filepath.Join(root, "right", "b.txt"), "old b")
	write(filepath.Join(root, "right", "photos", "y.jpg"), "old y")
	write(filepath.Join(root, "right", "photos", "z.jpg"), "old z")
	m := newModel(filepath.Join(root, "left"), filepath.Join(root, "right"), func(string) error { return nil }, func() {})
	m.Render(150, 20)
	return root, m
}

func read(t *testing.T, path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "(missing)"
	}
	return string(b)
}

// selectAllLeft selects photos/, a.txt and b.txt in the left panel.
func selectAllLeft(m *model) {
	m.Key(gui.KeyDown) // photos
	m.Rune(' ')
	m.Rune(' ') // a.txt
	m.Rune(' ') // b.txt
}

func TestCopyNoClash(t *testing.T) {
	root, m := twoDirs(t)
	m.Key(gui.KeyDown) // photos
	m.Key(gui.KeyDown) // a.txt
	m.Rune(' ')
	m.Rune('c')
	if m.confirm == nil || m.confirm.ChosenKey() != 'y' || m.confirm.Question != "Copy 1 file?" {
		t.Fatalf("C should ask with Yes chosen: %+v", m.confirm)
	}
	m.Key(gui.KeyEnter)
	if read(t, filepath.Join(root, "right", "a.txt")) != "new a" || read(t, filepath.Join(root, "left", "a.txt")) != "new a" {
		t.Fatal("a.txt not copied")
	}
	if m.selPanel != -1 {
		t.Fatal("the selection should disappear")
	}
}

func TestCopyClashes(t *testing.T) {
	for _, c := range []struct {
		key        rune
		b, y, x, z string
	}{
		{'o', "new b", "new y", "new x", "old z"},
		{'k', "old b", "old y", "new x", "old z"},
		{'b', "old b", "old y", "(missing)", "old z"},
	} {
		root, m := twoDirs(t)
		selectAllLeft(m)
		m.Rune('c')
		if m.confirm == nil || !strings.HasPrefix(m.confirm.Question, "2 of 4 items already exist") || m.confirm.ChosenKey() != 'b' {
			t.Fatalf("clash window: %+v", m.confirm)
		}
		m.Rune(c.key)
		r := filepath.Join(root, "right")
		got := []string{read(t, filepath.Join(r, "b.txt")), read(t, filepath.Join(r, "photos", "y.jpg")), read(t, filepath.Join(r, "photos", "x.jpg")), read(t, filepath.Join(r, "photos", "z.jpg"))}
		want := []string{c.b, c.y, c.x, c.z}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%c: got %v, want %v", c.key, got, want)
		}
		if c.key == 'b' && !m.selected(0, "a.txt") {
			t.Fatal("Back should keep the selection")
		}
	}
}

func TestMove(t *testing.T) {
	root, m := twoDirs(t)
	selectAllLeft(m)
	m.Rune('m')
	m.Rune('k') // keep old files
	r, l := filepath.Join(root, "right"), filepath.Join(root, "left")
	if read(t, filepath.Join(r, "a.txt")) != "new a" || read(t, filepath.Join(l, "a.txt")) != "(missing)" {
		t.Fatal("a.txt not moved")
	}
	if read(t, filepath.Join(r, "photos", "x.jpg")) != "new x" || read(t, filepath.Join(r, "photos", "y.jpg")) != "old y" {
		t.Fatal("photos not merged")
	}
	// The kept files stay where they were.
	if read(t, filepath.Join(l, "b.txt")) != "new b" || read(t, filepath.Join(l, "photos", "y.jpg")) != "new y" {
		t.Fatal("kept files should stay in the source")
	}
	if !strings.Contains(m.status[0].Text, "2 old files kept") {
		t.Fatalf("status %v", m.status)
	}

	// Moving with overwrite empties the source folder, which disappears.
	root, m = twoDirs(t)
	selectAllLeft(m)
	m.Rune('m')
	m.Rune('o')
	l = filepath.Join(root, "left")
	if _, err := os.Stat(filepath.Join(l, "photos")); err == nil {
		t.Fatal("the emptied photos folder should be gone")
	}
	if read(t, filepath.Join(root, "right", "b.txt")) != "new b" {
		t.Fatal("b.txt not overwritten")
	}
}

func TestTransferRefused(t *testing.T) {
	root := testDirs(t)
	m := newModel(root, root, func(string) error { return nil }, func() {})
	m.Render(150, 20)
	m.Key(gui.KeyDown)
	m.Rune(' ') // a
	m.Rune('c')
	if m.confirm != nil || !strings.Contains(m.status[0].Text, "same folder") {
		t.Fatalf("same folder: %v", m.status)
	}
	// Into itself: the right panel inside a/.
	m.Key(gui.KeyTab)
	m.Key(gui.KeyDown)
	m.Key(gui.KeyEnter) // right: root/a
	m.Rune('c')
	if m.confirm != nil || !strings.Contains(m.status[0].Text, "into itself") {
		t.Fatalf("into itself: %v", m.status)
	}
}

func TestHint(t *testing.T) {
	root := testDirs(t)
	m := newModel(root, root, func(string) error { return nil }, func() {})
	m.Render(150, 20)
	m.Key(gui.KeyDown) // a
	h := m.hint()
	for _, k := range []string{"c copy", "m move", "d trash", "r rename", "esc unselect"} {
		if strings.Contains(h, k) {
			t.Fatalf("no selection: %q should not be shown in %q", k, h)
		}
	}
	for _, k := range []string{"space select", "n new folder", ". show hidden files", "esc quit"} {
		if !strings.Contains(h, k) {
			t.Fatalf("no selection: %q missing in %q", k, h)
		}
	}
	m.Rune(' ') // a; same folder in both panels: no c or m
	h = m.hint()
	if strings.Contains(h, "c copy") || !strings.Contains(h, "r rename") || strings.Contains(h, "h show") {
		t.Fatalf("one selected, same folders: %q", h)
	}
	m.Key(gui.KeyTab)
	m.Key(gui.KeyDown)
	m.Key(gui.KeyDown)
	m.Key(gui.KeyEnter) // right: b
	m.Key(gui.KeyTab)
	m.Rune(' ') // b too
	h = m.hint()
	if !strings.Contains(h, "c copy") || strings.Contains(h, "r rename") {
		t.Fatalf("two selected: %q", h)
	}
	for range 3 { // B.txt → b → a → ..
		m.Key(gui.KeyUp)
	}
	if strings.Contains(m.hint(), "space select") {
		t.Fatalf("on ..: %q", m.hint())
	}
}

func TestFolderGone(t *testing.T) {
	root := testDirs(t)
	m := newModel(root, filepath.Join(root, "a"), func(string) error { return nil }, func() {})
	m.trash = func(p string) (string, error) { return "", os.RemoveAll(p) }
	m.Render(150, 20)
	// Left: delete a/, which the right panel shows.
	m.Key(gui.KeyDown)
	m.Rune(' ')
	m.Rune('d')
	m.Rune('y')
	if m.panels[1].path != filepath.Clean(root) {
		t.Fatalf("right panel on %s, want %s", m.panels[1].path, root)
	}
	if m.status == nil || strings.HasPrefix(m.status[0].Text, "✘") {
		t.Fatalf("status %v", m.status)
	}
}

func TestPathLine(t *testing.T) {
	for _, c := range []struct{ path, parent, last string }{
		{filepath.Join(home, "Projects", "trimlet"), "~/Projects/", "trimlet"},
		{"/", "", "/"},
		{"/usr/local", "/usr/", "local"},
	} {
		l := pathLine(c.path, 40, true, 0)
		if l[0].Text != c.parent || l[1].Text != c.last || gui.LineLen(l) != 40 {
			t.Fatalf("%s: %q %q, %d wide", c.path, l[0].Text, l[1].Text, gui.LineLen(l))
		}
	}
	if gui.LineLen(pathLine("/a/very/long/path/that/does/not/fit/in/the/panel", 20, false, 0)) != 20 {
		t.Fatal("a long path should be cut to the panel width")
	}
}

func TestFilter(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"photos", "projects", "apps", "Pics"} {
		os.Mkdir(filepath.Join(root, d), 0o755)
	}
	for _, f := range []string{"paper.pdf", "a.txt", "prova.txt", "fish.txt"} {
		os.WriteFile(filepath.Join(root, f), []byte("x"), 0o644)
	}
	m := newModel(root, root, func(string) error { return nil }, func() {})
	m.Render(150, 20)

	m.Rune('P')
	if got := strings.Join(names(m.panels[0]), " "); got != ".. photos Pics projects paper.pdf prova.txt" {
		t.Fatalf("filter p: %s", got)
	}
	if e, _ := m.panels[0].current(); e.name != "photos" {
		t.Fatalf("cursor on %q", e.name)
	}
	if got := strings.Join(names(m.panels[1]), " "); strings.Contains(got, "P") == false && !strings.Contains(got, "apps") {
		t.Fatal("the other panel should not be filtered")
	}
	// The last letter replaces the filter.
	m.Rune('F')
	if got := strings.Join(names(m.panels[0]), " "); got != ".. fish.txt" {
		t.Fatalf("filter f: %s", got)
	}
	// Lowercase letters stay commands: c without selection says so.
	m.Rune('c')
	if m.status == nil || !strings.Contains(m.status[0].Text, "No files selected") {
		t.Fatalf("c: %v", m.status)
	}
	// Esc removes the filter, keeping the cursor on fish.txt.
	m.Key(gui.KeyDown)
	m.Key(gui.KeyEsc)
	if m.confirm != nil || len(m.panels[0].entries) != 9 {
		t.Fatalf("esc: %d rows, window %v", len(m.panels[0].entries), m.confirm != nil)
	}
	if e, _ := m.panels[0].current(); e.name != "fish.txt" {
		t.Fatalf("cursor on %q after esc", e.name)
	}
	// Entering a folder drops the filter.
	m.Rune('P')
	m.Key(gui.KeyEnter) // photos
	if m.panels[0].filter != 0 {
		t.Fatal("a new folder should have no filter")
	}
	// No match.
	m.Key(gui.KeyBackspace)
	m.Rune('Z')
	if m.status == nil || !strings.Contains(m.status[0].Text, "No files starting") {
		t.Fatalf("no match: %v", m.status)
	}
	// With a selection, Shift + letter does nothing.
	m.Key(gui.KeyEsc)
	m.Key(gui.KeyDown)
	m.Rune(' ')
	m.Rune('P')
	if m.panels[0].filter != 0 || m.status == nil {
		t.Fatal("filter with a selection should only show a message")
	}
}

func TestSameFolder(t *testing.T) {
	root := testDirs(t)
	m := newModel(filepath.Join(root, "a"), root, func(string) error { return nil }, func() {})
	m.Render(150, 20)
	m.Key(gui.KeyTab)  // right: root
	m.Key(gui.KeyDown) // a
	m.Key(gui.KeyDown) // b
	m.Key(gui.KeyShiftTab)
	if m.active != 0 || m.panels[0].path != filepath.Clean(root) {
		t.Fatalf("active %d, left on %s", m.active, m.panels[0].path)
	}
	if e, _ := m.panels[0].current(); e.name != "b" {
		t.Fatalf("cursor on %q, want b", e.name)
	}
	if strings.Contains(m.hint(), "shift+tab") {
		t.Fatal("same folder in both panels: shift+tab should not be shown")
	}
}

func TestTransferBody(t *testing.T) {
	_, m := twoDirs(t)
	selectAllLeft(m)
	m.Rune('c')
	b := m.confirm.Body
	if len(b) < 5 || !strings.Contains(b[0][1].Text, "->") {
		t.Fatalf("body %v", b)
	}
	if b[2][0].Text != "  photos/" || b[3][0].Text != "  a.txt" || b[4][0].Text != "  b.txt" {
		t.Fatalf("names %q %q %q", b[2][0].Text, b[3][0].Text, b[4][0].Text)
	}
	if got := cutPath("/a/projects/trimlet/test-files/left", 22); got != "…/test-files/left" {
		t.Fatalf("cutPath: %q", got)
	}
	if got := cutPath("/a/a-very-long-folder-name-that-does-not-fit", 12); len([]rune(got)) != 12 {
		t.Fatalf("cutPath long: %q", got)
	}
}

// fakeTrash moves into a folder of the test, as the real Trash does.
func fakeTrash(t *testing.T) func(string) (string, error) {
	bin := t.TempDir()
	return func(p string) (string, error) {
		where := filepath.Join(bin, filepath.Base(p))
		return where, os.Rename(p, where)
	}
}

func TestUndo(t *testing.T) {
	// Rename.
	root := testDirs(t)
	m := newModel(root, root, func(string) error { return nil }, func() {})
	m.trash = fakeTrash(t)
	m.Render(150, 20)
	if strings.Contains(m.hint(), "undo") {
		t.Fatal("nothing to undo yet")
	}
	m.Key(gui.KeyDown) // a
	m.Rune(' ')
	m.Rune('r')
	m.Rune('z') // "az"
	m.Key(gui.KeyEnter)
	if !strings.Contains(m.hint(), "u undo rename") {
		t.Fatalf("hint %q", m.hint())
	}
	m.Rune('u')
	if m.confirm == nil || m.confirm.Question != `Undo: rename "az" back to "a"?` {
		t.Fatalf("question %+v", m.confirm)
	}
	m.Key(gui.KeyEnter)
	if _, err := os.Stat(filepath.Join(root, "a")); err != nil || m.last != nil {
		t.Fatalf("rename not undone: %v", err)
	}

	// Trash.
	m.Key(gui.KeyDown) // b (cursor was on a)
	m.Rune(' ')
	m.Rune('d')
	m.Rune('y')
	if _, err := os.Stat(filepath.Join(root, "b")); err == nil {
		t.Fatal("b should be in the trash")
	}
	m.Rune('u')
	m.Rune('y')
	if _, err := os.Stat(filepath.Join(root, "b")); err != nil {
		t.Fatalf("b not put back: %v", err)
	}

	// New folder: undone while empty.
	m.Rune('n')
	m.Rune('x')
	m.Key(gui.KeyEnter)
	m.Rune('u')
	m.Rune('y')
	if _, err := os.Stat(filepath.Join(root, "x")); err == nil {
		t.Fatal("x should be removed")
	}

	// Move with a merge, then undo: everything back as before.
	r, m2 := twoDirs(t)
	m2.trash = fakeTrash(t)
	selectAllLeft(m2)
	m2.Rune('m')
	m2.Rune('k')
	m2.Rune('u')
	m2.Rune('y')
	l := filepath.Join(r, "left")
	for _, f := range []string{"a.txt", "b.txt", "photos/x.jpg", "photos/y.jpg"} {
		if _, err := os.Stat(filepath.Join(l, f)); err != nil {
			t.Fatalf("%s not back: %v (status %v)", f, err, m2.status)
		}
	}
	if read(t, filepath.Join(r, "right", "photos", "y.jpg")) != "old y" || read(t, filepath.Join(r, "right", "photos", "x.jpg")) != "(missing)" {
		t.Fatal("the destination should be as before")
	}

	// Copy, then undo: the copies go, the old files stay.
	r, m3 := twoDirs(t)
	m3.trash = fakeTrash(t)
	selectAllLeft(m3)
	m3.Rune('c')
	m3.Rune('k')
	m3.Rune('u')
	m3.Rune('y')
	rr := filepath.Join(r, "right")
	if read(t, filepath.Join(rr, "a.txt")) != "(missing)" || read(t, filepath.Join(rr, "photos", "x.jpg")) != "(missing)" {
		t.Fatal("copies should be gone")
	}
	if read(t, filepath.Join(rr, "b.txt")) != "old b" || read(t, filepath.Join(rr, "photos", "z.jpg")) != "old z" {
		t.Fatal("old files should stay")
	}
}

func TestHomeAndSaved(t *testing.T) {
	root := testDirs(t)
	m := newModel(root, root, func(string) error { return nil }, func() {})
	var saved [2]string
	m.onDirs = func(l, r string) { saved = [2]string{l, r} }
	m.Render(150, 20)
	if !strings.Contains(m.hint(), "h home") {
		t.Fatalf("hint %q", m.hint())
	}
	m.Rune('h')
	if m.panels[0].path != home || saved[0] != home || saved[1] != filepath.Clean(root) {
		t.Fatalf("left %s, saved %v", m.panels[0].path, saved)
	}
	if strings.Contains(m.hint(), "h home") {
		t.Fatal("already home: h should not be shown")
	}
}
