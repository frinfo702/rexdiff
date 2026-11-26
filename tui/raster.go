package tui

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"sync"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// The styles of a font, the index of fontFiles and of a chain.
const (
	styleRegular = iota
	styleBold
	styleItalic
	styleBoldItalic
)

// fontFiles are the monospace fonts for each style, the first that exists
// wins. SF Mono comes with Terminal on macOS.
var fontFiles = [4][]string{
	{"/System/Applications/Utilities/Terminal.app/Contents/Resources/Fonts/SF-Mono-Regular.otf",
		"/System/Library/Fonts/Menlo.ttc",
		"/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
		"/usr/share/fonts/TTF/DejaVuSansMono.ttf"},
	{"/System/Applications/Utilities/Terminal.app/Contents/Resources/Fonts/SF-Mono-Bold.otf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf",
		"/usr/share/fonts/TTF/DejaVuSansMono-Bold.ttf"},
	{"/System/Applications/Utilities/Terminal.app/Contents/Resources/Fonts/SF-Mono-RegularItalic.otf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Oblique.ttf",
		"/usr/share/fonts/TTF/DejaVuSansMono-Oblique.ttf"},
	{"/System/Applications/Utilities/Terminal.app/Contents/Resources/Fonts/SF-Mono-BoldItalic.otf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSansMono-BoldOblique.ttf",
		"/usr/share/fonts/TTF/DejaVuSansMono-BoldOblique.ttf"},
}

// fallbackFiles have the glyphs that the monospace fonts lack: symbols, and
// Japanese and Chinese text.
var fallbackFiles = []string{
	"/System/Library/Fonts/Menlo.ttc",
	"/System/Library/Fonts/ヒラギノ角ゴシック W3.ttc",
	"/System/Library/Fonts/Apple Symbols.ttf",
	"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
}

// goFonts are the last fonts of each chain. They are in the binary.
var goFonts = [4][]byte{gomono.TTF, gomonobold.TTF, gomonoitalic.TTF, gomonobolditalic.TTF}

// fontChains are the fonts of each style, in the order of the search for a
// glyph. They load once, on the first frame.
var fontChains = sync.OnceValue(func() [4][]*sfnt.Font {
	var fallbacks []*sfnt.Font
	for _, p := range fallbackFiles {
		if f := loadFontFile(p); f != nil {
			fallbacks = append(fallbacks, f)
		}
	}
	var chains [4][]*sfnt.Font
	for s := range chains {
		for _, p := range fontFiles[s] {
			if f := loadFontFile(p); f != nil {
				chains[s] = append(chains[s], f)
				break
			}
		}
		chains[s] = append(chains[s], fallbacks...)
		f, _ := opentype.Parse(goFonts[s]) // the Go fonts parse
		chains[s] = append(chains[s], f)
	}
	return chains
})

// loadFontFile returns the font in the file at p, the first font of a
// collection, or nil.
func loadFontFile(p string) *sfnt.Font {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	if f, err := opentype.Parse(data); err == nil {
		return f
	}
	c, err := opentype.ParseCollection(data)
	if err != nil {
		return nil
	}
	f, err := c.Font(0)
	if err != nil {
		return nil
	}
	return f
}

// glyphKey names the picture of a cell's text.
type glyphKey struct {
	text  string
	style int
	width int // in cells
}

// raster draws lines of cells in pixels. The cells are cw by ch pixels.
type raster struct {
	cw, ch   int
	baseline int // from the top of a cell
	thin     int // the width of a line of a box
	chains   [4][]*sfnt.Font
	faces    [4][]font.Face
	buf      sfnt.Buffer

	mu     sync.Mutex // guards the faces, the glyphs, and the logos, which rows share
	glyphs map[glyphKey]*image.Alpha
	logos  map[int]*image.RGBA // the mask of the logo of each image ID, logoCols cells wide
	lines  sync.Pool           // *image.RGBA of a line, for the next line
}

// newRaster makes a raster for cells of cw by ch pixels. The font size
// makes the advance of the monospace font one cell, as in a terminal.
func newRaster(cw, ch int) *raster {
	r := &raster{cw: cw, ch: ch, chains: fontChains(), glyphs: map[glyphKey]*image.Alpha{}}
	r.thin = max(1, int(math.Round(float64(ch)/17)))
	mono := r.chains[styleRegular][0]
	upem := fixed.I(int(mono.UnitsPerEm()))
	adv := 0.6 // of an em, if the font has no zero
	if i, err := mono.GlyphIndex(&r.buf, '0'); err == nil && i != 0 {
		if a, err := mono.GlyphAdvance(&r.buf, i, upem, font.HintingNone); err == nil {
			adv = float64(a) / float64(upem)
		}
	}
	m, _ := mono.Metrics(&r.buf, upem, font.HintingNone)
	asc, desc := float64(m.Ascent)/float64(upem), float64(m.Descent)/float64(upem)
	size := min(float64(cw)/adv, float64(ch)/max(asc+desc, 1))
	r.baseline = int(math.Round((float64(ch)-(asc+desc)*size)/2 + asc*size))
	for s, chain := range r.chains {
		for _, f := range chain {
			face, _ := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
			r.faces[s] = append(r.faces[s], face)
		}
	}
	return r
}

// palette resolves the colours of cells.
type palette struct {
	fg, bg color.RGBA
	ansi   [16]color.RGBA
}

// rgba returns c in the colours of the terminal, or def if c is nil.
func (p *palette) rgba(c color.Color, def color.RGBA) color.RGBA {
	switch c := c.(type) {
	case nil:
		return def
	case ansi.BasicColor:
		return p.ansi[c&15]
	case ansi.IndexedColor:
		if c < 16 {
			return p.ansi[c]
		}
	}
	r, g, b, _ := c.RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 0xff}
}

// colors returns the text and the background colours of a cell.
func (p *palette) colors(s uv.Style) (fg, bg color.RGBA) {
	fg, bg = p.rgba(s.Fg, p.fg), p.rgba(s.Bg, p.bg)
	if s.Attrs&uv.AttrReverse != 0 {
		fg, bg = bg, fg
	}
	if s.Attrs&uv.AttrFaint != 0 {
		fg = mix(fg, bg, 0.5)
	}
	return fg, bg
}

// mix returns a moved toward b by t, from 0 to 1.
func mix(a, b color.RGBA, t float64) color.RGBA {
	m := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5) }
	return color.RGBA{m(a.R, b.R), m(a.G, b.G), m(a.B, b.B), 0xff}
}

// linePixels draws a line of cells, and returns its RGBA pixels compressed
// for the terminal.
func (r *raster) linePixels(cells uv.Line, p *palette, cursor int) []byte {
	img, _ := r.lines.Get().(*image.RGBA)
	if size := image.Rect(0, 0, len(cells)*r.cw, r.ch); img == nil || img.Rect != size {
		img = image.NewRGBA(size)
	}
	r.draw(img, cells, p, cursor)
	data := compress(img.Pix)
	r.lines.Put(img)
	return data
}

// draw draws a line of cells on img, a picture of the size of the line,
// over all of its pixels. cursor is the column of a cursor to draw, or -1.
func (r *raster) draw(img *image.RGBA, cells uv.Line, p *palette, cursor int) {
	for x := 0; x < len(cells); {
		c := cells[x]
		w := max(c.Width, 1)
		fg, bg := p.colors(c.Style)
		rect := image.Rect(x*r.cw, 0, min(x+w, len(cells))*r.cw, r.ch)
		draw.Draw(img, rect, image.NewUniform(bg), image.Point{}, draw.Src)
		if c.Style.Attrs&uv.AttrConceal == 0 && !r.logoCell(img, rect, c, p.fg) {
			r.cell(img, rect, c, fg)
		}
		x += w
	}
	if cursor >= 0 && cursor < len(cells) {
		bar := image.Rect(cursor*r.cw, 0, cursor*r.cw+2*r.thin, r.ch)
		draw.Draw(img, bar, image.NewUniform(p.fg), image.Point{}, draw.Src)
	}
}

// cell draws the text and the lines of one cell in rect.
func (r *raster) cell(img *image.RGBA, rect image.Rectangle, c uv.Cell, fg color.RGBA) {
	ink := image.NewUniform(fg)
	if rs := []rune(c.Content); len(rs) == 1 && r.shape(img, rect, rs[0], ink) {
		// drawn as a shape
	} else if c.Content != "" && c.Content != " " {
		style := styleRegular
		if c.Style.Attrs&uv.AttrBold != 0 {
			style |= styleBold
		}
		if c.Style.Attrs&uv.AttrItalic != 0 {
			style |= styleItalic
		}
		if mask := r.glyph(glyphKey{c.Content, style, max(c.Width, 1)}); mask != nil {
			draw.DrawMask(img, rect, ink, image.Point{}, mask, image.Point{}, draw.Over)
		}
	}
	if c.Style.Underline != uv.UnderlineNone {
		y := min(r.baseline+r.thin, r.ch-r.thin)
		uc := image.NewUniform(fg)
		if c.Style.UnderlineColor != nil {
			cr, cg, cb, _ := c.Style.UnderlineColor.RGBA()
			uc = image.NewUniform(color.RGBA{uint8(cr >> 8), uint8(cg >> 8), uint8(cb >> 8), 0xff})
		}
		draw.Draw(img, image.Rect(rect.Min.X, y, rect.Max.X, y+r.thin), uc, image.Point{}, draw.Src)
	}
	if c.Style.Attrs&uv.AttrStrikethrough != 0 {
		y := r.baseline - r.ch*3/10
		draw.Draw(img, image.Rect(rect.Min.X, y, rect.Max.X, y+r.thin), ink, image.Point{}, draw.Src)
	}
}

// glyph returns the coverage of the text of a cell, or nil for none. The
// text is in the middle of its cells. A font that lacks the first rune
// gives the next font of the chain the text.
func (r *raster) glyph(k glyphKey) *image.Alpha {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m, ok := r.glyphs[k]; ok {
		return m
	}
	first := []rune(k.text)[0]
	chain := r.chains[k.style]
	face := r.faces[k.style][len(chain)-1]
	for i, f := range chain {
		if g, err := f.GlyphIndex(&r.buf, first); err == nil && g != 0 {
			face = r.faces[k.style][i]
			break
		}
	}
	m := image.NewAlpha(image.Rect(0, 0, k.width*r.cw, r.ch))
	d := font.Drawer{Dst: m, Src: image.Opaque, Face: face}
	adv := d.MeasureString(k.text)
	d.Dot = fixed.Point26_6{X: max(fixed.I(k.width*r.cw)-adv, 0) / 2, Y: fixed.I(r.baseline)}
	d.DrawString(k.text)
	r.glyphs[k] = m
	return m
}

// The arms of a box line, from the middle of the cell to an edge.
const (
	armUp = 1 << iota
	armDown
	armLeft
	armRight
	heavy // the arms are thick
)

// boxArms are the box lines that rexdiff draws. They join the next cells
// with no gap, which a font does not do.
var boxArms = map[rune]int{
	'─': armLeft | armRight, '━': armLeft | armRight | heavy,
	'│': armUp | armDown, '┃': armUp | armDown | heavy,
	'┌': armRight | armDown, '┐': armLeft | armDown,
	'└': armUp | armRight, '┘': armUp | armLeft,
	'├': armUp | armDown | armRight, '┤': armUp | armDown | armLeft,
	'┬': armLeft | armRight | armDown, '┴': armLeft | armRight | armUp,
	'┼': armUp | armDown | armLeft | armRight,
}

// roundCorners are the arms of the round corners of a box.
var roundCorners = map[rune]int{
	'╭': armRight | armDown, '╮': armLeft | armDown,
	'╰': armUp | armRight, '╯': armUp | armLeft,
}

// shape draws a box line or a block in rect, and reports whether ch is one.
func (r *raster) shape(img *image.RGBA, rect image.Rectangle, ch rune, ink image.Image) bool {
	x0, y0, w, h := rect.Min.X, rect.Min.Y, r.cw, r.ch
	fill := func(x1, y1, x2, y2 int) {
		draw.Draw(img, image.Rect(x1, y1, x2, y2).Intersect(rect), ink, image.Point{}, draw.Src)
	}
	switch {
	case 0x2581 <= ch && ch <= 0x2588: // lower eighths, to the full block
		fill(x0, y0+h-h*int(ch-0x2580)/8, x0+w, y0+h)
	case 0x2589 <= ch && ch <= 0x258f: // left eighths
		fill(x0, y0, x0+w*int(0x2590-ch)/8, y0+h)
	case ch == '▀':
		fill(x0, y0, x0+w, y0+h/2)
	case ch == '▐':
		fill(x0+w/2, y0, x0+w, y0+h)
	case ch == '▔':
		fill(x0, y0, x0+w, y0+h/8)
	case ch == '▕':
		fill(x0+w-w/8, y0, x0+w, y0+h)
	default:
		arms, ok := boxArms[ch]
		round, isRound := roundCorners[ch]
		if !ok && !isRound {
			return false
		}
		t := r.thin
		if arms&heavy != 0 {
			t *= 2
		}
		mx, my := x0+(w-t)/2, y0+(h-t)/2 // the top left of the middle
		if isRound {
			r.corner(img, rect, round, ink)
			return true
		}
		if arms&armUp != 0 {
			fill(mx, y0, mx+t, my+t)
		}
		if arms&armDown != 0 {
			fill(mx, my, mx+t, y0+h)
		}
		if arms&armLeft != 0 {
			fill(x0, my, mx+t, my+t)
		}
		if arms&armRight != 0 {
			fill(mx, my, x0+w, my+t)
		}
	}
	return true
}

// corner draws a round corner: a quarter circle from the middle of the
// cell, and straight lines from it to the two edges of its arms.
func (r *raster) corner(img *image.RGBA, rect image.Rectangle, arms int, ink image.Image) {
	t := float64(r.thin)
	w, h := float64(r.cw), float64(r.ch)
	cx, cy := w/2, h/2 // the middle of the line
	rad := w / 2
	sx, sy := -1.0, -1.0 // where the centre of the circle is, from the middle
	if arms&armRight != 0 {
		sx = 1
	}
	if arms&armDown != 0 {
		sy = 1
	}
	ox, oy := cx+sx*rad, cy+sy*rad // the centre of the circle
	col := ink.At(0, 0)
	cr, cg, cb, _ := col.RGBA()
	for py := 0; py < r.ch; py++ {
		for px := 0; px < r.cw; px++ {
			x, y := float64(px)+0.5, float64(py)+0.5
			var cover float64
			switch {
			case (x-ox)*sx > 0: // the straight arm left or right
				cover = t/2 + 0.5 - math.Abs(y-cy)
			case (y-oy)*sy > 0: // the straight arm up or down
				cover = t/2 + 0.5 - math.Abs(x-cx)
			default:
				cover = t/2 + 0.5 - math.Abs(math.Hypot(x-ox, y-oy)-rad)
			}
			if cover <= 0 {
				continue
			}
			a := min(cover, 1)
			dst := img.RGBAAt(rect.Min.X+px, rect.Min.Y+py)
			blend := func(d uint8, s uint32) uint8 { return uint8(float64(d)*(1-a) + float64(s>>8)*a + 0.5) }
			img.SetRGBA(rect.Min.X+px, rect.Min.Y+py, color.RGBA{blend(dst.R, cr), blend(dst.G, cg), blend(dst.B, cb), 0xff})
		}
	}
}
