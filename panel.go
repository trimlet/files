package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

// entry is one row of a panel: a file, a folder, or ".." (up).
type entry struct {
	name string
	dir  bool
	up   bool // the ".." row
	size int64
	mod  time.Time
}

// panel is one of the two lists: the folder it shows, its rows, the cursor and the first visible row.
type panel struct {
	path    string
	entries []entry // the rows shown: all of them, or only those of the filter
	all     []entry // every row of the folder
	filter  rune    // 0, or the letter the shown names start with (Shift + letter)
	cursor  int
	top     int
}

// readPanel reads the folder at path: "..", then folders, then files, each alphabetical ignoring case.
// Hidden files and folders (names starting with ".") are left out unless hidden is true.
// Links are followed, so a link to a folder opens like a folder.
func readPanel(path string, hidden bool) (*panel, error) {
	path = filepath.Clean(path)
	list, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var dirs, files []entry
	for _, d := range list {
		if !hidden && strings.HasPrefix(d.Name(), ".") {
			continue
		}
		e := entry{name: d.Name(), dir: d.IsDir()}
		info, err := os.Stat(filepath.Join(path, d.Name()))
		if err != nil {
			info, err = d.Info() // a broken link: show the link itself
		}
		if err == nil {
			e.dir, e.size, e.mod = info.IsDir(), info.Size(), info.ModTime()
		}
		if e.dir {
			dirs = append(dirs, e)
		} else {
			files = append(files, e)
		}
	}
	byName := func(l []entry) {
		sort.Slice(l, func(i, j int) bool { return strings.ToLower(l[i].name) < strings.ToLower(l[j].name) })
	}
	byName(dirs)
	byName(files)

	p := &panel{path: path}
	if !isRoot(path) {
		p.entries = append(p.entries, entry{name: "..", dir: true, up: true})
	}
	p.entries = append(append(p.entries, dirs...), files...)
	p.all = p.entries
	return p, nil
}

// applyFilter shows only the rows starting with the letter r (any case), plus "..";
// r == 0 shows them all again. Folders stay before files.
func (p *panel) applyFilter(r rune) {
	p.filter = r
	if r == 0 {
		p.entries = p.all
		return
	}
	p.entries = nil
	for _, e := range p.all {
		if e.up || startsWith(e.name, r) {
			p.entries = append(p.entries, e)
		}
	}
}

func startsWith(name string, r rune) bool {
	for _, c := range name {
		return unicode.ToLower(c) == unicode.ToLower(r)
	}
	return false
}

func isRoot(path string) bool { return filepath.Dir(path) == path }

// current is the row under the cursor; ok is false in an empty folder.
func (p *panel) current() (entry, bool) {
	if p.cursor < 0 || p.cursor >= len(p.entries) {
		return entry{}, false
	}
	return p.entries[p.cursor], true
}

// find puts the cursor on the row called name, if there is one.
func (p *panel) find(name string) {
	for i, e := range p.entries {
		if !e.up && e.name == name {
			p.cursor = i
			return
		}
	}
}

func (p *panel) move(delta int) {
	p.cursor = min(max(p.cursor+delta, 0), max(len(p.entries)-1, 0))
}

// scroll moves the first visible row so that the cursor stays inside room rows.
func (p *panel) scroll(room int) {
	if p.cursor < p.top {
		p.top = p.cursor
	}
	if p.cursor >= p.top+room {
		p.top = p.cursor - room + 1
	}
	p.top = min(max(p.top, 0), max(len(p.entries)-room, 0))
}
