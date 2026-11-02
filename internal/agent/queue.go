package agent

import (
	"sync"
)

// Change is one column change in an approval preview.
type Change struct {
	Column string `json:"column"`
	From   string `json:"from"`
	To     string `json:"to"`
}

// Item is one write that waits for the user.
type Item struct {
	ID      string   `json:"id"`
	At      int64    `json:"at"`
	Client  string   `json:"client"`
	Conn    string   `json:"conn"`
	Reason  string   `json:"reason"`
	SQL     string   `json:"sql"`
	Impact  string   `json:"impact"`
	Changes []Change `json:"changes"`

	decide chan bool
}

// Queue holds pending writes in arrival order, newest first.
type Queue struct {
	mu    sync.Mutex
	items []*Item
}

func (q *Queue) add(it *Item) {
	q.mu.Lock()
	defer q.mu.Unlock()
	it.decide = make(chan bool, 1)
	q.items = append([]*Item{it}, q.items...)
}

// take removes an item and returns it, or nil.
func (q *Queue) take(id string) *Item {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, it := range q.items {
		if it.ID == id {
			q.items = append(q.items[:i], q.items[i+1:]...)
			return it
		}
	}
	return nil
}

// List returns a copy of the pending items.
func (q *Queue) List() []Item {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Item, len(q.items))
	for i, it := range q.items {
		out[i] = *it
		out[i].decide = nil
	}
	return out
}

// Len returns the number of pending items.
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}
