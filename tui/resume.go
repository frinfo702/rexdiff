package tui

// After an update, rexdiff starts again in the same terminal. The old
// program saves the comments and the place of the user in a file, and the
// new one reads it, so that the user continues from the same place.

import (
	"encoding/json/v2"
	"os"
	"time"

	"github.com/frinfo702/rexdiff/diff"
)

// savedState holds the comments, the scope, the view, and the places in
// the panes for a restart.
type savedState struct {
	Tab   tab        `json:"tab"`
	Scope Scope      `json:"scope"`
	Base  string     `json:"base"`
	From  string     `json:"from"`
	To    string     `json:"to"`
	Turn  *savedTurn `json:"turn"`

	// The state keeps the files by path, because the lists can change in a
	// restart.
	ChangePath string    `json:"change_path"`
	FilePath   string    `json:"file_path"`
	Changes    savedPane `json:"changes"`
	Viewer     savedPane `json:"viewer"`
	PR         savedPane `json:"pr"`

	Split      bool `json:"split"`
	Wrap       bool `json:"wrap"`
	Rendered   bool `json:"rendered"`
	ListLeft   bool `json:"list_left"`
	ListHidden bool `json:"list_hidden"`
	FocusList  bool `json:"focus_list"`
	ListW      int  `json:"list_w"`

	Comments *diff.Comments `json:"comments"`
	LastSent string         `json:"last_sent"`
	Beside   string         `json:"beside"`
}

// savedTurn is a [turnState], with exported fields for JSON.
type savedTurn struct {
	Block   string    `json:"block"`
	App     string    `json:"app"`
	Tree    string    `json:"tree"`
	Running bool      `json:"running"`
	At      time.Time `json:"at"`
}

// savedPane is the place in a pane.
type savedPane struct {
	Cur  int `json:"cur"`
	Top  int `json:"top"`
	Xoff int `json:"xoff"`
}

func (p *pane) place() savedPane { return savedPane{p.cur, p.top, p.xoff} }

// restore moves p to the place s, within its lines.
func (p *pane) restore(s savedPane) {
	last := max(len(p.lines)-1, 0)
	p.cur, p.top, p.xoff = min(s.Cur, last), min(s.Top, last), s.Xoff
}

// saveState writes the state to a new file, and returns its path.
func (m *model) saveState() (string, error) {
	s := savedState{
		Tab: m.tab, Scope: m.scope, Base: m.base, From: m.from, To: m.to,
		Changes: m.changes.place(),
		Viewer:  m.viewer.place(),
		PR:      m.prPane.place(),
		Split:   m.split, Wrap: m.wrap, Rendered: m.rendered,
		ListLeft: m.listLeft, ListHidden: m.listHidden, FocusList: m.focusList,
		ListW: m.listW, Comments: &m.comments, LastSent: m.lastSent, Beside: m.beside,
	}
	if t := m.turn; t != nil {
		s.Turn = &savedTurn{t.block, t.app, t.tree, t.running, t.at}
	}
	if m.prPlace != nil { // the PR has not loaded since the last restart
		s.PR = *m.prPlace
	}
	if f := m.changedFile(); f != nil {
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
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), f.Close()
}

// loadState reads the file that saveState wrote, and removes it.
func loadState(path string) (*savedState, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	os.Remove(path)
	s := &savedState{Comments: new(diff.Comments)}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, err
	}
	return s, nil
}

// restoreSettings sets the scope, the view, and the comments of s. Call
// it before the first reload, which reads the files of the scope.
func (m *model) restoreSettings(s *savedState) {
	m.tab, m.scope, m.base, m.from, m.to = s.Tab, s.Scope, s.Base, s.From, s.To
	if t := s.Turn; t != nil {
		m.turn = &turnState{block: t.Block, app: t.App, tree: t.Tree, running: t.Running, at: t.At}
	}
	m.split, m.wrap, m.rendered = s.Split, s.Wrap, s.Rendered
	m.listLeft, m.listHidden, m.focusList, m.listW = s.ListLeft, s.ListHidden, s.FocusList, s.ListW
	if s.Comments != nil {
		m.comments = *s.Comments
	}
	m.lastSent, m.beside = s.LastSent, s.Beside
}

// restorePlaces selects the files of s and puts the cursors where they
// were. Call it after the first reload.
func (m *model) restorePlaces(s *savedState) {
	for i, f := range m.files {
		if f.Path == s.ChangePath {
			m.changeSel = i
		}
	}
	m.allFiles, _ = m.repo.Files(m.ctx) // an error shows an empty list
	for i, e := range m.allFiles {
		if e.Path == s.FilePath {
			m.allSel = i
		}
	}
	m.renderChanges()
	m.refreshViewer()
	m.changes.restore(s.Changes)
	m.viewer.restore(s.Viewer)
	m.prPlace = &s.PR // the PR loads later, and onPR restores the place
	m.rerender()
}
