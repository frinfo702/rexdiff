package tui

// This file keeps the file list of each tab as a tree of directories, as
// in herdr-reviewr. The cursor of the list moves over the directories and
// the files. The pane shows the last file that the cursor is on.

import (
	"slices"
	"strings"

	"github.com/rednafi/rexdiff/internal/tree"
	"github.com/rednafi/rexdiff/vcs"
)

// navTree is the file list of a tab.
type navTree struct {
	entries []tree.Entry
	rows    []tree.Row
	cur     int // the row of the cursor
	// expanded is true if the directories start expanded. toggled has the
	// directories that the user changes from that start.
	expanded bool
	toggled  map[string]bool
}

// rebuild makes the rows of entries. The cursor stays on its directory or
// its file. If the row is gone, or the cursor is -1, the cursor goes to
// the file with index open, else to the first file.
func (t *navTree) rebuild(entries []tree.Entry, expanded bool, open int) {
	var dir, path string
	if t.cur >= 0 && t.cur < len(t.rows) {
		if r := t.rows[t.cur]; r.File < 0 {
			dir = r.Dir
		} else {
			path = t.entries[r.File].Path
		}
	}
	t.entries, t.expanded = entries, expanded
	t.rows = tree.Build(entries, t.toggled, expanded)
	t.cur = slices.IndexFunc(t.rows, func(r tree.Row) bool {
		if r.File < 0 {
			return dir != "" && r.Dir == dir
		}
		return path != "" && entries[r.File].Path == path
	})
	if t.cur < 0 {
		t.cur = t.rowOf(open)
	}
	if t.cur < 0 {
		t.cur = t.nextFile(-1, 1)
	}
	t.cur = max(t.cur, 0)
}

// file returns the index of the file at the cursor, or -1 on a directory.
func (t *navTree) file() int {
	if t.cur >= len(t.rows) {
		return -1
	}
	return t.rows[t.cur].File
}

// dir returns the row at the cursor if it is a directory.
func (t *navTree) dir() (tree.Row, bool) {
	if t.cur >= len(t.rows) || t.rows[t.cur].File >= 0 {
		return tree.Row{}, false
	}
	return t.rows[t.cur], true
}

// rowOf returns the row of the file with index i, or -1 if no row shows
// it.
func (t *navTree) rowOf(i int) int {
	if i < 0 {
		return -1
	}
	return slices.IndexFunc(t.rows, func(r tree.Row) bool { return r.File == i })
}

// nextFile returns the first file row after row from in direction d (1 or
// -1), or -1.
func (t *navTree) nextFile(from, d int) int {
	for r := from + d; r >= 0 && r < len(t.rows); r += d {
		if t.rows[r].File >= 0 {
			return r
		}
	}
	return -1
}

// setExpanded expands or collapses the directory dir. It reports whether
// the state changes.
func (t *navTree) setExpanded(dir string, want bool) bool {
	if (t.expanded != t.toggled[dir]) == want {
		return false
	}
	if t.toggled == nil {
		t.toggled = map[string]bool{}
	}
	if t.toggled[dir] {
		delete(t.toggled, dir)
	} else {
		t.toggled[dir] = true
	}
	return true
}

// reveal expands each directory above path. It reports whether one
// changes.
func (t *navTree) reveal(path string) bool {
	changed := false
	for dir := path; ; {
		i := strings.LastIndexByte(dir, '/')
		if i < 0 {
			return changed
		}
		dir = dir[:i]
		changed = t.setExpanded(dir, true) || changed
	}
}

// list returns the file list of the current tab, or nil on the PR tab.
func (m *model) list() *navTree {
	switch m.tab {
	case tabChanges:
		return &m.changeTree
	case tabFiles:
		return &m.fileTree
	}
	return nil
}

// buildChangeTree makes the rows of the Changes tab. The selected file is
// the file at the cursor, if the cursor is on a file.
func (m *model) buildChangeTree() {
	es := make([]tree.Entry, len(m.files))
	for i, f := range m.files {
		es[i] = tree.Entry{Path: f.Path, Changed: true}
	}
	t := &m.changeTree
	t.rebuild(es, true, m.changeSel)
	if i := t.file(); i >= 0 {
		m.changeSel = i
	}
	m.changeSel = max(m.changeSel, 0)
}

// buildFileTree makes the rows of the Files tab. A directory holds a
// change if a file under it is in the Changes tab. The selected file is
// the file at the cursor, if the cursor is on a file.
func (m *model) buildFileTree() {
	changed := make(map[string]bool, len(m.files))
	for _, f := range m.files {
		changed[f.Path] = true
	}
	es := make([]tree.Entry, len(m.allFiles))
	for i, e := range m.allFiles {
		p, isDir := strings.CutSuffix(e.Path, "/")
		es[i] = tree.Entry{Path: p, Changed: changed[p], Ignored: e.Ignored, Dir: isDir}
	}
	t := &m.fileTree
	t.rebuild(es, false, m.allSel)
	if i := t.file(); i >= 0 {
		m.allSel = i
	}
	m.allSel = min(max(m.allSel, 0), max(len(m.allFiles)-1, 0))
}

// buildTree makes the rows of the file list of the current tab.
func (m *model) buildTree() {
	switch m.tab {
	case tabChanges:
		m.buildChangeTree()
	case tabFiles:
		m.buildFileTree()
	}
}

// withIgnored adds to es the entries of each ignored directory that the
// user expands in the Files tab, and sorts es by path.
func (m *model) withIgnored(es []vcs.Entry) []vcs.Entry {
	// The loop also reads the entries that it adds, for the directories
	// in an ignored directory.
	for i := 0; i < len(es); i++ {
		if dir, ok := m.openIgnored(es[i]); ok {
			es = append(es, m.repo.IgnoredDir(dir)...)
		}
	}
	slices.SortStableFunc(es, func(a, b vcs.Entry) int { return strings.Compare(a.Path, b.Path) })
	return es
}

// shownIgnored returns the ignored paths that the Files tab shows, when
// that tab shows. They are the directories that the user expands and the
// open file. The watcher reports their changes, as in herdr-reviewr.
func (m *model) shownIgnored() []string {
	if m.tab != tabFiles {
		return nil
	}
	var paths []string
	for _, e := range m.allFiles {
		if dir, ok := m.openIgnored(e); ok {
			paths = append(paths, dir)
		}
	}
	if m.allSel < len(m.allFiles) {
		if e := m.allFiles[m.allSel]; e.Ignored && !strings.HasSuffix(e.Path, "/") {
			paths = append(paths, e.Path)
		}
	}
	return paths
}

// openIgnored returns the path of e without the last "/", and reports
// whether e is an ignored directory that the user expands in the Files tab.
func (m *model) openIgnored(e vcs.Entry) (string, bool) {
	dir, isDir := strings.CutSuffix(e.Path, "/")
	return dir, isDir && e.Ignored && m.fileTree.toggled[dir]
}

// onDir reports whether the file list has the focus and its cursor is on
// a directory.
func (m *model) onDir() bool {
	if t := m.list(); t != nil && m.focusList {
		_, ok := t.dir()
		return ok
	}
	return false
}

// setDir expands or collapses the directory at the cursor of the file
// list. An ignored directory reads its entries when it expands.
func (m *model) setDir(want bool) {
	t := m.list()
	r, ok := t.dir()
	if !ok || !t.setExpanded(r.Dir, want) {
		return
	}
	if m.tab == tabFiles && r.Ignored && want {
		m.readFiles()
	}
	m.buildTree()
}

// stepRow moves the cursor of the file list by d rows. On a file, the pane
// shows the file.
func (m *model) stepRow(d int) {
	t := m.list()
	if t == nil || len(t.rows) == 0 {
		return
	}
	t.cur = min(max(t.cur+d, 0), len(t.rows)-1)
	if i := t.file(); i >= 0 && i != m.listSel() {
		m.pickFile(i)
	}
}

// stepFile shows the next (d = 1) or the previous (d = -1) file of the
// list, past the directories. It starts from the cursor of the list when
// the list has the focus, else from the file that shows.
func (m *model) stepFile(d int) {
	t := m.list()
	if t == nil {
		return
	}
	from := t.cur
	if r := t.rowOf(m.listSel()); !m.focusList && r >= 0 {
		from = r
	}
	if r := t.nextFile(from, d); r >= 0 {
		m.pickFile(t.rows[r].File)
	}
}

// clickRow handles a click on row r of the file list. A file shows, and a
// directory expands or collapses.
func (m *model) clickRow(r int) {
	t := m.list()
	if t == nil || r < 0 || r >= len(t.rows) {
		return
	}
	m.focusList = true
	t.cur = r
	if d, ok := t.dir(); ok {
		m.setDir(!d.Expanded)
		return
	}
	m.pickFile(t.rows[r].File)
}

// revealFile expands the directories above path in the file list of the
// current tab, so that its row shows.
func (m *model) revealFile(path string) {
	if t := m.list(); t != nil && t.reveal(path) {
		m.buildTree()
	}
}
