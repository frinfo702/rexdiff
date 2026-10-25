package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rednafi/rexdiff/rex"
)

// TestEditorCommands checks the command and the kind of each editor.
func TestEditorCommands(t *testing.T) {
	const abs = "/r/a b.go"
	tests := []struct {
		template, editor string
		script           string
		window           bool
	}{
		// The editor of the settings gets the placeholders.
		{"/opt/homebrew/bin/nvim +{line} {file}", "", `/opt/homebrew/bin/nvim +12 '/r/a b.go'`, false},
		{"code -g {file}:{line}", "", `code -g '/r/a b.go':12`, true},
		{"zed {file}:{line}", "", `zed '/r/a b.go':12`, true},
		{"open -a Zed {file}", "", `open -a Zed '/r/a b.go'`, true},
		{"myeditor --at {line}", "", `myeditor --at 12 '/r/a b.go'`, false},
		// The editor of the environment gets the line as it expects it.
		{"", "nvim", `nvim +12 '/r/a b.go'`, false},
		{"", "/usr/bin/vim -u NONE", `/usr/bin/vim -u NONE +12 '/r/a b.go'`, false},
		{"", "micro", `micro +12 '/r/a b.go'`, false},
		{"", "hx", `hx '/r/a b.go:12'`, false},
		{"", "fresh", `fresh '/r/a b.go:12'`, false},
		{"", "code --wait", `code --wait -g '/r/a b.go:12'`, true},
		{"", "subl", `subl '/r/a b.go:12'`, true},
		{"", "idea", `idea --line 12 '/r/a b.go'`, true},
		{"", "mvim", `mvim +12 '/r/a b.go'`, true},
		// A path in quotes can have spaces.
		{"", `"/Apps/VS Code.app/bin/code" -w`, `"/Apps/VS Code.app/bin/code" -w -g '/r/a b.go:12'`, true},
		{"", `'/my apps/nvim'`, `'/my apps/nvim' +12 '/r/a b.go'`, false},
		// An unknown editor gets +line, and draws in the terminal.
		{"", "vim.basic", `vim.basic +12 '/r/a b.go'`, false},
		// A wait flag tells that an unknown editor has a window.
		{"", "myeditor --wait", `myeditor --wait +12 '/r/a b.go'`, true},
	}
	for _, tt := range tests {
		var got editorCommand
		if tt.template != "" {
			got = templateCommand(tt.template, abs, 12)
		} else {
			got = envCommand(tt.editor, abs, 12)
		}
		if got.script != tt.script || got.window != tt.window {
			t.Errorf("editor %q%q = %q, window %v; want %q, window %v",
				tt.template, tt.editor, got.script, got.window, tt.script, tt.window)
		}
	}
}

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
	tests := []struct {
		name, editor, env, want string
	}{
		{"settings", "/opt/homebrew/bin/nvim +{line} {file}", "", "/opt/homebrew/bin/nvim +4 "},
		{"environment", "", "nvim", "nvim +4 "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newTestModel(t)
			m.cfg.Editor = tt.editor
			t.Setenv("VISUAL", "")
			t.Setenv("EDITOR", tt.env)
			log := logRex(t, &m)
			if _, cmd := pressE(t, m); cmd != nil {
				t.Errorf("e returned a command %T, want none", cmd)
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
		t.Fatal("e returned no command")
	}
	if msg := cmd(); msg != nil {
		t.Fatalf("the editor failed: %v", msg)
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
		t.Error("a window editor opened a Rex block")
	}
}

// TestEditorError checks that the footer tells why an editor failed.
func TestEditorError(t *testing.T) {
	code := writeScript(t, t.TempDir(), "code", "echo 'code: no such file' >&2\nexit 3\n")
	tests := []struct {
		name, editor, path, want string
	}{
		{"fails", code + " -g {file}", os.Getenv("PATH"), "editor: exit status 3: code: no such file"},
		{"cannot start", "code -g {file}", "", "editor: "}, // sh is not in PATH
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newTestModel(t)
			m.cfg.Editor = tt.editor
			t.Setenv("PATH", tt.path)
			m, cmd := pressE(t, m)
			if cmd == nil {
				t.Fatal("e returned no command")
			}
			if m = step(t, m, cmd()); !strings.Contains(view(m), tt.want) {
				t.Errorf("the footer does not show %q:\n%s", tt.want, view(m))
			}
		})
	}
}
