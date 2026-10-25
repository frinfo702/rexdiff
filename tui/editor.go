package tui

import (
	"bytes"
	"cmp"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"
)

// lineArg is how an editor takes the line to open at.
type lineArg int

const (
	argNone   lineArg = iota // file
	argPlus                  // +42 file
	argSuffix                // file:42
	argGoto                  // -g file:42
	argFlag                  // --line 42 file
)

// dialect tells how an editor takes the line, and if it opens a window.
// An editor without a window draws in the terminal, so it needs a pane.
type dialect struct {
	line   lineArg
	window bool
}

// dialects has the editors that rexdiff knows, by program name. The table
// is the same as in herdr-reviewr. The table also lists fresh, open, and
// xdg-open.
var dialects = makeDialects()

func makeDialects() map[string]dialect {
	d := map[string]dialect{}
	add := func(dl dialect, names ...string) {
		for _, n := range names {
			d[n] = dl
		}
	}
	add(dialect{argPlus, false}, "vi", "vim", "nvim", "lvim", "vis", "joe", "nano", "micro", "kak")
	// Emacs can open a window or not. In a pane, it works in both cases.
	add(dialect{argPlus, false}, "emacs", "emacsclient")
	add(dialect{argSuffix, false}, "hx", "helix", "fresh")
	// MacVim and gVim open a window, unlike the other vi editors.
	add(dialect{argPlus, true}, "mvim", "gvim", "bbedit", "gedit")
	add(dialect{argGoto, true},
		"code", "code-insiders", "codium", "vscodium", "cursor", "windsurf", "positron")
	add(dialect{argSuffix, true}, "subl", "sublime_text", "zed", "zeditor", "zedit")
	add(dialect{argFlag, true}, "mate", "kate", "idea", "pycharm", "webstorm", "goland",
		"clion", "phpstorm", "rubymine", "rider", "datagrip", "rustrover", "dataspell", "fleet")
	add(dialect{argNone, true}, "open", "xdg-open")
	if runtime.GOOS == "darwin" {
		add(dialect{argFlag, true}, "xed") // the opener of Xcode
	}
	return d
}

// dialectOf returns the dialect of the program of command. An editor that
// rexdiff does not know takes +line and draws in the terminal. Most
// terminal editors take +line. A window editor also works in a pane. A
// wait flag tells that the editor has a window.
func dialectOf(command string) dialect {
	d := dialect{line: argPlus}
	if known, ok := dialects[filepath.Base(program(command))]; ok {
		d = known
	}
	words := strings.Fields(command)
	if slices.Contains(words, "--wait") || slices.Contains(words, "--block") {
		d.window = true
	}
	return d
}

// program returns the first word of command. A word in quotes can have
// spaces, for example "/Applications/Visual Studio Code.app/.../code".
func program(command string) string {
	command = strings.TrimSpace(command)
	for _, q := range []string{`"`, "'"} {
		if rest, ok := strings.CutPrefix(command, q); ok {
			word, _, _ := strings.Cut(rest, q)
			return word
		}
	}
	word, _, _ := strings.Cut(command, " ")
	return word
}

// editorCommand is a command that opens a file at a line.
type editorCommand struct {
	script string // the command, for sh -c
	window bool   // the editor opens a window, not in the terminal
}

// templateCommand returns the command that opens the file abs at line with
// template, the editor of the settings. The template gets abs in place of
// {file}, and line in place of {line}. Without {file}, abs goes at the end.
func templateCommand(template, abs string, line int) editorCommand {
	r := strings.NewReplacer("{file}", shellQuote(abs), "{line}", strconv.Itoa(line))
	script := r.Replace(template)
	if !strings.Contains(template, "{file}") {
		script += " " + shellQuote(abs)
	}
	return editorCommand{script, dialectOf(template).window}
}

// envCommand returns the command that opens the file abs at line with
// editor, the editor of the environment or of git. rexdiff adds abs and
// line in the form that the editor expects.
func envCommand(editor, abs string, line int) editorCommand {
	d := dialectOf(editor)
	n, file := strconv.Itoa(line), shellQuote(abs)
	at := shellQuote(abs + ":" + n)
	var args string
	switch d.line {
	case argNone:
		args = file
	case argPlus:
		args = "+" + n + " " + file
	case argSuffix:
		args = at
	case argGoto:
		args = "-g " + at
	case argFlag:
		args = "--line " + n + " " + file
	}
	return editorCommand{editor + " " + args, d.window}
}

// openEditor opens the file at path, at line. It uses the editor of the
// settings, else $VISUAL, $EDITOR, core.editor, or vi. A terminal editor
// opens in a new block beside rexdiff in Rex. Outside Rex, it takes the
// screen until it stops. A window editor starts in the background.
func (m *model) openEditor(path string, line int) tea.Cmd {
	abs := filepath.Join(m.repo.Root(), path)
	var ed editorCommand
	if m.cfg.Editor != "" {
		ed = templateCommand(m.cfg.Editor, abs, line)
	} else {
		editor := cmp.Or(
			strings.TrimSpace(os.Getenv("VISUAL")),
			strings.TrimSpace(os.Getenv("EDITOR")),
			m.repo.Editor(m.ctx),
			"vi",
		)
		ed = envCommand(editor, abs, line)
	}
	m.log.Debug("editor", "command", ed.script, "window", ed.window)
	if ed.window {
		return runWindowEditor(ed.script)
	}
	if m.rex != nil {
		err := m.rex.Split(m.ctx, m.repo.Root(), "sh", "-c", ed.script)
		if err == nil {
			return nil
		}
		m.log.Debug("editor split", "err", err)
	}
	done := func(err error) tea.Msg { return editorDoneMsg{err} }
	return tea.ExecProcess(exec.Command("sh", "-c", ed.script), done)
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
