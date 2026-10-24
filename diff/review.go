package diff

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Review returns the comments as herdr-reviewr text for a coding agent. It
// writes the prefix, if not empty, then the comments in [Comments.All] order.
// Blank lines separate them. The text has no control characters. A comment is:
//
//	a.go:3-4
//	 x := 1
//	+y := 2
//	Merge these lines.
func (cs *Comments) Review(prefix string) string {
	var parts []string
	if p := strings.TrimRight(Plain(prefix), " \n\t"); p != "" {
		parts = append(parts, p)
	}
	for _, c := range cs.All() {
		parts = append(parts, format(c))
	}
	return strings.Join(parts, "\n\n")
}

// format returns c as text for [Comments.Review].
func format(c *Comment) string {
	var b strings.Builder
	b.WriteString(Plain(c.Location()))
	if c.Stale {
		b.WriteString(" (stale: the code has changed since)")
	}
	// The rows as the diff shows them, so that the agent reads a real diff.
	if len(c.snippet) > 0 {
		for _, l := range c.snippet {
			b.WriteString("\n" + Plain(l))
		}
	} else {
		mark := "+"
		if c.Side == SideOld {
			mark = "-"
		}
		for _, l := range c.code {
			b.WriteString("\n" + mark + Plain(l))
		}
	}
	var text []string
	for l := range strings.SplitSeq(Plain(c.Text), "\n") {
		if strings.TrimSpace(l) != "" {
			text = append(text, strings.TrimRight(l, " \t"))
		}
	}
	b.WriteString("\n" + strings.Join(text, "\n"))
	return b.String()
}

// Plain returns s without control characters. This removes the escape
// sequences too, but keeps tabs and newlines. Use it on text from other
// people before you show or send it.
func Plain(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, s)
}

// Quote returns s in Go quotes if it has control characters or is not valid
// UTF-8. git shows such a path the same way. Then s stays on one line and
// cannot change the terminal.
func Quote(s string) string {
	if strings.ContainsFunc(s, unicode.IsControl) || !utf8.ValidString(s) {
		return strconv.Quote(s)
	}
	return s
}

// snippet returns rows as unified diff lines. See [NewComment].
func snippet(rows []Row, old, new string) []string {
	oldLines, newLines := Lines(old), Lines(new)
	line := func(ls []string, n int) string {
		if n >= 1 && n-1 < len(ls) {
			return ls[n-1]
		}
		return ""
	}
	var out, minus, plus []string
	flush := func() {
		out = append(append(out, minus...), plus...)
		minus, plus = nil, nil
	}
	for _, r := range rows {
		if r.Continued {
			continue
		}
		switch r.Kind {
		case KindEqual:
			flush()
			out = append(out, " "+line(newLines, r.New))
		case KindInsert:
			plus = append(plus, "+"+line(newLines, r.New))
		case KindDelete:
			minus = append(minus, "-"+line(oldLines, r.Old))
		case KindChange:
			minus = append(minus, "-"+line(oldLines, r.Old))
			plus = append(plus, "+"+line(newLines, r.New))
		}
	}
	flush()
	return out
}
