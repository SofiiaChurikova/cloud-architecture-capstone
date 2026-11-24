package metrics

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const TraceHeader = "X-Trace-Id"

var Buckets = []float64{.0005, .001, .0025, .005, .01, .02, .05, .1, .25, .5, 1, 2.5}

type Registry struct {
	mu   sync.Mutex
	list []func(io.Writer)
}

func (r *Registry) Add(f func(io.Writer)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.list = append(r.list, f)
}

func (r *Registry) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	r.mu.Lock()
	list := make([]func(io.Writer), len(r.list))
	copy(list, r.list)
	r.mu.Unlock()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	for _, f := range list {
		f(w)
	}
}

type Sample struct {
	Labels []string
	Value  float64
}

func WriteGauge(w io.Writer, name, help string, samples []Sample) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n", name, help, name)
	for _, s := range samples {
		fmt.Fprintf(w, "%s%s %s\n", name, labels(s.Labels), num(s.Value))
	}
}

func labels(kv []string) string {
	if len(kv) == 0 {
		return ""
	}
	parts := make([]string, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		v := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(kv[i+1])
		parts = append(parts, fmt.Sprintf(`%s="%s"`, kv[i], v))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func num(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

type histogram struct {
	counts []uint64
	sum    float64
	count  uint64
}

type HTTP struct {
	mu       sync.Mutex
	requests map[[2]string]uint64
	duration map[string]*histogram
	log      *slog.Logger
}

func New(reg *Registry, log *slog.Logger) *HTTP {
	m := &HTTP{requests: map[[2]string]uint64{}, duration: map[string]*histogram{}, log: log}
	reg.Add(m.write)
	return m
}

func (m *HTTP) observe(op string, status int, took time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests[[2]string{op, strconv.Itoa(status)}]++
	h := m.duration[op]
	if h == nil {
		h = &histogram{counts: make([]uint64, len(Buckets))}
		m.duration[op] = h
	}
	sec := took.Seconds()
	for i, b := range Buckets {
		if sec <= b {
			h.counts[i]++
		}
	}
	h.sum += sec
	h.count++
}

func (m *HTTP) write(w io.Writer) {
	m.mu.Lock()
	defer m.mu.Unlock()

	fmt.Fprint(w, "# HELP kv_requests_total Item requests by operation and status code.\n# TYPE kv_requests_total counter\n")
	keys := make([][2]string, 0, len(m.requests))
	for k := range m.requests {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i][0]+keys[i][1] < keys[j][0]+keys[j][1] })
	for _, k := range keys {
		fmt.Fprintf(w, "kv_requests_total%s %d\n", labels([]string{"op", k[0], "status", k[1]}), m.requests[k])
	}

	fmt.Fprint(w, "# HELP kv_request_duration_seconds How long item requests take.\n# TYPE kv_request_duration_seconds histogram\n")
	ops := make([]string, 0, len(m.duration))
	for op := range m.duration {
		ops = append(ops, op)
	}
	sort.Strings(ops)
	for _, op := range ops {
		h := m.duration[op]
		for i, b := range Buckets {
			fmt.Fprintf(w, "kv_request_duration_seconds_bucket%s %d\n", labels([]string{"op", op, "le", num(b)}), h.counts[i])
		}
		fmt.Fprintf(w, "kv_request_duration_seconds_bucket%s %d\n", labels([]string{"op", op, "le", "+Inf"}), h.count)
		fmt.Fprintf(w, "kv_request_duration_seconds_sum%s %s\n", labels([]string{"op", op}), num(h.sum))
		fmt.Fprintf(w, "kv_request_duration_seconds_count%s %d\n", labels([]string{"op", op}), h.count)
	}
}

func Op(r *http.Request) string {
	if !strings.Contains(r.URL.Path, "/items/") {
		return ""
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		return "read"
	case http.MethodPut:
		return "write"
	case http.MethodDelete:
		return "delete"
	}
	return ""
}

func quiet(r *http.Request) bool {
	switch r.URL.Path {
	case "/metrics", "/health", "/internal/log", "/internal/shards", "/internal/targets":
		return true
	}
	return false
}

func (m *HTTP) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(TraceHeader)
		if id == "" {
			id = NewTraceID()
			r.Header.Set(TraceHeader, id)
		}
		w.Header().Set(TraceHeader, id)

		sw := &statusWriter{ResponseWriter: w}
		start := time.Now()
		next.ServeHTTP(sw, r)
		took := time.Since(start)
		if sw.status == 0 {
			sw.status = http.StatusOK
		}

		if op := Op(r); op != "" {
			m.observe(op, sw.status, took)
		}
		if !quiet(r) {
			m.log.Info("request", "trace_id", id, "method", r.Method, "path", r.URL.Path,
				"status", sw.status, "ms", float64(took.Microseconds())/1000)
		}
	})
}

func NewTraceID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}
