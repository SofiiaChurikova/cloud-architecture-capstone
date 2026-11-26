package main

import (
	"bytes"
	"context"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

var cfg = config{up: 0.06, down: 0.02, upFor: 2 * time.Minute, downFor: 5 * time.Minute, cooldown: 5 * time.Minute, minTraffic: 1, downRate: 100}

func TestScaleUpOnlyAfterUpFor(t *testing.T) {
	d := &decider{cfg: cfg}
	t0 := time.Unix(0, 0)
	high := observation{p99: 0.08, traffic: 50, shards: 1}
	if a, _ := d.step(t0, high); a != "" {
		t.Fatalf("scaled at once: %s", a)
	}
	if a, _ := d.step(t0.Add(time.Minute), high); a != "" {
		t.Fatalf("scaled after 1 min: %s", a)
	}
	if a, _ := d.step(t0.Add(2*time.Minute), high); a != "up" {
		t.Fatalf("after 2 min: %q, want up", a)
	}
	if a, _ := d.step(t0.Add(3*time.Minute), observation{p99: 0.08, traffic: 50, shards: 2}); a != "" {
		t.Fatalf("2 shards and high p99: %q, want nothing (max is 2)", a)
	}
}

func TestDipResetsTheTimer(t *testing.T) {
	d := &decider{cfg: cfg}
	t0 := time.Unix(0, 0)
	d.step(t0, observation{p99: 0.08, traffic: 50, shards: 1})
	d.step(t0.Add(time.Minute), observation{p99: 0.03, traffic: 50, shards: 1})
	if a, _ := d.step(t0.Add(2*time.Minute), observation{p99: 0.08, traffic: 50, shards: 1}); a != "" {
		t.Fatalf("p99 dipped in the middle, still scaled: %s", a)
	}
}

func TestScaleDownAndCooldown(t *testing.T) {
	d := &decider{cfg: cfg}
	t0 := time.Unix(0, 0)
	idle := observation{p99: math.NaN(), traffic: 0, shards: 2}
	if a, _ := d.step(t0, idle); a != "" {
		t.Fatal("scaled down at once")
	}
	if a, _ := d.step(t0.Add(5*time.Minute), idle); a != "down" {
		t.Fatalf("idle for 5 min: %q, want down", a)
	}
	high := observation{p99: 0.09, traffic: 50, shards: 1}
	d.step(t0.Add(6*time.Minute), high)
	if a, reason := d.step(t0.Add(8*time.Minute), high); a != "" || reason != "cooldown" {
		t.Fatalf("inside the cooldown: %q %q", a, reason)
	}
	if a, _ := d.step(t0.Add(10*time.Minute), high); a != "up" {
		t.Fatalf("after the cooldown: %q, want up", a)
	}
}

func TestNoActionDuringAMove(t *testing.T) {
	d := &decider{cfg: cfg}
	t0 := time.Unix(0, 0)
	d.step(t0, observation{p99: 0.08, traffic: 50, shards: 1})
	if a, _ := d.step(t0.Add(3*time.Minute), observation{p99: 0.08, traffic: 50, shards: 1, moving: true}); a != "" {
		t.Fatal("scaled during a move")
	}
	if a, _ := d.step(t0.Add(4*time.Minute), observation{p99: 0.08, traffic: 50, shards: 1}); a != "" {
		t.Fatal("the timer did not restart after the move")
	}
}

type fakeCloud struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeCloud) stackBusy() (bool, error) { return false, nil }
func (f *fakeCloud) setShardCount(n int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "ShardCount="+string(rune('0'+n)))
	return nil
}

func TestScaleDownDrainsFirst(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	polls := 0
	prom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") == upQuery {
			w.Write([]byte(`{"status":"success","data":{"result":[{"value":[0,"1"]}]}}`))
			return
		}
		w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
	}))
	defer prom.Close()
	kv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path != "/health" && r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "no token", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/health":
			w.Write([]byte(`{"shards":2,"migration":"idle"}`))
		case "/admin/shards/shard-2/drain":
			calls = append(calls, "drain")
			w.WriteHeader(http.StatusAccepted)
		case "/admin/migration":
			polls++
			if polls < 3 {
				w.Write([]byte(`{"state":"draining","copied":10,"total":30}`))
				return
			}
			calls = append(calls, "idle")
			w.Write([]byte(`{"state":"idle"}`))
		}
	}))
	defer kv.Close()

	c := &fakeCloud{}
	s := &scaler{d: &decider{cfg: cfg}, prom: prom.URL, kv: kv.URL, token: "tok", drainID: "shard-2",
		http: http.DefaultClient, cloud: c, pollWait: time.Millisecond, log: slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))}
	t0 := time.Unix(0, 0)
	for _, at := range []time.Duration{0, 5 * time.Minute} {
		if err := s.tick(context.Background(), t0.Add(at)); err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	got := strings.Join(append(calls, c.calls...), ",")
	if got != "drain,idle,ShardCount=1" {
		t.Fatalf("order %s, want drain,idle,ShardCount=1", got)
	}
}

func TestStackUpdateCommand(t *testing.T) {
	var cmds [][]string
	a := awsCLI{stack: "capstone-app", run: func(args ...string) (string, error) {
		cmds = append(cmds, args)
		if args[1] == "describe-stacks" {
			return "NetworkStackName\tShardCount\tApiToken\n", nil
		}
		return "", nil
	}}
	if err := a.setShardCount(2); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(cmds[1], " ")
	for _, want := range []string{"update-stack", "--use-previous-template", "ParameterKey=NetworkStackName,UsePreviousValue=true",
		"ParameterKey=ApiToken,UsePreviousValue=true", "ParameterKey=ShardCount,ParameterValue=2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("no %q in: %s", want, got)
		}
	}
	if strings.Contains(got, "ShardCount,UsePreviousValue") {
		t.Fatalf("ShardCount kept its old value: %s", got)
	}
}

func TestNoDataIsNotIdle(t *testing.T) {
	d := &decider{cfg: cfg}
	t0 := time.Unix(0, 0)
	for _, o := range []observation{
		{p99: math.NaN(), traffic: 0, noData: true, shards: 2},
		{p99: math.NaN(), traffic: 200, shards: 2},
	} {
		for m := 0; m <= 10; m++ {
			if a, _ := d.step(t0.Add(time.Duration(m)*time.Minute), o); a != "" {
				t.Fatalf("%+v: action %q, want none", o, a)
			}
		}
	}
}

func TestErrorsScaleUp(t *testing.T) {
	d := &decider{cfg: cfg}
	t0 := time.Unix(0, 0)
	o := observation{p99: 0.01, traffic: 100, errors: 5, shards: 1}
	d.step(t0, o)
	if a, _ := d.step(t0.Add(2*time.Minute), o); a != "up" {
		t.Fatalf("5%% errors for 2 min: %q, want up", a)
	}
}

func TestBusyStoreDoesNotScaleDown(t *testing.T) {
	d := &decider{cfg: cfg}
	t0 := time.Unix(0, 0)
	o := observation{p99: 0.01, traffic: 300, shards: 2}
	for m := 0; m <= 10; m++ {
		if a, _ := d.step(t0.Add(time.Duration(m)*time.Minute), o); a != "" {
			t.Fatalf("fast but busy: %q, want none", a)
		}
	}
}
