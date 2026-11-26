package tui

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/sys/unix"
)

// The kitty renderer draws each line of the screen as a picture with the
// kitty graphics protocol, over the text that Bubble Tea draws. A line that
// changes gets a new picture, which replaces the old one at once, so the
// screen does not flicker. A line keeps its picture while it scrolls, so a
// scroll sends only the lines that come into view.
//
// The keys, the mouse, and the text of the pane work as in the text
// renderer: the text is still there, under the pictures.

// ttyOut is the terminal of Bubble Tea in the kitty renderer. It writes the
// pictures of a frame after the text of that frame. A clear of the screen in
// the text, as after a resize, then cannot delete the new pictures.
//
// The pictures are only on the alternate screen. Before Bubble Tea leaves
// it, to quit or to run an editor, ttyOut deletes them, or they would cover
// the shell. Until Bubble Tea comes back, the pictures wait.
//
// A terminal that reads slowly blocks a write. The writes hold wmu, and
// queue takes only mu, so a frame that waits for the terminal does not stop
// the program: the keys still work, and q still quits.
type ttyOut struct {
	*os.File
	wmu     sync.Mutex // orders the writes to the terminal
	mu      sync.Mutex // guards the fields below
	pending []byte
	timer   *time.Timer
	away    bool // Bubble Tea left the alternate screen
	leaves  int  // how many times Bubble Tea left it
}

// pictureWait is how long the pictures of a frame wait for the text of the
// frame. A frame with no new text does not come, so the pictures then go
// by themselves.
const pictureWait = 50 * time.Millisecond

// deleteAll deletes all the pictures on the screen and their data.
const deleteAll = "\x1b_Ga=d,d=A,q=2\x1b\\"

// Write writes p, the output of Bubble Tea, and then the pictures that wait.
func (o *ttyOut) Write(p []byte) (int, error) {
	o.wmu.Lock()
	defer o.wmu.Unlock()
	o.mu.Lock()
	leaving := !o.away && bytes.Contains(p, []byte(ansi.ResetModeAltScreenSaveCursor))
	if leaving {
		o.away, o.leaves, o.pending = true, o.leaves+1, nil
	}
	if bytes.Contains(p, []byte(ansi.SetModeAltScreenSaveCursor)) {
		o.away = false
	}
	var pictures []byte
	if !o.away {
		pictures, o.pending = o.pending, nil
	}
	o.mu.Unlock()

	if leaving {
		if _, err := o.WriteString(deleteAll); err != nil {
			return 0, err
		}
	}
	n, err := o.File.Write(p)
	if err == nil && len(pictures) > 0 {
		_, err = o.File.Write(pictures)
	}
	return n, err
}

// left returns how many times Bubble Tea left the alternate screen. The
// pictures are gone after each time.
func (o *ttyOut) left() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.leaves
}

// queue keeps b until the next write of Bubble Tea, or for pictureWait.
func (o *ttyOut) queue(b []byte) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.pending = append(o.pending, b...)
	if o.timer == nil {
		o.timer = time.AfterFunc(pictureWait, o.flush)
	} else {
		o.timer.Reset(pictureWait)
	}
}

func (o *ttyOut) flush() {
	o.wmu.Lock()
	defer o.wmu.Unlock()
	o.mu.Lock()
	var pictures []byte
	if !o.away {
		pictures, o.pending = o.pending, nil
	}
	o.mu.Unlock()
	if len(pictures) > 0 {
		_, _ = o.File.Write(pictures)
	}
}

// kittyScreen keeps the pictures in the terminal. Each picture is a line
// of the screen. Its kitty image ID comes from ids, and a placement shows it
// on row y with the placement ID y+1.
type kittyScreen struct {
	out        *ttyOut
	pal        palette
	raster     *raster
	cols, rows int
	cw, ch     int               // the cell in pixels, or 0 to ask the terminal
	scr        uv.ScreenBuffer   // the cells of the last frame, kept for the next
	last       string            // the content of the last frame
	lastCursor image.Point       // the cursor of the last frame, or -1, -1
	stale      bool              // draw all the lines again
	settled    atomic.Bool       // no colour changed for colorSettle
	settle     *time.Timer       // sets settled after a change of a colour
	wake       func()            // makes the program draw, or nil
	leaves     int               // ttyOut.left when the pictures were drawn
	placed     []uint32          // the image on each row, or 0
	ids        map[string]uint32 // the image of each line, by its text and cursor
	next       uint32            // the last image ID
}

// kittyWakeMsg makes the program draw the screen when its colours settle.
// Update has nothing to do for it.
type kittyWakeMsg struct{}

// colorSettle is how long the pictures wait after a colour changes. The
// terminal answers the questions for its colours one by one, and each answer
// changes the pictures of all the lines: they wait for the last one.
const colorSettle = 150 * time.Millisecond

// newKittyScreen makes a screen that draws on out. wake, if not nil, makes
// the program draw: it sends a message to the program. The screen draws
// after colorSettle, when the terminal had time to give its colours.
func newKittyScreen(out *ttyOut, wake func()) *kittyScreen {
	k := &kittyScreen{out: out, wake: wake}
	k.settle = time.AfterFunc(colorSettle, k.settleDone)
	k.pal.fg = color.RGBA{0xdd, 0xdd, 0xdd, 0xff}
	k.pal.bg = color.RGBA{0x1d, 0x1f, 0x21, 0xff}
	for i := range k.pal.ansi {
		r, g, b, _ := ansi.BasicColor(i).RGBA()
		k.pal.ansi[i] = color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 0xff}
	}
	return k
}

// requestKittyPalette asks the terminal for its 16 ANSI colours, which the
// pictures use for the cells in those colours.
func requestKittyPalette() tea.Msg {
	var b []byte
	for i := range 16 {
		b = fmt.Appendf(b, "\x1b]4;%d;?\x07", i)
	}
	return tea.RawMsg{Msg: string(b)}
}

// setColor sets the colour of the text (fg), the background (bg), or ANSI
// colour i, from 0 to 15. A nil colour keeps the colour.
func (k *kittyScreen) setColor(c *color.RGBA, to color.Color) {
	if to == nil {
		return
	}
	r, g, b, _ := to.RGBA()
	rgba := color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 0xff}
	if *c != rgba {
		*c = rgba
		k.stale = true
		k.settled.Store(false)
		k.settle.Reset(colorSettle)
	}
}

// resized tells the screen that the terminal changed its size. The cells
// can change their pixels and not their number, as on a move to a display
// of another scale.
func (k *kittyScreen) resized() { k.cw, k.ch = 0, 0 }

// redraw draws all the lines again on the next frame. The terminal can lose
// the pictures of a pane that the user does not look at.
func (k *kittyScreen) redraw() { k.stale = true }

// settleDone lets the screen draw in the colours that it has now.
func (k *kittyScreen) settleDone() {
	k.settled.Store(true)
	if k.wake != nil {
		k.wake()
	}
}

// colors sets the default colours of the cells.
func (k *kittyScreen) colors(fg, bg color.Color) {
	k.setColor(&k.pal.fg, fg)
	k.setColor(&k.pal.bg, bg)
}

// setANSI sets ANSI colour i from a reply of the terminal.
func (k *kittyScreen) setANSI(i int, c color.Color) {
	if 0 <= i && i < len(k.pal.ansi) {
		k.setColor(&k.pal.ansi[i], c)
	}
}

// cellPixels returns the size of a cell of the terminal in pixels. A
// terminal that does not tell gets the cells of a Retina Mac. The size
// only sets the sharpness: the terminal fits each picture to its cells.
func cellPixels(fd uintptr, cols, rows int) (cw, ch int) {
	ws, err := unix.IoctlGetWinsize(int(fd), unix.TIOCGWINSZ)
	if err != nil || ws.Xpixel == 0 || ws.Ypixel == 0 || cols <= 0 || rows <= 0 {
		return 16, 34
	}
	return max(int(ws.Xpixel)/cols, 1), max(int(ws.Ypixel)/rows, 1)
}

// draw shows content, a screen of cols by rows cells, with a cursor at
// cursor, if it is not nil. It sends only the lines that changed.
func (k *kittyScreen) draw(content string, cols, rows int, cursor *tea.Cursor) {
	if cols <= 0 || rows <= 0 || !k.settled.Load() {
		return
	}
	var b bytes.Buffer
	// Synchronized output: the terminal shows all the changes at once. Save
	// the cursor, for Bubble Tea moves it from where it left it.
	b.WriteString("\x1b[?2026h\x1b7")
	if k.cw == 0 || cols != k.cols || rows != k.rows {
		// Only a resize changes the cells. Asking each frame costs a
		// system call.
		k.cw, k.ch = cellPixels(k.out.Fd(), cols, rows)
	}
	cw, ch := k.cw, k.ch
	if k.raster == nil || k.raster.cw != cw || k.raster.ch != ch {
		k.raster = newRaster(cw, ch)
		k.stale = true
	}
	if leaves := k.out.left(); leaves != k.leaves {
		k.leaves, k.stale = leaves, true
	}
	if k.stale || cols != k.cols || rows != k.rows {
		b.WriteString(deleteAll)
		k.cols, k.rows, k.stale = cols, rows, false
		k.placed, k.ids = make([]uint32, rows), map[string]uint32{}
		k.last = ""
	}
	// Most messages, such as the tick of each second, change nothing.
	at := image.Pt(-1, -1)
	if cursor != nil {
		at = image.Pt(cursor.X, cursor.Y)
	}
	if content == k.last && at == k.lastCursor {
		return
	}
	k.last, k.lastCursor = content, at

	if k.scr.RenderBuffer == nil || k.scr.Width() != cols || k.scr.Height() != rows {
		k.scr = uv.NewScreenBuffer(cols, rows)
	} else {
		k.scr.Clear()
	}
	scr := k.scr
	uv.NewStyledString(content).Draw(scr, scr.Bounds())
	type picture struct {
		y, cursor int
		id        uint32
		data      []byte // the pixels, compressed
	}
	var news []*picture
	keys := make([]string, rows)
	for y := range rows {
		cx := -1
		if cursor != nil && cursor.Y == y {
			cx = cursor.X
		}
		keys[y] = scr.Line(y).Render() + "\x00" + strconv.Itoa(cx)
		if _, ok := k.ids[keys[y]]; !ok {
			k.next++
			k.ids[keys[y]] = k.next
			news = append(news, &picture{y: y, cursor: cx, id: k.next})
		}
	}
	var wg sync.WaitGroup
	for _, p := range news {
		wg.Go(func() { p.data = k.raster.linePixels(scr.Line(p.y), &k.pal, p.cursor) })
	}
	wg.Wait()
	for _, p := range news {
		upload(&b, p.id, p.data, cols*cw, ch)
	}
	changed := len(news) > 0
	for y, key := range keys {
		changed = changed || k.placed[y] != k.ids[key]
	}
	if !changed {
		return
	}
	// Place every row again, not only the rows that changed. Bubble Tea
	// scrolls the text with scroll regions, and a terminal moves the
	// pictures with the text, so a row with the same picture as before can
	// have it on another row now. A placement with the same image and
	// placement ID moves to where it is placed again.
	for y, key := range keys {
		id := k.ids[key]
		if old := k.placed[y]; old != 0 && old != id {
			fmt.Fprintf(&b, "\x1b_Ga=d,d=i,i=%d,p=%d,q=2\x1b\\", old, y+1)
		}
		// C=1 keeps the cursor still, so the last row does not scroll.
		fmt.Fprintf(&b, "\x1b[%d;1H\x1b_Ga=p,i=%d,p=%d,c=%d,r=1,C=1,q=2\x1b\\", y+1, id, y+1, cols)
		k.placed[y] = id
	}
	k.prune(&b)
	b.WriteString("\x1b8\x1b[?2026l")
	k.out.queue(b.Bytes())
}

// prune deletes the pictures that are not on the screen when there are
// many. The others stay for a scroll back.
func (k *kittyScreen) prune(b *bytes.Buffer) {
	if len(k.ids) <= 2*k.rows {
		return
	}
	shown := make(map[uint32]bool, len(k.placed))
	for _, id := range k.placed {
		shown[id] = true
	}
	for key, id := range k.ids {
		if !shown[id] {
			fmt.Fprintf(b, "\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", id)
			delete(k.ids, key)
		}
	}
}

// zlibWriters keeps the compressors of the pictures, which are large, for
// the next lines.
var zlibWriters sync.Pool

// compress returns the pixels compressed with zlib, as o=z of the protocol
// wants them. It is a few times faster than PNG, which picks a filter for
// each row of pixels, and the terminal does not have to undo the filters.
func compress(pix []byte) []byte {
	var b bytes.Buffer
	zw, _ := zlibWriters.Get().(*zlib.Writer)
	if zw == nil {
		zw, _ = zlib.NewWriterLevel(&b, zlib.BestSpeed) // the level is valid
	} else {
		zw.Reset(&b)
	}
	_, _ = zw.Write(pix) // a bytes.Buffer does not fail
	_ = zw.Close()
	zlibWriters.Put(zw)
	return b.Bytes()
}

// upload sends a picture of w by h RGBA pixels, compressed with zlib, as
// image id, in the chunks of the protocol.
func upload(b *bytes.Buffer, id uint32, data []byte, w, h int) {
	s := base64.StdEncoding.EncodeToString(data)
	const chunk = 4096
	for i := 0; i < len(s); i += chunk {
		end := min(i+chunk, len(s))
		more := 0
		if end < len(s) {
			more = 1
		}
		if i == 0 {
			fmt.Fprintf(b, "\x1b_Ga=t,f=32,o=z,s=%d,v=%d,i=%d,q=2,m=%d;%s\x1b\\",
				w, h, id, more, s[i:end])
		} else {
			fmt.Fprintf(b, "\x1b_Gm=%d;%s\x1b\\", more, s[i:end])
		}
	}
}
