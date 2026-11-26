package store

import (
	"encoding/json"
	"sync"
)

type Item struct {
	Value   json.RawMessage
	Version uint64
	Deleted bool
}

type Store struct {
	mu    sync.RWMutex
	items map[string]map[string]Item
}

func New() *Store {
	return &Store{items: map[string]map[string]Item{}}
}

func (s *Store) Apply(table, key string, it Item) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items[table] == nil {
		s.items[table] = map[string]Item{}
	}
	if old, ok := s.items[table][key]; ok && old.Version >= it.Version {
		return false
	}
	s.items[table][key] = it
	return true
}

func (s *Store) Purge(table, key string, version uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.items[table][key]; ok && old.Version <= version {
		delete(s.items[table], key)
		return true
	}
	return false
}

type Entry struct {
	Table string
	Key   string
	Item
}

func (s *Store) All() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Entry
	for t, items := range s.items {
		for k, it := range items {
			out = append(out, Entry{Table: t, Key: k, Item: it})
		}
	}
	return out
}

func (s *Store) Get(table, key string) (Item, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	it, ok := s.items[table][key]
	return it, ok
}

func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, t := range s.items {
		for _, it := range t {
			if !it.Deleted {
				n++
			}
		}
	}
	return n
}

func IsJSONObject(b []byte) bool {
	var m map[string]any
	return json.Unmarshal(b, &m) == nil
}
