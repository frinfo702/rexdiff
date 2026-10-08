package diff

import (
	"image/color"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/rednafi/rexdiff/internal/diffview"
)

// Theme sets the colours of the output. The lightness of Background selects
// the dark or the light diff colours.
type Theme struct {
	// Background and Foreground are the colours of the terminal. They are nil
	// if the terminal does not report them. Then no background is tinted.
	Background, Foreground color.Color
	// Green and Red are the ANSI colours 2 and 1 of the terminal. Inserted and
	// deleted lines are tinted toward them. If they are nil, fixed colours
	// are used.
	Green, Red color.Color
	// Syntax is the name of the chroma style of the syntax colours, for
	// example "monokai". "" uses the terminal palette of 16 colours.
	Syntax string
}

// terminal reports whether the syntax colours come from the terminal palette.
func (t Theme) terminal() bool { return t.Syntax == "" }

// dark reports whether the background is dark. An unknown background counts
// as dark.
func (t Theme) dark() bool {
	if t.Background == nil {
		return true
	}
	r, g, b, _ := t.Background.RGBA()
	hi, lo := max(r, g, b), min(r, g, b)
	// The HSL lightness, from 0 to 1.
	return float64(hi+lo)/2/0xffff < 0.5
}

// chroma returns the chroma style that Syntax names.
func (t Theme) chroma() *chroma.Style { return styles.Get(t.Syntax) }

// apply sets the colours of t on dv.
func (t Theme) apply(dv *diffview.DiffView) *diffview.DiffView {
	style := diffview.TerminalStyle(t.Background, t.Foreground, t.Green, t.Red, t.dark())
	dv = dv.Style(style)
	if t.terminal() {
		return dv.ANSISyntax()
	}
	return dv.ChromaStyle(t.chroma())
}
