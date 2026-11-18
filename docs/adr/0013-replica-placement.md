# ADR-0013: Where the replicas run

**Status:** Accepted

## Context
2 shards x 3 replicas + 1 coordinator. The sandbox allows at most 9 running instances, t2 / t3 up to medium, and the network from M1 has 2 AZs. The M2 AI review warned that 3 replicas per shard fill most of the limit.

## Options
1. **One EC2 per replica** - 7 instances, every replica fails on its own.
2. **Several replicas on one EC2** - fewer instances, but one dead instance takes down several replicas of the same shard, so the replication doesn't help.
3. **Dynamo-style placement** (from the AI review): fewer nodes, every key on the next 3 nodes of the ring. Fits M5 better, but it is leaderless by nature and the milestone doc asks for a replica set per shard.

## Decision
Option 1: 7 `t3.micro`. Every shard has its replicas in both private subnets, and the 2 leaders are in different AZs:

| Shard | a (leader) | b | c |
|---|---|---|---|
| shard-1 | private-a | private-b | private-a |
| shard-2 | private-b | private-a | private-b |

## Why
- A replica set per shard is what the task asks for.
- Every replica is its own instance, so stopping one node in the demo stops one replica only.
- 7 of 9 leaves room.

## Consequences
- (+) One node or one instance down = one replica down.
- (-) With 2 AZs one AZ has 2 of the 3 replicas of a shard. If that AZ goes down, the shard loses W=2. This was already in ADR-0003, a third AZ is still possible.
- (-) Replication traffic crosses AZs, and that costs money (lecture 2), see `docs/m3/budget.md`.
- (-) In M5 a third shard would be 3 more instances = 10, over the limit. I'll have to change this in M5 (fewer replicas per new shard or Dynamo-style placement).

**Where it lives:** `infra/app.yaml`
