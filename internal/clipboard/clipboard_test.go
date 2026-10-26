package clipboard_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rednafi/rexdiff/internal/clipboard"
	"github.com/rednafi/rexdiff/internal/testgit"
)

// fakeTool writes an executable script name in a new directory, and sets
// PATH to only that directory.
func fakeTool(t *testing.T, name, script string) string {
	t.Helper()
	dir := t.TempDir()
	testgit.Script(t, dir, name, script)
	t.Setenv("PATH", dir)
	return dir
}

// TestWrite checks that the first tool on PATH gets the text and the
// arguments.
func TestWrite(t *testing.T) {
	tests := []struct{ name, want string }{
		{"pbcopy", "hi\n"},
		{"wl-copy", "hi\n"},
		{"xclip", "-selection|clipboard|hi\n"},
		{"xsel", "--clipboard|--input|hi\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The script writes each argument and a bar, then the input.
			dir := fakeTool(t, tt.name, `{ for a; do printf '%s|' "$a"; done; /bin/cat; } > "${0%/*}/out"`+"\n")
			if err := clipboard.Write(t.Context(), "hi\n"); err != nil {
				t.Fatalf("Write: %v", err)
			}
			got, err := os.ReadFile(filepath.Join(dir, "out"))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("Write(hi) with %s: output = %q, want %q",
					tt.name, got, tt.want)
			}
		})
	}
}

// TestWriteFails checks the errors with no tool, and with a tool that
// fails.
func TestWriteFails(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if err := clipboard.Write(t.Context(), "x"); !errors.Is(err, clipboard.ErrNoTool) {
		t.Errorf("no tool: Write = %v, want ErrNoTool", err)
	}
	fakeTool(t, "pbcopy", "exit 1\n")
	err := clipboard.Write(t.Context(), "x")
	if err == nil || errors.Is(err, clipboard.ErrNoTool) {
		t.Errorf("pbcopy exits 1: Write = %v, want an error that is not ErrNoTool", err)
	}
}
