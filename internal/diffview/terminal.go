package diffview

import (
	"cmp"
	"image/color"

	"charm.land/lipgloss/v2"
)

// Signs marks an inserted line with + and a deleted line with -, in place
// of a bar.
func (v *View) Signs() *View {
	v.signs = true
	return v
}

// Wrap makes long lines continue on the next screen lines, instead of
// cutting them. The x offset does not apply.
func (v *View) Wrap() *View {
	v.wrap = true
	return v
}

// mix mixes a toward b by the amount t, from 0 to 1.
func mix(a, b color.Color, t float64) color.Color {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	// RGBA has 16 bits for each channel. Divide by 257 to get 8 bits.
	f := func(x, y uint32) uint8 { return uint8((float64(x)*(1-t) + float64(y)*t) / 257) }
	return color.RGBA{R: f(ar, br), G: f(ag, bg), B: f(ab, bb), A: 255}
}

// The amounts that TerminalStyle mixes into the background.
const (
	tintDark     = 0.16 // inserted and deleted lines on a dark background
	tintLight    = 0.13 // inserted and deleted lines on a light background
	tintNumber   = 1.7  // multiplies the tint for the numbers of those lines
	shadeDivider = 0.07 // the hunk headers, toward the foreground
	shadeMissing = 0.03 // the empty side of a split row, toward the foreground
)

// The tint colours when the terminal does not report its green or red.
const (
	fallbackGreen = "#2ea043"
	fallbackRed   = "#f85149"
)

// TerminalStyle makes a diff style from the terminal colours. These are the
// background, the foreground, and the ANSI green and red. The text colours
// are ANSI palette indices, so the terminal selects the real colours.
// TerminalStyle mixes the tints of inserted and deleted lines from the
// background toward green and red. If green or red is nil, it uses a fixed
// green or red. If bg is nil, it adds no tint.
func TerminalStyle(bg, fg, green, red color.Color, dark bool) Style {
	none := lipgloss.NoColor{}
	green = cmp.Or(green, lipgloss.Color(fallbackGreen))
	red = cmp.Or(red, lipgloss.Color(fallbackRed))
	amt := tintDark
	if !dark {
		amt = tintLight
	}
	tint := func(c color.Color, t float64) color.Color {
		if bg == nil {
			return none
		}
		return mix(bg, c, t)
	}
	subtle := func(t float64) color.Color {
		if bg == nil || fg == nil {
			return none
		}
		return mix(bg, fg, t)
	}
	plain := lipgloss.NewStyle()
	// changed is the style of an inserted or a deleted line, tinted toward c.
	changed := func(text string, c color.Color) lineStyle {
		fg := plain.Foreground(lipgloss.Color(text))
		return lineStyle{
			lineNumber: fg.Background(tint(c, amt*tintNumber)),
			symbol:     fg.Background(tint(c, amt)),
			code:       plain.Background(tint(c, amt)),
		}
	}
	divider := plain.Background(subtle(shadeDivider))
	missing := plain.Background(subtle(shadeMissing))
	return Style{
		dividerLine: lineStyle{
			lineNumber: divider.Foreground(lipgloss.Color(ansiGrey)),
			code:       divider.Foreground(lipgloss.Color(ansiCyan)),
		},
		missingLine: lineStyle{
			lineNumber: missing,
			symbol:     missing,
			code:       missing,
		},
		equalLine: lineStyle{
			lineNumber: plain.Foreground(lipgloss.Color(ansiGrey)),
			code:       plain,
		},
		insertLine: changed(ansiGreen, green),
		deleteLine: changed(ansiRed, red),
	}
}
