package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SofiiaChurikova/cloud-architecture-capstone/internal/metrics"
	"github.com/SofiiaChurikova/cloud-architecture-capstone/internal/ring"
	"github.com/SofiiaChurikova/cloud-architecture-capstone/internal/store"
)

const N = 3

var version = "dev"

func main() {
	port := env("PORT", "8080")
	internalPort := env("INTERNAL_PORT", "9080")
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "coordinator"))
	s := newServer()
	s.token = os.Getenv("API_TOKEN")
	s.W = envInt("W", 2)
	s.R = envInt("R", 2)
	if v := os.Getenv("ALLOWED_SHARDS"); v != "" {
		s.allowed = strings.Split(v, ",")
	}
	if v := os.Getenv("EXPECTED_SHARDS"); v != "" {
		s.expected = strings.Split(v, ",")
	}
	if dir := os.Getenv("DATA_DIR"); dir != "" {
		if err := s.loadTables(dir); err != nil {
			log.Fatal(err)
		}
	}
	log.Printf("coordinator listening on :%s, N=%d W=%d R=%d, expected shards %v, %d tables, token required: %v", port, N, s.W, s.R, s.expected, len(s.tables), s.token != "")
	go func() { log.Fatal(http.ListenAndServe(":"+internalPort, s.internalRoutes())) }()
	log.Fatal(http.ListenAndServe(":"+port, s.routes()))
}

type Table struct {
	Name         string `json:"name"`
	PartitionKey string `json:"partition_key"`
}

type Replica struct {
	ID      string `json:"id"`
	Replica string `json:"replica"`
	Role    string `json:"role"`
	Addr    string `json:"addr"`
	Offset  uint64 `json:"offset"`
	Alive   bool   `json:"alive"`

	seen time.Time
}

type group struct {
	ID       string    `json:"id"`
	Replicas []Replica `json:"replicas"`
}

func (g *group) leader() (Replica, bool) {
	for _, r := range g.Replicas {
		if r.Role == "leader" {
			return r, true
		}
	}
	return Replica{}, false
}

func (s *server) alive(r Replica) bool {
	return time.Since(r.seen) < s.deadAfter
}

type server struct {
	W, R      int
	expected  []string
	allowed   []string
	deadAfter time.Duration
	tablesAt  string
	token     string

	mu     sync.RWMutex
	tables map[string]Table
	groups map[string]*group

	ring     *ring.Ring
	local    *store.Store
	localVer atomic.Uint64
	next     atomic.Uint64
	client   *http.Client

	reg     *metrics.Registry
	metrics *metrics.HTTP

	mig        *migration
	lastMove   *migrationStatus
	removed    map[string]bool
	grace      time.Duration
	retryWait  time.Duration
	moveClient *http.Client
}

func newServer() *server {
	s := &server{
		W:         2,
		R:         2,
		deadAfter: 6 * time.Second,
		tables:    map[string]Table{},
		groups:    map[string]*group{},
		ring:      ring.New(),
		local:     store.New(),
		client: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				DialContext:         (&net.Dialer{Timeout: 300 * time.Millisecond}).DialContext,
				MaxIdleConns:        256,
				MaxIdleConnsPerHost: 256,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		reg:        &metrics.Registry{},
		removed:    map[string]bool{},
		grace:      10 * time.Second,
		retryWait:  time.Second,
		moveClient: &http.Client{Timeout: time.Minute},
	}
	s.reg.Add(s.writeMetrics)
	s.metrics = metrics.New(s.reg, slog.Default())
	return s
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /tables", s.auth(s.listTables))
	mux.Handle("PUT /tables/{table}", s.auth(s.createTable))
	mux.Handle("GET /tables/{table}", s.auth(s.getTable))
	mux.Handle("/tables/{table}/items/{key}", s.auth(s.item))
	mux.Handle("POST /admin/shards/{id}/drain", s.auth(s.drain))
	mux.Handle("GET /admin/migration", s.auth(s.migrationState))
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /version", s.versionInfo)
	return s.metrics.Wrap(mux)
}

func (s *server) internalRoutes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/shards", s.registerShard)
	mux.HandleFunc("GET /internal/shards", s.listShards)
	mux.HandleFunc("GET /internal/targets", s.targets)
	mux.Handle("GET /metrics", s.reg)
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /version", s.versionInfo)
	return mux
}

func (s *server) versionInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"service": "coordinator", "version": version})
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	replicas := 0
	for _, g := range s.groups {
		replicas += len(g.Replicas)
	}
	s.mu.RUnlock()
	shards := len(s.ring.Shards())
	move := s.status()
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "shards": shards, "replicas": replicas, "n": N, "w": s.W, "r": s.R, "migration": move.State, "move": move, "version": version})
}

func (s *server) auth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.token != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.token)) != 1 {
			http.Error(w, "missing or wrong token", http.StatusUnauthorized)
			return
		}
		next(w, r)
	})
}

func (s *server) listTables(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	out := make([]Table, 0, len(s.tables))
	for _, t := range s.tables {
		out = append(out, t)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, http.StatusOK, out)
}

type targetGroup struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

func (s *server) targets(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	out := []targetGroup{}
	for _, g := range s.groups {
		for _, rep := range g.Replicas {
			u, err := url.Parse(rep.Addr)
			if err != nil || u.Host == "" {
				continue
			}
			out = append(out, targetGroup{Targets: []string{u.Host}, Labels: map[string]string{"shard": g.ID, "replica": rep.Replica}})
		}
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Labels["replica"] < out[j].Labels["replica"] })
	writeJSON(w, http.StatusOK, out)
}

func (s *server) writeMetrics(w io.Writer) {
	s.mu.RLock()
	ids := make([]string, 0, len(s.groups))
	for id := range s.groups {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	samples := make([]metrics.Sample, 0, len(ids))
	for _, id := range ids {
		alive := 0
		for _, rep := range s.groups[id].Replicas {
			if s.alive(rep) {
				alive++
			}
		}
		samples = append(samples, metrics.Sample{Labels: []string{"shard", id}, Value: float64(alive)})
	}
	s.mu.RUnlock()
	metrics.WriteGauge(w, "kv_replicas_alive", "Replicas of a shard that sent a heartbeat in the last 6 s.", samples)

	var ringSamples []metrics.Sample
	for _, sh := range s.ring.Shards() {
		ringSamples = append(ringSamples, metrics.Sample{Labels: []string{"shard", sh.ID}, Value: 1})
	}
	metrics.WriteGauge(w, "kv_ring_shards", "Shards on the ring (1 per shard).", ringSamples)
	st := s.status()
	moving := 0.0
	if st.State != "idle" {
		moving = 1
	}
	metrics.WriteGauge(w, "kv_migration_in_progress", "1 while a shard is being added or drained.", []metrics.Sample{{Value: moving}})
	metrics.WriteGauge(w, "kv_migration_keys", "Keys to move and keys moved in the current shard move.", []metrics.Sample{
		{Labels: []string{"kind", "total"}, Value: float64(st.Total)},
		{Labels: []string{"kind", "copied"}, Value: float64(st.Copied)},
	})
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
	if err := s.saveTables(); err != nil {
		delete(s.tables, t.Name)
		http.Error(w, "could not save the table: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *server) loadTables(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	s.tablesAt = filepath.Join(dir, "tables.json")
	b, err := os.ReadFile(s.tablesAt)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &s.tables)
}

func (s *server) saveTables() error {
	if s.tablesAt == "" {
		return nil
	}
	b, err := json.Marshal(s.tables)
	if err != nil {
		return err
	}
	tmp := s.tablesAt + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.tablesAt)
}

func (s *server) missingShards() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []string
	for _, id := range s.expected {
		g := s.groups[id]
		if g == nil {
			out = append(out, id)
			continue
		}
		if _, ok := g.leader(); !ok {
			out = append(out, id)
		}
	}
	return out
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

func (s *server) group(id string) group {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g := s.groups[id]
	if g == nil {
		return group{ID: id}
	}
	return group{ID: g.ID, Replicas: append([]Replica(nil), g.Replicas...)}
}

func (s *server) item(w http.ResponseWriter, r *http.Request) {
	table, key := r.PathValue("table"), r.PathValue("key")
	if _, ok := s.table(table); !ok {
		http.Error(w, "table not found", http.StatusNotFound)
		return
	}
	wq, err1 := quorum(r, "w", s.W)
	rq, err2 := quorum(r, "r", s.R)
	if err1 != nil || err2 != nil {
		http.Error(w, fmt.Sprintf("w and r must be from 1 to %d", N), http.StatusBadRequest)
		return
	}

	if missing := s.missingShards(); len(missing) > 0 {
		http.Error(w, "waiting for shards: "+strings.Join(missing, ", "), http.StatusServiceUnavailable)
		return
	}
	owner, old, ok := s.owners(ring.Key(table, key))
	if !ok {
		s.localItem(w, r, table, key)
		return
	}
	g := s.group(owner)
	w.Header().Set("X-Shard-Id", g.ID)

	switch r.Method {
	case http.MethodPut, http.MethodDelete:
		s.write(w, r, g, wq)
	case http.MethodGet, http.MethodHead:
		s.read(w, r, g, rq, old)
	default:
		w.Header().Set("Allow", "PUT, GET, HEAD, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func quorum(r *http.Request, name string, def int) (int, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def, nil
	}
	x, err := strconv.Atoi(v)
	if err != nil || x < 1 || x > N {
		return 0, fmt.Errorf("bad %s", name)
	}
	return x, nil
}

func (s *server) write(w http.ResponseWriter, r *http.Request, g group, wq int) {
	leader, ok := g.leader()
	if !ok {
		http.Error(w, "no leader for "+g.ID, http.StatusServiceUnavailable)
		return
	}
	if !s.alive(leader) {
		http.Error(w, "leader "+leader.Replica+" is down", http.StatusServiceUnavailable)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, itemURL(leader.Addr, r)+fmt.Sprintf("?w=%d", wq), r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	req.Header.Set(metrics.TraceHeader, r.Header.Get(metrics.TraceHeader))

	resp, err := s.client.Do(req)
	if err != nil {
		http.Error(w, "leader "+leader.Replica+" unavailable", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("X-Replicas", leader.Replica)
	if v := resp.Header.Get("X-Version"); v != "" {
		w.Header().Set("X-Version", v)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

type answer struct {
	replica string
	status  int
	version uint64
	body    []byte
	err     error
}

func (s *server) ask(r *http.Request, rep Replica) answer {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, itemURL(rep.Addr, r), nil)
	if err != nil {
		return answer{replica: rep.Replica, err: err}
	}
	req.Header.Set(metrics.TraceHeader, r.Header.Get(metrics.TraceHeader))
	resp, err := s.client.Do(req)
	if err != nil {
		return answer{replica: rep.Replica, err: err}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return answer{replica: rep.Replica, err: err}
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return answer{replica: rep.Replica, err: fmt.Errorf("status %d", resp.StatusCode)}
	}
	v, _ := strconv.ParseUint(resp.Header.Get("X-Version"), 10, 64)
	return answer{replica: rep.Replica, status: resp.StatusCode, version: v, body: body}
}

func itemURL(addr string, r *http.Request) string {
	return addr + "/internal/items/" + url.PathEscape(r.PathValue("table")) + "/" + url.PathEscape(r.PathValue("key"))
}

func (s *server) read(w http.ResponseWriter, r *http.Request, g group, rq int, old string) {
	best, ids, err := s.collect(r, g, rq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if old != "" && best.status == http.StatusNotFound && best.version == 0 {
		b2, ids2, err := s.collect(r, s.group(old), rq)
		if err != nil {
			http.Error(w, "key is moving, old shard: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		best, ids = b2, append(ids, ids2...)
		w.Header().Set("X-Shard-Id", old)
		w.Header().Set("X-Moving-From", old)
	}
	w.Header().Set("X-Replicas", strings.Join(ids, ","))
	w.Header().Set("X-Version", strconv.FormatUint(best.version, 10))

	if best.status != http.StatusOK {
		if r.Method == http.MethodGet {
			http.Error(w, "item not found", http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		w.Write(best.body)
	}
}

func (s *server) collect(r *http.Request, g group, rq int) (answer, []string, error) {
	n := len(g.Replicas)
	if n < rq {
		return answer{}, nil, fmt.Errorf("%s has %d replicas, r=%d", g.ID, n, rq)
	}
	start := int(s.next.Add(1) % uint64(n))
	order := append(append([]Replica(nil), g.Replicas[start:]...), g.Replicas[:start]...)
	sort.SliceStable(order, func(i, j int) bool { return s.alive(order[i]) && !s.alive(order[j]) })

	results := make(chan answer, n)
	launched := 0
	launch := func() {
		rep := order[launched]
		launched++
		go func() { results <- s.ask(r, rep) }()
	}
	for launched < rq {
		launch()
	}

	var got []answer
	pending := rq
	for pending > 0 && len(got) < rq {
		a := <-results
		pending--
		if a.err != nil {
			if launched < n {
				launch()
				pending++
			}
			continue
		}
		got = append(got, a)
	}
	if len(got) < rq {
		return answer{}, nil, fmt.Errorf("only %d of %d replicas answered", len(got), rq)
	}

	best := got[0]
	ids := make([]string, 0, len(got))
	for _, a := range got {
		ids = append(ids, a.replica)
		if a.version > best.version {
			best = a
		}
	}
	return best, ids, nil
}

func (s *server) localItem(w http.ResponseWriter, r *http.Request, table, key string) {
	switch r.Method {
	case http.MethodPut, http.MethodDelete:
		it := store.Item{Version: s.localVer.Add(1), Deleted: r.Method == http.MethodDelete}
		if !it.Deleted {
			body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			if err != nil || !store.IsJSONObject(body) {
				http.Error(w, "body must be a JSON object", http.StatusBadRequest)
				return
			}
			it.Value = body
		}
		s.local.Apply(table, key, it)
		w.Header().Set("X-Version", strconv.FormatUint(it.Version, 10))
		if it.Deleted {
			w.WriteHeader(http.StatusNoContent)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	case http.MethodGet, http.MethodHead:
		it, ok := s.local.Get(table, key)
		w.Header().Set("X-Version", strconv.FormatUint(it.Version, 10))
		if !ok || it.Deleted {
			if r.Method == http.MethodGet {
				http.Error(w, "item not found", http.StatusNotFound)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			w.Write(it.Value)
		}
	default:
		w.Header().Set("Allow", "PUT, GET, HEAD, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *server) registerShard(w http.ResponseWriter, r *http.Request) {
	var rep Replica
	if err := json.NewDecoder(r.Body).Decode(&rep); err != nil || !validID.MatchString(rep.ID) || !validAddr(rep.Addr) {
		http.Error(w, `body must be {"id": "shard-1", "replica": "shard-1-a", "role": "leader", "addr": "http://..."}`, http.StatusBadRequest)
		return
	}
	if rep.Replica == "" {
		rep.Replica = rep.ID
	}
	if !validID.MatchString(rep.Replica) {
		http.Error(w, "replica must match "+validID.String(), http.StatusBadRequest)
		return
	}
	if rep.Role == "" {
		rep.Role = "leader"
	}
	if rep.Role != "leader" && rep.Role != "follower" {
		http.Error(w, "role must be leader or follower", http.StatusBadRequest)
		return
	}
	if len(s.allowed) > 0 && !slices.Contains(s.allowed, rep.ID) {
		http.Error(w, rep.ID+" is not in ALLOWED_SHARDS", http.StatusForbidden)
		return
	}
	rep.Alive = true
	rep.seen = time.Now()

	s.mu.Lock()
	if s.removed[rep.ID] {
		if rep.Offset > 0 {
			s.mu.Unlock()
			http.Error(w, rep.ID+" was removed; only new replicas with an empty log can join as "+rep.ID, http.StatusGone)
			return
		}
		delete(s.removed, rep.ID)
	}
	g := s.groups[rep.ID]
	if g == nil {
		g = &group{ID: rep.ID}
		s.groups[rep.ID] = g
	}
	found := false
	for i := range g.Replicas {
		if g.Replicas[i].Replica == rep.Replica {
			g.Replicas[i] = rep
			found = true
		}
	}
	if !found {
		g.Replicas = append(g.Replicas, rep)
		sort.Slice(g.Replicas, func(i, j int) bool { return g.Replicas[i].Replica < g.Replicas[j].Replica })
	}
	s.mu.Unlock()

	s.join(rep.ID)
	writeJSON(w, http.StatusOK, rep)
}

var validID = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

func validAddr(addr string) bool {
	u, err := url.Parse(addr)
	return err == nil && u.Scheme == "http" && u.Host != "" && (u.Path == "" || u.Path == "/")
}

func (s *server) listShards(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	out := make([]group, 0, len(s.groups))
	for _, g := range s.groups {
		reps := append([]Replica(nil), g.Replicas...)
		for i := range reps {
			reps[i].Alive = s.alive(reps[i])
		}
		out = append(out, group{ID: g.ID, Replicas: reps})
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	writeJSON(w, http.StatusOK, out)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func envInt(name string, def int) int {
	v, err := strconv.Atoi(os.Getenv(name))
	if err != nil || v < 1 || v > N {
		return def
	}
	return v
}
