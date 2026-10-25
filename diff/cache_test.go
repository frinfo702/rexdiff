package diff

import (
	"strings"
	"testing"
)

// TestCacheKeepsFile checks that RenderFile uses the colours that the cache
// keeps for the same file, and colours a changed file again.
func TestCacheKeepsFile(t *testing.T) {
	var c Cache
	RenderFile("a.go", "x := 1\n", Options{Cache: &c})
	c.file.out = "kept"
	if got := RenderFile("a.go", "x := 1\n", Options{Cache: &c}).Lines[0]; !strings.Contains(got, "kept") {
		t.Errorf("the same file shows %q, want the kept colours", got)
	}
	if got := RenderFile("a.go", "x := 2\n", Options{Cache: &c}).Lines[0]; strings.Contains(got, "kept") {
		t.Errorf("a changed file shows %q, the colours of the old file", got)
	}
}
