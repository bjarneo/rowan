// Package state keeps the parts of the session that are not settings:
// query history, recently opened items and editor drafts. It lives in
// state.json next to config.toml.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"rowan/internal/config"
)

// HistoryItem is one query that the user ran.
type HistoryItem struct {
	ID     string  `json:"id"`
	SQL    string  `json:"sql"`
	Conn   string  `json:"conn"`
	At     int64   `json:"at"`
	Rows   int64   `json:"rows"`
	Ms     float64 `json:"ms"`
	Status string  `json:"status"` // ok, error, blocked
}

// Recent is one item that the user opened.
type Recent struct {
	Kind   string `json:"kind"` // table, query, view
	Conn   string `json:"conn"`
	Schema string `json:"schema,omitempty"`
	Table  string `json:"table,omitempty"`
	Title  string `json:"title"`
	SQL    string `json:"sql,omitempty"`
	View   string `json:"view,omitempty"`
	At     int64  `json:"at"`
}

// State is the content of state.json.
type State struct {
	History  []HistoryItem     `json:"history"`
	Recents  []Recent          `json:"recents"`
	Drafts   map[string]string `json:"drafts"`
	LastConn string            `json:"lastConn"`
}

// Store guards the state and saves it after changes.
type Store struct {
	mu    sync.Mutex
	st    State
	timer *time.Timer
}

func path() string { return filepath.Join(config.Dir(), "state.json") }

// Load reads state.json.
func Load() *Store {
	s := &Store{st: State{Drafts: map[string]string{}}}
	if b, err := os.ReadFile(path()); err == nil {
		_ = json.Unmarshal(b, &s.st)
	}
	if s.st.Drafts == nil {
		s.st.Drafts = map[string]string{}
	}
	return s
}

// Get returns a copy of the state.
func (s *Store) Get() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.st
	out.History = append([]HistoryItem(nil), s.st.History...)
	out.Recents = append([]Recent(nil), s.st.Recents...)
	out.Drafts = map[string]string{}
	for k, v := range s.st.Drafts {
		out.Drafts[k] = v
	}
	return out
}

// AddHistory records a run. It keeps the newest 500.
func (s *Store) AddHistory(h HistoryItem) {
	s.mu.Lock()
	s.st.History = append([]HistoryItem{h}, s.st.History...)
	if len(s.st.History) > 500 {
		s.st.History = s.st.History[:500]
	}
	s.mu.Unlock()
	s.saveLater()
}

// AddRecent records an opened item and moves duplicates to the front.
func (s *Store) AddRecent(r Recent) {
	s.mu.Lock()
	out := []Recent{r}
	for _, x := range s.st.Recents {
		if x.Kind == r.Kind && x.Conn == r.Conn && x.Table == r.Table && x.Schema == r.Schema && x.SQL == r.SQL && x.View == r.View {
			continue
		}
		out = append(out, x)
	}
	if len(out) > 24 {
		out = out[:24]
	}
	s.st.Recents = out
	s.mu.Unlock()
	s.saveLater()
}

// SetDraft stores the editor text of a connection.
func (s *Store) SetDraft(conn, sql string) {
	s.mu.Lock()
	if sql == "" {
		delete(s.st.Drafts, conn)
	} else {
		s.st.Drafts[conn] = sql
	}
	s.mu.Unlock()
	s.saveLater()
}

// SetLastConn stores the active connection.
func (s *Store) SetLastConn(id string) {
	s.mu.Lock()
	changed := s.st.LastConn != id
	s.st.LastConn = id
	s.mu.Unlock()
	if changed {
		s.saveLater()
	}
}

// ForgetConn removes the data of a deleted connection.
func (s *Store) ForgetConn(id string) {
	s.mu.Lock()
	delete(s.st.Drafts, id)
	var rec []Recent
	for _, r := range s.st.Recents {
		if r.Conn != id {
			rec = append(rec, r)
		}
	}
	s.st.Recents = rec
	s.mu.Unlock()
	s.saveLater()
}

func (s *Store) saveLater() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(400*time.Millisecond, func() { _ = s.Flush() })
}

// Flush writes state.json now.
func (s *Store) Flush() error {
	s.mu.Lock()
	b, err := json.Marshal(s.st)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(config.Dir(), 0o700); err != nil {
		return err
	}
	return config.WriteFileAtomic(path(), b, 0o600)
}
