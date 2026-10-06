// Package diffview draws a diff in the terminal. It is vendored from crush and
// changed for rexdiff. See ../VENDORED.md.
package diffview

import (
	"fmt"
	"image/color"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/aymanbagabas/go-udiff"
	"github.com/charmbracelet/x/ansi"
)

const (
	leadingSymbolsSize = 2 // the width of a symbol, such as symInsert
	lineNumPadding     = 1 // the space on each side of a line number
	defaultTabWidth    = 8
	ellipsis           = "…" // shows where a line is cut
)

// The symbols before the code of a line.
const (
	symNone   = "  " // an equal line, a missing line, or a continued line
	symInsert = "+ "
	symDelete = "- "
)

type file struct {
	path    string
	content string
}

type layout int

const (
	layoutUnified layout = iota + 1
	layoutSplit
)

// DiffView draws the difference between two versions of a file.
type DiffView struct {
	layout      layout
	before      file
	after       file
	width       int
	xOffset     int
	style       Style
	tabWidth    int
	chromaStyle *chroma.Style
	ansiSyntax  bool // highlight with the terminal palette
	wrap        bool // wrap long lines instead of cutting them

	isComputed bool
	err        error
	unified    udiff.UnifiedDiff

	splitHunks []splitHunk

	codeWidth       int
	fullCodeWidth   int  // with the leading symbols
	extraColOnAfter bool // in the split layout, the new side is one cell wider
	beforeNumDigits int
	afterNumDigits  int
	heights         []int // the screen lines of each row, for Rows

	// The lexer is cached, because to match a file name is slow.
	cachedLexer chroma.Lexer

	// syntaxCache keeps the highlighted lines, so that equal lines are
	// highlighted once.
	syntaxCache map[syntaxKey]string
}

// New returns a DiffView with the unified layout and a tab width of 8.
func New() *DiffView {
	return &DiffView{
		layout:      layoutUnified,
		tabWidth:    defaultTabWidth,
		syntaxCache: make(map[syntaxKey]string),
	}
}

// Split sets the split layout: the old lines on the left, the new lines on
// the right.
func (dv *DiffView) Split() *DiffView {
	dv.layout = layoutSplit
	return dv
}

// Before sets the old version of the file.
func (dv *DiffView) Before(path, content string) *DiffView {
	dv.before = file{path: path, content: content}
	dv.clearCaches()
	return dv
}

// After sets the new version of the file.
func (dv *DiffView) After(path, content string) *DiffView {
	dv.after = file{path: path, content: content}
	dv.clearCaches()
	return dv
}

// clearCaches clears the caches when the content changes.
func (dv *DiffView) clearCaches() {
	dv.cachedLexer = nil
	clear(dv.syntaxCache)
	dv.isComputed = false
}

// Style sets the colours of the lines.
func (dv *DiffView) Style(style Style) *DiffView {
	dv.style = style
	return dv
}

// Width sets the maximum width of the output, in cells. Zero means no limit.
func (dv *DiffView) Width(width int) *DiffView {
	dv.width = width
	return dv
}

// XOffset sets the number of cells to scroll the code to the right.
func (dv *DiffView) XOffset(xOffset int) *DiffView {
	dv.xOffset = xOffset
	return dv
}

// TabWidth sets the number of spaces that replace a tab.
func (dv *DiffView) TabWidth(tabWidth int) *DiffView {
	dv.tabWidth = tabWidth
	return dv
}

// ChromaStyle sets the chroma style of the syntax colours. If style is nil
// and ANSISyntax is not set, the code has no syntax colours.
func (dv *DiffView) ChromaStyle(style *chroma.Style) *DiffView {
	dv.chromaStyle = style
	clear(dv.syntaxCache)
	return dv
}

// String returns the rendered diff.
func (dv *DiffView) String() string {
	dv.heights = dv.heights[:0]
	dv.normalizeLineEndings()
	if err := dv.computeDiff(); err != nil {
		return err.Error()
	}
	dv.replaceTabs()
	dv.convertDiffToSplit()
	dv.adjustStyles()
	dv.detectNumDigits()

	if dv.width <= 0 {
		dv.detectCodeWidth()
	} else {
		dv.resizeCodeWidth()
	}

	style := lipgloss.NewStyle()
	if dv.width > 0 {
		style = style.MaxWidth(dv.width)
	}
	if dv.layout == layoutSplit {
		return style.Render(strings.TrimSuffix(dv.renderSplit(), "\n"))
	}
	return style.Render(strings.TrimSuffix(dv.renderUnified(), "\n"))
}

// normalizeLineEndings changes CRLF line endings to LF.
func (dv *DiffView) normalizeLineEndings() {
	dv.before.content = strings.ReplaceAll(dv.before.content, "\r\n", "\n")
	dv.after.content = strings.ReplaceAll(dv.after.content, "\r\n", "\n")
}

// replaceTabs replaces the tabs in the lines of the diff with spaces. It runs
// after computeDiff, so that a change from a tab to spaces is a change.
func (dv *DiffView) replaceTabs() {
	spaces := strings.Repeat(" ", dv.tabWidth)
	for _, h := range dv.unified.Hunks {
		for i := range h.Lines {
			h.Lines[i].Content = strings.ReplaceAll(h.Lines[i].Content, "\t", spaces)
		}
	}
}

// computeDiff computes the difference between the two versions.
func (dv *DiffView) computeDiff() error {
	if dv.isComputed {
		return dv.err
	}
	dv.isComputed = true
	edits := udiff.Lines(dv.before.content, dv.after.content)
	dv.unified, dv.err = udiff.ToUnifiedDiff(
		dv.before.path,
		dv.after.path,
		dv.before.content,
		edits,
		udiff.DefaultContextLines,
	)
	return dv.err
}

// convertDiffToSplit makes the split hunks, if the layout is split.
func (dv *DiffView) convertDiffToSplit() {
	if dv.layout != layoutSplit {
		return
	}

	dv.splitHunks = make([]splitHunk, len(dv.unified.Hunks))
	for i, h := range dv.unified.Hunks {
		dv.splitHunks[i] = hunkToSplit(h)
	}
}

// adjustStyles adds padding and alignment to the line number styles.
func (dv *DiffView) adjustStyles() {
	setPadding := func(s lipgloss.Style) lipgloss.Style {
		return s.Padding(0, lineNumPadding).Align(lipgloss.Right)
	}
	dv.style.missingLine.lineNumber = setPadding(dv.style.missingLine.lineNumber)
	dv.style.dividerLine.lineNumber = setPadding(dv.style.dividerLine.lineNumber)
	dv.style.equalLine.lineNumber = setPadding(dv.style.equalLine.lineNumber)
	dv.style.insertLine.lineNumber = setPadding(dv.style.insertLine.lineNumber)
	dv.style.deleteLine.lineNumber = setPadding(dv.style.deleteLine.lineNumber)
}

// detectNumDigits finds the number of digits of the largest old and new line
// numbers.
func (dv *DiffView) detectNumDigits() {
	dv.beforeNumDigits = 0
	dv.afterNumDigits = 0

	for _, h := range dv.unified.Hunks {
		before := len(strconv.Itoa(h.FromLine + len(h.Lines)))
		after := len(strconv.Itoa(h.ToLine + len(h.Lines)))
		dv.beforeNumDigits = max(dv.beforeNumDigits, before)
		dv.afterNumDigits = max(dv.afterNumDigits, after)
	}
}

// detectCodeWidth finds the width of the widest code line.
func (dv *DiffView) detectCodeWidth() {
	switch dv.layout {
	case layoutUnified:
		dv.detectUnifiedCodeWidth()
	case layoutSplit:
		dv.detectSplitCodeWidth()
	}
	dv.fullCodeWidth = dv.codeWidth + leadingSymbolsSize
}

// codeWidthOf returns the width of the code of l, plus one cell.
func codeWidthOf(l *udiff.Line) int {
	return ansi.StringWidth(Escape(strings.TrimSuffix(l.Content, "\n"))) + 1
}

// detectUnifiedCodeWidth finds the width of the widest code line or hunk
// header in the unified layout.
func (dv *DiffView) detectUnifiedCodeWidth() {
	dv.codeWidth = 0
	for _, h := range dv.unified.Hunks {
		headerWidth := ansi.StringWidth(hunkHeader(h))
		for _, l := range h.Lines {
			dv.codeWidth = max(dv.codeWidth, codeWidthOf(&l), headerWidth)
		}
	}
}

// detectSplitCodeWidth finds the width of the widest code line or hunk
// header in the split layout.
func (dv *DiffView) detectSplitCodeWidth() {
	dv.codeWidth = 0
	for i, h := range dv.splitHunks {
		headerWidth := ansi.StringWidth(hunkHeader(dv.unified.Hunks[i]))
		for _, l := range h.lines {
			if l.before != nil {
				dv.codeWidth = max(dv.codeWidth, codeWidthOf(l.before), headerWidth)
			}
			if l.after != nil {
				dv.codeWidth = max(dv.codeWidth, codeWidthOf(l.after), headerWidth)
			}
		}
	}
}

// resizeCodeWidth sets the code width so that the lines fit in the width.
func (dv *DiffView) resizeCodeWidth() {
	fullNumWidth := dv.beforeNumDigits + dv.afterNumDigits
	fullNumWidth += lineNumPadding * 4 // left and right padding for both line numbers

	switch dv.layout {
	case layoutUnified:
		dv.codeWidth = dv.width - fullNumWidth - leadingSymbolsSize
	case layoutSplit:
		remainingWidth := dv.width - fullNumWidth - leadingSymbolsSize*2
		dv.codeWidth = remainingWidth / 2
		dv.extraColOnAfter = remainingWidth%2 != 0
	}

	dv.fullCodeWidth = dv.codeWidth + leadingSymbolsSize
}

// renderUnified renders the unified layout: the old and the new line
// numbers, then the code.
func (dv *DiffView) renderUnified() string {
	var b strings.Builder
	nums := func(ls lineStyle, before, after any) string {
		return dv.lineNum(ls, before, dv.beforeNumDigits) +
			dv.lineNum(ls, after, dv.afterNumDigits)
	}
	for _, h := range dv.unified.Hunks {
		b.WriteString(dv.unifiedHeader(h) + "\n")
		oldN, newN := h.FromLine, h.ToLine
		for _, l := range h.Lines {
			ls, sym := dv.kindStyle(l.Kind)
			var num string
			switch l.Kind {
			case udiff.Equal:
				num = nums(ls, oldN, newN)
				oldN++
				newN++
			case udiff.Insert:
				num = nums(ls, " ", newN)
				newN++
			case udiff.Delete:
				num = nums(ls, oldN, " ")
				oldN++
			}
			blank := nums(ls, " ", " ")
			dv.writeRow(&b, dv.codeLines(ls, num, blank, sym, l.Content, dv.codeWidth))
		}
	}
	return b.String()
}

// unifiedHeader renders the divider line of hunk h in the unified layout.
func (dv *DiffView) unifiedHeader(h *udiff.Hunk) string {
	div := dv.style.dividerLine
	header := ansi.Truncate(hunkHeader(h), dv.fullCodeWidth, ellipsis)
	return div.lineNumber.Render(pad(ellipsis, dv.beforeNumDigits)) +
		div.lineNumber.Render(pad(ellipsis, dv.afterNumDigits)) +
		div.code.Width(dv.fullCodeWidth).Render(header)
}

// renderSplit renders the split layout: the old lines on the left, the new
// lines on the right.
func (dv *DiffView) renderSplit() string {
	var b strings.Builder
	afterWidth := dv.codeWidth + btoi(dv.extraColOnAfter)
	for i, h := range dv.splitHunks {
		b.WriteString(dv.splitHeader(dv.unified.Hunks[i]) + "\n")
		oldN, newN := h.fromLine, h.toLine
		for _, l := range h.lines {
			left, leftFill := dv.splitSide(l.before, oldN, dv.beforeNumDigits, dv.codeWidth)
			right, rightFill := dv.splitSide(l.after, newN, dv.afterNumDigits, afterWidth)
			if l.before != nil {
				oldN++
			}
			if l.after != nil {
				newN++
			}
			n := max(len(left), len(right))
			left, right = fillTo(left, n, leftFill), fillTo(right, n, rightFill)
			lines := make([]string, n)
			for k := range n {
				lines[k] = left[k] + right[k]
			}
			dv.writeRow(&b, lines)
		}
	}
	return b.String()
}

// splitHeader renders the divider line of hunk h in the split layout. The
// header shows on the left side only.
func (dv *DiffView) splitHeader(h *udiff.Hunk) string {
	div := dv.style.dividerLine
	header := ansi.Truncate(hunkHeader(h), dv.fullCodeWidth, ellipsis)
	afterWidth := dv.fullCodeWidth + btoi(dv.extraColOnAfter)
	return div.lineNumber.Render(pad(ellipsis, dv.beforeNumDigits)) +
		div.code.Width(dv.fullCodeWidth).Render(header) +
		div.lineNumber.Render(pad(ellipsis, dv.afterNumDigits)) +
		div.code.Width(afterWidth).Render(" ")
}

// splitSide renders line l on one side of the split layout, one string for
// each screen line. l is nil if that side has no line, and n is its line
// number. The fill func renders an empty screen line, for when the other
// side has more screen lines.
func (dv *DiffView) splitSide(
	l *udiff.Line,
	n, digits, width int,
) (lines []string, fill func() string) {
	ls, sym, content := dv.style.missingLine, symNone, ""
	if l != nil {
		ls, sym = dv.kindStyle(l.Kind)
		content = l.Content
	}
	noNum := dv.lineNum(ls, " ", digits)
	num := noNum
	if l != nil {
		num = dv.lineNum(ls, n, digits)
	}
	fill = func() string { return dv.codeLines(ls, noNum, "", symNone, "", width)[0] }
	return dv.codeLines(ls, num, noNum, sym, content, width), fill
}

// fillTo adds the line that fill renders to lines, until lines has n lines.
func fillTo(lines []string, n int, fill func() string) []string {
	if len(lines) >= n {
		return lines
	}
	return append(lines, slices.Repeat([]string{fill()}, n-len(lines))...)
}

// kindStyle returns the style and the symbol of a line of kind k.
func (dv *DiffView) kindStyle(k udiff.OpKind) (lineStyle, string) {
	switch k {
	case udiff.Insert:
		return dv.style.insertLine, symInsert
	case udiff.Delete:
		return dv.style.deleteLine, symDelete
	}
	return dv.style.equalLine, symNone
}

// writeRow writes the screen lines of one row to b, and keeps their number
// for Rows.
func (dv *DiffView) writeRow(b *strings.Builder, lines []string) {
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	dv.heights = append(dv.heights, len(lines))
}

// lineNum renders the line number v in a column of digits cells.
func (dv *DiffView) lineNum(ls lineStyle, v any, digits int) string {
	return ls.lineNumber.Render(pad(v, digits))
}

// codeLines renders one source line: nums, sym, and the code in width
// cells. It cuts a long line. With wrap, the line continues on more screen
// lines, which show blank in place of nums and no sym.
func (dv *DiffView) codeLines(
	ls lineStyle,
	nums, blank, sym, content string,
	width int,
) []string {
	code := dv.highlightCode(strings.TrimSuffix(content, "\n"), ls.code.GetBackground())
	parts := []string{code}
	if dv.wrap {
		// Wrap can keep a leading space on a full line, so cut again.
		w := max(width, 1)
		parts = strings.Split(ansi.Hardwrap(lipgloss.Wrap(code, w, ""), w, true), "\n")
		for i, p := range parts {
			parts[i] = ansi.Truncate(p, w, "") // a wide character can be wider than w
		}
	} else {
		code = ansi.GraphemeWidth.Cut(code, dv.xOffset, len(code))
		parts[0] = ansi.Truncate(code, width, ellipsis)
		if dv.xOffset > 0 && strings.TrimSpace(parts[0]) != "" {
			sym = sym[:1] + ellipsis
		}
	}
	full := lipgloss.NewStyle().MaxWidth(width + leadingSymbolsSize)
	lines := make([]string, len(parts))
	for i, p := range parts {
		if i > 0 {
			nums, sym = blank, symNone
		}
		lines[i] = nums + full.Render(ls.symbol.Render(sym)+ls.code.Width(width).Render(p))
	}
	return lines
}

// hunkHeader returns the header line of hunk h, for example
// "  @@ -1,3 +1,4 @@ ".
func hunkHeader(h *udiff.Hunk) string {
	before, after := hunkLineCounts(h)
	return fmt.Sprintf("  @@ -%d,%d +%d,%d @@ ", h.FromLine, before, h.ToLine, after)
}

// hunkLineCounts returns the number of old and new lines in hunk h.
func hunkLineCounts(h *udiff.Hunk) (before, after int) {
	for _, l := range h.Lines {
		switch l.Kind {
		case udiff.Equal:
			before++
			after++
		case udiff.Insert:
			after++
		case udiff.Delete:
			before++
		}
	}
	return before, after
}

// highlightCode returns source with syntax colours on the background
// bgColor. The syntax cache keeps the result, so equal lines are
// highlighted once. If highlighting fails, it returns source as it is.
func (dv *DiffView) highlightCode(source string, bgColor color.Color) string {
	// Chroma makes a lone CR a line break, so show it as a symbol first.
	source = strings.ReplaceAll(source, "\r", "\u240d")
	if dv.chromaStyle == nil && !dv.ansiSyntax {
		return source
	}
	key := newSyntaxKey(source, bgColor)
	if cached, ok := dv.syntaxCache[key]; ok {
		return cached
	}
	it, err := dv.chromaLexer().Tokenise(nil, source)
	if err != nil {
		return source
	}
	var b strings.Builder
	if err := dv.chromaFormatter(bgColor).Format(&b, dv.chromaStyle, it); err != nil {
		return source
	}
	out := b.String()
	dv.syntaxCache[key] = out
	return out
}

// syntaxKey is the key of the syntax cache: the source and its background
// colour. noColor is true for no background, because lipgloss.NoColor has
// the same RGBA value as black.
type syntaxKey struct {
	source     string
	noColor    bool
	r, g, b, a uint32
}

// newSyntaxKey returns the cache key of source on the background bgColor.
func newSyntaxKey(source string, bgColor color.Color) syntaxKey {
	k := syntaxKey{source: source}
	if _, none := bgColor.(lipgloss.NoColor); bgColor == nil || none {
		k.noColor = true
		return k
	}
	k.r, k.g, k.b, k.a = bgColor.RGBA()
	return k
}

// chromaLexer returns the lexer of the old file, and keeps it.
func (dv *DiffView) chromaLexer() chroma.Lexer {
	if dv.cachedLexer == nil {
		dv.cachedLexer = Lexer(dv.before.path, dv.before.content)
	}
	return dv.cachedLexer
}

// Lexer returns the coalesced lexer for the file at path. If no lexer
// matches the name, it selects one from the content, else the fallback.
func Lexer(path, content string) chroma.Lexer {
	// matchLexer is memoized and already coalesced.
	if l := matchLexer(path); l != nil {
		return l
	}
	l := lexers.Analyse(content)
	if l == nil {
		l = lexers.Fallback
	}
	return chroma.Coalesce(l)
}

// chromaFormatter returns the formatter that ANSISyntax selects.
func (dv *DiffView) chromaFormatter(bgColor color.Color) chroma.Formatter {
	if dv.ansiSyntax {
		return ANSIFormatter(bgColor, processChromaValue)
	}
	return Formatter(bgColor, processChromaValue)
}

// processChromaValue escapes the text of a token for the terminal. A code
// line has no line break, so it removes the trailing ones.
func processChromaValue(value string) string {
	return Escape(strings.TrimRight(value, "\n"))
}
