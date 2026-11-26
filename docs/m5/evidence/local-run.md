# M5 - local run

Run on 2026-11-26 with the binaries from `cmd/` built with `-ldflags "-X main.version=test123"`: coordinator (`API_TOKEN=tok`, `EXPECTED_SHARDS=shard-1`), shard-1-a/b/c, later shard-2-a/b/c, Prometheus 2.53.3, the official MCP client (Python `mcp`).

## Adding a shard

1000 keys written with 1 shard, then shard-2 started:

```
health 1 shard:       {"migration":"idle","replicas":3,"shards":1,"version":"test123",...}
shard-1-a             kv_keys 1000

coordinator log:      shard-2 joined, moving its keys to it
                      moved 499 keys from shard-1
                      adding shard-2: done, 499 keys moved

health after:         {"migration":"idle","replicas":6,"shards":2,...}
/admin/migration:     {"state":"idle","last":{"state":"adding","shard":"shard-2","copied":499,"total":499}}

kv_keys  shard-1-a/b/c 501 / 501 / 501    shard-2-a/b/c 499 / 499 / 499
reads not 200 after the move: 0 of 1000
```

Only the 499 keys that belong to shard-2 on the new ring were copied; shard-1 dropped them after (purge), on all its replicas.

## Draining a shard

```
POST /admin/shards/shard-2/drain   {"state":"draining","shard":"shard-2",...}
/admin/migration after:            {"state":"idle","last":{"state":"draining","shard":"shard-2","copied":499,"total":499}}
health:                            {"replicas":3,"shards":1,...}
kv_keys  shard-1-a/b/c             1000 / 1000 / 1000
reads not 200 after the drain:     0 of 1000

old shard-2-a log:  heartbeat failed: coordinator answered 410 Gone
POST /admin/shards/shard-1/drain without token: 401
with token:                                     can't drain the last shard
/version  coordinator: {"service":"coordinator","version":"test123"}
          shard-1-a:   {"replica":"shard-1-a","service":"shard","version":"test123"}
```

## Autoscaler (dry run)

8 parallel writers with `w=3`, autoscaler with small thresholds so it decides in seconds (`--dry-run --every 5s --up 1ms --up-for 10s --cooldown 1m`):

```
{"msg":"tick","p99_ms":9.8,"writes_per_s":64.1,"shards":1,"action":"","reason":""}
{"msg":"tick","p99_ms":9.8,"writes_per_s":64.1,"shards":1,"action":"","reason":""}
{"msg":"tick","p99_ms":9.6,"writes_per_s":113.7,"shards":1,"action":"up","reason":"write p99 above 1 ms for 10s"}
{"msg":"tick","p99_ms":9.6,"writes_per_s":113.7,"shards":1,"action":"","reason":"cooldown"}
```

## Ops agent (official MCP client)

shard-1-c stopped:

```
server: kvstore-ops | instructions: You are the ops agent of a small key-value store ...
tools: ['get_alerts', 'query_metric', 'get_cluster']
get_cluster   ok {"health":{"migration":"idle","replicas":3,"shards":1,...},"migration":{"last":{"state":"draining","shard":"shard-2","copied":499,"total":499},"state":"idle",...}}
get_alerts    ok {"alerts":[{"name":"NodeDown","state":"firing","labels":{"replica":"shard-1-c","shard":"shard-1",...},"summary":"replicas shard-1-c (localhost:9103) does not answer"}, ...]}
query_metric  ok {"series":[{"labels":{},"value":"0.004944444444444444"}],"total":1,"type":"vector"}
```
