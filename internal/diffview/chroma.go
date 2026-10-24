package diffview

import (
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// lexerCache keeps the lexer of each file name, nil if none matches. To
// match a name is slow, because lexers.Match tests the patterns of each
// lexer. The mutex makes the cache safe for more than one goroutine.
var (
	lexerCacheMu sync.RWMutex
	lexerCache   = map[string]chroma.Lexer{}
)

// matchLexer returns the coalesced chroma lexer for the file name, or nil if
// no lexer matches. The lexer is safe to use again.
func matchLexer(fileName string) chroma.Lexer {
	lexerCacheMu.RLock()
	l, ok := lexerCache[fileName]
	lexerCacheMu.RUnlock()
	if ok {
		return l
	}
	if l = lexers.Match(fileName); l != nil {
		l = chroma.Coalesce(l)
	}
	lexerCacheMu.Lock()
	lexerCache[fileName] = l
	lexerCacheMu.Unlock()
	return l
}

// renderLines styles each line of value alone, and joins them with plain
// newlines. A token can contain newlines, for example a chroma CommentSingle
// ends with one. Lip Gloss pads all lines of a multi-line string to one
// width. This adds spaces to the line after a comment.
func renderLines(s lipgloss.Style, value string) string {
	if !strings.Contains(value, "\n") {
		return s.Render(value)
	}
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = s.Render(line)
	}
	return strings.Join(lines, "\n")
}
