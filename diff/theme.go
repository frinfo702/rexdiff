package diff

import (
	"image/color"

	"github.com/rednafi/rexdiff/internal/diffview"
)

// Theme sets the colours of the output. The lightness of Background selects
// the dark or the light diff colours.
type Theme struct {
	// Background and Foreground are the colours of the terminal. They are nil
	// if the terminal does not report them. Then the diff tints no background.
	Background, Foreground color.Color
	// Green and Red are the ANSI colours 2 and 1 of the terminal. Inserted and
	// deleted lines get a tint toward them. If they are nil, the diff uses
	// fixed colours.
	Green, Red color.Color
}

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

// apply sets the colours of t on dv. The syntax colours come from the
// terminal palette of 16 colours.
func (t Theme) apply(dv *diffview.View) *diffview.View {
	return dv.Style(diffview.TerminalStyle(t.Background, t.Foreground, t.Green, t.Red, t.dark()))
}
