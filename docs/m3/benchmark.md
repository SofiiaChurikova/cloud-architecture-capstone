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
```

Every run uses its own table, so the runs don't read each other's data.

## Results

Run on 2026-11-18. Ops/sec is for the whole run (50% reads, 50% updates), the other columns are per operation type.

| Run | Total ops/sec | UPDATE ops/sec | UPDATE avg (ms) | UPDATE p99 (ms) | READ avg (ms) | READ p99 (ms) | Read errors |
|---|---|---|---|---|---|---|---|
| M2 baseline (no replicas) | - | 2985 | 2.64 | 21.2 | 2.63 | 21.0 | 0 |
| W=1, R=1 | 1746 | 875 | 16.25 | 40.7 | 1.75 | 5.5 | 1 |
| W=1, R=3 | 1743 | 875 | 16.12 | 31.0 | 1.94 | 3.4 | 0 |
| W=2, R=2 | 1754 | 874 | 16.05 | 32.8 | 1.92 | 3.6 | 0 |
| W=3, R=1 | 1659 | 822 | 17.43 | 39.7 | 1.75 | 4.6 | 2 |

Load phase (10 000 inserts, no errors in any run):

| Run | INSERT ops/sec | INSERT avg (ms) | INSERT p99 (ms) |
|---|---|---|---|
| W=1 | 947 / 939 | 16.70 / 16.84 | 32.2 / 32.1 |
| W=2 | 827 | 19.03 | 54.2 |
| W=3 | 844 | 18.73 | 34.9 |

## What I see

- **Writes got ~3.4x slower than in M2 (2985 -> ~875 update ops/s, 2.6 -> ~16 ms avg), and W almost doesn't matter.** W=1, 2, 3 give 16.3 / 16.1 / 17.4 ms. So the cost is not waiting for followers, it's the fsync on the leader. Every write is appended and fsynced under one mutex (ADR-0012), so writes to one leader go one by one: ~875 / 2 leaders = ~440 writes/s per leader, about 2.3 ms per fsync on gp2. The 16 YCSB threads queue behind that, which is where the 16 ms avg comes from. In M2 a write was only a map update in memory.
- **Waiting for followers is visible in the load phase:** W=1 ~945 inserts/s and 16.7 ms, W=2 / W=3 ~835 inserts/s and ~19 ms. That's the extra round trip: the follower long poll returns, the follower fsyncs, and its next pull is the ack. W=2 and W=3 cost about the same, because the leader waits for followers in parallel and both are usually equally fast.
- **Reads didn't get slower: ~1.75-1.94 ms vs 2.63 ms in M2, p99 3-6 ms vs 21 ms.** Reads don't touch the disk. R=3 adds ~0.2 ms to the average compared with R=1 (1.94 vs 1.75), because the coordinator asks the replicas in parallel and waits for the slowest of 3. Reads are probably faster than in M2 because the whole system does fewer ops/s now, most YCSB threads wait on writes, so there's less load on the coordinator - I didn't measure this separately.
- **R + W > N didn't cost anything extra here** (W=2, R=2 has the same numbers as W=1, R=1), because the price of writes is the fsync, not the quorum.
- **Read errors:** 1 of ~25 000 reads in W=1, R=1 and 2 in W=3, R=1, all answered in 1-2 ms. go-ycsb counts every non-200 answer as an error and doesn't print the status, and the load phase had no errors, so I don't know the cause for sure. For W=3 it can't be a follower that doesn't have the key yet (every insert was confirmed by all 3 replicas). I'd log the status in `bench/kvstore/db.go` before the next run to find out.
- **What would make writes faster** (not done, out of M3 scope): group commit - one fsync for several writes that arrived together, which is what real databases do; or a gp3 / faster disk.

The numbers are copied from the final summary lines of go-ycsb (`READ`, `UPDATE`, `INSERT`, `TOTAL`) of each run.
