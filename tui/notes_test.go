package tui

import (
	"strings"
	"testing"
)

// TestSkippedFileNotes checks the notes in place of a binary file and of a
// file that is too large, in both tabs.
func TestSkippedFileNotes(t *testing.T) {
	tests := []struct{ content, want string }{
		{"a\x00b\n", "binary file · no line comments"},
		{strings.Repeat("x\n", 50_001), "file too large to show"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			m, dir := newTestModel(t)
			// The name sorts first, so that both tabs select the file.
			write(t, dir, "0.txt", tt.content)
			m.reload()
			m.pickFile(0)
			if !shows(m, tt.want) {
				t.Errorf("Changes got:\n%s\nwant %q", view(m), tt.want)
			}
			if m = press(t, m, "2"); !shows(m, tt.want) {
				t.Errorf("Files got:\n%s\nwant %q", view(m), tt.want)
			}
		})
	}
}
