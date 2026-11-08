package tui

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func BenchmarkFindTmp(b *testing.B) {
	m, dir := newTestModel(b)
	var src strings.Builder
	for i := range 20000 {
		src.WriteString("\tx := compute(" + strings.Repeat("a", i%40) + ")\n")
	}
	write(b, dir, "big.go", "package a\nfunc f() {\n"+src.String()+"}\n")
	m.reload()
	m.pickFile(1)
	m = step(b, m, tea.BackgroundColorMsg{Color: color.Black})
	keys := []tea.KeyPressMsg{{Code: '/', Text: "/"}}
	for _, r := range "comput" {
		keys = append(keys, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	keys = append(keys, tea.KeyPressMsg{Code: tea.KeyEscape})
	for b.Loop() {
		for _, k := range keys {
			next, _ := m.Update(k)
			m = *next.(*model)
			_ = m.View()
		}
	}
}

func BenchmarkPaintTmp(b *testing.B) {
	m, dir := newTestModel(b)
	var src strings.Builder
	for range 200 {
		src.WriteString("\tx := compute(a, c, c, c, c, c, c, c, c, c, c, c, c, c, c, c, c, c, c, c, c)\n")
	}
	write(b, dir, "big.go", "package a\nfunc f() {\n"+src.String()+"}\n")
	m.reload()
	m.pickFile(1)
	m = step(b, m, tea.BackgroundColorMsg{Color: color.Black})
	for _, s := range []string{"/", "c"} {
		next, _ := m.Update(tea.KeyPressMsg{Code: rune(s[0]), Text: s})
		m = *next.(*model)
	}
	for b.Loop() {
		_ = m.View()
	}
}
