# M4 - local run

Run on 2026-11-24 with the binaries from `cmd/` (coordinator + shard-1-a/b/c on localhost, `API_TOKEN=tok`), Prometheus 2.53.3 with `monitoring/prometheus.yml`, and the official MCP client (Python `mcp` 2.3.0).

## Trace ID through the coordinator and the leader

```
$ curl -i -X PUT -H "Authorization: Bearer tok" "localhost:8080/tables/users/items/7?w=3" -d '{"id":"7"}'
X-Trace-Id: 0fb76c9dbce4b16e

coordinator: {"level":"INFO","msg":"request","service":"coordinator","trace_id":"0fb76c9dbce4b16e","method":"PUT","path":"/tables/users/items/7","status":200,"ms":19.094}
shard-1-a:   {"level":"INFO","msg":"request","service":"shard","replica":"shard-1-a","trace_id":"0fb76c9dbce4b16e","method":"PUT","path":"/internal/items/users/7","status":200,"ms":18.538}
```

## Token

```
$ curl -o /dev/null -w "%{http_code}" localhost:8080/tables
401
```

## Prometheus targets from the coordinator

```
$ curl localhost:8080/internal/targets
[{"targets":["localhost:9101"],"labels":{"replica":"shard-1-a","shard":"shard-1"}},
 {"targets":["localhost:9102"],"labels":{"replica":"shard-1-b","shard":"shard-1"}},
 {"targets":["localhost:9103"],"labels":{"replica":"shard-1-c","shard":"shard-1"}}]

Prometheus targets: coordinator up, shard-1-a up, shard-1-b up, shard-1-c up
```

## Metrics (part)

```
coordinator:
kv_replicas_alive{shard="shard-1"} 3
kv_requests_total{op="read",status="200"} 3
kv_requests_total{op="write",status="200"} 2
kv_request_duration_seconds_count{op="write"} 2

shard-1-a (leader):
kv_keys 1
kv_log_offset 3
kv_is_leader 1
kv_replication_lag_seconds{follower="shard-1-b"} 0
kv_replication_lag_entries{follower="shard-1-b"} 0
```

## Alerts after stopping shard-1-c

I killed shard-1-c and kept writing with `w=2`. After ~55 s:

```
NodeDown              firing  replicas shard-1-c (localhost:9103) does not answer
ShardMissingReplicas  firing  shard-1 has 2 of 3 replicas alive
ReplicationLag        firing  shard-1-c is 39.5s behind its leader
```

## MCP with the official client

```
server: kvstore 2025-06-18
tools: ['list_tables', 'exists_item', 'read_item', 'create_item', 'delete_item']
list_tables  ok    {"tables":[{"name":"users","partition_key":"id"}]}
create_item  ok    {"version":"1795533185124636"}
read_item    ok    {"item":{"id":"42","name":"Ann"},"version":"1795533185124636"}
exists_item  ok    {"exists":true}
delete_item  error not deleted: ask the user and call again with confirm=true
delete_item  ok    {"deleted":true}
read_item    error item not found (status 404)
exists_item  error table not found
```

The version is the leader time in microseconds (ADR-0017).

## After the AI review

Same setup, coordinator with the internal port 9080:

```
$ curl -o /dev/null -w "%{http_code}" localhost:8080/internal/shards
404

Prometheus targets:
coordinator  http://coordinator:9080/metrics  up
shard-1-a    http://localhost:9101/metrics    up
shard-1-b    http://localhost:9102/metrics    up
shard-1-c    http://localhost:9103/metrics    up
```

shard-1-c stopped, writes with `w=3`:

```
NodeDown              firing  replicas shard-1-c (localhost:9103) does not answer
ShardMissingReplicas  firing  shard-1 has 2 of 3 replicas alive
ReplicationLag        firing  shard-1-c is 4m 21s behind its leader

coordinator log:
{"level":"ERROR","msg":"request","service":"coordinator","trace_id":"1bfcd7a0df9e35d2","method":"PUT","path":"/tables/users/items/z1","status":503,"ms":2002.588,"shard_id":"shard-1","replicas":"shard-1-a","version":"1795534667902276","error":"only 2 of 3 replicas confirmed"}
```

`HighErrorRate` did not fire: every `w=3` write waited 2 s, so there was less than 1 request per second, and the alert needs more traffic than that on purpose.

MCP:

```
delete_item  ok     {"deleted":true}
create_item  ok     {"version":"1795534553139059"}
create_item  error  item already exists: this tool only creates new items, it does not overwrite
read_item    ok     {"item":{"id":"42"},"version":"1795534553139059"}
```
