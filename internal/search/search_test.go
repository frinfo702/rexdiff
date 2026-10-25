package search

import (
	"fmt"
	"slices"
	"testing"
)

// TestFiles checks the order and the limit of the files.
func TestFiles(t *testing.T) {
	t.Parallel()
	paths := []string{"a.go", "b.go", "c/d.go", "main.go"}
	tests := []struct {
		name, q   string
		recent    []string
		limit     int
		want      []string
		wantTotal int
	}{
		{"empty query", "", nil, 50, paths, 4},
		{"recent first", "", []string{"main.go", "b.go"}, 50, []string{"main.go", "b.go", "a.go", "c/d.go"}, 4},
		{"a gone recent pick", "", []string{"gone.go", "b.go"}, 50, []string{"b.go", "a.go", "c/d.go", "main.go"}, 4},
		{"empty query limit", "", []string{"main.go"}, 2, []string{"main.go", "a.go"}, 4},
		{"fuzzy", "dgo", nil, 50, []string{"c/d.go"}, 1},
		{"recent does not rank a query", "go", []string{"main.go"}, 2, []string{"a.go", "b.go"}, 4},
		{"no match", "zzz", nil, 50, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, total := Files(paths, tt.recent, tt.q, tt.limit)
			if !slices.Equal(got, tt.want) || total != tt.wantTotal {
				t.Errorf("Files(%q, %q) = %q, %d; want %q, %d",
					tt.q, tt.recent, got, total, tt.want, tt.wantTotal)
			}
		})
	}
}

// TestFilesLimit checks that a query shows at most the limit, and counts
// all the matches.
func TestFilesLimit(t *testing.T) {
	t.Parallel()
	var paths []string
	for i := range 60 {
		paths = append(paths, fmt.Sprintf("f%02d.txt", i))
	}
	got, total := Files(paths, nil, "f", FileLimit)
	if len(got) != FileLimit || total != 60 {
		t.Errorf("Files(f) = %d paths, total %d; want %d, 60", len(got), total, FileLimit)
	}
}
