package store

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
)

type Store struct {
	mu    sync.RWMutex
	items map[string]map[string][]byte
}

func New() *Store {
	return &Store{items: map[string]map[string][]byte{}}
}

func (s *Store) Put(table, key string, value []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items[table] == nil {
		s.items[table] = map[string][]byte{}
	}
	s.items[table][key] = value
}

func (s *Store) Get(table, key string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.items[table][key]
	return v, ok
}

func (s *Store) Delete(table, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items[table], key)
}

func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, t := range s.items {
		n += len(t)
	}
	return n
}

func Handler(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		table, key := r.PathValue("table"), r.PathValue("key")

		switch r.Method {
		case http.MethodPut:
			body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			if err != nil || !isJSONObject(body) {
				http.Error(w, "body must be a JSON object", http.StatusBadRequest)
				return
			}
			s.Put(table, key, body)
			w.WriteHeader(http.StatusOK)

		case http.MethodGet:
			v, ok := s.Get(table, key)
			if !ok {
				http.Error(w, "item not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(v)

		case http.MethodHead:
			if _, ok := s.Get(table, key); !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)

		case http.MethodDelete:
			s.Delete(table, key)
			w.WriteHeader(http.StatusNoContent)

		default:
			w.Header().Set("Allow", "PUT, GET, HEAD, DELETE")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func isJSONObject(b []byte) bool {
	var m map[string]any
	return json.Unmarshal(b, &m) == nil
}
