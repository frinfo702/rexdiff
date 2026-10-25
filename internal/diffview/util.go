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
