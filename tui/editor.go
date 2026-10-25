package tui

import (
	"bytes"
	"errors"
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
// settings, else $VISUAL, $EDITOR, or core.editor. A terminal editor opens
// in a new block beside rexdiff in Rex. Outside Rex, a terminal editor
// takes the screen until the editor stops. A window editor starts in the
// background. The messages are the same as in herdr-reviewr.
func (m *model) openEditor(path string, line int) tea.Cmd {
	abs := filepath.Join(m.repo.Root(), path)
	var git string
	if m.cfg.Editor == "" {
		git = m.repo.Editor(m.ctx)
	}
	ed, err := editor.Resolve(m.cfg.Editor, os.Getenv("VISUAL"), os.Getenv("EDITOR"), git, abs, line)
	if err != nil {
		m.flash(err.Error())
		return nil
	}
	// The editor makes the file again when it saves, so do not open a
	// file that is not on disk.
	if fi, err := os.Stat(abs); err != nil || !fi.Mode().IsRegular() {
		m.flash(path + " is gone")
		return nil
	}
	if _, err := exec.LookPath(ed.Program); err != nil {
		m.flash("editor not found: " + ed.Program)
		return nil
	}
	m.log.Debug("editor", "command", ed.Script, "window", ed.Window)
	if ed.Window {
		return m.runWindowEditor(path, ed.Script)
	}
	if m.rex != nil {
		err := m.rex.Split(m.ctx, m.repo.Root(), "sh", "-c", ed.Script)
		if err == nil {
			m.flash("opened " + path)
			return nil
		}
		m.log.Debug("editor split", "err", err)
	}
	return tea.ExecProcess(command("sh", "-c", ed.Script), func(err error) tea.Msg {
		return editorDoneMsg{path: path, err: err}
	})
}

// runWindowEditor starts a window editor for path. The editor gets its
// own session and no terminal, so that it cannot read the keys of rexdiff.
// If the editor fails, the returned command gives the error and the last
// line of the stderr of the editor.
func (m *model) runWindowEditor(path, script string) tea.Cmd {
	cmd := command("sh", "-c", script)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		m.flash("editor failed: " + err.Error())
		return nil
	}
	m.flash("opened " + path)
	return func() tea.Msg {
		err := cmd.Wait()
		if err == nil {
			return nil
		}
		s := strings.TrimSpace(stderr.String())
		if last := s[strings.LastIndexByte(s, '\n')+1:]; last != "" {
			err = fmt.Errorf("%w: %s", err, last)
		}
		return editorDoneMsg{window: true, err: err}
	}
}

// onEditorDone shows how the editor stops. A terminal editor that stops
// with an error code can still save the file.
func (m *model) onEditorDone(msg editorDoneMsg) {
	exit, isExit := errors.AsType[*exec.ExitError](msg.err)
	switch {
	case msg.err == nil:
		m.flash("edited " + msg.path)
	case !msg.window && isExit:
		m.flash("editor exited with " + exit.Error())
	default:
		m.flash("editor failed: " + msg.err.Error())
	}
	m.rerender()
}

// openURL opens url with the URL opener of the settings, else with
// the opener of the system.
func openURL(cfg config, url string) error {
	if cfg.URLOpener != "" {
		script := strings.ReplaceAll(cfg.URLOpener, "{url}", editor.ShellQuote(url))
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
	cmd := command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// command returns the command for an editor or a URL opener. The command
// has no context, because the program must not stop when rexdiff stops.
// For example, a kill of the editor can lose the edits of the user.
func command(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...) //nolint:noctx // see the doc comment
}
