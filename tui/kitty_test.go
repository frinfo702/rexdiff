package tui

import (
	"fmt"
	"image/color"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// testOut returns a ttyOut on a file, and a function that sends what the
// timer would send and returns all that went to the file since the last
// call.
func testOut(t *testing.T) (*ttyOut, func() string) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "tty")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	out := &ttyOut{File: f}
	seen := 0
	return out, func() string {
		out.flush()
		b, err := os.ReadFile(f.Name())
		if err != nil {
			t.Fatal(err)
		}
		s := string(b[seen:])
		seen = len(b)
		return s
	}
}

// TestKittyDrawSendsOnlyNewLines checks that a scroll by one line uploads
// one picture and moves the others, and that the same screen sends nothing.
func TestKittyDrawSendsOnlyNewLines(t *testing.T) {
	out, sent := testOut(t)
	k := newKittyScreen(out, nil)
	k.settled.Store(true)
	k.draw("a\nb\nc", 3, 3, nil)
	if got := strings.Count(sent(), "a=t,"); got != 3 {
		t.Fatalf("first frame: %d uploads, want 3", got)
	}
	k.draw("a\nb\nc", 3, 3, nil)
	if got := sent(); got != "" {
		t.Fatalf("same frame sent %q", got)
	}
	k.draw("b\nc\nd", 3, 3, nil)
	s := sent()
	if got := strings.Count(s, "a=t,"); got != 1 {
		t.Fatalf("scroll: %d uploads, want 1", got)
	}
	if got := strings.Count(s, "a=p,"); got != 3 {
		t.Fatalf("scroll: %d placements, want 3", got)
	}
	k.draw("b\nc\nd", 4, 3, nil) // a resize draws all again
	if s := sent(); !strings.Contains(s, "d=A") || strings.Count(s, "a=t,") != 3 {
		t.Fatalf("resize did not draw again: %q", s)
	}
}

// TestKittyLeaveAltScreen checks that the pictures go before Bubble Tea
// leaves the alternate screen, to quit or to run an editor, that no picture
// goes to the shell, and that the pictures come back with the program.
func TestKittyLeaveAltScreen(t *testing.T) {
	out, sent := testOut(t)
	k := newKittyScreen(out, nil)
	k.settled.Store(true)

	k.draw("a", 1, 1, nil)
	_, _ = out.Write([]byte("frame"))
	k.draw("b", 1, 1, nil) // the last frame
	_, _ = out.Write([]byte("bye" + ansi.ResetModeAltScreenSaveCursor))
	got := sent()
	leave := strings.Index(got, "bye")
	if del := strings.LastIndex(got, deleteAll); del < 0 || del > leave {
		t.Fatalf("no delete before leaving: %q", got)
	}
	if strings.Contains(got[leave:], "a=p,") {
		t.Fatalf("a picture after leaving: %q", got[leave:])
	}
	k.draw("c", 1, 1, nil) // a frame after leaving
	if strings.Contains(sent(), "a=p,") {
		t.Fatal("a picture went to the shell")
	}

	k.draw("c", 1, 1, nil) // after the editor: draw all again
	_, _ = out.Write([]byte(ansi.SetModeAltScreenSaveCursor + "back"))
	got = sent()
	if back := strings.Index(got, "back"); back < 0 || !strings.Contains(got[back:], "a=p,") {
		t.Fatalf("no picture after coming back: %q", got)
	}
}

// TestKittyColorsSettle checks that the answers of the terminal for its
// colours, which come one by one, make one new drawing and not one each.
func TestKittyColorsSettle(t *testing.T) {
	out, sent := testOut(t)
	woke := make(chan struct{}, 1)
	k := newKittyScreen(out, func() { woke <- struct{}{} })
	<-woke
	k.draw("a", 1, 1, nil)
	sent()
	for i := range 16 {
		k.setANSI(i, color.RGBA{uint8(i), 0, 0, 0xff})
		k.draw("a", 1, 1, nil)
	}
	if s := sent(); s != "" {
		t.Fatalf("drew before the colours settled: %q", s)
	}
	<-woke
	k.draw("a", 1, 1, nil)
	if n := strings.Count(sent(), "a=t,"); n != 1 {
		t.Fatalf("%d uploads after the colours settled, want 1", n)
	}
}

// TestKittyPlacesAllRowsAgain checks that a frame with a change places the
// rows that did not change too. A scroll region of Bubble Tea moves their
// pictures in the terminal, so they must go back to their rows. Only the
// row that changed deletes its old picture.
func TestKittyPlacesAllRowsAgain(t *testing.T) {
	out, sent := testOut(t)
	k := newKittyScreen(out, nil)
	k.settled.Store(true)
	k.draw("x\nx\na", 1, 3, nil)
	sent()
	k.draw("x\nx\nb", 1, 3, nil)
	s := sent()
	for y := 1; y <= 3; y++ {
		if !strings.Contains(s, fmt.Sprintf("\x1b[%d;1H\x1b_Ga=p,", y)) {
			t.Errorf("row %d not placed again: %q", y, s)
		}
	}
	if n := strings.Count(s, "a=d,d=i,"); n != 1 {
		t.Errorf("%d placements deleted, want 1: %q", n, s)
	}
}

// BenchmarkKittyMove moves the cursor down a large file on a large screen
// in the kitty renderer, as a user does with j. Each frame changes a few
// lines. Profile it with:
//
//	go test ./tui -run '^$' -bench KittyMove -cpuprofile cpu.out
func BenchmarkKittyMove(b *testing.B) {
	benchKitty(b, tea.KeyPressMsg{Code: 'j', Text: "j"})
}

// BenchmarkKittyRedraw draws a screen of 200 by 60 cells whose lines all
// change in each frame, as after a jump to another file.
func BenchmarkKittyRedraw(b *testing.B) {
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		b.Fatal(err)
	}
	defer null.Close()
	k := newKittyScreen(&ttyOut{File: null}, nil)
	k.settled.Store(true)
	line := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Background(lipgloss.Color("#1d2b1f"))
	n := 0
	for b.Loop() {
		var s strings.Builder
		for y := range 60 {
			n++
			s.WriteString(line.Width(200).Render(fmt.Sprintf("%6d  x := compute(%d) // 日本語", n, y)) + "\n")
		}
		k.draw(s.String(), 200, 60, nil)
		k.out.flush()
	}
}

func benchKitty(b *testing.B, key tea.KeyPressMsg) {
	m, dir := newTestModel(b)
	var src strings.Builder
	for i := range 5000 {
		src.WriteString("\tx := compute(" + strings.Repeat("a", i%40) + ") // 日本語\n")
	}
	write(b, dir, "big.go", "package a\nfunc f() {\n"+src.String()+"}\n")
	m.reload()
	m.pickFile(1)
	m = step(b, m, tea.WindowSizeMsg{Width: 200, Height: 60})
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		b.Fatal(err)
	}
	defer null.Close()
	m.kitty = newKittyScreen(&ttyOut{File: null}, nil)
	m.kitty.settled.Store(true)
	_ = m.View()
	for b.Loop() {
		next, _ := m.Update(key)
		m = *next.(*model)
		_ = m.View()
		m.kitty.out.flush()
	}
}

// TestKittyRedrawAndResize checks that a focus draws all the lines again,
// for the terminal can lose the pictures of a pane that is not seen, and
// that a resize asks the terminal for the pixels of a cell again, though
// the columns and the rows stay.
func TestKittyRedrawAndResize(t *testing.T) {
	out, sent := testOut(t)
	k := newKittyScreen(out, nil)
	k.settled.Store(true)
	k.draw("a\nb", 1, 2, nil)
	sent()

	k.redraw()
	k.draw("a\nb", 1, 2, nil)
	if s := sent(); !strings.Contains(s, deleteAll) || strings.Count(s, "a=t,") != 2 {
		t.Fatalf("a focus did not draw all again: %q", s)
	}

	k.cw, k.ch = 8, 17 // as on a display of another scale
	k.raster = newRaster(8, 17)
	k.resized()
	k.draw("a\nb", 1, 2, nil)
	if k.cw == 8 {
		t.Fatal("a resize did not ask the terminal for the cell again")
	}
	if s := sent(); strings.Count(s, "a=t,") != 2 {
		t.Fatalf("new cells did not draw all again: %q", s)
	}
}
