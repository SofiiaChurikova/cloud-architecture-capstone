package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/SofiiaChurikova/cloud-architecture-capstone/internal/ring"
	"github.com/SofiiaChurikova/cloud-architecture-capstone/internal/store"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("coordinator listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, newServer().routes()))
}

type Table struct {
	Name         string `json:"name"`
	PartitionKey string `json:"partition_key"`
}

type server struct {
	mu     sync.RWMutex
	tables map[string]Table

	ring   *ring.Ring
	local  *store.Store
	client *http.Client
}

func newServer() *server {
	return &server{
		tables: map[string]Table{},
		ring:   ring.New(),
		local:  store.New(),
		client: &http.Client{
			Timeout:   5 * time.Second,
			Transport: &http.Transport{MaxIdleConns: 256, MaxIdleConnsPerHost: 256, IdleConnTimeout: 90 * time.Second},
		},
	}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /tables/{table}", s.createTable)
	mux.HandleFunc("GET /tables/{table}", s.getTable)
	mux.HandleFunc("/tables/{table}/items/{key}", s.item)
	mux.HandleFunc("POST /internal/shards", s.registerShard)
	mux.HandleFunc("GET /internal/shards", s.listShards)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "shards": len(s.ring.Shards())})
	})
	return mux
}

func (s *server) createTable(w http.ResponseWriter, r *http.Request) {
	var t Table
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil || t.PartitionKey == "" {
		http.Error(w, `body must be {"partition_key": "..."}`, http.StatusBadRequest)
		return
	}
	t.Name = r.PathValue("table")

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tables[t.Name]; ok {
		http.Error(w, "table already exists", http.StatusConflict)
		return
	}
	s.tables[t.Name] = t
	writeJSON(w, http.StatusOK, t)
}

func (s *server) getTable(w http.ResponseWriter, r *http.Request) {
	t, ok := s.table(r.PathValue("table"))
	if !ok {
		http.Error(w, "table not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *server) table(name string) (Table, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tables[name]
	return t, ok
}

func (s *server) item(w http.ResponseWriter, r *http.Request) {
	table, key := r.PathValue("table"), r.PathValue("key")
	if _, ok := s.table(table); !ok {
		http.Error(w, "table not found", http.StatusNotFound)
		return
	}

	shard, ok := s.ring.Get(ring.Key(table, key))
	if !ok {
		store.Handler(s.local)(w, r)
		return
	}
	s.forward(w, r, shard)
}

func (s *server) forward(w http.ResponseWriter, r *http.Request, shard ring.Shard) {
	req, err := http.NewRequestWithContext(r.Context(), r.Method, shard.Addr+r.URL.Path, r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", r.Header.Get("Content-Type"))

	resp, err := s.client.Do(req)
	if err != nil {
		http.Error(w, "shard "+shard.ID+" unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("X-Shard-Id", shard.ID)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (s *server) registerShard(w http.ResponseWriter, r *http.Request) {
	var sh ring.Shard
	if err := json.NewDecoder(r.Body).Decode(&sh); err != nil || sh.ID == "" || sh.Addr == "" {
		http.Error(w, `body must be {"id": "...", "addr": "http://..."}`, http.StatusBadRequest)
		return
	}
	s.ring.Add(sh)
	writeJSON(w, http.StatusOK, sh)
}

func (s *server) listShards(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.ring.Shards())
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
