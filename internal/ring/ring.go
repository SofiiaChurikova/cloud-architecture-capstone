package ring

import (
	"sort"
	"strconv"
	"sync"

	"github.com/cespare/xxhash/v2"
)

const VNodes = 128

type Shard struct {
	ID   string `json:"id"`
	Addr string `json:"addr"`
}

type Ring struct {
	mu     sync.RWMutex
	points []uint64
	owner  map[uint64]string
	shards map[string]Shard
}

func New() *Ring {
	return &Ring{owner: map[uint64]string{}, shards: map[string]Shard{}}
}

func Key(table, partitionKey string) string {
	return table + "#" + partitionKey
}

func (r *Ring) Add(s Shard) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.shards[s.ID]; ok {
		r.shards[s.ID] = s
		return
	}
	r.shards[s.ID] = s
	for i := 0; i < VNodes; i++ {
		h := xxhash.Sum64String(s.ID + "-" + strconv.Itoa(i))
		r.owner[h] = s.ID
		r.points = append(r.points, h)
	}
	sort.Slice(r.points, func(i, j int) bool { return r.points[i] < r.points[j] })
}

func (r *Ring) Get(key string) (Shard, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.points) == 0 {
		return Shard{}, false
	}
	h := xxhash.Sum64String(key)
	i := sort.Search(len(r.points), func(i int) bool { return r.points[i] >= h })
	if i == len(r.points) {
		i = 0
	}
	return r.shards[r.owner[r.points[i]]], true
}

func (r *Ring) Shards() []Shard {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Shard, 0, len(r.shards))
	for _, s := range r.shards {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
