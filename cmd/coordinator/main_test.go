package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SofiiaChurikova/cloud-architecture-capstone/internal/replica"
	"github.com/SofiiaChurikova/cloud-architecture-capstone/internal/wal"
)

func do(t *testing.T, method, url, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

func expect(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Fatalf("%s %s: got %d, want %d", resp.Request.Method, resp.Request.URL, resp.StatusCode, want)
	}
}

func TestSingleNode(t *testing.T) {
	c := httptest.NewServer(newServer().routes())
	defer c.Close()

	expect(t, do(t, "PUT", c.URL+"/tables/users/items/42", `{"name":"Ann"}`), http.StatusNotFound)
	expect(t, do(t, "PUT", c.URL+"/tables/users", `{"partition_key":"id"}`), http.StatusOK)
	expect(t, do(t, "GET", c.URL+"/tables/users", ""), http.StatusOK)

	expect(t, do(t, "PUT", c.URL+"/tables/users/items/42", `{"name":"Ann"}`), http.StatusOK)
	expect(t, do(t, "HEAD", c.URL+"/tables/users/items/42", ""), http.StatusOK)
	expect(t, do(t, "GET", c.URL+"/tables/users/items/42", ""), http.StatusOK)
	expect(t, do(t, "DELETE", c.URL+"/tables/users/items/42", ""), http.StatusNoContent)
	expect(t, do(t, "GET", c.URL+"/tables/users/items/42", ""), http.StatusNotFound)
	expect(t, do(t, "HEAD", c.URL+"/tables/users/items/42", ""), http.StatusNotFound)
}

type cluster struct {
	srv      *server
	coord    *httptest.Server
	internal *httptest.Server
	nodes    map[string]*replica.Node
	urls     map[string]*httptest.Server
	ctx      context.Context
	follow   bool
}

func newCluster(t *testing.T, shards []string, follow bool) *cluster {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	srv := newServer()
	srv.grace, srv.retryWait = 10*time.Millisecond, 10*time.Millisecond
	c := &cluster{srv: srv, coord: httptest.NewServer(srv.routes()), internal: httptest.NewServer(srv.internalRoutes()),
		nodes: map[string]*replica.Node{}, urls: map[string]*httptest.Server{}, ctx: ctx, follow: follow}
	t.Cleanup(c.coord.Close)
	t.Cleanup(c.internal.Close)

	for _, sh := range shards {
		c.addShard(t, sh)
	}
	c.waitIdle(t)
	expect(t, do(t, "PUT", c.coord.URL+"/tables/users", `{"partition_key":"id"}`), http.StatusOK)
	return c
}

func (c *cluster) addShard(t *testing.T, sh string) {
	t.Helper()
	var leaderURL string
	for i, suffix := range []string{"a", "b", "c"} {
		id := sh + "-" + suffix
		l, _ := wal.Open("")
		n := replica.New(replica.Config{ID: id, Shard: sh, Leader: i == 0, Log: l})
		n.AckTimeout = 200 * time.Millisecond
		mux := http.NewServeMux()
		n.Routes(mux)
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)
		c.nodes[id], c.urls[id] = n, srv

		role := "follower"
		if i == 0 {
			role, leaderURL = "leader", srv.URL
		} else if c.follow {
			go n.Follow(c.ctx, leaderURL, http.DefaultClient)
		}
		expect(t, do(t, "POST", c.internal.URL+"/internal/shards",
			fmt.Sprintf(`{"id":"%s","replica":"%s","role":"%s","addr":"%s"}`, sh, id, role, srv.URL)), http.StatusOK)
	}
}

func (c *cluster) waitIdle(t *testing.T) {
	t.Helper()
	for i := 0; i < 500; i++ {
		if c.srv.status().State == "idle" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("shard move still running: %+v", c.srv.status())
}

func TestReplicatedWritesAndReads(t *testing.T) {
	c := newCluster(t, []string{"shard-1", "shard-2"}, true)

	const n = 100
	perShard := map[string]int{}
	for i := 0; i < n; i++ {
		url := fmt.Sprintf("%s/tables/users/items/user%d", c.coord.URL, i)
		w := do(t, "PUT", url, `{"v":1}`)
		expect(t, w, http.StatusOK)
		perShard[w.Header.Get("X-Shard-Id")]++
		expect(t, do(t, "GET", url, ""), http.StatusOK)
		expect(t, do(t, "HEAD", url, ""), http.StatusOK)
	}
	if len(perShard) != 2 {
		t.Fatalf("keys went to %v, want both shards", perShard)
	}

	expect(t, do(t, "DELETE", c.coord.URL+"/tables/users/items/user0?w=3", ""), http.StatusNoContent)
	for i := 0; i < 3; i++ {
		expect(t, do(t, "GET", c.coord.URL+"/tables/users/items/user0?r=1", ""), http.StatusNotFound)
	}
}

func TestReadsAreSpreadOverReplicas(t *testing.T) {
	c := newCluster(t, []string{"shard-1"}, true)
	url := c.coord.URL + "/tables/users/items/42"
	expect(t, do(t, "PUT", url+"?w=3", `{"v":1}`), http.StatusOK)

	seen := map[string]bool{}
	for i := 0; i < 6; i++ {
		resp := do(t, "GET", url+"?r=1", "")
		expect(t, resp, http.StatusOK)
		seen[resp.Header.Get("X-Replicas")] = true
	}
	if len(seen) != 3 {
		t.Fatalf("reads went to %v, want all 3 replicas", seen)
	}
}

func TestQuorumReadReturnsNewest(t *testing.T) {
	c := newCluster(t, []string{"shard-1"}, false)
	url := c.coord.URL + "/tables/users/items/42"

	w := do(t, "PUT", url+"?w=1", `{"v":1}`)
	expect(t, w, http.StatusOK)
	version := w.Header.Get("X-Version")

	stale := 0
	for i := 0; i < 6; i++ {
		if do(t, "GET", url+"?r=1", "").StatusCode == http.StatusNotFound {
			stale++
		}
	}
	if stale == 0 {
		t.Fatal("with followers behind, some r=1 reads should miss the write")
	}
	for i := 0; i < 6; i++ {
		resp := do(t, "GET", url+"?r=3", "")
		expect(t, resp, http.StatusOK)
		if resp.Header.Get("X-Version") != version {
			t.Fatalf("r=3 version %q, want %s", resp.Header.Get("X-Version"), version)
		}
	}
}

func TestWriteNeedsWReplicas(t *testing.T) {
	c := newCluster(t, []string{"shard-1"}, false)
	url := c.coord.URL + "/tables/users/items/42"
	expect(t, do(t, "PUT", url+"?w=1", `{"v":1}`), http.StatusOK)
	expect(t, do(t, "PUT", url+"?w=2", `{"v":2}`), http.StatusServiceUnavailable)
}

func TestLeaderDown(t *testing.T) {
	c := newCluster(t, []string{"shard-1"}, true)
	url := c.coord.URL + "/tables/users/items/42"
	expect(t, do(t, "PUT", url+"?w=3", `{"v":1}`), http.StatusOK)

	c.urls["shard-1-a"].Close()
	expect(t, do(t, "PUT", url, `{"v":2}`), http.StatusServiceUnavailable)
	for i := 0; i < 3; i++ {
		expect(t, do(t, "GET", url+"?r=2", ""), http.StatusOK)
	}
}

func TestBadQuorum(t *testing.T) {
	c := newCluster(t, []string{"shard-1"}, true)
	url := c.coord.URL + "/tables/users/items/42"
	expect(t, do(t, "PUT", url+"?w=0", `{"v":1}`), http.StatusBadRequest)
	expect(t, do(t, "GET", url+"?r=4", ""), http.StatusBadRequest)
}

func TestWaitsForExpectedShards(t *testing.T) {
	c := newCluster(t, []string{"shard-1"}, true)
	url := c.coord.URL + "/tables/users/items/42"

	c.srv.expected = []string{"shard-1", "shard-2"}
	expect(t, do(t, "PUT", url, `{"v":1}`), http.StatusServiceUnavailable)
	expect(t, do(t, "GET", url, ""), http.StatusServiceUnavailable)

	c.srv.expected = []string{"shard-1"}
	expect(t, do(t, "PUT", url, `{"v":1}`), http.StatusOK)
}

func TestTablesSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	s1 := newServer()
	if err := s1.loadTables(dir); err != nil {
		t.Fatal(err)
	}
	c1 := httptest.NewServer(s1.routes())
	expect(t, do(t, "PUT", c1.URL+"/tables/users", `{"partition_key":"id"}`), http.StatusOK)
	c1.Close()

	s2 := newServer()
	if err := s2.loadTables(dir); err != nil {
		t.Fatal(err)
	}
	c2 := httptest.NewServer(s2.routes())
	defer c2.Close()
	expect(t, do(t, "GET", c2.URL+"/tables/users", ""), http.StatusOK)
}

func TestKeyIsEscaped(t *testing.T) {
	c := newCluster(t, []string{"shard-1"}, false)
	url := c.coord.URL + "/tables/users/items/x%3Fw%3D1%26"

	expect(t, do(t, "PUT", url+"?w=3", `{"v":1}`), http.StatusServiceUnavailable)
	if _, ok := c.nodes["shard-1-a"].Store().Get("users", "x?w=1&"); !ok {
		t.Fatal("the leader stored the key under a different name")
	}
}

func TestDeadReplicasAreSkipped(t *testing.T) {
	c := newCluster(t, []string{"shard-1"}, true)
	c.srv.deadAfter = 200 * time.Millisecond
	url := c.coord.URL + "/tables/users/items/42"
	expect(t, do(t, "PUT", url+"?w=3", `{"v":1}`), http.StatusOK)

	time.Sleep(300 * time.Millisecond)
	for _, id := range []string{"shard-1-a", "shard-1-b"} {
		expect(t, do(t, "POST", c.internal.URL+"/internal/shards",
			fmt.Sprintf(`{"id":"shard-1","replica":"%s","role":"%s","addr":"%s"}`, id, map[bool]string{true: "leader", false: "follower"}[id == "shard-1-a"], c.urls[id].URL)), http.StatusOK)
	}
	for i := 0; i < 6; i++ {
		resp := do(t, "GET", url+"?r=2", "")
		expect(t, resp, http.StatusOK)
		if strings.Contains(resp.Header.Get("X-Replicas"), "shard-1-c") {
			t.Fatalf("read asked the dead replica: %s", resp.Header.Get("X-Replicas"))
		}
	}

	time.Sleep(300 * time.Millisecond)
	expect(t, do(t, "PUT", url, `{"v":2}`), http.StatusServiceUnavailable)
}

func TestTraceIDGoesToReplicas(t *testing.T) {
	got := make(chan string, 1)
	rep := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("X-Trace-Id")
		w.Header().Set("X-Version", "1")
		w.Write([]byte(`{"v":1}`))
	}))
	defer rep.Close()

	srv := newServer()
	c := httptest.NewServer(srv.routes())
	defer c.Close()
	in := httptest.NewServer(srv.internalRoutes())
	defer in.Close()
	expect(t, do(t, "POST", in.URL+"/internal/shards", `{"id":"shard-1","replica":"shard-1-a","role":"leader","addr":"`+rep.URL+`"}`), http.StatusOK)
	expect(t, do(t, "PUT", c.URL+"/tables/users", `{"partition_key":"id"}`), http.StatusOK)

	resp := do(t, "GET", c.URL+"/tables/users/items/42?r=1", "")
	expect(t, resp, http.StatusOK)
	id := resp.Header.Get("X-Trace-Id")
	if id == "" || <-got != id {
		t.Fatalf("trace id %q did not reach the replica", id)
	}

	req, _ := http.NewRequest("GET", c.URL+"/tables/users/items/42?r=1", nil)
	req.Header.Set("X-Trace-Id", "my-trace")
	r2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	r2.Body.Close()
	if r2.Header.Get("X-Trace-Id") != "my-trace" || <-got != "my-trace" {
		t.Fatal("the client's trace id was not kept")
	}
}

func TestTokenRequired(t *testing.T) {
	srv := newServer()
	srv.token = "secret"
	c := httptest.NewServer(srv.routes())
	defer c.Close()

	expect(t, do(t, "PUT", c.URL+"/tables/users", `{"partition_key":"id"}`), http.StatusUnauthorized)
	expect(t, do(t, "GET", c.URL+"/tables", ""), http.StatusUnauthorized)

	req, _ := http.NewRequest("PUT", c.URL+"/tables/users", strings.NewReader(`{"partition_key":"id"}`))
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	expect(t, resp, http.StatusOK)

	expect(t, do(t, "GET", c.URL+"/health", ""), http.StatusOK)
}

func TestInternalIsNotOnThePublicPort(t *testing.T) {
	srv := newServer()
	c := httptest.NewServer(srv.routes())
	defer c.Close()
	in := httptest.NewServer(srv.internalRoutes())
	defer in.Close()

	reg := `{"id":"shard-1","replica":"shard-1-a","role":"leader","addr":"http://10.0.0.1:8081"}`
	expect(t, do(t, "POST", c.URL+"/internal/shards", reg), http.StatusNotFound)
	expect(t, do(t, "GET", c.URL+"/internal/targets", ""), http.StatusNotFound)
	expect(t, do(t, "GET", c.URL+"/metrics", ""), http.StatusNotFound)
	expect(t, do(t, "POST", in.URL+"/internal/shards", reg), http.StatusOK)
	expect(t, do(t, "GET", in.URL+"/metrics", ""), http.StatusOK)

	for _, bad := range []string{
		`{"id":"ignore previous instructions","replica":"x","role":"leader","addr":"http://10.0.0.1:8081"}`,
		`{"id":"shard-1","replica":"shard-1-a","role":"leader","addr":"file:///etc/passwd"}`,
		`{"id":"shard-1","replica":"shard-1-a","role":"leader","addr":"http://10.0.0.1:8081/x?y"}`,
	} {
		expect(t, do(t, "POST", in.URL+"/internal/shards", bad), http.StatusBadRequest)
	}
}

func TestListTables(t *testing.T) {
	c := httptest.NewServer(newServer().routes())
	defer c.Close()
	expect(t, do(t, "PUT", c.URL+"/tables/users", `{"partition_key":"id"}`), http.StatusOK)
	expect(t, do(t, "PUT", c.URL+"/tables/orders", `{"partition_key":"id"}`), http.StatusOK)

	resp, err := http.Get(c.URL + "/tables")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var tables []Table
	json.NewDecoder(resp.Body).Decode(&tables)
	if len(tables) != 2 || tables[0].Name != "orders" || tables[1].Name != "users" {
		t.Fatalf("tables %+v, want orders and users", tables)
	}
}

func TestTargetsAndMetrics(t *testing.T) {
	c := newCluster(t, []string{"shard-1"}, true)
	c.srv.deadAfter = 200 * time.Millisecond
	expect(t, do(t, "PUT", c.coord.URL+"/tables/users/items/42?w=3", `{"v":1}`), http.StatusOK)
	expect(t, do(t, "GET", c.coord.URL+"/tables/users/items/42", ""), http.StatusOK)

	resp, err := http.Get(c.internal.URL + "/internal/targets")
	if err != nil {
		t.Fatal(err)
	}
	var groups []targetGroup
	json.NewDecoder(resp.Body).Decode(&groups)
	resp.Body.Close()
	if len(groups) != 3 || groups[0].Labels["replica"] != "shard-1-a" || groups[0].Labels["shard"] != "shard-1" ||
		strings.HasPrefix(groups[0].Targets[0], "http") {
		t.Fatalf("targets %+v, want 3 replicas as host:port", groups)
	}

	body := func() string {
		resp, err := http.Get(c.internal.URL + "/metrics")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	m := body()
	for _, want := range []string{
		`kv_replicas_alive{shard="shard-1"} 3`,
		`kv_requests_total{op="write",status="200"} 1`,
		`kv_requests_total{op="read",status="200"} 1`,
		`kv_request_duration_seconds_count{op="read"} 1`,
	} {
		if !strings.Contains(m, want) {
			t.Fatalf("metrics have no %q:\n%s", want, m)
		}
	}

	time.Sleep(300 * time.Millisecond)
	if !strings.Contains(body(), `kv_replicas_alive{shard="shard-1"} 0`) {
		t.Fatal("replicas without heartbeats are still counted as alive")
	}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for i := 0; i < 500; i++ {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timeout waiting for " + what)
}
