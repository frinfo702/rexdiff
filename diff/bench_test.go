package diff_test

import (
	"strings"
	"testing"

	"github.com/rednafi/rexdiff/diff"
)

// BenchmarkRenderFile renders a file of 4000 lines that the cache keeps
// highlighted, as when the user scrolls a file view sideways.
func BenchmarkRenderFile(b *testing.B) {
	src := strings.Repeat("func f(x int) int {\n\treturn x * 2 // comment\n}\n\n", 1000)
	o := diff.Options{Width: 120, Cache: new(diff.Cache)}
	diff.RenderFile("a.go", src, o)
	b.ReportAllocs()
	for b.Loop() {
		diff.RenderFile("a.go", src, o)
	}
}
