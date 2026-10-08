package diff

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rednafi/rexdiff/internal/diffview"
)

// lineNumberColor is the ANSI palette colour of the line numbers: bright
// black.
const lineNumberColor = "8"

// RenderFile shows src, the content of the file at path, with line numbers
// and syntax colours. Each row is a [KindEqual] row, with the same line
// number on the two sides. The line numbers count as the [SideNew] side.
func RenderFile(path, src string, o Options) *Diff {
	// Chroma makes a lone CR a line break, so show it as a symbol. Then the
	// lines are the same as [Lines] gives.
	src = strings.NewReplacer(
		"\r\n", "\n",
		"\r", "\u240d",
		"\t", strings.Repeat(" ", tabWidth),
	).Replace(src)
	hl := strings.Split(highlight(path, src, o.Theme), "\n")
	hl = hl[:min(len(hl), len(Lines(src)))] // chroma can add a line break at the end
	digits := len(fmt.Sprint(len(hl)))
	numWidth := digits + 2 // a space on each side of the number
	numStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(lineNumberColor))
	d := &Diff{newNum: numWidth}
	code := o.Width - numWidth - 1 // the width that the number and a space leave
	blank := strings.Repeat(" ", numWidth+1)
	for i, l := range hl {
		for k, p := range fileLineParts(l, code, o) {
			num := blank
			if k == 0 {
				num = numStyle.Render(fmt.Sprintf(" %*d ", digits, i+1)) + " "
			}
			line := num + p
			if o.Width > 0 { // the number or a wide character can be too wide
				line = ansi.Truncate(line, o.Width, "")
			}
			d.Lines = append(d.Lines, line)
			d.Rows = append(d.Rows, Row{
				Kind:      KindEqual,
				Old:       i + 1,
				New:       i + 1,
				Continued: k > 0,
			})
		}
	}
	return d
}

// fileLineParts returns the screen lines of the highlighted line l. If o has
// a width, each part fits in code cells. With [Options.Wrap], a long line
// gives more than one part. Else the line skips [Options.Offset] cells and
// is cut.
func fileLineParts(l string, code int, o Options) []string {
	switch {
	case o.Width > 0 && code <= 0:
		return []string{""}
	case o.Width > 0 && o.Wrap:
		// Wrap can keep a leading space on a full line, so cut again.
		wrapped := ansi.Hardwrap(lipgloss.Wrap(l, code, ""), code, true)
		return strings.Split(wrapped, "\n")
	case o.Width > 0:
		return []string{ansi.Truncate(ansi.TruncateLeft(l, o.Offset, ""), code, "…")}
	}
	return []string{ansi.TruncateLeft(l, o.Offset, "")}
}

// highlight colours the whole file at once, so that tokens that span lines
// get the correct colour. If it cannot highlight src, it returns src with
// its control characters replaced.
func highlight(path, src string, t Theme) string {
	it, err := diffview.Lexer(path, src).Tokenise(nil, src)
	if err != nil {
		return escapeKeepNewlines(src)
	}
	var f chroma.Formatter
	var st *chroma.Style
	if t.terminal() {
		f = diffview.ANSIFormatter(lipgloss.NoColor{}, escapeKeepNewlines)
	} else {
		f, st = diffview.Formatter(lipgloss.NoColor{}, escapeKeepNewlines), t.chroma()
	}
	var b strings.Builder
	if f.Format(&b, st, it) != nil {
		return escapeKeepNewlines(src)
	}
	return b.String()
}

// escapeKeepNewlines replaces the control characters in a token, but keeps
// its line breaks. The caller splits the output into lines.
func escapeKeepNewlines(s string) string {
	parts := strings.Split(s, "\n")
	for i, p := range parts {
		parts[i] = diffview.Escape(p)
	}
	return strings.Join(parts, "\n")
}
