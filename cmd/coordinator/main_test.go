package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SofiiaChurikova/cloud-architecture-capstone/internal/store"
)

func do(t *testing.T, method, url, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func expect(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Fatalf("%s %s: got %d, want %d", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode, want)
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

func TestWithTwoShards(t *testing.T) {
	stores := []*store.Store{store.New(), store.New()}
	var shards []*httptest.Server
	for _, st := range stores {
		mux := http.NewServeMux()
		mux.HandleFunc("/tables/{table}/items/{key}", store.Handler(st))
		srv := httptest.NewServer(mux)
		defer srv.Close()
		shards = append(shards, srv)
	}

	c := httptest.NewServer(newServer().routes())
	defer c.Close()
	expect(t, do(t, "POST", c.URL+"/internal/shards", `{"id":"shard-1","addr":"`+shards[0].URL+`"}`), http.StatusOK)
	expect(t, do(t, "POST", c.URL+"/internal/shards", `{"id":"shard-2","addr":"`+shards[1].URL+`"}`), http.StatusOK)
	expect(t, do(t, "PUT", c.URL+"/tables/users", `{"partition_key":"id"}`), http.StatusOK)

	const n = 200
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("user%d", i)
		expect(t, do(t, "PUT", c.URL+"/tables/users/items/"+key, `{"v":1}`), http.StatusOK)

		get := do(t, "GET", c.URL+"/tables/users/items/"+key, "")
		expect(t, get, http.StatusOK)
		if get.Header.Get("X-Shard-Id") == "" {
			t.Fatal("response has no X-Shard-Id")
		}
	}

	if got := stores[0].Len() + stores[1].Len(); got != n {
		t.Fatalf("shards hold %d items, want %d", got, n)
	}
	if stores[0].Len() == 0 || stores[1].Len() == 0 {
		t.Fatalf("one shard is empty: %d / %d", stores[0].Len(), stores[1].Len())
	}
	t.Logf("shard-1: %d items, shard-2: %d items", stores[0].Len(), stores[1].Len())
}
