package diffview

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// pad right-aligns v in a column of width cells.
func pad(v any, width int) string {
	s := fmt.Sprint(v)
	w := ansi.StringWidth(s)
	if w >= width {
		return s
	}
	return strings.Repeat(" ", width-w) + s
}

// btoi returns 1 if b is true, else 0.
func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// CutLeft removes the first n cells of s, which can have ANSI codes. If
// the cut is inside a wide character, a space takes the place of the rest
// of that character. Thus the text after the cut moves by n cells.
func CutLeft(s string, n int) string {
	want := max(ansi.StringWidth(s)-n, 0)
	cut := ansi.TruncateLeft(s, n, "")
	for ansi.StringWidth(cut) > want {
		n++
		cut = ansi.TruncateLeft(s, n, "")
	}
	return strings.Repeat(" ", want-ansi.StringWidth(cut)) + cut
}
