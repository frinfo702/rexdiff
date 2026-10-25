package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rednafi/rexdiff/diff"
	"github.com/rednafi/rexdiff/rex"
)

// writeScript writes an executable sh script with body to dir/name, and
// returns its path.
func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// fakeRex makes m use a rex in a new directory that runs body. It returns
// the directory.
func fakeRex(t *testing.T, m *model, body string) string {
	t.Helper()
	dir := t.TempDir()
	writeScript(t, dir, "rex", body)
	t.Setenv("REX_SESSION", "S")
	t.Setenv("REX_BLOCK", "B")
	t.Setenv("REX_BIN_DIR", dir)
	m.rex = rex.FromEnv()
	return dir
}

// logRex makes m use a rex that writes its arguments to a log. It returns
// the path of the log.
func logRex(t *testing.T, m *model) string {
	t.Helper()
	dir := fakeRex(t, m, `printf '%s\n' "$*" >> "$(dirname "$0")/rex.log"`+"\n")
	return filepath.Join(dir, "rex.log")
}

// pressE moves the cursor to the inserted line 4 of a.go and presses e. It
// returns the command of the key.
func pressE(t *testing.T, m model) (model, tea.Cmd) {
	t.Helper()
	m = press(t, m, "j", "j", "j", "j", "j")
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	return *next.(*model), cmd
}

// TestTerminalEditorInRex checks that a terminal editor opens at the line
// in a new Rex block, and that e returns no command.
func TestTerminalEditorInRex(t *testing.T) {
	bin := t.TempDir()
	nvim := writeScript(t, bin, "nvim", "")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	tests := []struct {
		name, editor, env, want string
	}{
		{"settings", nvim + " +{line} {file}", "", nvim + " +4 "},
		{"environment", "", "nvim", "nvim +4 "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newTestModel(t)
			m.cfg.Editor = tt.editor
			t.Setenv("VISUAL", "")
			t.Setenv("EDITOR", tt.env)
			log := logRex(t, &m)
			m, cmd := pressE(t, m)
			if cmd != nil {
				t.Errorf("e returns a command %T, want none", cmd)
			}
			if !shows(m, "opened a.go") {
				t.Errorf("view got:\n%s\nwant \"opened a.go\"", view(m))
			}
			b, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			root := m.repo.Root()
			want := "split -s S -b B --cwd " + root + " -- sh -c " + tt.want +
				"'" + filepath.Join(root, "a.go") + "'"
			if got := strings.TrimSpace(string(b)); got != want {
				t.Errorf("rex got\n%s\nwant\n%s", got, want)
			}
		})
	}
}

// TestWindowEditor checks that a window editor gets the file and the line,
// and runs without a terminal.
func TestWindowEditor(t *testing.T) {
	m, _ := newTestModel(t)
	bin := t.TempDir()
	out := filepath.Join(bin, "out")
	code := writeScript(t, bin, "code", `{ echo "$*"; if (exec 3</dev/tty) 2>/dev/null; then echo tty; else echo no tty; fi; } > '`+out+"'\n")
	m.cfg.Editor = code + " -g {file}:{line}"
	log := logRex(t, &m)
	m, cmd := pressE(t, m)
	if cmd == nil {
		t.Fatal("e returns no command, want a command")
	}
	if msg := cmd(); msg != nil {
		t.Fatalf("the editor command = %v, want nil", msg)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(m.repo.Root(), "a.go")
	if want := "-g " + abs + ":4\nno tty\n"; string(b) != want {
		t.Errorf("the editor got\n%q\nwant\n%q", b, want)
	}
	if _, err := os.Stat(log); err == nil {
		t.Errorf("rex log %s exists, want no Rex block for a window editor",
			log)
	}
}

// TestEditorError checks that the footer tells why an editor fails.
func TestEditorError(t *testing.T) {
	code := writeScript(t, t.TempDir(), "code", "echo 'code: no such file' >&2\nexit 3\n")
	tests := []struct {
		name, editor, path, want string
	}{
		{"fails", code + " -g {file}", os.Getenv("PATH"), "editor failed: exit status 3: code: no such file"},
		{"cannot start", code + " -g {file}", "", "editor failed: "}, // sh is not in PATH
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newTestModel(t)
			m.cfg.Editor = tt.editor
			t.Setenv("PATH", tt.path)
			m, cmd := pressE(t, m)
			if cmd != nil {
				m = step(t, m, cmd())
			}
			if !shows(m, tt.want) {
				t.Errorf("view got:\n%s\nwant %q in the footer", view(m), tt.want)
			}
		})
	}
}

// TestEditorNotOpened checks the messages when rexdiff opens no editor.
func TestEditorNotOpened(t *testing.T) {
	tests := []struct {
		name, editor, env string
		remove            bool
		want              string
	}{
		{"unset", "", "", false, "no editor: set `editor` in rexdiff.lua, $EDITOR, or git's core.editor"},
		{"no program", "'' {file}", "", false, "the editor command names no program"},
		{"no program env", "", `""`, false, "the editor command names no program"},
		{"file gone", "", "nvim", true, "a.go is gone"},
		{"not found", "", "no-such-editor-x", false, "editor not found: no-such-editor-x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newTestModel(t)
			m.cfg.Editor = tt.editor
			t.Setenv("VISUAL", "")
			t.Setenv("EDITOR", tt.env)
			t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
			t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			log := logRex(t, &m)
			if tt.remove {
				if err := os.Remove(filepath.Join(m.repo.Root(), "a.go")); err != nil {
					t.Fatal(err)
				}
			}
			m, cmd := pressE(t, m)
			if cmd != nil {
				t.Errorf("e returns a command %T, want none", cmd)
			}
			if !shows(m, tt.want) {
				t.Errorf("view got:\n%s\nwant %q in the footer", view(m), tt.want)
			}
			if _, err := os.Stat(log); err == nil {
				t.Errorf("rex log %s exists, want no Rex block", log)
			}
		})
	}
}

// TestTerminalEditorDone checks the message after a terminal editor stops
// outside Rex.
func TestTerminalEditorDone(t *testing.T) {
	m, _ := newTestModel(t)
	m = step(t, m, editorDoneMsg{path: "a.go"})
	if !shows(m, "edited a.go") {
		t.Errorf("view got:\n%s\nwant \"edited a.go\"", view(m))
	}
	err := exec.Command("sh", "-c", "exit 2").Run()
	m = step(t, m, editorDoneMsg{path: "a.go", err: err})
	if !shows(m, "editor exited with exit status 2") {
		t.Errorf("view got:\n%s\nwant \"editor exited with exit status 2\"", view(m))
	}
}

// TestNewLineNear checks the line that the editor opens for each kind of
// row.
func TestNewLineNear(t *testing.T) {
	rows := []diff.Row{
		{Kind: diff.KindHunk},
		{Kind: diff.KindEqual, Old: 1, New: 1},
		{Kind: diff.KindDelete, Old: 2},
		{Kind: diff.KindInsert, New: 2},
		{Kind: diff.KindDelete, Old: 3},
	}
	for i, want := range []int{1, 1, 2, 2, 2} {
		if got := newLineNear(rows, i); got != want {
			t.Errorf("newLineNear(row %d) = %d, want %d", i, got, want)
		}
	}
	if got := newLineNear(rows[2:3], 0); got != 1 {
		t.Errorf("newLineNear(a lone deleted line) = %d, want 1", got)
	}
}
