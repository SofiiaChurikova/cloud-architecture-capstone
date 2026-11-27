# M5 benchmark - YCSB with autoscaling

Same workload as M2 / M3 (`bench/workload-a.properties`: 50% read / 50% update, zipfian, 10 000 records), from CloudShell in us-east-1, session of 2026-11-27. The autoscaler ran on my laptop (`cmd/autoscaler`) with the default rules: up when write p99 > 60 ms (or > 1% failed writes) for 2 min, down when < 20 ms and under 100 writes/s (or no traffic) for 5 min, 5 min cooldown, no action without data (ADR-0020).

Raw output: `evidence/ycsb-long.txt`, `evidence/ycsb-slow-growth.txt`, `evidence/ycsb-slow-growth-2.txt`, `evidence/ycsb-spike.txt`; autoscaler decisions: `evidence/autoscaler-run1.log`, `evidence/autoscaler-run2.log`; screenshots in `evidence/`. The timeline of the whole session is in `evidence/aws-demo.md`.

## How I ran it

```bash
URL=http://<coordinator-ip>:8080; read -s TOKEN
COMMON="-P bench/workload-a.properties -p kvstore.url=$URL -p kvstore.token=$TOKEN -p table=m5"
./go-ycsb load kvstore $COMMON -p threadcount=16

# long high load: 64 threads, no target
./go-ycsb run kvstore $COMMON -p threadcount=64 -p operationcount=10000000

# slow growth: fixed rate steps, operationcount = target x seconds
for t in 200 400 800; do
  ./go-ycsb run kvstore $COMMON -p threadcount=32 -p target=$t -p operationcount=$((t*120))
done

# spike: 3 min quiet, then 64 threads
./go-ycsb run kvstore $COMMON -p threadcount=4 -p target=100 -p operationcount=18000
./go-ycsb run kvstore $COMMON -p threadcount=64 -p operationcount=150000
```

This go-ycsb build ignores `maxexecutiontime` (the long run went 1110 s instead of 900 s), so the step length is set with `operationcount`. `target` is total ops/s, half of it are writes.

## Results

### 1. Long high load (64 threads, starts on 1 shard)

| Time | Shards | Writes/s | Write p99 (coordinator) | What happened |
|---|---|---|---|---|
| 00:32 | 1 | 470 | ~248 ms | load starts; YCSB update avg 132 ms, p99 203 ms |
| 00:34:25 | 1 | 470 | ~248 ms | 2 min above 60 ms -> **scale up** |
| 00:35:24 | 2 | 520 | ~248 ms | shard-2 on the ring, 4 839 keys moved, 0 errors |
| 00:36-00:40 | 2 | **~900** | ~240 ms | throughput x2; coordinator p50 165 -> 65 ms |
| 00:41 / 00:46 | 2 | falls | up to 2.5 s | shard-1 leader, then shard-2 leader stop answering (~262 000 / ~266 000 log entries) |
| 00:47-00:51 | 2 | 100% errors | no data | autoscaler: "no data from Prometheus, doing nothing" - it did not drain under load |

With 64 threads and no target the client always fills the queue again, so a second shard gives more throughput, not lower p99. p99 on the coordinator stays ~240 ms also because the histogram has no bucket between 100 and 250 ms.

### 2. Slow growth (fresh cluster, starts on 1 shard)

| Step | Shards | Writes/s | YCSB update avg | YCSB update p99 | Coordinator write p99 |
|---|---|---|---|---|---|
| 200 ops/s | 1 | 100 | 8.9 ms | 28 ms | 20-40 ms |
| 400 ops/s | 1 | 200 | 10.1 ms | 43 ms | 40-50 ms |
| 800 ops/s, 2 min | 1 | 400 | 23.6 ms | 138 ms | 130-200 ms |
| 800 ops/s, 5 min | 1 -> 2 | 400 | 18.9 ms | 91 ms | 85-107 ms, **scale up 01:49:26**, shard-2 on the ring 01:50:24 |
| 800 ops/s, 2 min | 2 | 400 | **12.5 ms** | 98 ms | 76-120 ms |

- **One shard holds ~200 writes/s with p99 under 60 ms**, at 400 writes/s it doesn't. This is C1 from the AI review: the scale-down limit of 100 writes/s is half of it, as planned.
- The first 2-minute step at 400 writes/s was too short for the 2-minute rule; p99 also dipped to 58 ms once, which restarted the timer. Both are the rule working as written.
- On 2 shards avg went 18.9 -> 12.5 ms and p50 15 -> 9 ms, but p99 stayed. Per shard: shard-1 56 ms, the new shard-2 137 ms. The new instances are slow at first (fresh gp2 volumes from the AMI snapshot are slow on the first write to each block), so a shard right after scale up is not at full speed yet.

### 3. Spike (1 shard)

| Phase | Writes/s | YCSB update avg | YCSB update p99 | What happened |
|---|---|---|---|---|
| quiet, 3 min | 50 | 7.4 ms | 19 ms | fine |
| spike, 64 threads | up to 1 345 incl. errors | 134 ms | 341 ms | the shard-1 leader stopped answering after ~50 s (log ~275 000 entries); the autoscaler never saw 2 minutes of high p99, no scale up |

A spike is over (or has broken something) before a new shard can help: the decision alone needs 2 minutes, and a new shard needs another ~1 minute (ADR-0020).

### Active nodes over time

Shards on the ring / replica nodes, from the autoscaler logs and the Grafana panel "Scaling: shards on the ring and active nodes":

```
00:02  1 -> 2 shards (deploy)      6 nodes
00:28  2 -> 1 (idle 5 min)         3 nodes
00:35  1 -> 2 (long load)          6 nodes
00:57  2 -> 1 (idle after reboot)  3 nodes
--- redeploy ---
01:33  2 -> 1 (idle)               3 nodes
01:50  1 -> 2 (slow growth)        6 nodes
01:59  2 -> 1 (idle)               3 nodes
```

## What I found: the leader runs out of memory on long runs

The leader stopped answering 4 times, always when its log reached **~260 000-320 000 entries**, at 200 to 450 writes/s. I could only see the console after the first time: there were no kernel OOM lines, the process just stopped answering, and after a reboot it took ~4 minutes to read its log back. The M3 log (ADR-0012) keeps every entry in memory and is never compacted, and a t3.micro has 1 GB. So the real limit of my store is not the write rate but the total number of writes since the deploy. The fix is a snapshot of the store plus truncating the log (and a follower that is behind starts from the snapshot) - I didn't do it, it's outside the M5 task.

## Compared with M2

| | M2 (2 shards, no replicas) | M3 (2 shards x 3, W=2) | M5 (1-2 shards x 3, W=2, autoscaling) |
|---|---|---|---|
| Updates/s, saturated | 2 985 | ~875 | 470 on 1 shard, ~900 on 2 |
| Update avg | 2.64 ms | ~16 ms | 8-12 ms at 100-200 writes/s per shard |
| Update p99 | 21.2 ms | - | 28-43 ms at 100-200 writes/s on 1 shard |
| Read p99 | 21.0 ms | 3-6 ms | 6-14 ms |

**Where I won:**
- The store changes its size by itself: down to 1 shard without load (~0.068 instead of ~0.102 $/h, `budget.md`) and back to 2 under load, with keys moved and 0 failed requests during the moves.
- Data survives the moves and a leader restart: 10 000 keys before and after every move.
- Under load, 2 shards give 2x the throughput of 1 shard.

**Where I lost:**
- Writes are ~3x slower than M2, because every write waits for W=2 replicas and a fsync (M3).
- A spike is not absorbed: scale up takes minutes.
- Long runs hit the memory limit of the in-memory log, which M2 didn't have because it didn't keep a log.
- Each pattern ran once (3-hour session), so the numbers are from one run.
