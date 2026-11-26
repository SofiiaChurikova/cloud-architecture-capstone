package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/SofiiaChurikova/cloud-architecture-capstone/internal/replica"
	"github.com/SofiiaChurikova/cloud-architecture-capstone/internal/ring"
)

const batchSize = 500

type migration struct {
	Kind  string
	Shard string

	next  *ring.Ring
	from  []string
	to    []string
	ready atomic.Bool

	copied atomic.Int64
	total  atomic.Int64
}

type migrationStatus struct {
	State  string           `json:"state"`
	Shard  string           `json:"shard,omitempty"`
	Copied int64            `json:"copied"`
	Total  int64            `json:"total"`
	Last   *migrationStatus `json:"last,omitempty"`
}

var errBusy = errors.New("another shard move is running")

func (s *server) status() migrationStatus {
	s.mu.RLock()
	m, last := s.mig, s.lastMove
	s.mu.RUnlock()
	if m == nil {
		return migrationStatus{State: "idle", Last: last}
	}
	return migrationStatus{State: m.Kind, Shard: m.Shard, Copied: m.copied.Load(), Total: m.total.Load()}
}

func (s *server) owners(key string) (owner, old string, ok bool) {
	s.mu.RLock()
	m := s.mig
	s.mu.RUnlock()
	cur, ok := s.ring.Get(key)
	if m == nil || !m.ready.Load() {
		return cur.ID, "", ok
	}
	nxt, okNext := m.next.Get(key)
	if !okNext {
		return cur.ID, "", ok
	}
	if !ok || cur.ID == nxt.ID {
		return nxt.ID, "", true
	}
	return nxt.ID, cur.ID, true
}

func (s *server) join(id string) {
	s.mu.Lock()
	g := s.groups[id]
	if g == nil || s.onRing(id) {
		s.mu.Unlock()
		return
	}
	leader, ok := g.leader()
	if !ok {
		s.mu.Unlock()
		return
	}
	if len(s.ring.Shards()) == 0 {
		s.ring.Add(ring.Shard{ID: id, Addr: leader.Addr})
		s.mu.Unlock()
		log.Printf("%s is on the ring", id)
		return
	}
	s.mu.Unlock()
	if err := s.startMigration("adding", id); err == nil {
		log.Printf("%s joined, moving its keys to it", id)
	}
}

func (s *server) onRing(id string) bool {
	for _, sh := range s.ring.Shards() {
		if sh.ID == id {
			return true
		}
	}
	return false
}

func (s *server) startMigration(kind, id string) error {
	s.mu.Lock()
	if s.mig != nil {
		s.mu.Unlock()
		return errBusy
	}
	cur := s.ring.Shards()
	var next []ring.Shard
	var from, to []string
	switch kind {
	case "adding":
		leader, _ := s.groups[id].leader()
		next = append(append(next, cur...), ring.Shard{ID: id, Addr: leader.Addr})
		for _, sh := range cur {
			from = append(from, sh.ID)
		}
		to = []string{id}
	case "draining":
		for _, sh := range cur {
			if sh.ID != id {
				next = append(next, sh)
				to = append(to, sh.ID)
			}
		}
		from = []string{id}
	}
	m := &migration{Kind: kind, Shard: id, next: ring.Of(next), from: from, to: to}
	s.mig = m
	s.mu.Unlock()

	go s.migrate(m)
	return nil
}

func (s *server) migrate(m *migration) {
	var floor uint64
	for _, src := range m.from {
		var v struct {
			Version uint64 `json:"version"`
		}
		s.retry(func() error { return s.call(src, http.MethodGet, "/internal/version", nil, &v) })
		floor = max(floor, v.Version)
	}
	for _, dst := range m.to {
		s.retry(func() error {
			return s.call(dst, http.MethodPost, "/internal/floor", map[string]uint64{"version": floor}, nil)
		})
	}
	m.ready.Store(true)
	time.Sleep(s.grace)

	for _, src := range m.from {
		var items []replica.Item
		s.retry(func() error { return s.call(src, http.MethodGet, "/internal/dump", nil, &items) })

		byOwner := map[string][]replica.Item{}
		var moved []replica.Item
		for _, it := range items {
			owner, ok := m.next.Get(ring.Key(it.Table, it.Key))
			if ok && owner.ID != src {
				byOwner[owner.ID] = append(byOwner[owner.ID], it)
				moved = append(moved, it)
			}
		}
		m.total.Add(int64(len(moved)))

		for dst, list := range byOwner {
			for _, batch := range batches(list) {
				s.retry(func() error {
					return s.call(dst, http.MethodPost, fmt.Sprintf("/internal/import?w=%d", s.W), batch, nil)
				})
				m.copied.Add(int64(len(batch)))
			}
		}
		if m.Kind == "adding" {
			for _, batch := range batches(moved) {
				s.retry(func() error {
					return s.call(src, http.MethodPost, fmt.Sprintf("/internal/purge?w=%d", s.W), batch, nil)
				})
			}
		}
		log.Printf("moved %d keys from %s", len(moved), src)
	}

	s.mu.Lock()
	switch m.Kind {
	case "adding":
		leader, _ := s.groups[m.Shard].leader()
		s.ring.Add(ring.Shard{ID: m.Shard, Addr: leader.Addr})
	case "draining":
		s.ring.Remove(m.Shard)
		delete(s.groups, m.Shard)
		s.removed[m.Shard] = true
	}
	s.mig = nil
	s.lastMove = &migrationStatus{State: m.Kind, Shard: m.Shard, Copied: m.copied.Load(), Total: m.total.Load()}
	s.mu.Unlock()
	log.Printf("%s %s: done, %d keys moved", m.Kind, m.Shard, m.copied.Load())
}

func batches(items []replica.Item) [][]replica.Item {
	var out [][]replica.Item
	for len(items) > 0 {
		n := min(batchSize, len(items))
		out = append(out, items[:n])
		items = items[n:]
	}
	return out
}

func (s *server) retry(f func() error) {
	for {
		err := f()
		if err == nil {
			return
		}
		log.Printf("shard move: %v, trying again", err)
		time.Sleep(s.retryWait)
	}
}

func (s *server) call(shard, method, path string, in, out any) error {
	g := s.group(shard)
	leader, ok := g.leader()
	if !ok {
		return fmt.Errorf("%s has no leader", shard)
	}
	var body bytes.Buffer
	if in != nil {
		if err := json.NewEncoder(&body).Encode(in); err != nil {
			return err
		}
	}
	req, err := http.NewRequest(method, leader.Addr+path, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.moveClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", shard, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s %s: %s", shard, path, resp.Status)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (s *server) drain(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.RLock()
	on, size := s.onRing(id), len(s.ring.Shards())
	s.mu.RUnlock()
	switch {
	case !on:
		http.Error(w, id+" is not on the ring", http.StatusNotFound)
		return
	case size <= 1:
		http.Error(w, "can't drain the last shard", http.StatusConflict)
		return
	}
	if err := s.startMigration("draining", id); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusAccepted, s.status())
}

func (s *server) migrationState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.status())
}
