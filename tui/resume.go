package tui

// After an update, rexdiff starts again in the same terminal. The old
// program saves the comments and the place of the user in a file. The new
// program reads the file, so that the user continues from the same place.

import (
	"encoding/json/v2"
	"os"
	"slices"
	"time"

	"github.com/rednafi/rexdiff/diff"
	"github.com/rednafi/rexdiff/vcs"
)

// resumeState holds the comments, the scope, the view, the expanded folds,
// and the places in the panes for a restart.
type resumeState struct {
	Tab   tab         `json:"tab"`
	Scope Scope       `json:"scope"`
	Base  string      `json:"base"`
	From  string      `json:"from"`
	To    string      `json:"to"`
	Turn  *resumeTurn `json:"turn"`

	// The state keeps the files by path, because the lists can change in a
	// restart.
	ChangePath string     `json:"change_path"`
	FilePath   string     `json:"file_path"`
	Changes    resumePane `json:"changes"`
	Viewer     resumePane `json:"viewer"`
	PR         resumePane `json:"pr"`

	Split    bool     `json:"split"`
	Wrap     bool     `json:"wrap"`
	Rendered bool     `json:"rendered"`
	ListPos  position `json:"list_pos"`
	// ListLeft is the place of the list in the state of an older version.
	// Then ListPos is "".
	ListLeft   bool `json:"list_left,omitzero"`
	ListHidden bool `json:"list_hidden"`
	FocusList  bool `json:"focus_list"`
	ListW      int  `json:"list_w"`
	ListPct    int  `json:"list_pct,omitzero"`

	Folds map[string]*openFolds `json:"folds"`

	Comments *diff.Comments `json:"comments"`
	LastSent string         `json:"last_sent"`
	Beside   string         `json:"beside"`
}

// resumeTurn is a [turnState], with exported fields for JSON.
type resumeTurn struct {
	Block   string    `json:"block"`
	App     string    `json:"app"`
	Tree    string    `json:"tree"`
	Running bool      `json:"running"`
	At      time.Time `json:"at"`
}

// resumePane is the place in a pane.
type resumePane struct {
	Cur  int `json:"cur"`
	Top  int `json:"top"`
	Xoff int `json:"xoff"`
}

// place returns the place of p, for the resume state.
func (p *pane) place() resumePane { return resumePane{p.cur, p.top, p.xoff} }

// restore moves p to the place s, within its lines.
func (p *pane) restore(s resumePane) {
	last := max(len(p.lines)-1, 0)
	p.cur, p.top, p.xoff = min(s.Cur, last), min(s.Top, last), s.Xoff
}

// saveResume writes the state to a new file, and returns its path.
func (m *model) saveResume() (string, error) {
	s := resumeState{
		Tab: m.tab, Scope: m.scope, Base: m.base, From: m.from, To: m.to,
		Changes: m.changePane.place(),
		Viewer:  m.allPane.place(),
		PR:      m.prPane.place(),
		Split:   m.split, Wrap: m.wrap, Rendered: m.rendered,
		ListPos: m.listPos, ListHidden: m.listHidden, FocusList: m.focusList,
		ListW: m.listW, ListPct: m.listPct, Folds: m.folds, Comments: &m.comments, LastSent: m.lastSent, Beside: m.beside,
	}
	if t := m.turn; t != nil {
		s.Turn = &resumeTurn{t.block, t.app, t.tree, t.running, t.at}
	}
	if m.prPlace != nil { // the PR has not loaded since the last restart
		s.PR = *m.prPlace
	}
	if f := m.selectedChange(); f != nil {
		s.ChangePath = f.Path
	}
	if m.allSel < len(m.allFiles) {
		s.FilePath = m.allFiles[m.allSel].Path
	}
	b, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp("", "rexdiff-state-*.json")
	if err != nil {
		return "", err
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), f.Close()
}

// loadResume reads the file that saveResume wrote, and removes it.
func loadResume(path string) (*resumeState, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// A file that stays in the temporary directory does no harm.
	_ = os.Remove(path)
	s := &resumeState{Comments: new(diff.Comments)}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, err
	}
	return s, nil
}

// restoreSettings sets the scope, the view, and the comments of s. Call
// it before the first reload, which reads the files of the scope.
func (m *model) restoreSettings(s *resumeState) {
	m.tab, m.scope, m.base, m.from, m.to = s.Tab, s.Scope, s.Base, s.From, s.To
	if t := s.Turn; t != nil {
		m.turn = &turnState{block: t.Block, app: t.App, tree: t.Tree, running: t.Running, at: t.At}
	}
	m.split, m.wrap, m.rendered = s.Split, s.Wrap, s.Rendered
	m.listPos, m.listHidden, m.focusList, m.listW = s.ListPos, s.ListHidden, s.FocusList, s.ListW
	switch {
	case m.listPos != "":
	case s.ListLeft:
		m.listPos = positionLeft
	default:
		m.listPos = positionRight
	}
	if s.ListPct > 0 {
		m.listPct = s.ListPct
	}
	m.folds = s.Folds
	if s.Comments != nil {
		m.comments = *s.Comments
		m.comments.SetOldRev(m.diffRev())
	}
	m.lastSent, m.beside = s.LastSent, s.Beside
}

// restorePlaces selects the files of s and moves the cursors to the places
// in s. Call it after the first reload.
func (m *model) restorePlaces(s *resumeState) {
	if i := slices.IndexFunc(m.changeFiles, func(f changedFile) bool { return f.Path == s.ChangePath }); i >= 0 {
		m.changeSel = i
	}
	m.allFiles, _ = m.repo.Files(m.ctx) // an error shows an empty list
	if i := slices.IndexFunc(m.allFiles, func(e vcs.Entry) bool { return e.Path == s.FilePath }); i >= 0 {
		m.allSel = i
	}
	// The cursors go to the selected files, and the directories above
	// them expand.
	m.changeTree.reveal(s.ChangePath)
	m.allTree.reveal(s.FilePath)
	m.changeTree.cur, m.allTree.cur = -1, -1
	m.buildTrees()
	m.renderChanges()
	m.refreshViewer()
	m.changePane.restore(s.Changes)
	m.allPane.restore(s.Viewer)
	m.prPlace = &s.PR // the PR loads later, and onPR restores the place
	m.rerender()
}
