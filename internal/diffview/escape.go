package diffview

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// The Unicode Control Pictures that Escape shows in place of a control
// character.
const (
	// The picture of U+0000. The pictures of U+0001 to U+001F follow it.
	controlPictures = '\u2400'
	delPicture      = '\u2421'
)

// Escape replaces each control character with its Unicode Control Picture,
// so the terminal shows it and does not run it. A C1 control becomes
// U+FFFD, because it has no picture.
func Escape(content string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 0 && r <= 0x1f: // C0 controls
			return controlPictures + r
		case r == ansi.DEL:
			return delPicture
		case r >= 0x80 && r <= 0x9f: // C1 controls
			return utf8.RuneError
		}
		return r
	}, content)
}
