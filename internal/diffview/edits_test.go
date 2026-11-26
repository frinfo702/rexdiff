package diffview

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"

	"github.com/aymanbagabas/go-udiff"
)

// TestEditsMatchUdiff checks that Edits gives the same edits as
// udiff.Lines, which it replaces. The lines come from a few values, so
// that many of them repeat.
func TestEditsMatchUdiff(t *testing.T) {
	values := []string{"", "a", "b", "}", "\tx := 1", "\r", "\xe2", "é"}
	text := func(r *rand.Rand) string {
		var b strings.Builder
		for range r.IntN(30) {
			b.WriteString(values[r.IntN(len(values))] + "\n")
		}
		if r.IntN(2) == 0 { // a last line without LF
			b.WriteString(values[r.IntN(len(values))])
		}
		return b.String()
	}
	r := rand.New(rand.NewPCG(1, 2))
	for range 10_000 {
		before, after := text(r), text(r)
		if got, want := Edits(before, after), udiff.Lines(before, after); !reflect.DeepEqual(got, want) {
			t.Fatalf("Edits(%q, %q)\n got %v\nwant %v", before, after, got, want)
		}
	}
}

// BenchmarkDiffHunks diffs two versions of a file of 100,000 lines, as
// Render does before it draws them:
//
//	go test ./internal/diffview -run '^$' -bench DiffHunks -count 10
func BenchmarkDiffHunks(b *testing.B) {
	const n = 100_000
	lines := make([]string, n)
	for i := range lines {
		switch i % 10 {
		case 0:
			lines[i] = "}"
		case 1:
			lines[i] = ""
		default:
			lines[i] = fmt.Sprintf("\tx%d := compute(%q, %d) // a line of a large file", i, strings.Repeat("a", i%40), i)
		}
	}
	before := strings.Join(lines, "\n") + "\n"

	// Edits changes one line in every 2,000.
	edited := append([]string(nil), lines...)
	for i := 1000; i < n; i += 2000 {
		edited[i] += " // changed"
	}
	// Insert adds a block of 2,000 lines in the middle.
	inserted := append(append(append([]string(nil), lines[:n/2]...), lines[:2000]...), lines[n/2:]...)

	for _, c := range []struct{ name, after string }{
		{"edits", strings.Join(edited, "\n") + "\n"},
		{"insert", strings.Join(inserted, "\n") + "\n"},
	} {
		b.Run(c.name, func(b *testing.B) {
			b.SetBytes(int64(len(before) + len(c.after)))
			b.ReportAllocs()
			for b.Loop() {
				diffHunks(before, c.after, 8, 3)
			}
		})
	}
}
