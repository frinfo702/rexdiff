package tui

// The base picker picks the base of the branch scope.

import (
	"strings"
)

// openBasePicker lets the user pick the base of the branch scope. The
// current base has a tag. As in herdr-reviewr, it does nothing if the base
// comes from the flag.
func (m *model) openBasePicker() {
	if m.baseFlag {
		return
	}
	ov := newOverlay(overlayBase, "base branch for the branch scope (or type any ref)")
	branches, err := m.repo.Branches(m.ctx)
	if err != nil {
		m.flash("branch list failed: " + err.Error())
		return
	}
	var items []pickItem
	for _, b := range branches {
		tag := ""
		if b == m.base {
			tag = "current"
		}
		items = append(items, pickItem{label: b, id: b, tag: tag})
	}
	ov.setItems(items)
	m.open(ov)
}

// baseRef returns the ref that the base picker picks, with the selected
// item it. A typed ref, for example a tag, wins over the first match. A
// match that the user moves to wins over the typed text. If the ref is
// not valid, ok is false.
func (m *model) baseRef(it *pickItem) (ref string, ok bool) {
	ref = strings.TrimSpace(m.overlay.input.Value())
	_, err := m.repo.Resolve(m.ctx, ref)
	if it != nil && (ref == "" || err != nil || m.overlay.cur > 0) {
		ref = it.id
		_, err = m.repo.Resolve(m.ctx, ref)
	}
	return ref, ref != "" && err == nil
}
