package diffview

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// The Unicode Control Pictures that Escape shows in place of a control
// character.
const (
	controlPictures = '\u2400' // the picture of U+0000; U+0001 to U+001F follow it
	delPicture      = '\u2421'
)

// Escape replaces each control character with its Unicode Control Picture,
// so the terminal shows it and does not run it. A C1 control becomes
// U+FFFD, because it has no picture.
func Escape(content string) string {
	var sb strings.Builder
	sb.Grow(len(content))
	for _, r := range content {
		switch {
		case r >= 0 && r <= 0x1f: // C0 controls
			sb.WriteRune(controlPictures + r)
		case r == ansi.DEL:
			sb.WriteRune(delPicture)
		case r >= 0x80 && r <= 0x9f: // C1 controls
			sb.WriteRune(utf8.RuneError)
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
