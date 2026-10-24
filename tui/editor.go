package tui

import (
	"cmp"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// openEditor opens the file at path, at line. It uses the editor of the
// settings, else $VISUAL, $EDITOR, core.editor, or vi. In Rex, the
// editor opens in a new block beside rexdiff.
func (m *model) openEditor(path string, line int) tea.Cmd {
	abs := filepath.Join(m.repo.Root(), path)
	if m.cfg.Editor != "" {
		r := strings.NewReplacer("{file}", shellQuote(abs), "{line}", strconv.Itoa(line))
		if err := start("sh", "-c", r.Replace(m.cfg.Editor)); err != nil {
			m.flash("editor: " + err.Error())
		}
		return nil
	}
	ed := cmp.Or(
		os.Getenv("VISUAL"),
		os.Getenv("EDITOR"),
		m.repo.Editor(m.ctx),
		"vi",
	)
	script := fmt.Sprintf("%s +%d %s", ed, line, shellQuote(abs))
	if m.rex != nil {
		if err := m.rex.Split(m.ctx, m.repo.Root(), "sh", "-c", script); err == nil {
			return nil
		}
	}
	done := func(err error) tea.Msg { return editorDoneMsg{err} }
	return tea.ExecProcess(exec.Command("sh", "-c", script), done)
}

// openURL opens url with the URL opener of the settings, else with
// the opener of the system.
func openURL(cfg config, url string) error {
	if cfg.URLOpener != "" {
		script := strings.ReplaceAll(cfg.URLOpener, "{url}", shellQuote(url))
		return start("sh", "-c", script)
	}
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	return start(opener, url)
}

// start starts a program and does not wait for it to stop. A goroutine
// waits for it, so that it does not stay a zombie process.
func start(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

// shellQuote quotes s as one word for sh.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
