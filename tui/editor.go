package tui

import (
	"bytes"
	"cmp"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"

	"github.com/rednafi/rexdiff/internal/editor"
)

// openEditor opens the file at path, at line. It uses the editor of the
// settings, else $VISUAL, $EDITOR, core.editor, or vi. A terminal editor
// opens in a new block beside rexdiff in Rex. Outside Rex, a terminal
// editor takes the screen until the editor stops. A window editor starts in
// the background.
func (m *model) openEditor(path string, line int) tea.Cmd {
	abs := filepath.Join(m.repo.Root(), path)
	var ed editor.Command
	if m.cfg.Editor != "" {
		ed = editor.FromTemplate(m.cfg.Editor, abs, line)
	} else {
		e := cmp.Or(
			strings.TrimSpace(os.Getenv("VISUAL")),
			strings.TrimSpace(os.Getenv("EDITOR")),
			m.repo.Editor(m.ctx),
			"vi",
		)
		ed = editor.FromEditor(e, abs, line)
	}
	m.log.Debug("editor", "command", ed.Script, "window", ed.Window)
	if ed.Window {
		return runWindowEditor(ed.Script)
	}
	if m.rex != nil {
		err := m.rex.Split(m.ctx, m.repo.Root(), "sh", "-c", ed.Script)
		if err == nil {
			return nil
		}
		m.log.Debug("editor split", "err", err)
	}
	done := func(err error) tea.Msg { return editorDoneMsg{err} }
	return tea.ExecProcess(exec.Command("sh", "-c", ed.Script), done)
}

// runWindowEditor starts a window editor. The editor gets its own session
// and no terminal, so that it cannot read the keys of rexdiff. If the
// editor fails, the returned command gives the error and the last line of
// the stderr of the editor.
func runWindowEditor(script string) tea.Cmd {
	cmd := exec.Command("sh", "-c", script)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return func() tea.Msg { return editorDoneMsg{err} }
	}
	return func() tea.Msg {
		err := cmd.Wait()
		if err == nil {
			return nil
		}
		lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
		if last := lines[len(lines)-1]; last != "" {
			err = fmt.Errorf("%w: %s", err, last)
		}
		return editorDoneMsg{err}
	}
}

// openURL opens url with the URL opener of the settings, else with
// the opener of the system.
func openURL(cfg config, url string) error {
	if cfg.URLOpener != "" {
		script := strings.ReplaceAll(cfg.URLOpener, "{url}", editor.Quote(url))
		return start("sh", "-c", script)
	}
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	return start(opener, url)
}

// start starts a program and does not wait for it to stop. A goroutine
// waits for the program, so that the program does not stay a zombie
// process.
func start(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
