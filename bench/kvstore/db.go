package kvstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/magiconair/properties"
	"github.com/pingcap/go-ycsb/pkg/ycsb"
)

type creator struct{}

type kvDB struct {
	url    string
	client *http.Client
}

func init() {
	ycsb.RegisterDBCreator("kvstore", creator{})
}

func (creator) Create(p *properties.Properties) (ycsb.DB, error) {
	db := &kvDB{
		url: p.GetString("kvstore.url", "http://localhost:8080"),
		client: &http.Client{
			Timeout:   5 * time.Second,
			Transport: &http.Transport{MaxIdleConns: 256, MaxIdleConnsPerHost: 256, IdleConnTimeout: 90 * time.Second},
		},
	}
	table := p.GetString("table", "usertable")
	resp, err := db.do(context.Background(), http.MethodPut, db.url+"/tables/"+table, []byte(`{"partition_key":"id"}`))
	if err != nil {
		return nil, err
	}
	if resp != http.StatusOK && resp != http.StatusConflict {
		return nil, fmt.Errorf("create table: status %d", resp)
	}
	return db, nil
}

func (db *kvDB) Close() error { return nil }

func (db *kvDB) InitThread(ctx context.Context, _ int, _ int) context.Context { return ctx }

func (db *kvDB) CleanupThread(_ context.Context) {}

func (db *kvDB) itemURL(table, key string) string {
	return db.url + "/tables/" + table + "/items/" + key
}

func (db *kvDB) Read(ctx context.Context, table, key string, _ []string) (map[string][]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, db.itemURL(table, key), nil)
	resp, err := db.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("read: status %d", resp.StatusCode)
	}
	var item map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(item))
	for k, v := range item {
		out[k] = []byte(v)
	}
	return out, nil
}

func (db *kvDB) Scan(_ context.Context, _ string, _ string, _ int, _ []string) ([]map[string][]byte, error) {
	return nil, errors.New("scan is not supported")
}

func (db *kvDB) Update(ctx context.Context, table, key string, values map[string][]byte) error {
	return db.Insert(ctx, table, key, values)
}

func (db *kvDB) Insert(ctx context.Context, table, key string, values map[string][]byte) error {
	item := make(map[string]string, len(values))
	for k, v := range values {
		item[k] = string(v)
	}
	body, _ := json.Marshal(item)
	status, err := db.do(ctx, http.MethodPut, db.itemURL(table, key), body)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("put: status %d", status)
	}
	return nil
}

func (db *kvDB) Delete(ctx context.Context, table, key string) error {
	status, err := db.do(ctx, http.MethodDelete, db.itemURL(table, key), nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("delete: status %d", status)
	}
	return nil
}

func (db *kvDB) do(ctx context.Context, method, url string, body []byte) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := db.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}
