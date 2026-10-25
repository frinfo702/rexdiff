package vcs

import "time"

// SetClock sets the clock of w. A clock that does not move turns off the
// pause between the checks.
func SetClock(w *Watcher, now func() time.Time) { w.now = now }
