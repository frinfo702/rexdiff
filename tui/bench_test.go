package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// BenchmarkReview reloads a large change and scrolls through it, as a user
// does. Use it to profile the program:
//
//	go test ./tui -run '^$' -bench Review -cpuprofile cpu.out -memprofile mem.out
func BenchmarkReview(b *testing.B) {
	m, dir := newTestModel(b)
	// 20 new files of 2,000 lines each.
	var src strings.Builder
	for i := range 2000 {
		src.WriteString("\tx := compute(" + strings.Repeat("a", i%40) + ")\n")
	}
	content := "package a\nfunc f() {\n" + src.String() + "}\n"
	for i := range 20 {
		write(b, dir, "big"+string(rune('a'+i))+".go", content)
	}
	// Show a large file: reload keeps the selected file.
	m.reload()
	m.pickFile(1)
	down := tea.KeyPressMsg{Code: 'j', Text: "j"}
	for b.Loop() {
		m.reload()
		for range 100 {
			next, _ := m.Update(down)
			m = *next.(*model)
			_ = m.View()
		}
	}
}
