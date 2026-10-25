package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/rednafi/rexdiff/internal/testgit"
	"github.com/rednafi/rexdiff/vcs"
)

// TestSkippedFileNotes checks the notes in place of a binary file and of a
// file that is too large, in both tabs.
func TestSkippedFileNotes(t *testing.T) {
	tests := []struct{ content, want string }{
		{"a\x00b\n", "binary file · no line comments"},
		{strings.Repeat("x\n", 50_001), "file too large to show"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			m, dir := newTestModel(t)
			// The name sorts first, so that both tabs select the file.
			testgit.WriteFile(t, dir, "0.txt", tt.content)
			m.reload()
			m.pickFile(0)
			if !shows(m, tt.want) {
				t.Errorf("Changes got:\n%s\nwant %q", view(m), tt.want)
			}
			if m = press(t, m, "2"); !shows(m, tt.want) {
				t.Errorf("Files got:\n%s\nwant %q", view(m), tt.want)
			}
		})
	}
}

// TestEmptyNotes checks the notes of an empty scope, of an empty list of
// files, of an empty file, and of a file that rexdiff cannot read.
func TestEmptyNotes(t *testing.T) {
	m, dir := newTestModel(t)
	testgit.WriteFile(t, dir, "a.go", "package a\n\nfunc A() int {\n\treturn 1\n}\n")
	if m.reload(); !shows(m, "no changes") {
		t.Errorf("an empty scope got:\n%s\nwant \"no changes\"", view(m))
	}

	testgit.WriteFile(t, dir, "0.txt", "")
	if m = press(t, m, "2"); !shows(m, "empty file") {
		t.Errorf("an empty file got:\n%s\nwant \"empty file\"", view(m))
	}
	if err := os.Remove(filepath.Join(dir, "0.txt")); err != nil {
		t.Fatal(err)
	}
	if m.refreshViewer(); !shows(m, "couldn't read this file") {
		t.Errorf("a removed file got:\n%s\nwant \"couldn't read this file\"", view(m))
	}

	m.allFiles = []vcs.Entry{}
	m.buildFileTree()
	if m.renderViewer(); !shows(m, "no files") {
		t.Errorf("no files got:\n%s\nwant \"no files\"", view(m))
	}
}

// TestStatusMessages checks the short messages in the footer, and how
// long they show.
func TestStatusMessages(t *testing.T) {
	m, _ := newTestModel(t)
	clk := setClock(&m)
	m = press(t, m, "]", "c", "h", "i", "enter")
	if m.status != "comment added" {
		t.Errorf("a new comment: status %q, want \"comment added\"", m.status)
	}
	clk.advance(flashTime)
	if m = step(t, m, tickMsg{}); m.status != "comment added" {
		t.Errorf("a tick at the end of the flash: status %q, want it kept", m.status)
	}
	clk.advance(time.Nanosecond)
	if m = step(t, m, tickMsg{}); m.status != "" {
		t.Errorf("a tick after the flash: status %q, want none", m.status)
	}
	m = press(t, m, "e", "!", "enter")
	if m.status != "comment updated" {
		t.Errorf("an edit: status %q, want \"comment updated\"", m.status)
	}
	m = press(t, m, "d")
	if m.status != "comment deleted" || m.comments.Len() != 0 {
		t.Errorf("d: status %q, %d comments; want \"comment deleted\", 0",
			m.status, m.comments.Len())
	}

	m = press(t, m, "]", "c", "x", "enter", "l", "d")
	if m.status != "comment deleted" || m.comments.Len() != 0 {
		t.Errorf("d in the list: status %q, %d comments; want \"comment deleted\", 0",
			m.status, m.comments.Len())
	}

	m = step(t, m, tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m = press(t, m, "z", "z", "z", "enter")
	if !strings.HasSuffix(lastLine(m), "no matches") {
		t.Errorf("a find with no hit: footer %q, want \"no matches\"", lastLine(m))
	}
	m = press(t, m, "esc")

	if m = press(t, m, "B", "z", "z", "z", "z"); !shows(m, "no matches") {
		t.Errorf("a picker with no match got:\n%s\nwant \"no matches\"", view(m))
	}
}

// TestRenderedKey checks that m acts only on a markdown file that renders
// some text.
func TestRenderedKey(t *testing.T) {
	m, dir := newTestModel(t)
	if m = press(t, m, "m"); m.rendered {
		t.Error("m on a.go turns on rendered markdown, want no change")
	}
	testgit.WriteFile(t, dir, "0.md", "\n\n")
	m.reload()
	m.pickFile(0)
	m = press(t, m, "m")
	if m.rendered || m.status != "nothing here renders" {
		t.Errorf("m on a blank 0.md: rendered %v, status %q; want false, \"nothing here renders\"",
			m.rendered, m.status)
	}
	testgit.WriteFile(t, dir, "0.md", "# Title\n")
	m.reload()
	if m = press(t, m, "m"); !m.rendered {
		t.Error("m on 0.md does not turn on rendered markdown")
	}
}

// TestGotoLineInput checks the go-to-line prompt. It takes only digits
// and "$", and a paste keeps only a line number.
func TestGotoLineInput(t *testing.T) {
	m := newFoldModel(t)
	m = press(t, m, ":", "a", "b", "c")
	if got := m.overlay.input.Value(); got != "" {
		t.Errorf(":abc types %q, want nothing", got)
	}
	m = press(t, m, "1", "$")
	if got := m.overlay.input.Value(); got != "$" {
		t.Errorf(":1$ types %q, want \"$\"", got)
	}
	m = press(t, m, "7")
	if got := m.overlay.input.Value(); got != "7" {
		t.Errorf(":$7 types %q, want \"7\"", got)
	}
	m = step(t, m, tea.PasteMsg{Content: " `12`, "})
	if got := m.overlay.input.Value(); got != "12" {
		t.Errorf("a paste of `12`, gives %q, want \"12\"", got)
	}
	m = step(t, m, tea.PasteMsg{Content: "f.txt:3"})
	if got := m.overlay.input.Value(); got != "12" {
		t.Errorf("a paste of f.txt:3 gives %q, want \"12\" kept", got)
	}
}

// TestGotoLineClamps checks that : clamps the line to the file, and opens
// the fold that has the line.
func TestGotoLineClamps(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"999999", 40},
		{"99999999999999999999", 40},
		{"$", 40},
		{"0", 1},
		{"1", 1},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			m := newFoldModel(t)
			m = press(t, m, ":")
			m = step(t, m, tea.PasteMsg{Content: tt.input})
			m = press(t, m, "enter")
			p := &m.changes
			if r := p.rows[p.cur]; r.New != tt.want {
				t.Errorf(":%s: the cursor is on %+v, want line %d", tt.input, r, tt.want)
			}
		})
	}
}

// TestGotoLineDeletedFile checks that : uses the old lines of a deleted
// file.
func TestGotoLineDeletedFile(t *testing.T) {
	m := newFoldModel(t)
	if err := os.Remove(filepath.Join(m.repo.Root(), "f.txt")); err != nil {
		t.Fatal(err)
	}
	m.reload()
	m = press(t, m, ":", "2", "5", "enter")
	p := &m.changes
	if r := p.rows[p.cur]; r.Old != 25 {
		t.Errorf("the cursor is on %+v, want old line 25", r)
	}
}
