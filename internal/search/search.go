// Package search finds the files of the search screen. The limits come
// from herdr-reviewr.
package search

import (
	"slices"

	"github.com/sahilm/fuzzy"
)

// The search shows at most these numbers of results. A "… more" row
// tells that there are others.
const (
	FileLimit = 50
	CodeLimit = 200
)

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
