package metrics

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFailedRequestsAreNotInLatency(t *testing.T) {
	reg := &Registry{}
	var logs bytes.Buffer
	m := New(reg, slog.New(slog.NewJSONHandler(&logs, nil)))
	h := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("fail") != "" {
			w.Header().Set("X-Shard-Id", "shard-1")
			http.Error(w, "leader shard-1-a is down", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/tables/t/items/1", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("PUT", "/tables/t/items/1?fail=1", nil))

	out := httptest.NewRecorder()
	reg.ServeHTTP(out, httptest.NewRequest("GET", "/metrics", nil))
	body := out.Body.String()
	for _, want := range []string{
		`kv_requests_total{op="read",status="200"} 1`,
		`kv_requests_total{op="write",status="503"} 1`,
		`kv_request_duration_seconds_count{op="read"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("no %q in:\n%s", want, body)
		}
	}
	if strings.Contains(body, `kv_request_duration_seconds_count{op="write"}`) {
		t.Fatal("a 503 got into the latency histogram")
	}
	if !strings.Contains(logs.String(), `"level":"ERROR"`) || !strings.Contains(logs.String(), `"error":"leader shard-1-a is down"`) ||
		!strings.Contains(logs.String(), `"shard_id":"shard-1"`) {
		t.Fatalf("error log line has no level, reason or shard:\n%s", logs.String())
	}
}

func TestBadTraceIDIsReplaced(t *testing.T) {
	m := New(&Registry{}, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	h := m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	for in, keep := range map[string]bool{"abc-123": true, strings.Repeat("a", 65): false, "a b\n{": false} {
		req := httptest.NewRequest("GET", "/health", nil)
		req.Header.Set(TraceHeader, in)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		got := rec.Header().Get(TraceHeader)
		if (got == in) != keep || got == "" {
			t.Fatalf("trace id %q -> %q", in, got)
		}
	}
}
