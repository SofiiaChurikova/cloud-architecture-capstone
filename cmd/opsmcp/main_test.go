package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SofiiaChurikova/cloud-architecture-capstone/internal/mcpserver"
)

func call(t *testing.T, s *mcpserver.Server, name, args string) (string, bool) {
	t.Helper()
	resp := s.Handle(context.Background(), mcpserver.Request{ID: json.RawMessage("1"), Method: "tools/call",
		Params: json.RawMessage(`{"name":"` + name + `","arguments":` + args + `}`)})
	if resp.Error != nil {
		t.Fatalf("%s: %+v", name, resp.Error)
	}
	b, _ := json.Marshal(resp.Result)
	var r struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	json.Unmarshal(b, &r)
	return r.Content[0].Text, r.IsError
}

func fakeCluster(t *testing.T) (*httptest.Server, *httptest.Server) {
	prom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/alerts":
			w.Write([]byte(`{"status":"success","data":{"alerts":[{"labels":{"alertname":"NodeDown","replica":"shard-1-b"},"annotations":{"summary":"replicas shard-1-b does not answer"},"state":"firing","activeAt":"2026-11-26T10:00:00Z"}]}}`))
		case "/api/v1/query":
			if r.URL.Query().Get("query") == "bad(" {
				w.Write([]byte(`{"status":"error","error":"parse error"}`))
				return
			}
			w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1,"0.072"]}]}}`))
		}
	}))
	kv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.Write([]byte(`{"status":"ok","shards":1,"replicas":3,"migration":"idle","move":{"state":"idle"}}`))
		}
	}))
	t.Cleanup(prom.Close)
	t.Cleanup(kv.Close)
	return prom, kv
}

func TestOpsTools(t *testing.T) {
	prom, kv := fakeCluster(t)
	s := newServer(&ops{prom: prom.URL, kv: kv.URL, http: &http.Client{Timeout: time.Second}})

	if out, isErr := call(t, s, "get_alerts", `{}`); isErr || !strings.Contains(out, `"name":"NodeDown"`) || !strings.Contains(out, "shard-1-b") || strings.Contains(out, "does not answer") {
		t.Fatalf("get_alerts: %s", out)
	}
	if out, isErr := call(t, s, "query_metric", `{"query":"histogram_quantile(0.99, x)"}`); isErr || !strings.Contains(out, `"value":"0.072"`) {
		t.Fatalf("query_metric: %s", out)
	}
	if out, isErr := call(t, s, "query_metric", `{"query":"bad("}`); !isErr || !strings.Contains(out, "parse error") {
		t.Fatalf("bad query: %s", out)
	}
	if out, isErr := call(t, s, "get_cluster", `{}`); isErr || !strings.Contains(out, `"shards":1`) || !strings.Contains(out, `"state":"idle"`) {
		t.Fatalf("get_cluster: %s", out)
	}
}

func TestOnlyReadTools(t *testing.T) {
	s := newServer(&ops{})
	if len(s.Tools) != 3 {
		t.Fatalf("%d tools, want 3", len(s.Tools))
	}
	for _, tl := range s.Tools {
		if tl.Annotations["readOnlyHint"] != true {
			t.Fatalf("%s is not read only", tl.Name)
		}
	}
	init := s.Handle(context.Background(), mcpserver.Request{ID: json.RawMessage("1"), Method: "initialize", Params: json.RawMessage(`{}`)})
	b, _ := json.Marshal(init.Result)
	if !strings.Contains(string(b), "SCALE UP") {
		t.Fatal("initialize has no agent instructions")
	}
}

func TestRulesAreInToolDescription(t *testing.T) {
	for _, tl := range newServer(&ops{}).Tools {
		if tl.Name == "get_cluster" && !strings.Contains(tl.Description, "SCALE DOWN") {
			t.Fatal("get_cluster description has no rules")
		}
	}
}
