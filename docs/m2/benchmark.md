# M2 benchmark (YCSB)

Baseline for the sharded store. In M3 and M5 I run the same thing and compare.

## Setup
- 1 coordinator + 2 shards, all t3.micro, us-east-1
- YCSB: go-ycsb v1.0.3 with my binding `bench/kvstore` (every operation is one REST call to the coordinator)
- Client: AWS CloudShell in us-east-1, so the network is close to the cluster
- Workload A (`bench/workload-a.properties`): 50% read / 50% update, zipfian, 10 000 records, 50 000 operations, 16 threads
- Update = PUT of the whole item, because my API has no partial update

## How I ran it
```
docker run --rm -v "$PWD":/w -w /w golang:1.22 ./bench/build-ycsb.sh   # gives ./go-ycsb for linux
# upload go-ycsb and bench/workload-a.properties to CloudShell, then:
URL=http://<coordinator-ip>:8080
curl $URL/health        # start only when it shows "shards":2
./go-ycsb load kvstore -P workload-a.properties -p kvstore.url=$URL -p threadcount=16
./go-ycsb run  kvstore -P workload-a.properties -p kvstore.url=$URL -p threadcount=16 | tee ycsb-output.txt
aws s3 cp ycsb-output.txt s3://capstone-code-<account>/ycsb/m2/
aws s3 cp workload-a.properties s3://capstone-code-<account>/ycsb/m2/
```

The bucket has versioning on (lecture 8). The sandbox deletes it at the end of the session, so the copy in the repo is the main one.

## Results

| | ops/sec | avg latency (ms) | p99 latency (ms) |
|---|---|---|---|
| load (INSERT) | TODO | TODO | TODO |
| run - READ | TODO | TODO | TODO |
| run - UPDATE | TODO | TODO | TODO |
| run - total | TODO | - | - |

## What I see
TODO - 2-3 sentences: where the time goes (client -> coordinator -> shard is 2 network hops), what would be the bottleneck.

Raw output: `docs/m2/evidence/ycsb-output.txt`
