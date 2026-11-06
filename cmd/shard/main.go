package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/SofiiaChurikova/cloud-architecture-capstone/internal/ring"
	"github.com/SofiiaChurikova/cloud-architecture-capstone/internal/store"
)

func main() {
	port := env("PORT", "8081")
	id := env("SHARD_ID", "shard-1")
	addr := env("ADVERTISE_ADDR", "http://localhost:"+port)
	coordinator := env("COORDINATOR_URL", "http://localhost:8080")

	s := store.New()
	mux := http.NewServeMux()
	mux.HandleFunc("/tables/{table}/items/{key}", store.Handler(s))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"id": id, "items": s.Len()})
	})

	go register(coordinator, ring.Shard{ID: id, Addr: addr})

	log.Printf("shard %s listening on :%s", id, port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func register(coordinator string, me ring.Shard) {
	body, _ := json.Marshal(me)
	for {
		resp, err := http.Post(coordinator+"/internal/shards", "application/json", bytes.NewReader(body))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				log.Printf("registered in %s as %s (%s)", coordinator, me.ID, me.Addr)
				return
			}
			log.Printf("register failed: %s", resp.Status)
		} else {
			log.Printf("register failed: %v", err)
		}
		time.Sleep(2 * time.Second)
	}
}

func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}
