# M2 benchmark (YCSB)

Baseline for the sharded store. In M3 and M5 I run the same thing and compare.

## Setup
- 1 coordinator + 2 shards, all t3.micro, us-east-1 (coordinator and shard-1 in AZ a, shard-2 in AZ b)
- YCSB: go-ycsb v1.0.3 with my binding `bench/kvstore` (every operation is one REST call to the coordinator)
- Client: AWS CloudShell in us-east-1, so the network is close to the cluster
- Workload A (`bench/workload-a.properties`): 50% read / 50% update, zipfian, 10 000 records, 50 000 operations, 16 threads
- Update = PUT of the whole item, because my API has no partial update
- Run on 2026-11-07

## How I ran it
In CloudShell (Go installed to `/tmp`, the 3 files from `bench/` uploaded):
```
export PATH=/tmp/go/bin:$PATH GOPATH=/tmp/gopath GOCACHE=/tmp/gocache
./bench/build-ycsb.sh                      # builds ./go-ycsb with my binding
URL=http://<coordinator-ip>:8080
curl $URL/health                           # start only when it shows "shards":2
./go-ycsb load kvstore -P bench/workload-a.properties -p kvstore.url=$URL -p threadcount=16 | tee ycsb-load.txt
./go-ycsb run  kvstore -P bench/workload-a.properties -p kvstore.url=$URL -p threadcount=16 | tee ycsb-run.txt
aws s3 cp ycsb-load.txt s3://capstone-code-<account>/ycsb/m2/
aws s3 cp ycsb-run.txt  s3://capstone-code-<account>/ycsb/m2/
aws s3 cp bench/workload-a.properties s3://capstone-code-<account>/ycsb/m2/
```

The bucket has versioning on (lecture 8). The sandbox deletes it at the end of the session, so the copy in the repo is the main one.

## Results

| | ops/sec | avg (ms) | p50 (ms) | p95 (ms) | p99 (ms) |
|---|---|---|---|---|---|
| load - INSERT | 7174 | 2.17 | 1.77 | 3.41 | 16.2 |
| run - READ | 3006 | 2.63 | 1.81 | 6.67 | 21.0 |
| run - UPDATE | 2985 | 2.64 | 1.81 | 6.82 | 21.2 |
| run - total | 5990 | 2.63 | 1.81 | 6.74 | 21.0 |

## What I see
- A typical request takes ~1.8 ms, and almost all of it is network: client -> coordinator -> shard is 2 hops, the work with the map in memory is microseconds. Half of the keys are on shard-2 in the other AZ, so they also cross AZs.
- Reads and updates cost the same, because both are one HTTP call to one shard and the store has no disk.
- p99 is ~11x the median (21 ms vs 1.8 ms). The tail comes from the single coordinator on a `t3.micro`: all 16 threads go through it, it parses and forwards every request, and `t3` is burstable. It is the bottleneck of the system, not the shards.
- This is the baseline for M3: replication adds a disk write (fsync) and waiting for replicas, so I expect p99 to grow.

Raw output: `docs/m2/evidence/ycsb/ycsb-load.txt`, `docs/m2/evidence/ycsb/ycsb-run.txt`
