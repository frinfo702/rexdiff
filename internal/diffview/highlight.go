package diffview

import (
	"image/color"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// byName keeps the lexer of each file name, or nil if none matches, because
// lexers.Match tests the patterns of every lexer.
var byName sync.Map // string -> chroma.Lexer

// Lexer returns the coalesced lexer for the file at path. If no lexer
// matches the name, it selects one from content, and else the fallback.
func Lexer(path, content string) chroma.Lexer {
	v, ok := byName.Load(path)
	if !ok {
		var l chroma.Lexer
		if l = lexers.Match(path); l != nil {
			l = chroma.Coalesce(l)
		}
		v, _ = byName.LoadOrStore(path, l)
	}
	if l, _ := v.(chroma.Lexer); l != nil {
		return l
	}
	l := lexers.Analyse(content)
	if l == nil {
		l = lexers.Fallback
	}
	return chroma.Coalesce(l)
}

// highlighter colours single code lines, and keeps each result, so that a
// line that shows twice is coloured once.
type highlighter struct {
	lexer chroma.Lexer
	done  map[hlKey]string
}

// hlKey is a line on a background. The background is an interface, so that
// no background (lipgloss.NoColor) differs from black.
type hlKey struct {
	text string
	bg   color.Color
}

func newHighlighter(path, content string) highlighter {
	return highlighter{lexer: Lexer(path, content), done: map[hlKey]string{}}
}

// line returns text with syntax colours on the background bg. If chroma
// fails, it returns the text with its control characters escaped.
func (h highlighter) line(text string, bg color.Color) string {
	if text == "" {
		return ""
	}
	k := hlKey{text, bg}
	if s, ok := h.done[k]; ok {
		return s
	}
	// Chroma makes a lone CR a line break.
	src := strings.ReplaceAll(text, "\r", "␍")
	out := Escape(src)
	if it, err := h.lexer.Tokenise(nil, src); err == nil {
		var b strings.Builder
		f := ANSIFormatter(bg, func(v string) string { return Escape(strings.TrimRight(v, "\n")) })
		if f.Format(&b, nil, it) == nil {
			out = b.String()
		}
	}
	h.done[k] = out
	return out
}

// Escape replaces each control character with its Unicode Control Picture,
// so that the terminal shows it and does not run it. A C1 control, which has
// no picture, becomes U+FFFD.
func Escape(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r < 0x20:
			return 0x2400 + r
		case r == 0x7f:
			return 0x2421
		case r >= 0x80 && r < 0xa0:
			return utf8.RuneError
		}
		return r
	}, s)
}
