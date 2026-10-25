// Package search finds the files of the search screen, and the lines of
// the find band. The limits and the smart case come from herdr-reviewr.
package search

import (
	"slices"
	"time"

	"github.com/sahilm/fuzzy"
)

// The limits of the results. A "… more" row tells that there are others.
const (
	// FileLimit is the largest number of files that the search shows.
	FileLimit = 50
	// CodeLimit is the largest number of matching lines that the search
	// shows.
	CodeLimit = 200
)

// CodeBudget is the time that a code search takes at most. After it, the
// search shows the lines that it finds, and a "+" tells that there can be
// others.
const CodeBudget = 80 * time.Millisecond

// Files returns at most limit of paths that match q, the best match
// first, and the number of all the matches. For an empty q, the paths in
// recent come first, in their order, and then the other paths.
// herdr-reviewr ranks the files of an empty query by use. rexdiff keeps
// no history, so recent has the picks of this session.
func Files(paths, recent []string, q string, limit int) (hits []string, total int) {
	if q == "" {
		for _, p := range recent {
			if slices.Contains(paths, p) && !slices.Contains(hits, p) {
				hits = append(hits, p)
			}
		}
		total = len(paths)
		for _, p := range paths {
			if len(hits) >= limit {
				break
			}
			if !slices.Contains(hits, p) {
				hits = append(hits, p)
			}
		}
		return hits[:min(len(hits), limit)], total
	}
	ms := fuzzy.Find(q, paths)
	for _, m := range ms[:min(len(ms), limit)] {
		hits = append(hits, m.Str)
	}
	return hits, len(ms)
}
