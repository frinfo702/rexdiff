package diffview

import (
	"strings"
	"testing"
)

// TestCacheShared checks that a View uses the lines that another View
// highlighted into the same Cache.
func TestCacheShared(t *testing.T) {
	render := func(c *Cache) string {
		out, _, _ := New().Cache(c).Before("a.go", "x := 1\n").After("a.go", "x := 2\n").Render()
		return out
	}
	c := &Cache{}
	render(c)
	if len(c.lines) == 0 {
		t.Fatal("the cache is empty after a render")
	}
	for k := range c.lines {
		c.lines[k] = "kept"
	}
	if got := render(c); strings.Count(got, "kept") != 2 {
		t.Errorf("a View with the same cache shows\n%s\nwant the two kept lines", got)
	}
	if got := render(&Cache{}); strings.Contains(got, "kept") {
		t.Errorf("a View with a new cache shows\n%s\nwant no kept lines", got)
	}
}

// TestCacheLimit checks that a full Cache empties, so that its memory stays
// small.
func TestCacheLimit(t *testing.T) {
	var c Cache
	line := strings.Repeat("x", 1<<20)
	for i := range maxCacheSize >> 20 {
		c.put(syntaxKey{r: uint32(i)}, line)
	}
	if got, want := len(c.lines), maxCacheSize>>20; got != want {
		t.Errorf("the cache has %d lines, want %d", got, want)
	}
	c.put(syntaxKey{source: "a"}, "")
	if len(c.lines) != 1 || c.size != 1 {
		t.Errorf("the full cache has %d lines of %d bytes after a put, want 1 of 1", len(c.lines), c.size)
	}
}

// TestCacheNil checks that a View with a nil Cache renders.
func TestCacheNil(t *testing.T) {
	if out, _, _ := New().Cache(nil).Before("a.go", "x\n").After("a.go", "y\n").Render(); out == "" {
		t.Error("the render is empty")
	}
}
