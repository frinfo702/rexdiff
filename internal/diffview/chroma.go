package diffview

import (
	"image/color"
	"io"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// lexerCache keeps the lexer of each file name, nil if none matches. To
// match a name is slow, because lexers.Match tests the patterns of each
// lexer. The mutex makes the cache safe for more than one goroutine.
var (
	lexerCacheMu sync.RWMutex
	lexerCache   = map[string]chroma.Lexer{}
)

// matchLexer returns the coalesced chroma lexer for the file name, or nil if
// no lexer matches. The lexer is safe to use again.
func matchLexer(fileName string) chroma.Lexer {
	lexerCacheMu.RLock()
	l, ok := lexerCache[fileName]
	lexerCacheMu.RUnlock()
	if ok {
		return l
	}
	if l = lexers.Match(fileName); l != nil {
		l = chroma.Coalesce(l)
	}
	lexerCacheMu.Lock()
	lexerCache[fileName] = l
	lexerCacheMu.Unlock()
	return l
}

// Formatter returns a chroma formatter that colours the tokens with the
// chroma style, on the background bgColor. If processValue is not nil, it
// changes the text of each token first.
func Formatter(bgColor color.Color, processValue func(string) string) chroma.Formatter {
	format := func(w io.Writer, style *chroma.Style, it chroma.Iterator) error {
		styleOf := func(t chroma.TokenType) (lipgloss.Style, bool) {
			return entryStyle(style.Get(t), bgColor)
		}
		return writeTokens(w, it, processValue, styleOf)
	}
	return chroma.FormatterFunc(format)
}

// entryStyle returns the style of a chroma style entry, on the background
// bgColor. It returns false if the entry is empty.
func entryStyle(entry chroma.StyleEntry, bgColor color.Color) (lipgloss.Style, bool) {
	if entry.IsZero() {
		return lipgloss.Style{}, false
	}
	s := lipgloss.NewStyle().Background(bgColor)
	if entry.Bold == chroma.Yes {
		s = s.Bold(true)
	}
	if entry.Underline == chroma.Yes {
		s = s.Underline(true)
	}
	if entry.Italic == chroma.Yes {
		s = s.Italic(true)
	}
	if entry.Colour.IsSet() {
		s = s.Foreground(lipgloss.Color(entry.Colour.String()))
	}
	return s, true
}

// writeTokens writes the tokens of it to w. If processValue is not nil, it
// changes the text of each token first. styleOf returns the style of a
// token type, or false to write the token with no style.
func writeTokens(
	w io.Writer,
	it chroma.Iterator,
	processValue func(string) string,
	styleOf func(chroma.TokenType) (lipgloss.Style, bool),
) error {
	for token := it(); token != chroma.EOF; token = it() {
		value := token.Value
		if processValue != nil {
			value = processValue(value)
		}
		if s, ok := styleOf(token.Type); ok {
			value = renderLines(s, value)
		}
		if _, err := io.WriteString(w, value); err != nil {
			return err
		}
	}
	return nil
}

// renderLines styles each line of value alone, and joins them with plain
// newlines. A token can contain newlines, for example a chroma CommentSingle
// ends with one. Lip Gloss pads all lines of a multi-line string to one
// width. This adds spaces to the line after a comment.
func renderLines(s lipgloss.Style, value string) string {
	if !strings.Contains(value, "\n") {
		return s.Render(value)
	}
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = s.Render(line)
	}
	return strings.Join(lines, "\n")
}
