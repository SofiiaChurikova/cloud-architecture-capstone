package ring

import (
	"fmt"
	"math/rand"
	"strconv"
	"testing"
)

func TestEmptyRing(t *testing.T) {
	r := New()
	if _, ok := r.Get("users#1"); ok {
		t.Fatal("empty ring should not return a shard")
	}
}

func TestSameKeySameShard(t *testing.T) {
	r := New()
	r.Add(Shard{ID: "shard-1", Addr: "http://a"})
	r.Add(Shard{ID: "shard-2", Addr: "http://b"})

	first, _ := r.Get(Key("users", "42"))
	for i := 0; i < 100; i++ {
		s, _ := r.Get(Key("users", "42"))
		if s.ID != first.ID {
			t.Fatalf("key moved from %s to %s", first.ID, s.ID)
		}
	}
}

func TestKeysAreSpreadEvenly(t *testing.T) {
	r := New()
	r.Add(Shard{ID: "shard-1", Addr: "http://a"})
	r.Add(Shard{ID: "shard-2", Addr: "http://b"})

	const n = 100000
	count := map[string]int{}
	rnd := rand.New(rand.NewSource(1))
	for i := 0; i < n; i++ {
		s, _ := r.Get(Key("usertable", fmt.Sprintf("user%d", rnd.Int63())))
		count[s.ID]++
	}
	for id, c := range count {
		share := float64(c) / n * 100
		t.Logf("%s: %.1f%%", id, share)
		if share < 45 || share > 55 {
			t.Errorf("%s got %.1f%% of keys, want 45-55%%", id, share)
		}
	}
}

func TestAddSameShardTwice(t *testing.T) {
	r := New()
	r.Add(Shard{ID: "shard-1", Addr: "http://old"})
	r.Add(Shard{ID: "shard-1", Addr: "http://new"})

	if got := len(r.Shards()); got != 1 {
		t.Fatalf("want 1 shard, got %d", got)
	}
	if s, _ := r.Get("x"); s.Addr != "http://new" {
		t.Fatalf("address not updated: %s", s.Addr)
	}
}

func TestRemoveMovesOnlyThatShardsKeys(t *testing.T) {
	two := Of([]Shard{{ID: "shard-1"}, {ID: "shard-2"}})
	one := Of([]Shard{{ID: "shard-1"}, {ID: "shard-2"}})
	one.Remove("shard-2")

	moved, stayed := 0, 0
	for i := 0; i < 10000; i++ {
		k := Key("users", strconv.Itoa(i))
		a, _ := two.Get(k)
		b, _ := one.Get(k)
		if b.ID != "shard-1" {
			t.Fatalf("key %s still maps to %s after remove", k, b.ID)
		}
		if a.ID == "shard-2" {
			moved++
		} else {
			stayed++
		}
	}
	if moved < 3000 || stayed < 3000 {
		t.Fatalf("moved %d, stayed %d, want about half each", moved, stayed)
	}
	if len(one.Shards()) != 1 {
		t.Fatalf("shards %v", one.Shards())
	}
}
