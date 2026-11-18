# Local run - coordinator + 2 shards x 3 replicas

Coordinator on :8080, `shard-1-a/b/c` on :8081-8083, `shard-2-a/b/c` on :8084-8086, every replica with its own `DATA_DIR`. Binaries from `go build`.

```
$ curl -s localhost:8080/health
{"n":3,"r":2,"replicas":6,"shards":2,"status":"ok","w":2}

$ curl -s localhost:8080/internal/shards
[{"id":"shard-1","replicas":[{"replica":"shard-1-a","role":"leader",...},{"replica":"shard-1-b","role":"follower",...},{"replica":"shard-1-c","role":"follower",...}]},
 {"id":"shard-2","replicas":[{"replica":"shard-2-a","role":"leader",...},{"replica":"shard-2-b","role":"follower",...},{"replica":"shard-2-c","role":"follower",...}]}]
```

## Write and reads spread over replicas

```
$ PUT /tables/users/items/1  (w=2 by default)
200 OK  X-Shard-Id: shard-2  X-Replicas: shard-2-a  X-Version: 1

$ GET /tables/users/items/1?r=1   (3 times)
200 OK  X-Replicas: shard-2-b  X-Version: 1
200 OK  X-Replicas: shard-2-c  X-Version: 1
200 OK  X-Replicas: shard-2-a  X-Version: 1
```

## One follower down

```
$ stop shard-2-b

$ PUT /tables/users/items/1  w=2
200 OK  X-Replicas: shard-2-a  X-Version: 2

$ PUT /tables/users/items/1?w=3
503 Service Unavailable  X-Version: 3        # only 2 of 3 replicas confirmed

$ GET /tables/users/items/1?r=2
200 OK  X-Replicas: shard-2-c,shard-2-a  X-Version: 3
{"id":"1","name":"v3"}
```

The `w=3` write got `503`, but it is in the leader's log (version 3), so it's still there (ADR-0010).

## Follower starts again

```
shard-2-b: log has 1 entries
shard-2-b: resync from offset 2
```

It rebuilt offset 1 from its own file and asked the leader only from offset 2.

```
$ curl localhost:808{4,5,6}/health
{"items":1,"offset":3,"replica":"shard-2-a","role":"leader","shard":"shard-2"}
{"items":1,"offset":3,"replica":"shard-2-b","role":"follower","shard":"shard-2"}
{"items":1,"offset":3,"replica":"shard-2-c","role":"follower","shard":"shard-2"}

$ GET /tables/users/items/1?r=1   (3 times)
200 OK  X-Replicas: shard-2-b  X-Version: 3
200 OK  X-Replicas: shard-2-c  X-Version: 3
200 OK  X-Replicas: shard-2-a  X-Version: 3

$ tail -3 shard-2-b/log.jsonl
{"offset":1,"table":"users","key":"1","value":{"id":"1","name":"v1"}}
{"offset":2,"table":"users","key":"1","value":{"id":"1","name":"v2"}}
{"offset":3,"table":"users","key":"1","value":{"id":"1","name":"v3"}}
```

## Leader down

```
$ stop shard-2-a

$ PUT /tables/users/items/1
503 Service Unavailable                      # no leader, no writes for shard-2

$ GET /tables/users/items/1?r=2
200 OK  X-Replicas: shard-2-b,shard-2-c  X-Version: 3
```

Reads still work from the 2 followers, writes to shard-2 stop until the leader is back.

`go vet ./...` and `go test -race ./...` pass. Tests also check that with followers behind, `r=1` reads can miss a `w=1` write and `r=3` always returns it (`TestQuorumReadReturnsNewest`).

## After the AI review (ADR-0014)

Same 7 processes, coordinator with `EXPECTED_SHARDS=shard-1,shard-2` and its own `DATA_DIR`.

```
$ PUT /tables/users/items/1   (before the replicas start)
503 waiting for shards: shard-1, shard-2

(replicas start, 6 writes)
200 200 200 200 200 200

$ restart the coordinator
$ GET /tables/users                 -> {"name":"users","partition_key":"id"}   # loaded from tables.json
$ GET /tables/users/items/1         -> 503                                      # replicas not back yet
(2 s later, after the heartbeats)
$ GET /tables/users/items/1         -> 200

$ kill -STOP shard-1-b              # no answer at all, like a stopped EC2
$ GET ?r=2 (6 times)
200 0.001s | 200 0.0007s | 200 5.0s | 200 0.0009s | 200 0.0008s | 200 0.0008s

(6 s later)
$ GET /internal/shards
shard-1-a true, shard-1-b false, shard-1-c true, shard-2-a/b/c true
$ GET ?r=2 (6 times)
200 0.0009s | 200 0.0008s | 200 0.0008s | 200 0.0007s | 200 0.0008s | 200 0.0007s
```

Before the coordinator noticed it, one read still waited 5 s for the frozen follower. After 6 s without a heartbeat it is marked down and reads don't go there.
