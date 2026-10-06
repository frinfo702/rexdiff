package diffview

import (
	"image/color"
	"io"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
)

// The colours of the ANSI palette of the terminal, as Lip Gloss colours.
const (
	ansiRed     = "1"
	ansiGreen   = "2"
	ansiYellow  = "3"
	ansiBlue    = "4"
	ansiMagenta = "5"
	ansiCyan    = "6"
	ansiGrey    = "8" // bright black
)

// ansiFor returns the ANSI palette colour of a token type. If ok is false,
// the token has the default foreground of the terminal.
func ansiFor(t chroma.TokenType) (c string, italic, ok bool) {
	switch {
	case t.InCategory(chroma.Comment):
		return ansiGrey, true, true
	case t == chroma.KeywordType, t == chroma.NameBuiltin,
		t.InSubCategory(chroma.NameClass):
		return ansiCyan, false, true
	case t.InCategory(chroma.Keyword):
		return ansiMagenta, false, true
	case t.InSubCategory(chroma.LiteralString):
		return ansiGreen, false, true
	case t.InSubCategory(chroma.LiteralNumber), t == chroma.NameConstant:
		return ansiYellow, false, true
	case t == chroma.NameFunction, t == chroma.NameFunctionMagic,
		t == chroma.NameTag, t == chroma.NameAttribute:
		return ansiBlue, false, true
	}
	return "", false, false
}

// ANSIFormatter is like [Formatter], but it ignores the chroma style and uses
// the ANSI palette colours.
func ANSIFormatter(bgColor color.Color, processValue func(string) string) chroma.Formatter {
	format := func(w io.Writer, _ *chroma.Style, it chroma.Iterator) error {
		plain := lipgloss.NewStyle().Background(bgColor)
		styleOf := func(t chroma.TokenType) (lipgloss.Style, bool) {
			if c, italic, ok := ansiFor(t); ok {
				return plain.Foreground(lipgloss.Color(c)).Italic(italic), true
			}
			return plain, true
		}
		return writeTokens(w, it, processValue, styleOf)
	}
	return chroma.FormatterFunc(format)
}
