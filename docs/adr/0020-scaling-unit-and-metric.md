# ADR-0020: What scales, on which metric, with which thresholds

**Status:** Accepted

## Context
M5 asks to scale the shards on p99 latency (not CPU) and scale down when the metric stays low for N minutes.

What limits me:
- the sandbox allows 9 instances; today a shard is 3 instances (leader + 2 followers, ADR-0013), plus the coordinator and the monitoring node;
- instances have no IAM role, so nothing inside AWS can call the AWS API to add or remove instances, and CloudWatch can't see my p99 (it lives in Prometheus, ADR-0015);
- from the M3 benchmark: writes are limited by the fsync on the shard leader (writes to one log go one by one), reads take ~2 ms. More shards = more leaders = less queue per leader.

## Options for what scales
1. **A whole shard (3 instances)** - nothing changes in the replica code. 2 + 3 x 2 = 8 instances, so only 1 or 2 shards fit.
2. **Several replicas per instance**, scaling by one instance - small steps and more than 2 shards, but every instance needs an agent that starts and stops replicas, and the coordinator needs placement logic. Much more code.
3. **Bigger instances (vertical)** - doesn't help the one-fsync-at-a-time queue on the leader.

## Options for who scales
1. **CloudWatch alarm + Auto Scaling policy** - needs my p99 in CloudWatch, so something with credentials has to copy it there anyway; and scaling down must first move the data, which AWS doesn't know how to do.
2. **A small program on my laptop with the lab credentials** - reads Prometheus, decides, changes the stack.

## Options for the metric
- **Write p99** - what actually grows under load (fsync queue), and what a new shard fixes.
- **Read p99** - stays ~2 ms, adding a shard doesn't change it much.
- **CPU** - not allowed by the task, and on t3 it shows credit exhaustion, not my queue.

## Decision
- Scaling unit: a whole shard. `ShardCount` in the app stack is 1 or 2; 2 is the normal state (M2 asks for at least 2), under no load it goes to 1.
- `cmd/autoscaler` on my laptop, every 15 s:
  - metric: write p99 on the coordinator, `histogram_quantile(0.99, ...{op=~"write|delete"}[1m])`, only answers that are not `5xx` (ADR-0016);
  - **scale up** when p99 > **60 ms** for **2 minutes** - 60% of the 100 ms write SLO, so it reacts before the SLO is broken, and a new shard needs ~3 minutes (stack update + boot + move);
  - **scale down** when p99 < **20 ms** or there is no traffic for **5 minutes** - low enough that one shard can take it; the longer window avoids flapping;
  - **cooldown 5 minutes** after any action, because the move itself makes p99 jump.
- Scale up: `aws cloudformation update-stack ... ShardCount=2`, the new replicas register on their own and the coordinator starts the move (ADR-0021).
- Scale down: drain shard-2 through the coordinator, wait until the move is done, then `ShardCount=1`. Instances are deleted only after their data is somewhere else.

## Consequences
- (+) No change in how a replica works; instances are only created and deleted from the template.
- (+) Scaling down never deletes data that wasn't moved.
- (-) Big steps (3 instances) and at most 2 shards. More shards need several replicas per instance (option 2).
- (-) Scale up takes ~3 minutes, so a short spike is over before the new shard helps.
- (-) The autoscaler works only while it runs on my laptop. In a real setup it would run in the cluster with an IAM role.
- (-) The thresholds come from one benchmark on t3.micro; they will be checked in the final YCSB.

**Where it lives:** `cmd/autoscaler`, `infra/app.yaml` (`ShardCount`)
