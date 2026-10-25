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
	"slices"
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
	tm := &term{t: t, emu: vt.NewEmulator(int(w), int(h)), wait: sync.OnceValue(cmd.Wait), done: make(chan struct{})}
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
	// The input of rexdiff comes from the emulator, as from a terminal:
	// the keys and the mouse of the test, and the answers to the queries
	// of rexdiff, for example for the colours.
	input := make(chan struct{})
	go func() {
		defer close(input)
		_, _ = io.Copy(f, tm.emu)
	}()
	t.Cleanup(func() {
		_ = tm.wait() // t.Context is done, so Cancel stops rexdiff
		_ = f.Close()
		<-tm.done
		// Close of the pipe, not of the emulator, ends the copy. The Close
		// of the emulator races with its Read.
		_ = tm.emu.InputPipe().(io.Closer).Close()
		<-input
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

// keys types each key. A key is a name in keyCodes, or else a text. As
// in a terminal, an esc and the key after it can join into one key, so esc
// must be the last key. Wait for the screen after it, as a user does.
func (tm *term) keys(keys ...string) {
	tm.t.Helper()
	if i := slices.Index(keys, "esc"); i >= 0 && i != len(keys)-1 {
		tm.t.Fatalf("keys %q: esc must be the last key", keys)
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for _, k := range keys {
		if code, ok := keyCodes[k]; ok {
			tm.emu.SendKey(code)
		} else {
			tm.emu.SendText(k)
		}
	}
}

// typed types each character of text.
func (tm *term) typed(text string) {
	tm.t.Helper()
	tm.keys(strings.Split(text, "")...)
}

// keyCodes are the named keys.
var keyCodes = map[string]vt.KeyPressEvent{
	"enter":  {Code: vt.KeyEnter},
	"esc":    {Code: vt.KeyEscape},
	"tab":    {Code: vt.KeyTab},
	"right":  {Code: vt.KeyRight},
	"left":   {Code: vt.KeyLeft},
	"pgdown": {Code: vt.KeyPgDown},
	"pgup":   {Code: vt.KeyPgUp},
	"ctrl+d": {Code: 'd', Mod: vt.ModCtrl},
	"ctrl+u": {Code: 'u', Mod: vt.ModCtrl},
	"ctrl+f": {Code: 'f', Mod: vt.ModCtrl},
	"ctrl+j": {Code: 'j', Mod: vt.ModCtrl},
}

// at returns the column and the line of text on the screen, from 0.
func (tm *term) at(text string) (x, y int) {
	tm.t.Helper()
	s := tm.waitFor(text)
	for i, l := range strings.Split(s, "\n") {
		if before, _, ok := strings.Cut(l, text); ok {
			return utf8.RuneCountInString(before), i
		}
	}
	tm.t.Fatalf("the screen does not show %s. It shows:\n%s", text, s)
	return 0, 0
}

// mouse sends the mouse event m, as the terminal encodes it.
func (tm *term) mouse(m vt.Mouse) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.emu.SendMouse(m)
}

// click clicks text on the screen the given number of times in a row.
func (tm *term) click(text string, times int) {
	tm.t.Helper()
	x, y := tm.at(text)
	for range times {
		tm.mouse(vt.MouseClick{X: x, Y: y, Button: vt.MouseLeft})
		tm.mouse(vt.MouseRelease{X: x, Y: y, Button: vt.MouseLeft})
	}
}

// drag drags the mouse from the start of text from to the start of text
// to.
func (tm *term) drag(from, to string) {
	tm.t.Helper()
	x0, y0 := tm.at(from)
	x1, y1 := tm.at(to)
	tm.mouse(vt.MouseClick{X: x0, Y: y0, Button: vt.MouseLeft})
	tm.mouse(vt.MouseMotion{X: x1, Y: y1, Button: vt.MouseLeft})
	tm.mouse(vt.MouseRelease{X: x1, Y: y1, Button: vt.MouseLeft})
}

// wheel turns the mouse wheel down n times over text.
func (tm *term) wheel(text string, n int) {
	tm.t.Helper()
	x, y := tm.at(text)
	for range n {
		tm.mouse(vt.MouseWheel{X: x, Y: y, Button: vt.MouseWheelDown})
	}
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
