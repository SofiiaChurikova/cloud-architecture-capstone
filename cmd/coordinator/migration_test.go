package main

import (
	"fmt"
	"github.com/SofiiaChurikova/cloud-architecture-capstone/internal/replica"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/SofiiaChurikova/cloud-architecture-capstone/internal/ring"
)

func ownerOn(shards []string, key string) string {
	var list []ring.Shard
	for _, s := range shards {
		list = append(list, ring.Shard{ID: s})
	}
	sh, _ := ring.Of(list).Get(ring.Key("users", key))
	return sh.ID
}

func get(t *testing.T, url string) (int, string, http.Header) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), resp.Header
}

func TestAddShardMovesOnlyItsKeys(t *testing.T) {
	c := newCluster(t, []string{"shard-1"}, true)
	const n = 300
	for i := 0; i < n; i++ {
		expect(t, do(t, "PUT", fmt.Sprintf("%s/tables/users/items/k%d?w=3", c.coord.URL, i), fmt.Sprintf(`{"i":%d}`, i)), http.StatusOK)
	}
	if got := c.nodes["shard-1-a"].Store().Len(); got != n {
		t.Fatalf("shard-1 has %d keys before, want %d", got, n)
	}

	c.addShard(t, "shard-2")
	c.waitIdle(t)

	want2 := 0
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("k%d", i)
		owner := ownerOn([]string{"shard-1", "shard-2"}, key)
		if owner == "shard-2" {
			want2++
		}
		_, inOld := c.nodes["shard-1-a"].Store().Get("users", key)
		_, inNew := c.nodes["shard-2-a"].Store().Get("users", key)
		if (owner == "shard-2") != inNew || (owner == "shard-1") != inOld {
			t.Fatalf("%s owned by %s: on shard-1 %v, on shard-2 %v", key, owner, inOld, inNew)
		}
		code, body, _ := get(t, fmt.Sprintf("%s/tables/users/items/%s", c.coord.URL, key))
		if code != http.StatusOK || body != fmt.Sprintf(`{"i":%d}`, i) {
			t.Fatalf("read %s after the move: %d %s", key, code, body)
		}
	}
	if want2 < n/4 || want2 > 3*n/4 {
		t.Fatalf("%d of %d keys moved, want about half", want2, n)
	}
	waitFor(t, "followers of shard-2 to get the moved keys", func() bool {
		return c.nodes["shard-2-b"].Store().Len() == want2 && c.nodes["shard-1-c"].Store().Len() == n-want2
	})
	if got := c.srv.status(); got.State != "idle" || got.Last == nil || got.Last.Total != int64(want2) || got.Last.Copied != int64(want2) {
		t.Fatalf("status %+v, want the last move with %d keys", got, want2)
	}
}

func TestReadsAndWritesDuringTheMove(t *testing.T) {
	c := newCluster(t, []string{"shard-1"}, true)
	var moving []string
	for i := 0; len(moving) < 5; i++ {
		key := fmt.Sprintf("k%d", i)
		expect(t, do(t, "PUT", fmt.Sprintf("%s/tables/users/items/%s?w=3", c.coord.URL, key), `{"v":"old"}`), http.StatusOK)
		if ownerOn([]string{"shard-1", "shard-2"}, key) == "shard-2" {
			moving = append(moving, key)
		}
	}

	c.srv.grace = 500 * time.Millisecond
	c.addShard(t, "shard-2")
	waitFor(t, "routing to switch", func() bool {
		c.srv.mu.RLock()
		defer c.srv.mu.RUnlock()
		return c.srv.mig != nil && c.srv.mig.ready.Load()
	})

	code, body, h := get(t, c.coord.URL+"/tables/users/items/"+moving[0])
	if code != http.StatusOK || body != `{"v":"old"}` || h.Get("X-Moving-From") != "shard-1" {
		t.Fatalf("read during the move: %d %s from %q, want the old value from shard-1", code, body, h.Get("X-Moving-From"))
	}
	w := do(t, "PUT", c.coord.URL+"/tables/users/items/"+moving[1], `{"v":"new"}`)
	expect(t, w, http.StatusOK)
	if w.Header.Get("X-Shard-Id") != "shard-2" {
		t.Fatalf("write during the move went to %s, want shard-2", w.Header.Get("X-Shard-Id"))
	}
	expect(t, do(t, "DELETE", c.coord.URL+"/tables/users/items/"+moving[2], ""), http.StatusNoContent)

	c.waitIdle(t)
	if _, body, _ := get(t, c.coord.URL+"/tables/users/items/"+moving[1]); body != `{"v":"new"}` {
		t.Fatalf("the copy of the old value replaced a write made during the move: %s", body)
	}
	if code, _, _ := get(t, c.coord.URL+"/tables/users/items/"+moving[2]); code != http.StatusNotFound {
		t.Fatalf("a delete made during the move came back: %d", code)
	}
	if _, body, _ := get(t, c.coord.URL+"/tables/users/items/"+moving[0]); body != `{"v":"old"}` {
		t.Fatalf("moved key: %s", body)
	}
}

func TestDrainShard(t *testing.T) {
	c := newCluster(t, []string{"shard-1", "shard-2"}, true)
	const n = 200
	for i := 0; i < n; i++ {
		expect(t, do(t, "PUT", fmt.Sprintf("%s/tables/users/items/k%d?w=3", c.coord.URL, i), fmt.Sprintf(`{"i":%d}`, i)), http.StatusOK)
	}
	if c.nodes["shard-2-a"].Store().Len() == 0 {
		t.Fatal("shard-2 got no keys")
	}

	expect(t, do(t, "POST", c.coord.URL+"/admin/shards/shard-9/drain", ""), http.StatusNotFound)
	expect(t, do(t, "POST", c.coord.URL+"/admin/shards/shard-2/drain", ""), http.StatusAccepted)
	c.waitIdle(t)

	if got := c.nodes["shard-1-a"].Store().Len(); got != n {
		t.Fatalf("shard-1 has %d keys after the drain, want %d", got, n)
	}
	for i := 0; i < n; i++ {
		code, body, h := get(t, fmt.Sprintf("%s/tables/users/items/k%d", c.coord.URL, i))
		if code != http.StatusOK || body != fmt.Sprintf(`{"i":%d}`, i) || h.Get("X-Shard-Id") != "shard-1" {
			t.Fatalf("k%d after the drain: %d %s from %s", i, code, body, h.Get("X-Shard-Id"))
		}
	}
	if _, body, _ := get(t, c.coord.URL+"/health"); !strings.Contains(body, `"shards":1`) {
		t.Fatalf("health after the drain: %s", body)
	}
	expect(t, do(t, "POST", c.coord.URL+"/admin/shards/shard-1/drain", ""), http.StatusConflict)

	old := fmt.Sprintf(`{"id":"shard-2","replica":"shard-2-a","role":"leader","addr":"%s","offset":120}`, c.urls["shard-2-a"].URL)
	expect(t, do(t, "POST", c.internal.URL+"/internal/shards", old), http.StatusGone)
	fresh := `{"id":"shard-2","replica":"shard-2-a","role":"leader","addr":"http://10.0.0.9:8081","offset":0}`
	expect(t, do(t, "POST", c.internal.URL+"/internal/shards", fresh), http.StatusOK)
}

func TestOnlyOneMoveAtATime(t *testing.T) {
	c := newCluster(t, []string{"shard-1", "shard-2"}, true)
	c.srv.grace = 300 * time.Millisecond
	expect(t, do(t, "POST", c.coord.URL+"/admin/shards/shard-2/drain", ""), http.StatusAccepted)
	expect(t, do(t, "POST", c.coord.URL+"/admin/shards/shard-2/drain", ""), http.StatusConflict)
	_, body, _ := get(t, c.coord.URL+"/admin/migration")
	if !strings.Contains(body, `"state":"draining"`) {
		t.Fatalf("migration state: %s", body)
	}
	c.waitIdle(t)
}

func TestJoinWaitsForAllReplicas(t *testing.T) {
	c := newCluster(t, []string{"shard-1"}, true)
	expect(t, do(t, "POST", c.internal.URL+"/internal/shards",
		`{"id":"shard-2","replica":"shard-2-a","role":"leader","addr":"http://127.0.0.1:1"}`), http.StatusOK)
	time.Sleep(50 * time.Millisecond)
	if st := c.srv.status(); st.State != "idle" || len(c.srv.ring.Shards()) != 1 {
		t.Fatalf("shard-2 joined with only its leader: %+v, ring %d", st, len(c.srv.ring.Shards()))
	}
}

func TestOnlyAllowedShardsRegister(t *testing.T) {
	c := newCluster(t, []string{"shard-1"}, true)
	c.srv.allowed = []string{"shard-1", "shard-2"}
	expect(t, do(t, "POST", c.internal.URL+"/internal/shards",
		`{"id":"shard-3","replica":"shard-3-a","role":"leader","addr":"http://127.0.0.1:1"}`), http.StatusForbidden)
}

func TestLateWriteOnTheOldShardIsDropped(t *testing.T) {
	c := newCluster(t, []string{"shard-1"}, true)
	c.addShard(t, "shard-2")
	c.waitIdle(t)
	var key string
	for i := 0; key == ""; i++ {
		if k := fmt.Sprintf("k%d", i); ownerOn([]string{"shard-1", "shard-2"}, k) == "shard-2" {
			key = k
		}
	}
	expect(t, do(t, "PUT", c.urls["shard-1-a"].URL+"/internal/items/users/"+key+"?w=1", `{"v":"copied"}`), http.StatusOK)
	copied, _ := c.nodes["shard-1-a"].Store().Get("users", key)
	expect(t, do(t, "PUT", c.urls["shard-1-a"].URL+"/internal/items/users/"+key+"?w=1", `{"v":"late"}`), http.StatusOK)

	m := &migration{next: c.srv.ring}
	c.srv.purge("shard-1", m, []replica.Item{{Table: "users", Key: key, Version: copied.Version}})
	if it, ok := c.nodes["shard-1-a"].Store().Get("users", key); ok {
		t.Fatalf("a late write for a moved key stayed on shard-1: %s", it.Value)
	}
}
