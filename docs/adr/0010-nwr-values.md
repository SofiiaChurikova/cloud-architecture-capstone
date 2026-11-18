# ADR-0010: Values of N / W / R

**Status:** Accepted

## Context
Every shard has replicas now. I need to decide how many (N), how many of them must have a write before it counts (W), and how many a read asks (R). If R + W > N, the replicas that got the write and the replicas that are read always overlap, so a read sees the last committed write.

My access pattern: YCSB workload A, 50% reads and 50% updates of the same keys. Writes are as important as reads.

## Options
N = 3, the task says 3+. More replicas = more instances, and the sandbox allows 9.

| W | R | R + W > 3? | Good for | Bad |
|---|---|---|---|---|
| 1 | 1 | no | fastest | a read can be old |
| 1 | 3 | yes | fast writes (logs, metrics) | a read needs all 3 replicas up |
| 2 | 2 | yes | balance | both a write and a read wait for a second replica |
| 3 | 1 | yes | fast reads | a write fails if any replica is down |

## Decision
N = 3, default W = 2, R = 2. W and R can be changed for one request with `?w=` and `?r=`, so the benchmark and the demo can try other values.

## Why
- With 50/50 reads and writes neither side should be the slow one, so 2 / 2.
- One replica can be down and both writes and reads still work. With W=3 or R=3 one dead node stops writes or reads.
- W=1 (leader only, async followers) is what the milestone doc says for single-leader. I keep it as an option and measure it, but by default I want a write to be on 2 disks before I say OK.

## Consequences
- (+) Survives one replica down for both reads and writes.
- (+) W / R per request makes it easy to show the difference in the demo and in YCSB.
- (-) Every write waits for one follower, every read waits for 2 replicas, so latency is higher than in M2.
- (-) A client can choose W=1, R=1 and get old data. That's allowed on purpose, for the comparison.
- (-) A `503` on a write doesn't mean the write is gone. The leader already has it in its log, and followers get it later. There is no rollback. A retry is a new write with a new version, so it's not idempotent and can overwrite another client's write made in between.
- (-) Replicas apply an entry before it's committed, so a read that includes the leader can see a write that later returned `503`, and two reads in a row can go from new to old. R + W > N gives "a read sees the last committed write", not linearizability.
- (-) Version = log offset of the shard. That only works while the shard has one fixed leader. With failover (M4) and moving keys between shards (M5) the version needs a term or a clock, I'll decide that in the M4 ADR.

**Where it lives:** `cmd/coordinator/main.go` (`W`, `R` env, `w` / `r` query), `internal/replica` (leader waits for W)
