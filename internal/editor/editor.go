// Package editor makes the sh command that opens a file at a line in a
// text editor. It also tells if the editor opens a window or draws in the
// terminal.
package editor

import (
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

// Command is a command that opens a file at a line.
type Command struct {
	Script string // the command, for sh -c
	Window bool   // the editor opens a window and does not draw in the terminal
}

// Template returns the command that opens file at line with template,
// a command that the user writes. The template gets file in place of
// {file}, and line in place of {line}. Without {file}, file goes at the
// end.
func Template(template, file string, line int) Command {
	r := strings.NewReplacer("{file}", ShellQuote(file), "{line}", strconv.Itoa(line))
	script := r.Replace(template)
	if !strings.Contains(template, "{file}") {
		script += " " + ShellQuote(file)
	}
	return Command{script, dialectOf(template).window}
}

// Program returns the command that opens file at line with command, the
// command of an editor program such as the value of $EDITOR. Program adds
// file and line in the form that the editor expects.
func Program(command, file string, line int) Command {
	d := dialectOf(command)
	n, f := strconv.Itoa(line), ShellQuote(file)
	at := ShellQuote(file + ":" + n)
	var args string
	switch d.line {
	case argNone:
		args = f
	case argPlus:
		args = "+" + n + " " + f
	case argSuffix:
		args = at
	case argGoto:
		args = "-g " + at
	case argFlag:
		args = "--line " + n + " " + f
	}
	return Command{command + " " + args, d.window}
}

// ShellQuote quotes s as one word for sh.
func ShellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

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
type dialect struct {
	line   lineArg
	window bool
}

// dialects has the known editors, by program name. The table is the same
// as in herdr-reviewr. The table also lists fresh, open, and xdg-open.
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

// dialectOf returns the dialect of the program of command. An unknown
// editor takes +line and draws in the terminal. Most terminal editors take
// +line. A window editor also works in a pane. A wait flag tells that the
// editor has a window.
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
