package diffview

import (
	"strings"
)

// The words that start a declaration of a function, a type, or a module in
// most languages, and the modifiers that can come before them.
var (
	keywords  = wordSet("def class func fn fun function struct enum union trait impl interface type module mod namespace object record protocol extension actor macro sub proc")
	modifiers = wordSet("export pub public private protected internal static async abstract final sealed open override default unsafe inline virtual extern data const")
)

func wordSet(s string) map[string]bool {
	set := map[string]bool{}
	for _, w := range strings.Fields(s) {
		set[w] = true
	}
	return set
}

// space is the set of \s in a regular expression.
const space = " \t\n\f\r"

// isDeclaration reports whether l declares a function, a type, or a
// module: after indent and modifiers such as "pub" or "pub(crate)", the
// first word is a keyword such as "func". It matches the same lines as
// the regular expression
//
//	^\s*(?:(?:modifier|pub\([a-z]+\))\s+)*keyword\b
//
// but scopesOf calls it on each line of a file, and the regular expression
// took most of the time of a large diff.
func isDeclaration(l string) bool {
	s := strings.TrimLeft(l, space)
	for {
		n := 0
		for n < len(s) && isWordByte(s[n]) {
			n++
		}
		w, rest := s[:n], s[n:]
		if keywords[w] {
			return true
		}
		if !modifiers[w] {
			return false
		}
		if w == "pub" && strings.HasPrefix(rest, "(") {
			if i := strings.IndexByte(rest, ')'); i > 1 && strings.Trim(rest[1:i], "abcdefghijklmnopqrstuvwxyz") == "" {
				rest = rest[i+1:]
			}
		}
		s = strings.TrimLeft(rest, space)
		if len(s) == len(rest) { // a modifier needs space after it
			return false
		}
	}
}

// isWordByte reports whether b is in \w, the bytes of a word.
func isWordByte(b byte) bool {
	return b == '_' || '0' <= b && b <= '9' || 'a' <= b|0x20 && b|0x20 <= 'z'
}

// scopesOf returns the scope of each line in firsts (0-based, ascending):
// the line that encloses it, as "def foo(" or "func (v *View) text() string {",
// or "" if none does. The scope is the nearest line above with less indent
// that is a declaration, or that starts at column 0 with a word, as git's
// default funcname does. One pass over lines finds all the scopes.
// ponytail: a keyword heuristic, not a parser; it can miss a method in a
// language without keywords (Java, C++), or pick a multiline statement.
func scopesOf(lines []string, firsts []int) []string {
	indent := func(s string) int { return len(s) - len(strings.TrimLeft(s, " \t")) }
	type decl struct {
		indent int
		line   string
	}
	var stack []decl // the declarations above the current line, by growing indent
	scopes := make([]string, len(firsts))
	i := 0
	for k, first := range firsts {
		for ; i < min(first, len(lines)); i++ {
			l := strings.TrimRight(lines[i], "\r")
			if strings.TrimSpace(l) == "" {
				continue
			}
			d := indent(l)
			c := l[d]
			if d == 0 && (c == '_' || c == '$' || 'a' <= c|0x20 && c|0x20 <= 'z') ||
				isDeclaration(l) {
				for len(stack) > 0 && stack[len(stack)-1].indent >= d {
					stack = stack[:len(stack)-1]
				}
				stack = append(stack, decl{d, l})
			}
		}
		if first < 0 || first >= len(lines) {
			continue
		}
		target := indent(lines[first])
		for _, l := range lines[first:] { // the first line can be blank
			if strings.TrimSpace(l) != "" {
				target = indent(l)
				break
			}
		}
		for j := len(stack) - 1; j >= 0; j-- {
			if stack[j].indent < target {
				scopes[k] = Escape(strings.ReplaceAll(strings.TrimSpace(stack[j].line), "\t", " "))
				break
			}
		}
	}
	return scopes
}
