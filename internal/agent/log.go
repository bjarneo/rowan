package agent

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"rowan/internal/config"
)

// Entry is one agent call in the activity log.
type Entry struct {
	ID     string  `json:"id"`
	At     int64   `json:"at"` // unix milliseconds
	Client string  `json:"client"`
	Tool   string  `json:"tool"`
	Conn   string  `json:"conn"` // connection id, or "" for list_connections
	SQL    string  `json:"sql"`
	Rows   int64   `json:"rows"`
	Ms     float64 `json:"ms"`
	Status string  `json:"status"` // allowed, truncated, approved, denied, blocked, error
	Note   string  `json:"note"`
	Reason string  `json:"reason,omitempty"`
}

// Log keeps the newest entries in memory and appends every entry to
// activity.jsonl in the config directory.
type Log struct {
	mu      sync.Mutex
	entries []Entry // newest last
	file    *os.File
}

const logKeep = 1000

func logPath() string { return filepath.Join(config.Dir(), "activity.jsonl") }

// OpenLog loads the newest entries from disk.
func OpenLog() *Log {
	l := &Log{}
	if f, err := os.Open(logPath()); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for sc.Scan() {
			var e Entry
			if json.Unmarshal(sc.Bytes(), &e) == nil {
				l.entries = append(l.entries, e)
				if len(l.entries) > 2*logKeep {
					l.entries = append([]Entry(nil), l.entries[len(l.entries)-logKeep:]...)
				}
			}
		}
		f.Close()
	}
	if len(l.entries) > logKeep {
		l.entries = l.entries[len(l.entries)-logKeep:]
	}
	if fi, err := os.Stat(logPath()); err == nil && fi.Size() > 8<<20 {
		_ = os.Rename(logPath(), logPath()+".1")
	}
	_ = os.MkdirAll(config.Dir(), 0o700)
	if f, err := os.OpenFile(logPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
		l.file = f
	}
	return l
}

// Add appends an entry.
func (l *Log) Add(e Entry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, e)
	if len(l.entries) > 2*logKeep {
		l.entries = append([]Entry(nil), l.entries[len(l.entries)-logKeep:]...)
	}
	if l.file != nil {
		if b, err := json.Marshal(e); err == nil {
			_, _ = l.file.Write(append(b, '\n'))
		}
	}
}

// Recent returns up to n entries, newest first.
func (l *Log) Recent(n int) []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n > len(l.entries) {
		n = len(l.entries)
	}
	out := make([]Entry, n)
	for i := 0; i < n; i++ {
		out[i] = l.entries[len(l.entries)-1-i]
	}
	return out
}

// Close closes the log file.
func (l *Log) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
}
