// Package test runs the rexdiff program as a user does. Each test starts
// the binary in a terminal, types keys, clicks, and reads the screen. A
// fake rex stands in for Rex, and the other tools are fakes on PATH. git
// is real.
package test

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"

	"github.com/rednafi/rexdiff/internal/testgit"
)

var (
	home     string // the HOME of the user, for the real Rex
	buildDir string // the directory of the binaries that the tests build
)

func TestMain(m *testing.M) {
	home = os.Getenv("HOME")
	var err error
	if buildDir, err = os.MkdirTemp("", "rexdiff-test"); err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(buildDir) }()
	testgit.Main(m)
}

// buildDev builds rexdiff once.
var buildDev = sync.OnceValues(func() (string, error) {
	return build(filepath.Join(buildDir, "dev"), "")
})

// binary returns the path of a rexdiff that the package builds once.
func binary(t *testing.T) string {
	t.Helper()
	bin, err := buildDev()
	if err != nil {
		t.Fatal(err)
	}
	return bin
}

// build builds rexdiff with the version into dir, and returns its path.
// A run with the race detector builds a rexdiff with it.
func build(dir, version string) (string, error) {
	bin := filepath.Join(dir, "rexdiff")
	args := []string{"build", "-o", bin, "-ldflags", "-X main.version=" + version}
	if raceOn {
		args = append(args, "-race")
	}
	cmd := exec.CommandContext(context.Background(), "go", append(args, "../cmd/rexdiff")...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build: %w\n%s", err, out)
	}
	return bin, nil
}

// userEnv are the variables of the user that change what rexdiff does.
// Each run starts with them empty.
var userEnv = []string{
	"REX_SESSION", "REX_BLOCK", "REX_BIN_DIR", "REX_CONFIG",
	"REX_AGENT_BLOCK", "REXDIFF_LOG", "REXDIFF_RESUME", "EDITOR", "VISUAL",
}

// opts are the options of a rexdiff run.
type opts struct {
	args   []string // the flags; start adds the repo
	width  uint16   // 0 means 160
	height uint16   // 0 means 40
	rex    *fakeRex // nil runs rexdiff outside Rex
	path   string   // a directory of fake tools, or ""
	env    []string // more variables
}

// term is a rexdiff that runs in a terminal.
type term struct {
	t    *testing.T
	pty  *os.File
	emu  *vt.Emulator
	mu   sync.Mutex    // guards emu
	wait func() error  // waits for rexdiff once, and keeps its error
	done chan struct{} // closed when the screen gets no more output
}

// start runs rexdiff on repo in a new terminal. The test stops it at the
// end.
func start(t *testing.T, repo string, o opts) *term {
	t.Helper()
	return startBin(t, binary(t), repo, o)
}

// startBin is start with the rexdiff at bin.
func startBin(t *testing.T, bin, repo string, o opts) *term {
	t.Helper()
	w, h := cmp.Or(o.width, 160), cmp.Or(o.height, 40)
	cmd := exec.CommandContext(t.Context(), bin, append(o.args, repo)...)
	cmd.Dir = repo
	// A stop lets rexdiff remove its files before it exits.
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 2 * time.Second
	path := "/usr/bin:/bin:/usr/sbin:/sbin"
	if git, err := exec.LookPath("git"); err == nil {
		path = filepath.Dir(git) + string(os.PathListSeparator) + path
	}
	if o.path != "" {
		path = o.path + string(os.PathListSeparator) + path
	}
	cmd.Env = append(os.Environ(), "PATH="+path, "TERM=xterm-256color")
	for _, k := range userEnv {
		cmd.Env = append(cmd.Env, k+"=")
	}
	if o.rex != nil {
		cmd.Env = append(cmd.Env, "REX_SESSION=S", "REX_BLOCK=block:self",
			"REX_BIN_DIR="+o.rex.dir)
	}
	cmd.Env = append(cmd.Env, o.env...)
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: w, Rows: h})
	if err != nil {
		t.Fatal(err)
	}
	tm := &term{t: t, pty: f, emu: vt.NewEmulator(int(w), int(h)), wait: sync.OnceValue(cmd.Wait), done: make(chan struct{})}
	go func() {
		defer close(tm.done)
		buf := make([]byte, 32<<10)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				tm.mu.Lock()
				_, _ = tm.emu.Write(buf[:n])
				tm.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	// The emulator answers the queries of rexdiff, for example for the
	// colours. This copy ends with the test process, because the Close of
	// the emulator races with its Read.
	go func() { _, _ = io.Copy(f, tm.emu) }()
	t.Cleanup(func() {
		_ = tm.wait() // t.Context is done, so Cancel stops rexdiff
		_ = f.Close()
		<-tm.done
	})
	tm.waitFor("1 Changes")
	return tm
}

// screen returns the text on the screen.
func (tm *term) screen() string {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.emu.String()
}

// keys types each key. A key is a text, or a name in keyCodes.
func (tm *term) keys(keys ...string) {
	tm.t.Helper()
	for _, k := range keys {
		s, ok := keyCodes[k]
		if !ok {
			s = k
		}
		if _, err := tm.pty.WriteString(s); err != nil {
			tm.t.Fatal(err)
		}
		// A pause lets rexdiff take the key. A lone esc needs a longer
		// one, else it joins the next key.
		pause := 20 * time.Millisecond
		if k == "esc" {
			pause = 150 * time.Millisecond
		}
		time.Sleep(pause)
	}
}

// typed types each character of text.
func (tm *term) typed(text string) {
	tm.t.Helper()
	tm.keys(strings.Split(text, "")...)
}

// keyCodes are the bytes of the named keys.
var keyCodes = map[string]string{
	"enter":  "\r",
	"esc":    "\x1b",
	"tab":    "\t",
	"right":  "\x1b[C",
	"left":   "\x1b[D",
	"pgdown": "\x1b[6~",
	"pgup":   "\x1b[5~",
	"ctrl+d": "\x04",
	"ctrl+u": "\x15",
	"ctrl+f": "\x06",
	"ctrl+j": "\n",
}

// at returns the column and the line of text on the screen, from 0.
func (tm *term) at(text string) (x, y int) {
	tm.t.Helper()
	for i, l := range strings.Split(tm.waitFor(text), "\n") {
		if before, _, ok := strings.Cut(l, text); ok {
			return utf8.RuneCountInString(before), i
		}
	}
	return -1, -1
}

// mouse sends a mouse event with the SGR code b at column x of line y.
// end is "M" for a press or a motion, and "m" for a release.
func (tm *term) mouse(b, x, y int, end string) {
	tm.t.Helper()
	seq := "\x1b[<" + strconv.Itoa(b) + ";" + strconv.Itoa(x+1) + ";" + strconv.Itoa(y+1) + end
	if _, err := tm.pty.WriteString(seq); err != nil {
		tm.t.Fatal(err)
	}
}

// click clicks text on the screen the given number of times in a row.
func (tm *term) click(text string, times int) {
	tm.t.Helper()
	x, y := tm.at(text)
	for range times {
		tm.mouse(0, x, y, "M")
		tm.mouse(0, x, y, "m")
		time.Sleep(50 * time.Millisecond) // as a hand clicks
	}
}

// drag drags the mouse from the start of text from to the start of text
// to.
func (tm *term) drag(from, to string) {
	tm.t.Helper()
	x0, y0 := tm.at(from)
	x1, y1 := tm.at(to)
	tm.mouse(0, x0, y0, "M")
	tm.mouse(32, x1, y1, "M") // a motion with the button down
	tm.mouse(0, x1, y1, "m")
}

// waitFor waits until the screen shows each of texts, and returns the
// screen.
func (tm *term) waitFor(texts ...string) string {
	tm.t.Helper()
	return tm.waitUntil(func(s string) bool {
		for _, text := range texts {
			if !strings.Contains(s, text) {
				return false
			}
		}
		return true
	}, "show "+strings.Join(texts, ", "))
}

// waitGone waits until the screen shows none of texts.
func (tm *term) waitGone(texts ...string) string {
	tm.t.Helper()
	return tm.waitUntil(func(s string) bool {
		for _, text := range texts {
			if strings.Contains(s, text) {
				return false
			}
		}
		return true
	}, "not show "+strings.Join(texts, ", "))
}

// waitCursor waits until the line of the cursor shows text.
func (tm *term) waitCursor(text string) {
	tm.t.Helper()
	tm.waitUntil(func(s string) bool {
		for _, l := range lines(s, "▶") {
			if strings.Contains(l, text) {
				return true
			}
		}
		return false
	}, "put the cursor on "+text)
}

// waitUntil waits until ok is true for the screen, and returns the screen.
func (tm *term) waitUntil(ok func(string) bool, what string) string {
	tm.t.Helper()
	var s string
	eventually(tm.t, func() bool { s = tm.screen(); return ok(s) }, func() string {
		return "the screen does not " + what + ". It shows:\n" + s
	})
	return s
}

// eventually waits until ok is true. It fails the test with the text of
// why after a while.
func eventually(t *testing.T, ok func() bool, why func() string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal(why())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// waitFile waits until the file at path has content, and returns it.
func waitFile(t *testing.T, path string, content func(string) bool) string {
	t.Helper()
	var s string
	eventually(t, func() bool {
		b, _ := os.ReadFile(path)
		s = string(b)
		return content(s)
	}, func() string { return path + " has " + strconv.Quote(s) })
	return s
}

// exited waits until rexdiff exits and returns its error.
func (tm *term) exited() error {
	tm.t.Helper()
	errc := make(chan error, 1)
	go func() { errc <- tm.wait() }()
	select {
	case err := <-errc:
		return err
	case <-time.After(15 * time.Second):
		tm.t.Fatalf("rexdiff does not exit. The screen shows:\n%s", tm.screen())
		return nil
	}
}
