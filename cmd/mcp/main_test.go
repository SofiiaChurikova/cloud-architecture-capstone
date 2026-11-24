package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func fakeStore(t *testing.T, token string) *httptest.Server {
	var mu sync.Mutex
	items := map[string][]byte{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /tables", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"name":"users","partition_key":"id"}]`))
	})
	mux.HandleFunc("GET /tables/{table}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("table") != "users" {
			http.Error(w, "table not found", http.StatusNotFound)
			return
		}
		w.Write([]byte(`{"name":"users","partition_key":"id"}`))
	})
	mux.HandleFunc("/tables/{table}/items/{key}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("table") != "users" {
			http.Error(w, "table not found", http.StatusNotFound)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		key := r.PathValue("key")
		switch r.Method {
		case http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			items[key] = b
			w.Header().Set("X-Version", "7")
		case http.MethodDelete:
			delete(items, key)
			w.WriteHeader(http.StatusNoContent)
		default:
			b, ok := items[key]
			if !ok {
				http.Error(w, "item not found", http.StatusNotFound)
				return
			}
			w.Header().Set("X-Version", "7")
			w.Write(b)
		}
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "missing or wrong token", http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestServer(t *testing.T, storeURL, token string) *server {
	return newServer(&client{url: storeURL, token: token, http: &http.Client{Timeout: 2 * time.Second}})
}

type callResult struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Structured map[string]any `json:"structuredContent"`
	IsError    bool           `json:"isError"`
}

func call(t *testing.T, s *server, name, args string) callResult {
	t.Helper()
	resp := s.handle(context.Background(), request{ID: json.RawMessage("1"), Method: "tools/call",
		Params: json.RawMessage(`{"name":"` + name + `","arguments":` + args + `}`)})
	if resp.Error != nil {
		t.Fatalf("%s: rpc error %+v", name, resp.Error)
	}
	b, _ := json.Marshal(resp.Result)
	var r callResult
	json.Unmarshal(b, &r)
	return r
}

func TestTools(t *testing.T) {
	s := newTestServer(t, fakeStore(t, "secret").URL, "secret")

	if r := call(t, s, "exists_item", `{"table":"users","key":"42"}`); r.IsError || r.Structured["exists"] != false {
		t.Fatalf("exists before create: %+v", r)
	}
	if r := call(t, s, "create_item", `{"table":"users","key":"42","item":{"id":"42","name":"Ann"}}`); r.IsError || r.Structured["version"] != "7" {
		t.Fatalf("create: %+v", r)
	}
	r := call(t, s, "read_item", `{"table":"users","key":"42"}`)
	if r.IsError || r.Structured["item"].(map[string]any)["name"] != "Ann" {
		t.Fatalf("read: %+v", r)
	}
	if r := call(t, s, "list_tables", `{}`); r.IsError || !strings.Contains(r.Content[0].Text, "users") {
		t.Fatalf("list_tables: %+v", r)
	}
}

func TestDeleteNeedsConfirm(t *testing.T) {
	s := newTestServer(t, fakeStore(t, "secret").URL, "secret")
	call(t, s, "create_item", `{"table":"users","key":"42","item":{"id":"42"}}`)

	r := call(t, s, "delete_item", `{"table":"users","key":"42"}`)
	if !r.IsError || !strings.Contains(r.Content[0].Text, "confirm") {
		t.Fatalf("delete without confirm: %+v", r)
	}
	if r := call(t, s, "exists_item", `{"table":"users","key":"42"}`); r.Structured["exists"] != true {
		t.Fatal("the item was deleted without confirm")
	}
	if r := call(t, s, "delete_item", `{"table":"users","key":"42","confirm":true}`); r.IsError {
		t.Fatalf("delete with confirm: %+v", r)
	}
	if r := call(t, s, "read_item", `{"table":"users","key":"42"}`); !r.IsError || !strings.Contains(r.Content[0].Text, "item not found") {
		t.Fatalf("read after delete: %+v", r)
	}
}

func TestErrors(t *testing.T) {
	store := fakeStore(t, "secret")
	s := newTestServer(t, store.URL, "secret")
	if r := call(t, s, "exists_item", `{"table":"nope","key":"1"}`); !r.IsError || !strings.Contains(r.Content[0].Text, "table not found") {
		t.Fatalf("missing table: %+v", r)
	}
	if r := call(t, s, "read_item", `{"table":"","key":"1"}`); !r.IsError {
		t.Fatal("empty table name was accepted")
	}

	bad := newTestServer(t, store.URL, "wrong")
	if r := call(t, bad, "list_tables", `{}`); !r.IsError || !strings.Contains(r.Content[0].Text, "KV_TOKEN") {
		t.Fatalf("wrong token: %+v", r)
	}

	down := newTestServer(t, "http://127.0.0.1:1", "")
	if r := call(t, down, "list_tables", `{}`); !r.IsError || !strings.Contains(r.Content[0].Text, "not reachable") {
		t.Fatalf("store down: %+v", r)
	}

	resp := s.handle(context.Background(), request{ID: json.RawMessage("1"), Method: "tools/call", Params: json.RawMessage(`{"name":"drop_table"}`)})
	if resp.Error == nil {
		t.Fatal("unknown tool did not give an error")
	}
}

func TestProtocolOverStdio(t *testing.T) {
	s := newTestServer(t, fakeStore(t, "").URL, "")
	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"nope"}`,
	}, "\n") + "\n")
	var out strings.Builder
	if err := s.run(context.Background(), in, &out); err != nil {
		t.Fatal(err)
	}

	sc := bufio.NewScanner(strings.NewReader(out.String()))
	var lines []map[string]any
	for sc.Scan() {
		var m map[string]any
		json.Unmarshal(sc.Bytes(), &m)
		lines = append(lines, m)
	}
	if len(lines) != 3 {
		t.Fatalf("got %d answers, want 3 (no answer to a notification):\n%s", len(lines), out.String())
	}
	if v := lines[0]["result"].(map[string]any)["protocolVersion"]; v != "2025-03-26" {
		t.Fatalf("protocol version %v, want the client's 2025-03-26", v)
	}
	tools := lines[1]["result"].(map[string]any)["tools"].([]any)
	var names []string
	for _, tl := range tools {
		names = append(names, tl.(map[string]any)["name"].(string))
	}
	if strings.Join(names, ",") != "list_tables,exists_item,read_item,create_item,delete_item" {
		t.Fatalf("tools %v", names)
	}
	if lines[2]["error"].(map[string]any)["code"] != float64(-32601) {
		t.Fatalf("unknown method: %v", lines[2])
	}
}
