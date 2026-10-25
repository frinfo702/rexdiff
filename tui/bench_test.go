package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rednafi/rexdiff/diff"
	"github.com/rednafi/rexdiff/internal/testgit"
	"github.com/rednafi/rexdiff/vcs"
)

// BenchmarkReview reloads a large change and scrolls through it, as a user
// does. Use it to profile the program:
//
//	go test ./tui -run '^$' -bench Review \
//		-cpuprofile cpu.out -memprofile mem.out
func BenchmarkReview(b *testing.B) {
	m, dir := newTestModel(b)
	// The benchmark makes 20 new files of 2,000 lines each.
	var src strings.Builder
	for i := range 2000 {
		src.WriteString("\tx := compute(" + strings.Repeat("a", i%40) + ")\n")
	}
	content := "package a\nfunc f() {\n" + src.String() + "}\n"
	for i := range 20 {
		testgit.WriteFile(b, dir, "big"+string(rune('a'+i))+".go", content)
	}
	// Select a large file. The reload keeps the selected file.
	m.reload()
	m.pickFile(1)
	down := tea.KeyPressMsg{Code: 'j', Text: "j"}
	for b.Loop() {
		m.reload()
		for range 100 {
			m = step(b, m, down)
			_ = m.View()
		}
	}
}

// BenchmarkScroll scrolls a large change sideways. It scrolls in the Changes
// tab with the folds closed and open, and in the Files tab.
//
//	go test ./tui -run '^$' -bench Scroll
func BenchmarkScroll(b *testing.B) {
	for _, name := range []string{"folded", "open", "file"} {
		b.Run(name, func(b *testing.B) {
			m := bigChange(b)
			switch name {
			case "open":
				for {
					i := slices.IndexFunc(m.changes.rows, func(r diff.Row) bool { return r.Fold.Lines > 0 })
					if i < 0 {
						break
					}
					m.expandFold(m.changes.rows[i].Fold)
				}
			case "file":
				m.setTab(tabFiles)
				m.pickFile(slices.IndexFunc(m.allFiles, func(e vcs.Entry) bool { return e.Path == "big.go" }))
			}
			right := tea.KeyPressMsg{Code: tea.KeyRight}
			left := tea.KeyPressMsg{Code: tea.KeyLeft}
			for b.Loop() {
				m = step(b, m, right)
				_ = m.View()
				m = step(b, m, left)
				_ = m.View()
			}
		})
	}
}

// bigChange returns a model that shows a change to big.go, a file of 5,000
// long lines. The change edits each 100th line.
func bigChange(b *testing.B) model {
	b.Helper()
	m, dir := newTestModel(b)
	var old, cur strings.Builder
	for i := range 5000 {
		l := fmt.Sprintf("\tx%d := compute(%q, %d) // a long line with words\n", i, strings.Repeat("a", i%40), i*7)
		old.WriteString(l)
		if i%100 == 0 {
			l = "\ty := 1\n"
		}
		cur.WriteString(l)
	}
	testgit.WriteFile(b, dir, "big.go", "package a\nfunc f() {\n"+old.String()+"}\n")
	testgit.Git(b, dir, "add", ".")
	testgit.Git(b, dir, "commit", "-qm", "big")
	testgit.WriteFile(b, dir, "big.go", "package a\nfunc f() {\n"+cur.String()+"}\n")
	m.reload()
	m.pickFile(slices.IndexFunc(m.files, func(f changedFile) bool { return f.Path == "big.go" }))
	return m
}
