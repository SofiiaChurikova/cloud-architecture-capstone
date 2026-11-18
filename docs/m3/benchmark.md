# M3 benchmark (YCSB)

Same workload as the M2 baseline (`docs/m2/benchmark.md`), now with 2 shards x 3 replicas. I run it for W=1, 2, 3. R is set so that R + W > 3 in every run, except the first one, which is the "no quorum" case.

## Setup
- 1 coordinator + 6 replicas, all `t3.micro`, us-east-1, replicas in 2 AZs
- Disks: 8 GB `gp2`, same as in M2, so M2 vs M3 compares the same disks. gp2 has 100 IOPS base and bursts to 3000 IOPS from a 5.4M I/O credit bucket. One run is ~17 500 fsync per leader, 4 runs are far below the bucket, so the burst doesn't run out in the middle (AI review point 4)
- go-ycsb with `bench/kvstore`, workload A (`bench/workload-a.properties`): 50% read / 50% update, zipfian, 10 000 records, 50 000 operations, 16 threads
- Client: AWS CloudShell in us-east-1
- W and R per run with `-p kvstore.w=.. -p kvstore.r=..` (sent as `?w=&r=`)

## How I ran it
In CloudShell, same as M2, but `bench/kvstore/db.go` uploaded again (the M3 version sends `w` and `r`) and `./bench/build-ycsb.sh` run again:
```
URL=http://<coordinator-ip>:8080
curl $URL/health        # start only when it shows "shards":2,"replicas":6
for wr in "1 1" "1 3" "2 2" "3 1"; do
  set -- $wr
  ./go-ycsb load kvstore -P bench/workload-a.properties -p kvstore.url=$URL -p kvstore.w=$1 -p kvstore.r=$2 -p threadcount=16 -p table=w$1r$2 | tee load-w$1r$2.txt
  ./go-ycsb run  kvstore -P bench/workload-a.properties -p kvstore.url=$URL -p kvstore.w=$1 -p kvstore.r=$2 -p threadcount=16 -p table=w$1r$2 | tee run-w$1r$2.txt
done
aws s3 cp . s3://capstone-code-<account>/ycsb/m3/ --recursive --exclude "*" --include "*.txt"
```

Every run uses its own table, so the runs don't read each other's data.

## Results

| Run | UPDATE ops/sec | UPDATE avg (ms) | UPDATE p99 (ms) | READ avg (ms) | READ p99 (ms) |
|---|---|---|---|---|---|
| M2 baseline (no replicas) | 2985 | 2.64 | 21.2 | 2.63 | 21.0 |
| W=1, R=1 | TODO | TODO | TODO | TODO | TODO |
| W=1, R=3 | TODO | TODO | TODO | TODO | TODO |
| W=2, R=2 | TODO | TODO | TODO | TODO | TODO |
| W=3, R=1 | TODO | TODO | TODO | TODO | TODO |

## What I see
TODO - after the run: how much p99 grew compared with M2 and why (fsync on every write, waiting for followers, more hops for reads with R > 1, cross-AZ).

Raw output: `docs/m3/evidence/ycsb/`
