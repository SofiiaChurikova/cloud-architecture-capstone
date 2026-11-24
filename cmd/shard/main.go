package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/SofiiaChurikova/cloud-architecture-capstone/internal/metrics"
	"github.com/SofiiaChurikova/cloud-architecture-capstone/internal/replica"
	"github.com/SofiiaChurikova/cloud-architecture-capstone/internal/wal"
)

type registration struct {
	ID      string `json:"id"`
	Replica string `json:"replica"`
	Role    string `json:"role"`
	Addr    string `json:"addr"`
}

func main() {
	port := env("PORT", "8081")
	shard := env("SHARD_ID", "shard-1")
	id := env("REPLICA_ID", shard+"-a")
	role := env("ROLE", "leader")
	addr := env("ADVERTISE_ADDR", "http://localhost:"+port)
	coordinator := env("COORDINATOR_URL", "http://localhost:8080")
	leader := os.Getenv("LEADER_URL")
	dir := os.Getenv("DATA_DIR")

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "shard", "replica", id)
	slog.SetDefault(logger)

	path := ""
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatal(err)
		}
		path = filepath.Join(dir, "log.jsonl")
	}
	l, err := wal.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("%s: log has %d entries", id, l.Last())

	isLeader := role == "leader"
	if !isLeader && leader == "" {
		log.Fatal("a follower needs LEADER_URL")
	}
	n := replica.New(replica.Config{ID: id, Shard: shard, Leader: isLeader, Log: l})
	mux := http.NewServeMux()
	n.Routes(mux)
	reg := &metrics.Registry{}
	reg.Add(n.WriteMetrics)
	mux.Handle("GET /metrics", reg)
	handler := metrics.New(reg, logger).Wrap(mux)

	if !isLeader {
		go n.Follow(context.Background(), leader, &http.Client{Timeout: 10 * time.Second})
	}
	go register(coordinator, registration{ID: shard, Replica: id, Role: role, Addr: addr})

	log.Printf("%s (%s of %s) listening on :%s", id, role, shard, port)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}

func register(coordinator string, me registration) {
	body, _ := json.Marshal(me)
	client := &http.Client{Timeout: 2 * time.Second}
	ok := false
	for {
		resp, err := client.Post(coordinator+"/internal/shards", "application/json", bytes.NewReader(body))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				err = fmt.Errorf("coordinator answered %s", resp.Status)
			}
		}
		switch {
		case err == nil && !ok:
			log.Printf("registered in %s as %s (%s)", coordinator, me.Replica, me.Addr)
		case err != nil && ok:
			log.Printf("heartbeat failed: %v", err)
		case err != nil:
			log.Printf("register failed: %v", err)
		}
		ok = err == nil
		time.Sleep(2 * time.Second)
	}
}

func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}
