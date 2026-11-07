package diffview

import (
	"regexp"
	"strings"
)

// declaration matches a line that declares a function, a type, or a
// module in most languages, after optional modifiers such as "pub".
var declaration = regexp.MustCompile(`^\s*(?:(?:export|pub(?:\([a-z]+\))?|public|private|protected|internal|static|async|abstract|final|sealed|open|override|default|unsafe|inline|virtual|extern|data|const)\s+)*(?:def|class|func|fn|fun|function|struct|enum|union|trait|impl|interface|type|module|mod|namespace|object|record|protocol|extension|actor|macro|sub|proc)\b`)

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
			if declaration.MatchString(l) ||
				d == 0 && (c == '_' || c == '$' || 'a' <= c|0x20 && c|0x20 <= 'z') {
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
