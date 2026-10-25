package editor_test

import (
	"testing"

	"github.com/rednafi/rexdiff/internal/editor"
)

// TestCommands checks the command and the kind of each editor.
func TestCommands(t *testing.T) {
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
		var got editor.Command
		if tt.template != "" {
			got = editor.Template(tt.template, abs, 12)
		} else {
			got = editor.Program(tt.editor, abs, 12)
		}
		if got.Script != tt.script || got.Window != tt.window {
			t.Errorf("editor %q%q = %+v, want script %q, window %v",
				tt.template, tt.editor, got, tt.script, tt.window)
		}
	}
}

// TestCommandProgram checks the program of a command. An empty word in
// quotes names no program.
func TestCommandProgram(t *testing.T) {
	tests := []struct{ command, want string }{
		{"nvim", "nvim"},
		{"  /usr/bin/vim -u NONE", "/usr/bin/vim"},
		{`"/Apps/VS Code.app/bin/code" -w`, "/Apps/VS Code.app/bin/code"},
		{`'/my apps/nvim'`, "/my apps/nvim"},
		{`""`, ""},
		{"'' vim", ""},
	}
	for _, tt := range tests {
		if got := editor.Program(tt.command, "/a", 1).Program; got != tt.want {
			t.Errorf("Program(%q).Program = %q, want %q", tt.command, got, tt.want)
		}
		if got := editor.Template(tt.command, "/a", 1).Program; got != tt.want {
			t.Errorf("Template(%q).Program = %q, want %q", tt.command, got, tt.want)
		}
	}
}
