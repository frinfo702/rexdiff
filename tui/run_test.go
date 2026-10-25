package tui

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestStartBadResume checks that a state file that Run cannot read shows
// why in the footer, and that the changes still load.
func TestStartBadResume(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, content, want string }{
		{"not JSON", "{oops", "cannot read the comments from before the update: "},
		{"missing", "", "cannot read the comments from before the update: open "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, _ := newTestModel(t)
			path := filepath.Join(t.TempDir(), "state.json")
			if tt.content != "" {
				if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			m = startModel(t.Context(), Options{Repo: m.repo, Resume: path}, defaultConfig())
			if !strings.HasPrefix(m.status, tt.want) {
				t.Errorf("status %q, want %q", m.status, tt.want+"…")
			}
			if got := paths(m); !slices.Equal(got, []string{"a.go"}) {
				t.Errorf("files %v, want [a.go]", got)
			}
			if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("stat the state file: %v, want it removed", err)
			}
		})
	}
}

// TestStartSetsLabel checks that Run labels the block of rexdiff in Rex.
func TestStartSetsLabel(t *testing.T) {
	m, _ := newTestModel(t)
	log := logRex(t, &m)
	startModel(t.Context(), Options{Repo: m.repo, Rex: m.rex}, defaultConfig())
	b, err := os.ReadFile(log)
	want := `api call -s S session.set_block_label {"block_id":"B","label":"rexdiff"}`
	if err != nil || !strings.Contains(string(b), want) {
		t.Errorf("rex calls %q (%v), want %s", b, err, want)
	}
}
