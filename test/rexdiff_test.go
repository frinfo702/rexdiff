// Package test runs the rexdiff program as a user does. Each test starts
// the binary in a terminal, types keys, and reads the screen. A fake rex
// stands in for Rex, and the other tools are fakes on PATH. git is real.
package test

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"

	"github.com/rednafi/rexdiff/internal/testgit"
)

func TestMain(m *testing.M) { testgit.Main(m) }

var (
	buildOnce sync.Once
	binPath   string
	buildErr  error
)

// binary returns the path of a rexdiff binary that the package builds
// once.
func binary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "rexdiff-test")
		if err != nil {
			buildErr = err
			return
		}
		binPath = filepath.Join(dir, "rexdiff")
		out, err := exec.CommandContext(context.Background(), "go", "build", "-o", binPath, "../cmd/rexdiff").CombinedOutput()
		if err != nil {
			buildErr = &buildError{err, string(out)}
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return binPath
}

// buildError is an error of go build with its output.
type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return "go build: " + e.err.Error() + "\n" + e.out }

// opts are the options of a rexdiff run.
type opts struct {
	args   []string // the flags; start adds the repo
	width  int      // 0 means 160
	height int      // 0 means 40
	rex    *fakeRex // nil runs rexdiff outside Rex
	path   string   // a directory of fake tools, or ""
	env    []string // more variables
}

// term is a rexdiff that runs in a terminal.
type term struct {
	t    *testing.T
	pty  *os.File
	emu  *vt.Emulator
	mu   sync.Mutex // guards emu
	cmd  *exec.Cmd
	done chan struct{}
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
	w, h := o.width, o.height
	if w == 0 {
		w = 160
	}
	if h == 0 {
		h = 40
	}
	cmd := exec.CommandContext(t.Context(), bin, append(o.args, repo)...)
	cmd.Dir = repo
	path := "/usr/bin:/bin:/usr/sbin:/sbin"
	if git, err := exec.LookPath("git"); err == nil {
		path = filepath.Dir(git) + ":" + path
	}
	if o.path != "" {
		path = o.path + ":" + path
	}
	cmd.Env = append(os.Environ(),
		"PATH="+path, "TERM=xterm-256color", "REX_SESSION=", "REX_BLOCK=",
		"REX_BIN_DIR=", "REX_AGENT_BLOCK=", "EDITOR=", "VISUAL=", "REXDIFF_LOG=")
	if o.rex != nil {
		cmd.Env = append(cmd.Env, "REX_SESSION=S", "REX_BLOCK=block:self",
			"REX_BIN_DIR="+o.rex.dir)
	}
	cmd.Env = append(cmd.Env, o.env...)
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)})
	if err != nil {
		t.Fatal(err)
	}
	tm := &term{t: t, pty: f, emu: vt.NewEmulator(w, h), cmd: cmd, done: make(chan struct{})}
	go func() { // the screen
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
	go func() { _, _ = io.Copy(f, tm.emu) }() // the keys and the replies
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = f.Close()
		<-tm.done
		// The emulator stays open. Its Close races with its Read.
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

// keys types each key. A key is a text, or a name such as "enter".
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

// keyCodes are the bytes of the named keys.
var keyCodes = map[string]string{
	"enter":  "\r",
	"esc":    "\x1b",
	"tab":    "\t",
	"space":  " ",
	"up":     "\x1b[A",
	"down":   "\x1b[B",
	"right":  "\x1b[C",
	"left":   "\x1b[D",
	"ctrl+d": "\x04",
	"ctrl+u": "\x15",
	"ctrl+f": "\x06",
	"bs":     "\x7f",
}

// click clicks text on the screen the given number of times in a row.
func (tm *term) click(text string, times int) {
	tm.t.Helper()
	x, y := -1, -1
	for i, l := range strings.Split(tm.waitFor(text), "\n") {
		if before, _, ok := strings.Cut(l, text); ok {
			x, y = utf8.RuneCountInString(before), i
			break
		}
	}
	at := strconv.Itoa(x+1) + ";" + strconv.Itoa(y+1)
	for range times {
		for _, end := range []string{"M", "m"} { // press, release
			if _, err := tm.pty.WriteString("\x1b[<0;" + at + end); err != nil {
				tm.t.Fatal(err)
			}
		}
	}
}

// waitFor waits until the screen shows each of texts. It fails the test
// after a while.
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

// waitUntil waits until ok is true for the screen.
func (tm *term) waitUntil(ok func(string) bool, what string) string {
	tm.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		s := tm.screen()
		if ok(s) {
			return s
		}
		if time.Now().After(deadline) {
			tm.t.Fatalf("the screen does not %s. It shows:\n%s", what, s)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// exited waits until rexdiff exits and returns its error.
func (tm *term) exited() error {
	tm.t.Helper()
	errc := make(chan error, 1)
	go func() { errc <- tm.cmd.Wait() }()
	select {
	case err := <-errc:
		return err
	case <-time.After(10 * time.Second):
		tm.t.Fatalf("rexdiff does not exit. The screen shows:\n%s", tm.screen())
		return nil
	}
}
