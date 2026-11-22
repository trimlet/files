package main

import (
	"fmt"
	"image/color"
	"os"
	"strings"

	"github.com/trimlet/gui"
)

type line = gui.Line

// hint lists only the keys that do something right now (author: "non vengano mostrati i tasti inutili").
func (m *model) hint() string {
	p := m.panels[m.active]
	cur, ok := p.current()
	items, src := m.selection()
	var keys []string
	add := func(when bool, k string) {
		if when {
			keys = append(keys, k)
		}
	}
	add(len(p.entries) > 1, "↑↓ move")
	add(true, "tab panel")
	add(m.panels[1-m.active].path != p.path, "shift+tab same folder")
	add(ok, "enter open")
	add(!isRoot(p.path), "backspace up")
	add(ok && !cur.up, "space select")
	sel := len(items) > 0
	other := sel && m.panels[1-m.selPanel].path != src
	add(other, "c copy")
	add(other, "m move")
	add(sel, "d trash")
	add(len(items) == 1, "r rename")
	add(true, "n new folder")
	add(home != "" && p.path != home, "h home")
	if m.last != nil {
		add(true, "u undo "+m.last.label())
	}
	add(!sel, "shift+letter filter")
	if p.filter != 0 {
		add(true, "esc show all")
	} else if !sel {
		if m.hidden {
			add(true, ". hide hidden files")
		} else {
			add(true, ". show hidden files")
		}
		add(true, "esc quit")
	} else {
		add(true, "esc unselect")
	}
	return strings.Join(keys, " · ")
}

// Render draws the two panels side by side, the status and the hint.
func (m *model) Render(cols, rows int) []line {
	m.cols, m.rows = cols, rows
	width := max((cols-3)/2, 10) // each panel; 3 = " │ " between them
	room := m.listRoom()
	left := m.renderPanel(0, width, room)
	right := m.renderPanel(1, width, room)

	var lines []line
	sep := gui.Colored(" │ ", gui.Muted)
	for r := range left {
		l := append(append(line{}, left[r]...), sep)
		lines = append(lines, append(l, right[r]...))
	}
	h := m.hint()
	if m.edit != nil {
		verb := "rename"
		if m.edit.newDir {
			verb = "create"
		}
		h = fmt.Sprintf(editHint, verb)
	}
	if m.confirm != nil {
		gui.Overlay(lines, m.confirm.Box(), cols, 0, len(lines))
		h = m.confirm.Hint()
	}
	lines = append(lines, nil, m.status, hintLine(h, cols, m.flash))
	return lines
}

// listRoom is how many rows of a panel fit: the window less the top margin, the path and its line,
// the empty line, the status and the hint.
func (m *model) listRoom() int {
	return max(m.rows-6, 1)
}

// renderPanel gives exactly room+2 lines of width characters: the path, its line, then the rows.
func (m *model) renderPanel(i, width, room int) []line {
	p := m.panels[i]
	p.scroll(room)
	active := i == m.active

	lines := []line{pathLine(p.path, width, active, p.filter), ruleLine(width, active)}

	for r := 0; r < room; r++ {
		n := p.top + r
		if n >= len(p.entries) {
			lines = append(lines, line{gui.Plain(strings.Repeat(" ", width))})
			continue
		}
		var field *editField
		if m.edit != nil && m.edit.panel == i && n == p.cursor {
			field = m.edit
		}
		l := m.renderEntry(i, p.entries[n], width, field)
		if active && n == p.cursor {
			l = highlight(l, width)
		}
		lines = append(lines, l)
	}
	return lines
}

// renderEntry is one row: mark, name, size and date. On narrow windows the date is left out.
// field, if given, is the name being written in this row.
func (m *model) renderEntry(i int, e entry, width int, field *editField) line {
	const sizeW, dateW = 8, 16
	nameW := width - 2 - 1 - sizeW - 2 - dateW
	showDate := nameW >= 12
	if !showDate {
		nameW = width - 2 - 1 - sizeW
	}

	name := e.name
	if e.dir && !e.up {
		name += "/"
	}
	mark := "  "
	nameSpan := gui.Plain(pad(gui.Truncate(name, nameW), nameW))
	switch {
	case m.selected(i, e.name):
		mark = "• "
		nameSpan = gui.Strong(nameSpan.Text, gui.Amber)
	case e.dir:
		nameSpan.Bold = true
	}
	nameCells := line{nameSpan}
	if field != nil {
		nameCells = field.render(nameW)
	}

	size, date := "", ""
	if !e.dir {
		size = humanSize(e.size)
	}
	if !e.up && !e.mod.IsZero() {
		date = e.mod.Format("2006-01-02 15:04")
	}
	l := append(line{gui.Colored(mark, gui.Amber)}, nameCells...)
	l = append(l, gui.Colored(fmt.Sprintf(" %*s", sizeW, size), gui.Muted))
	if showDate {
		l = append(l, gui.Colored(fmt.Sprintf("  %-*s", dateW, date), gui.Muted))
	}
	return l
}

// The hint line is a band of its own at the bottom of the window: a lighter background across the
// whole width, each key in green bold and what it does in light grey (author, 2026-11-19: it was
// all grey and hard to see).
var (
	hintBg   color.Color = gui.Palettes["grey"][9]
	hintText color.Color = gui.Palettes["grey"][3]
)

// hintLine draws a hint such as "↑↓ move · tab panel": the first word of each part is the key.
// The key equal to flash is drawn lit (green background, dark text): the key just pressed.
func hintLine(h string, cols int, flash string) line {
	l := line{{Text: " ", Bg: hintBg}}
	for i, part := range strings.Split(h, " · ") {
		if i > 0 {
			l = append(l, gui.Span{Text: "   ", Bg: hintBg})
		}
		key, what, _ := strings.Cut(part, " ")
		k := gui.Span{Text: "[" + key + "]", Color: gui.Accent, Bold: true, Bg: hintBg}
		if key == flash {
			k.Color, k.Bg = gui.Background, gui.Accent
		}
		l = append(l, k)
		if what != "" {
			l = append(l, gui.Span{Text: " " + what, Color: hintText, Bg: hintBg})
		}
	}
	if n := gui.LineLen(l); n < cols {
		l = append(l, gui.Span{Text: strings.Repeat(" ", cols-n), Bg: hintBg})
	}
	return l
}

// The path at the top of a panel, with a line under it that separates it from the files: the line is
// white in the active panel, grey in the other (see pathLine and ruleLine; docs/apps/files.md 9).

// home is the user's folder, written "~" in the paths at the top.
var home, _ = os.UserHomeDir()

// shortPath writes path with "~" for the user's folder.
func shortPath(path string) string {
	if home != "" && inside(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

// pathLine draws the path of a panel on one line, width characters wide: the whole path in white,
// the folder you are in green bold. In the other panel the same, with the folder in amber (copper 400).
// (Author, 2026-11-19: readable without effort, the last folder standing out, no bands.)
// filter, if set, is written after the path: only "p".
func pathLine(path string, width int, active bool, filter rune) line {
	var suffix gui.Span
	if filter != 0 {
		suffix = gui.Colored(fmt.Sprintf("   only %q", string(filter)), hintText)
	}
	d := truncLeft(shortPath(path), max(width-gui.LineLen(line{suffix}), 1))
	parent, last := "", d
	if i := strings.LastIndexAny(d, `/\\`); i >= 0 && i < len(d)-1 {
		parent, last = d[:i+1], d[i+1:]
	}
	l := line{gui.Colored(parent, gui.White), gui.Strong(last, gui.Accent)}
	if !active {
		l = line{gui.Colored(parent, gui.White), gui.Strong(last, gui.Amber)}
	}
	return padLine(append(l, suffix), width)
}

// padLine fills l with spaces up to width characters.
func padLine(l line, width int) line {
	if n := gui.LineLen(l); n < width {
		l = append(l, gui.Plain(strings.Repeat(" ", width-n)))
	}
	return l
}

// ruleLine is the line under the path: white in the active panel, dark grey in the other one.
func ruleLine(width int, active bool) line {
	var c color.Color = gui.Palettes["grey"][8]
	if active {
		c = gui.White
	}
	return line{gui.Colored(strings.Repeat("─", width), c)}
}

// highlight gives the row the background of the cursor, keeping the spans that have their own
// (the text cursor of a name being written).
func highlight(l line, width int) line {
	out := make(line, 0, len(l)+1)
	for _, s := range l {
		if s.Bg == nil {
			s.Bg = gui.SelectedBg
		}
		out = append(out, s)
	}
	if n := gui.LineLen(l); n < width {
		out = append(out, gui.Span{Text: strings.Repeat(" ", width-n), Bg: gui.SelectedBg})
	}
	return out
}

// humanSize writes a size in B, KB, MB or GB.
func humanSize(n int64) string {
	const k = 1024
	switch {
	case n < k:
		return fmt.Sprintf("%d B", n)
	case n < k*k:
		return fmt.Sprintf("%.0f KB", float64(n)/k)
	case n < k*k*k:
		return fmt.Sprintf("%.1f MB", float64(n)/(k*k))
	default:
		return fmt.Sprintf("%.1f GB", float64(n)/(k*k*k))
	}
}

// truncLeft cuts s to n characters keeping the end (the deepest folders of a path).
func truncLeft(s string, n int) string {
	r := []rune(s)
	if n <= 0 || len(r) <= n {
		return s
	}
	return "…" + string(r[len(r)-n+1:])
}

// pad fills s with spaces up to n characters.
func pad(s string, n int) string {
	if l := len([]rune(s)); l < n {
		return s + strings.Repeat(" ", n-l)
	}
	return s
}
