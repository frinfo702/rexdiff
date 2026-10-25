package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

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
	if m.rex == nil {
		return m.runHere(path, ed.Script)
	}
	c, ctx, root := m.rex, m.ctx, m.repo.Root()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, rexTimeout)
		defer cancel()
		return splitMsg{path, ed.Script, c.Split(ctx, root, "sh", "-c", ed.Script)}
	}
}

// onSplit tells that the editor opens in a new pane. If Rex cannot open
// the pane, the editor runs in this pane.
func (m *model) onSplit(msg splitMsg) tea.Cmd {
	if msg.err == nil {
		m.flash("opened " + msg.path)
		return nil
	}
	m.log.Debug("editor split", "err", msg.err)
	return m.runHere(msg.path, msg.script)
}

// runHere runs the editor script in this pane, in place of rexdiff.
func (m *model) runHere(path, script string) tea.Cmd {
	return tea.ExecProcess(command("sh", "-c", script), func(err error) tea.Msg {
		return editorDoneMsg{path: path, err: err}
	})
}

// runWindowEditor starts a window editor for path. The editor gets its
// own session and no terminal, so that it cannot read the keys of rexdiff.
// If the editor fails, the returned command gives the error and the last
// line of the stderr of the editor.
func (m *model) runWindowEditor(path, script string) tea.Cmd {
	wait, err := detach("editor", "sh", "-c", script)
	if err != nil {
		m.flash("editor failed: " + err.Error())
		return nil
	}
	m.flash("opened " + path)
	return wait
}

// detach starts a program in its own session and with no terminal, so
// that it cannot read the keys of rexdiff. The returned command waits for
// the program. If the program fails, the command gives a detachedMsg with
// the last line of the stderr of the program.
func detach(what, name string, args ...string) (tea.Cmd, error) {
	cmd := command(name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// A child of the program can keep stderr open after the program ends.
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return func() tea.Msg {
		err := cmd.Wait()
		if err == nil || errors.Is(err, exec.ErrWaitDelay) {
			return nil
		}
		s := strings.TrimSpace(stderr.String())
		if last := s[strings.LastIndexByte(s, '\n')+1:]; last != "" {
			err = fmt.Errorf("%w: %s", err, last)
		}
		return detachedMsg{what, err}
	}, nil
}

// onEditorDone shows how the editor in this pane stops. An editor that
// stops with an error code can still save the file.
func (m *model) onEditorDone(msg editorDoneMsg) {
	exit, isExit := errors.AsType[*exec.ExitError](msg.err)
	switch {
	case msg.err == nil:
		m.flash("edited " + msg.path)
	case isExit:
		m.flash("editor stopped: " + exit.Error())
	default:
		m.flash("editor failed: " + msg.err.Error())
	}
	m.rerender()
}

// openURL opens url with the URL opener of the settings, else with
// the opener of the system. The command waits for the opener.
func openURL(cfg config, url string) (tea.Cmd, error) {
	if cfg.URLOpener != "" {
		script := strings.ReplaceAll(cfg.URLOpener, "{url}", editor.ShellQuote(url))
		return detach("open", "sh", "-c", script)
	}
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	return detach("open", opener, url)
}

// command returns the command for an editor or a URL opener. The command
// has no context, because the program must not stop when rexdiff stops.
// For example, a kill of the editor can lose the edits of the user.
func command(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...) //nolint:noctx // see the doc comment
}
