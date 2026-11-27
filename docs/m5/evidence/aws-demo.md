# M5 - AWS demo, 2026-11-27

Stack from the pipeline (commit `d26a5d6`, `ShardCount=2`), autoscaler and ops agent on my laptop, YCSB from CloudShell. Times are local (UTC+2). The autoscaler logs are `autoscaler-run1.log` (first cluster) and `autoscaler-run2.log` (after the redeploy). Benchmark numbers: `../benchmark.md`.

## 1. Start

`/health`: `"shards":2, "replicas":6, "migration":"idle", "version":"d26a5d6"`, `/version`: `{"service":"coordinator","version":"d26a5d6"}`, `/tables` with the token: `200`.

YCSB load, 10 000 keys in 12.5 s. Keys per shard (`max by (shard) (kv_keys)`):

| shard-1 | shard-2 |
|---|---|
| 5 161 | 4 839 |

Screenshots: `before-keys.png`, `before-scaling.png`.

## 2. Scale down without load

```
00:22:54  autoscaler started, 2 shards, 0 writes/s
00:28:04  "action":"down", "reason":"write p99 below 20 ms and under 100 writes/s (or no traffic) for 5m0s"
00:28:39  shards 1, stack_busy (CloudFormation deletes Shard2A/B/C)
00:29:18  stack done, cooldown
```

`/health` after: `"shards":1, "replicas":3, "last":{"state":"draining","shard":"shard-2","copied":4839,"total":4839}`. Keys: shard-1 **10 000**. The "Keys per shard" panel still showed the old 4 839 for the deleted shard (last value of a series that is gone); the panel query is now instant, so a removed shard disappears. Screenshots: `after-drain-keys.png`, `after-drain-move.png`.

## 3. Scale up under load

YCSB 64 threads from 00:32:

```
00:32:20  p99 248.8 ms, 172.9 writes/s, shards 1, cooldown
00:34:25  "action":"up", "reason":"write p99 above 60 ms or more than 1% errors for 2m0s"
00:34:47  stack_busy (CloudFormation creates Shard2A/B/C)
00:35:24  shards 2, 4 839 keys moved to shard-2, 0 errors
00:36:17  894 writes/s (was 470)
```

Keys after: shard-1 5 162, shard-2 4 839 - the same keys went back (the extra key is `user10000`: YCSB's key range is `[0, 10000]`, an update created it). Screenshots: `after-scaleup-keys.png`, `after-scaleup-scaling.png` (shards 2 -> 1 -> 2, replica nodes 6 -> 3 -> 6), `after-scaleup-throughput.png`.

## 4. Ops agent (Claude Desktop, only `kvstore-ops` on)

- First try (`ops-agent-idle-before-fix.png`): the agent read the cluster right but didn't pick one action and took the `last` move for a stuck one. Claude Desktop didn't use the instructions from `initialize` (AI review point 24). I put the rules into the `get_cluster` tool description and explained `last`.
- Idle, 2 shards (`ops-agent-idle-1.png`, `-2.png`): **SCALE DOWN**, with `up=1` for all targets, 0 writes/s for 5 min, risks (traffic may come back, the drain must finish before the nodes go), what to check after.
- Under load, 2 shards (`ops-agent-load-1.png`, `-2.png`): **DO NOTHING** - p99 241 ms at 889 writes/s and `HighWriteLatency` firing, but there are already 2 shards; load is even (435 / 452 writes/s), so the cause is something both shards share and a person should look at disks / replication.

## 5. The leaders stop answering

```
00:41:22  p99 556 ms, errors 0.9/s       shard-1-a stops answering (log ~262 000 entries)
00:46     shard-2-a the same (~266 000 entries)
00:47:02  p99 -, 3 929 writes/s, all errors -> "no data from Prometheus, doing nothing"
```

Without the "no data" rule from the AI review the autoscaler would have counted this as an idle store and drained shard-2 under load. Console output of the leaders had no kernel OOM lines. After `aws ec2 reboot-instances` both leaders were back in ~4 minutes (reading the log), no data lost. Then 5 minutes without traffic:

```
00:56:55  "action":"down"   4 839 keys to shard-1
00:57:30  shards 1, stack_busy
00:58:42  stack done
```

Screenshots: `long-run-leaders-down-1.png`, `-2.png`.

The same happened in the slow-growth run at 01:13 (shard-1-a, ~321 000 entries, `slow-run-leader-down-*.png`), so I redeployed the stack to start again with empty logs.

## 6. After the redeploy

```
01:27     YCSB load 10 000 keys (5 161 / 4 839)
01:33:42  "action":"down", 4 839 keys moved, shards 1
01:34-01:41  slow growth 100 -> 200 -> 400 writes/s
01:49:26  "action":"up" (400 writes/s, p99 85-107 ms for 2 min)
01:50:24  shards 2
01:59:50  "action":"down" (idle)
02:02-02:07  spike; shard-1-a stops answering after ~50 s (~275 000 entries), no action ("no data")
```

Screenshot: `spike-leader-down.png`.

## What this shows

- Scale up and scale down are done by the autoscaler on write p99 and traffic, not CPU; 4 downs and 2 ups in one session, each move with all keys (4 839) and 0 failed requests.
- Before / after shard sizes: 5 161 / 4 839 -> 10 000 / 0 -> 5 162 / 4 839.
- The ops agent proposes actions with numbers and reasons and can't change anything.
- The leader of a shard can't take more than ~260 000-320 000 log entries on a t3.micro (the M3 log is kept in memory) - see `../benchmark.md`.
