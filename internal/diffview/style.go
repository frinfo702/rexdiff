package diffview

import (
	"charm.land/lipgloss/v2"
)

// lineStyle is the style of the parts of one type of line.
type lineStyle struct {
	lineNumber lipgloss.Style
	symbol     lipgloss.Style
	code       lipgloss.Style
}

// Style is the style of each type of line. Make one with [TerminalStyle].
type Style struct {
	dividerLine lineStyle
	missingLine lineStyle
	equalLine   lineStyle
	insertLine  lineStyle
	deleteLine  lineStyle
}
