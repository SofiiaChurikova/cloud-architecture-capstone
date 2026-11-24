package replica

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SofiiaChurikova/cloud-architecture-capstone/internal/wal"
)

func start(t *testing.T, n *Node) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	n.Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func memLog() *wal.Log {
	l, _ := wal.Open("")
	l.Now = func() time.Time { return time.UnixMicro(0) }
	return l
}

func put(t *testing.T, url, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, url, strings.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timeout waiting for " + what)
}

func TestFollowersGetEveryWrite(t *testing.T) {
	leader := New(Config{ID: "s1-a", Shard: "s1", Leader: true, Log: memLog()})
	lsrv := start(t, leader)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var followers []*Node
	for _, id := range []string{"s1-b", "s1-c"} {
		f := New(Config{ID: id, Shard: "s1", Log: memLog()})
		go f.Follow(ctx, lsrv.URL, http.DefaultClient)
		followers = append(followers, f)
	}

	for i := 0; i < 20; i++ {
		resp := put(t, fmt.Sprintf("%s/internal/items/users/%d?w=3", lsrv.URL, i), `{"v":1}`)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("write %d with w=3: %d", i, resp.StatusCode)
		}
	}
	for _, f := range followers {
		if got := f.Store().Len(); got != 20 {
			t.Fatalf("%s has %d items after w=3 writes, want 20", f.cfg.ID, got)
		}
		if f.cfg.Log.Last() != 20 {
			t.Fatalf("%s offset %d, want 20", f.cfg.ID, f.cfg.Log.Last())
		}
	}
}

func TestWriteWaitsForW(t *testing.T) {
	leader := New(Config{ID: "s1-a", Shard: "s1", Leader: true, Log: memLog()})
	leader.AckTimeout = 100 * time.Millisecond
	lsrv := start(t, leader)

	if resp := put(t, lsrv.URL+"/internal/items/users/1?w=1", `{"v":1}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("w=1 without followers: %d, want 200", resp.StatusCode)
	}
	resp := put(t, lsrv.URL+"/internal/items/users/2?w=2", `{"v":1}`)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("w=2 without followers: %d, want 503", resp.StatusCode)
	}
	if resp.Header.Get("X-Version") != "2" {
		t.Fatalf("X-Version %q, want 2", resp.Header.Get("X-Version"))
	}
}

func TestFollowerRefusesWrites(t *testing.T) {
	f := New(Config{ID: "s1-b", Shard: "s1", Log: memLog()})
	srv := start(t, f)
	if resp := put(t, srv.URL+"/internal/items/users/1", `{"v":1}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("write to follower: %d, want 409", resp.StatusCode)
	}
}

func TestDeleteIsANewVersion(t *testing.T) {
	leader := New(Config{ID: "s1-a", Shard: "s1", Leader: true, Log: memLog()})
	srv := start(t, leader)
	put(t, srv.URL+"/internal/items/users/1", `{"v":1}`)
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/internal/items/users/1", nil)
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || resp.Header.Get("X-Version") != "2" {
		t.Fatalf("delete: %d, version %q", resp.StatusCode, resp.Header.Get("X-Version"))
	}
	get, _ := http.Get(srv.URL + "/internal/items/users/1")
	get.Body.Close()
	if get.StatusCode != http.StatusNotFound || get.Header.Get("X-Version") != "2" || get.Header.Get("X-Deleted") != "true" {
		t.Fatalf("read after delete: %d, version %q, deleted %q", get.StatusCode, get.Header.Get("X-Version"), get.Header.Get("X-Deleted"))
	}
}

func TestFollowerResyncsFromItsOffset(t *testing.T) {
	leader := New(Config{ID: "s1-a", Shard: "s1", Leader: true, Log: memLog()})
	mux := http.NewServeMux()
	leader.Routes(mux)
	var mu sync.Mutex
	var froms []string
	lsrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/internal/log" {
			mu.Lock()
			froms = append(froms, r.URL.Query().Get("from"))
			mu.Unlock()
		}
		mux.ServeHTTP(w, r)
	}))
	defer lsrv.Close()

	path := filepath.Join(t.TempDir(), "log.jsonl")
	l, _ := wal.Open(path)
	f := New(Config{ID: "s1-b", Shard: "s1", Log: l})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { f.Follow(ctx, lsrv.URL, http.DefaultClient); close(done) }()

	for i := 1; i <= 5; i++ {
		put(t, fmt.Sprintf("%s/internal/items/users/%d?w=2", lsrv.URL, i), `{"v":1}`)
	}
	cancel()
	<-done
	l.Close()

	for i := 6; i <= 10; i++ {
		put(t, fmt.Sprintf("%s/internal/items/users/%d", lsrv.URL, i), `{"v":1}`)
	}

	l, _ = wal.Open(path)
	f = New(Config{ID: "s1-b", Shard: "s1", Log: l})
	if got := f.Store().Len(); got != 5 {
		t.Fatalf("after restart the follower rebuilt %d items from its file, want 5", got)
	}
	mu.Lock()
	froms = nil
	mu.Unlock()

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	go f.Follow(ctx, lsrv.URL, http.DefaultClient)
	waitFor(t, "follower to catch up", func() bool { return f.Store().Len() == 10 })

	mu.Lock()
	defer mu.Unlock()
	if len(froms) == 0 || froms[0] != "6" {
		t.Fatalf("first pull after restart asked from=%v, want 6", froms)
	}
}

func TestLagGrowsWhenFollowerStops(t *testing.T) {
	leader := New(Config{ID: "s1-a", Shard: "s1", Leader: true, Log: memLog()})
	leader.cfg.Log.Now = time.Now
	lsrv := start(t, leader)
	f := New(Config{ID: "s1-b", Shard: "s1", Log: memLog()})
	ctx, cancel := context.WithCancel(context.Background())
	go f.Follow(ctx, lsrv.URL, http.DefaultClient)

	put(t, lsrv.URL+"/internal/items/users/1?w=2", `{"v":1}`)
	waitFor(t, "follower to confirm", func() bool { return leader.Lag(time.Now())["s1-b"].Entries == 0 })

	cancel()
	time.Sleep(50 * time.Millisecond)
	put(t, lsrv.URL+"/internal/items/users/2", `{"v":1}`)
	put(t, lsrv.URL+"/internal/items/users/3", `{"v":1}`)

	lag := leader.Lag(time.Now().Add(3 * time.Second))["s1-b"]
	if lag.Entries != 2 || lag.Seconds < 3 {
		t.Fatalf("lag %+v, want 2 entries and at least 3 s", lag)
	}
}

func TestMetrics(t *testing.T) {
	leader := New(Config{ID: "s1-a", Shard: "s1", Leader: true, Log: memLog()})
	lsrv := start(t, leader)
	put(t, lsrv.URL+"/internal/items/users/1", `{"v":1}`)
	put(t, lsrv.URL+"/internal/items/users/2", `{"v":1}`)
	leader.setAck("s1-b", 1)

	var b strings.Builder
	leader.WriteMetrics(&b)
	for _, want := range []string{"kv_keys 2\n", "kv_log_offset 2\n", "kv_is_leader 1\n", `kv_replication_lag_entries{follower="s1-b"} 1`} {
		if !strings.Contains(b.String(), want) {
			t.Fatalf("metrics have no %q:\n%s", want, b.String())
		}
	}
}

func TestAckPastTheLogIsRefused(t *testing.T) {
	leader := New(Config{ID: "s1-a", Shard: "s1", Leader: true, Log: memLog()})
	leader.AckTimeout = 100 * time.Millisecond
	srv := start(t, leader)
	for _, name := range []string{"ghost1", "ghost2"} {
		resp, err := http.Get(srv.URL + "/internal/log?replica=" + name + "&from=1000000000")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("from past the log: %d, want 409", resp.StatusCode)
		}
	}
	if resp := put(t, srv.URL+"/internal/items/users/1?w=3", `{"v":1}`); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("w=3 without real followers: %d, want 503", resp.StatusCode)
	}
}

func TestReplacedFollowerStartsFromZero(t *testing.T) {
	leader := New(Config{ID: "s1-a", Shard: "s1", Leader: true, Log: memLog()})
	for i := 0; i < 3; i++ {
		leader.cfg.Log.Append(wal.Entry{Table: "t", Key: "k"})
	}
	leader.setAck("s1-b", 3)
	leader.setAck("s1-b", 0)
	if lag := leader.Lag(time.Now())["s1-b"]; lag.Entries != 3 {
		t.Fatalf("a follower that came back empty shows %d entries behind, want 3", lag.Entries)
	}
}
