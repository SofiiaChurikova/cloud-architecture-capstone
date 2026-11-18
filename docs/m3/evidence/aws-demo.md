# M3 demo in AWS (task 3.2)

Run on 2026-11-18 against the stack deployed by the pipeline from `main` (7 x t3.micro, us-east-1). Coordinator `http://98.83.233.216:8080` (the IP changes every session). Output is copied from my terminal, `Date` headers removed.

## 1. Cluster is up

```
$ curl $URL/health
{"n":3,"r":2,"replicas":6,"shards":2,"status":"ok","w":2}

$ curl -s $URL/internal/shards
shard-1: shard-1-a leader (10.20.16.126), shard-1-b follower (10.20.44.196), shard-1-c follower (10.20.16.186), all alive
shard-2: shard-2-a leader (10.20.39.204), shard-2-b follower (10.20.22.150), shard-2-c follower (10.20.45.218), all alive
```

## 2. Write goes through the leader, reads are spread over replicas

```
$ curl -X PUT $URL/tables/users -d '{"partition_key":"id"}'
{"name":"users","partition_key":"id"}

$ curl -i -X PUT $URL/tables/users/items/42 -d '{"id":"42","v":1}'
HTTP/1.1 200 OK
X-Replicas: shard-1-a
X-Shard-Id: shard-1
X-Version: 1

$ curl -i "$URL/tables/users/items/42?r=1"      # 3 times
X-Replicas: shard-1-b   X-Version: 1   {"id":"42","v":1}
X-Replicas: shard-1-c   X-Version: 1   {"id":"42","v":1}
X-Replicas: shard-1-a   X-Version: 1   {"id":"42","v":1}

$ curl -i "$URL/tables/users/items/42?r=3"
HTTP/1.1 200 OK
X-Replicas: shard-1-c,shard-1-a,shard-1-b
X-Version: 1
{"id":"42","v":1}
```

Key 42 is on shard-1. With R=1 the coordinator asks a different replica every time (round robin), and both followers already have version 1.

## 3. A follower goes down

```
$ aws ec2 stop-instances --instance-ids i-07e980b5e3300ac36     # shard-1-b
"CurrentState": "stopping", "PreviousState": "running"

$ sleep 10; curl -s $URL/internal/shards
shard-1-b ... "alive":false        (everything else alive)
```

## 4. Quorum with 2 of 3 replicas

```
$ curl -i -X PUT "$URL/tables/users/items/42?w=2" -d '{"id":"42","v":2}'
HTTP/1.1 200 OK
X-Replicas: shard-1-a
X-Version: 2

$ curl -i -X PUT "$URL/tables/users/items/42?w=3" -d '{"id":"42","v":3}'
HTTP/1.1 503 Service Unavailable
X-Replicas: shard-1-a
X-Version: 3
only 2 of 3 replicas confirmed

$ curl -i "$URL/tables/users/items/42?r=2"
HTTP/1.1 200 OK
X-Replicas: shard-1-a,shard-1-c
X-Version: 3
{"id":"42","v":3}
```

- W=2 works with one follower down (leader + shard-1-c), W=3 can't get 3 acks and returns 503 after the 2 s ack timeout.
- R=2 still works, it asks the 2 live replicas.
- The W=3 write got 503, but it is in the leader's log as version 3 and the next read returns it. 503 means "not confirmed by W replicas", not "not written" - there is no rollback. This is the limitation written in ADR-0010 consequences.

## 5. The follower comes back and catches up

```
$ aws ec2 start-instances --instance-ids i-07e980b5e3300ac36
"CurrentState": "pending", "PreviousState": "stopped"

$ sleep 90; curl -s $URL/internal/shards
shard-1-b ... "alive":true

$ curl -i "$URL/tables/users/items/42?r=3"
HTTP/1.1 200 OK
X-Replicas: shard-1-c,shard-1-a,shard-1-b
X-Version: 3
{"id":"42","v":3}
```

shard-1-b missed versions 2 and 3 while it was stopped. After the start it opened its own log from the EBS disk (last offset 1) and asked the leader for entries from offset 2. With R=3 all three replicas answer, and the newest version is 3.

To check that shard-1-b itself caught up (and the 3 didn't just come from another replica), R=1 asks one replica at a time:

```
$ for i in 1 2 3 4 5 6; do curl -si "$URL/tables/users/items/42?r=1" | grep -E "X-Replicas|X-Version"; done
X-Replicas: shard-1-b   X-Version: 3
X-Replicas: shard-1-c   X-Version: 3
X-Replicas: shard-1-a   X-Version: 3
X-Replicas: shard-1-b   X-Version: 3
X-Replicas: shard-1-c   X-Version: 3
X-Replicas: shard-1-a   X-Version: 3
```

shard-1-b answers version 3 on its own, so the resync pulled the 2 missed entries.
