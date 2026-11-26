package replica

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/SofiiaChurikova/cloud-architecture-capstone/internal/metrics"
	"github.com/SofiiaChurikova/cloud-architecture-capstone/internal/store"
	"github.com/SofiiaChurikova/cloud-architecture-capstone/internal/wal"
)

type Config struct {
	ID     string
	Shard  string
	Leader bool
	Log    *wal.Log
}

type Node struct {
	cfg   Config
	store *store.Store

	mu    sync.Mutex
	acked map[string]uint64
	ackCh chan struct{}

	AckTimeout time.Duration
	PollWait   time.Duration
}

func New(cfg Config) *Node {
	n := &Node{
		cfg:        cfg,
		store:      store.New(),
		acked:      map[string]uint64{},
		ackCh:      make(chan struct{}),
		AckTimeout: 2 * time.Second,
		PollWait:   time.Second,
	}
	for _, e := range cfg.Log.From(1, 0) {
		n.apply(e)
	}
	return n
}

func (n *Node) Store() *store.Store { return n.store }

func (n *Node) apply(e wal.Entry) {
	if e.Purge {
		n.store.Purge(e.Table, e.Key, e.Version)
		return
	}
	n.store.Apply(e.Table, e.Key, store.Item{Value: e.Value, Version: e.Version, Deleted: e.Deleted})
}

func (n *Node) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/internal/items/{table}/{key}", n.item)
	mux.HandleFunc("GET /internal/log", n.serveLog)
	mux.HandleFunc("GET /internal/dump", n.leaderOnly(n.dump))
	mux.HandleFunc("GET /internal/version", n.leaderOnly(n.maxVersion))
	mux.HandleFunc("POST /internal/floor", n.leaderOnly(n.floor))
	mux.HandleFunc("POST /internal/import", n.leaderOnly(n.importItems))
	mux.HandleFunc("POST /internal/purge", n.leaderOnly(n.purge))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		role := "follower"
		if n.cfg.Leader {
			role = "leader"
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"replica": n.cfg.ID, "shard": n.cfg.Shard, "role": role,
			"offset": n.cfg.Log.Last(), "items": n.store.Len(),
		})
	})
}

func (n *Node) item(w http.ResponseWriter, r *http.Request) {
	table, key := r.PathValue("table"), r.PathValue("key")
	w.Header().Set("X-Replica-Id", n.cfg.ID)

	switch r.Method {
	case http.MethodGet, http.MethodHead:
		it, ok := n.store.Get(table, key)
		w.Header().Set("X-Version", strconv.FormatUint(it.Version, 10))
		if !ok || it.Deleted {
			if it.Deleted {
				w.Header().Set("X-Deleted", "true")
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			w.Write(it.Value)
		}

	case http.MethodPut, http.MethodDelete:
		if !n.cfg.Leader {
			http.Error(w, "not the leader", http.StatusConflict)
			return
		}
		need := 1
		if v := r.URL.Query().Get("w"); v != "" {
			x, err := strconv.Atoi(v)
			if err != nil || x < 1 {
				http.Error(w, "w must be a number from 1", http.StatusBadRequest)
				return
			}
			need = x
		}
		e := wal.Entry{Table: table, Key: key, Deleted: r.Method == http.MethodDelete}
		if !e.Deleted {
			body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			if err != nil || !store.IsJSONObject(body) {
				http.Error(w, "body must be a JSON object", http.StatusBadRequest)
				return
			}
			e.Value = body
		}
		e, err := n.cfg.Log.Append(e)
		if err != nil {
			http.Error(w, "log write failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		n.apply(e)
		w.Header().Set("X-Version", strconv.FormatUint(e.Version, 10))

		if got := n.waitAcks(r.Context(), e.Offset, need); got < need {
			http.Error(w, fmt.Sprintf("only %d of %d replicas confirmed", got, need), http.StatusServiceUnavailable)
			return
		}
		if e.Deleted {
			w.WriteHeader(http.StatusNoContent)
		} else {
			w.WriteHeader(http.StatusOK)
		}

	default:
		w.Header().Set("Allow", "PUT, GET, HEAD, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type Item struct {
	Table   string          `json:"table"`
	Key     string          `json:"key"`
	Value   json.RawMessage `json:"value,omitempty"`
	Version uint64          `json:"version"`
	Deleted bool            `json:"deleted,omitempty"`
}

func (n *Node) leaderOnly(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !n.cfg.Leader {
			http.Error(w, "not the leader", http.StatusConflict)
			return
		}
		h(w, r)
	}
}

func (n *Node) dump(w http.ResponseWriter, r *http.Request) {
	all := n.store.All()
	items := make([]Item, 0, len(all))
	for _, e := range all {
		items = append(items, Item{Table: e.Table, Key: e.Key, Value: e.Value, Version: e.Version, Deleted: e.Deleted})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

func (n *Node) maxVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]uint64{"version": n.cfg.Log.MaxVersion()})
}

func (n *Node) floor(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Version uint64 `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `body must be {"version": N}`, http.StatusBadRequest)
		return
	}
	n.cfg.Log.Floor(body.Version)
	w.WriteHeader(http.StatusNoContent)
}

func (n *Node) importItems(w http.ResponseWriter, r *http.Request) {
	var items []Item
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<20)).Decode(&items); err != nil {
		http.Error(w, "body must be a list of items", http.StatusBadRequest)
		return
	}
	n.writeEntries(w, r, items, false)
}

func (n *Node) purge(w http.ResponseWriter, r *http.Request) {
	var items []Item
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<20)).Decode(&items); err != nil {
		http.Error(w, "body must be a list of items", http.StatusBadRequest)
		return
	}
	n.writeEntries(w, r, items, true)
}

func (n *Node) writeEntries(w http.ResponseWriter, r *http.Request, items []Item, purge bool) {
	need := 1
	if v, err := strconv.Atoi(r.URL.Query().Get("w")); err == nil && v > 1 {
		need = v
	}
	written := 0
	var last uint64
	for _, it := range items {
		old, ok := n.store.Get(it.Table, it.Key)
		if purge {
			if !ok || old.Version > it.Version {
				continue
			}
		} else if ok && old.Version >= it.Version {
			continue
		}
		e := wal.Entry{Table: it.Table, Key: it.Key, Version: it.Version, Purge: purge}
		if !purge {
			e.Value, e.Deleted = it.Value, it.Deleted
		}
		e, err := n.cfg.Log.Import(e)
		if err != nil {
			http.Error(w, "log write failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		n.apply(e)
		last = e.Offset
		written++
	}
	if written > 0 {
		if got := n.waitAcks(r.Context(), last, need); got < need {
			http.Error(w, fmt.Sprintf("only %d of %d replicas confirmed", got, need), http.StatusServiceUnavailable)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"written": written})
}

func (n *Node) count(offset uint64) int {
	c := 1
	for _, a := range n.acked {
		if a >= offset {
			c++
		}
	}
	return c
}

func (n *Node) waitAcks(ctx context.Context, offset uint64, need int) int {
	ctx, cancel := context.WithTimeout(ctx, n.AckTimeout)
	defer cancel()
	for {
		n.mu.Lock()
		c, ch := n.count(offset), n.ackCh
		n.mu.Unlock()
		if c >= need {
			return c
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return c
		}
	}
}

func (n *Node) setAck(replica string, offset uint64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	old, ok := n.acked[replica]
	n.acked[replica] = offset
	if !ok || offset > old {
		close(n.ackCh)
		n.ackCh = make(chan struct{})
	}
}

func (n *Node) serveLog(w http.ResponseWriter, r *http.Request) {
	if !n.cfg.Leader {
		http.Error(w, "not the leader", http.StatusConflict)
		return
	}
	from, err := strconv.ParseUint(r.URL.Query().Get("from"), 10, 64)
	if err != nil || from < 1 {
		http.Error(w, "from must be a number from 1", http.StatusBadRequest)
		return
	}
	if last := n.cfg.Log.Last(); from-1 > last {
		http.Error(w, fmt.Sprintf("from=%d is past the end of my log (%d), the follower log is from another leader", from, last), http.StatusConflict)
		return
	}
	if f := r.URL.Query().Get("replica"); f != "" {
		n.setAck(f, from-1)
	}

	ctx, cancel := context.WithTimeout(r.Context(), n.PollWait)
	defer cancel()
	n.cfg.Log.Wait(ctx, from)

	entries := n.cfg.Log.From(from, 500)
	if entries == nil {
		entries = []wal.Entry{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func (n *Node) Follow(ctx context.Context, leader string, client *http.Client) {
	log.Printf("%s: resync from offset %d", n.cfg.ID, n.cfg.Log.Last()+1)
	for ctx.Err() == nil {
		from := n.cfg.Log.Last() + 1
		entries, err := n.pull(ctx, leader, from, client)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("%s: pull from %d failed: %v", n.cfg.ID, from, err)
				sleep(ctx, time.Second)
			}
			continue
		}
		if len(entries) == 0 {
			continue
		}
		if err := n.cfg.Log.AppendAt(entries); err != nil {
			log.Printf("%s: %v", n.cfg.ID, err)
			if !errors.Is(err, wal.ErrGap) {
				sleep(ctx, time.Second)
			}
			continue
		}
		for _, e := range entries {
			n.apply(e)
		}
	}
}

func (n *Node) pull(ctx context.Context, leader string, from uint64, client *http.Client) ([]wal.Entry, error) {
	url := fmt.Sprintf("%s/internal/log?from=%d&replica=%s", leader, from, n.cfg.ID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("leader answered %s", resp.Status)
	}
	var entries []wal.Entry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, err
	}
	return entries, nil
}

type Lag struct {
	Entries uint64
	Seconds float64
}

func (n *Node) Lag(now time.Time) map[string]Lag {
	n.mu.Lock()
	acked := make(map[string]uint64, len(n.acked))
	for f, a := range n.acked {
		acked[f] = a
	}
	n.mu.Unlock()

	last := n.cfg.Log.Last()
	out := make(map[string]Lag, len(acked))
	for f, a := range acked {
		var l Lag
		if a < last {
			l.Entries = last - a
			if e := n.cfg.Log.From(a+1, 1); len(e) == 1 {
				l.Seconds = max(0, now.Sub(time.UnixMicro(int64(e[0].Version))).Seconds())
			}
		}
		out[f] = l
	}
	return out
}

func (n *Node) WriteMetrics(w io.Writer) {
	metrics.WriteGauge(w, "kv_keys", "Keys this replica holds, deleted ones not counted.", []metrics.Sample{{Value: float64(n.store.Len())}})
	metrics.WriteGauge(w, "kv_log_offset", "Last offset in this replica's log.", []metrics.Sample{{Value: float64(n.cfg.Log.Last())}})
	leader := 0.0
	if n.cfg.Leader {
		leader = 1
	}
	metrics.WriteGauge(w, "kv_is_leader", "1 if this replica is the leader of its shard.", []metrics.Sample{{Value: leader}})
	if !n.cfg.Leader {
		return
	}
	lag := n.Lag(time.Now())
	followers := make([]string, 0, len(lag))
	for f := range lag {
		followers = append(followers, f)
	}
	sort.Strings(followers)
	var secs, entries []metrics.Sample
	for _, f := range followers {
		secs = append(secs, metrics.Sample{Labels: []string{"follower", f}, Value: lag[f].Seconds})
		entries = append(entries, metrics.Sample{Labels: []string{"follower", f}, Value: float64(lag[f].Entries)})
	}
	metrics.WriteGauge(w, "kv_replication_lag_seconds", "How long ago the oldest entry the follower has not confirmed was written.", secs)
	metrics.WriteGauge(w, "kv_replication_lag_entries", "How many entries the follower is behind.", entries)
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}
