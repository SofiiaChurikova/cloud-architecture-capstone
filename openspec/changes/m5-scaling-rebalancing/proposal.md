# Proposal

## Why

Until now the number of shards was fixed in the template. If the load grows, write latency grows with it, because every write waits for one fsync on its shard leader (M3 benchmark). M5 makes the store react on its own: add a shard when p99 is too high, remove it when the load is gone, and move only the keys whose owner changed. An ops agent reads the alerts and metrics from M4 and suggests what to do.

## What Changes

- Shards can be added and removed while the store is running. The coordinator puts a new shard on the ring and moves the keys that now belong to it in the background; during the move, reads go to the new owner first and to the old one if the key isn't there yet. Removing a shard is the same in the other direction
- The scaling unit is a whole shard (leader + 2 followers = 3 instances). The app stack gets a `ShardCount` parameter (1 or 2). 2 is the normal state, so M2's "at least 2 shards" still holds; under no load the store goes down to 1
- `cmd/autoscaler`: runs on my laptop with the lab credentials. It reads write p99 from Prometheus every 15 s and changes `ShardCount` with CloudFormation; before removing a shard it asks the coordinator to drain it
- `cmd/opsmcp`: a read-only MCP server (alerts, Prometheus queries, cluster state) and instructions for Claude, so an agent can suggest scale up / scale down / rebalance with reasons. It can't do anything
- `/version` on the coordinator and replicas (git commit of the build)
- Final YCSB with autoscaling on: slow growth, a spike, long high load

## Capabilities

### New Capabilities

- `shard-rebalancing`: adding and removing shards, moving keys, reads during the move
- `autoscaling`: what is measured, thresholds, timers, how instances are added and removed
- `ops-agent`: what the agent reads, what it suggests, what it can't do
- `build-info`: health and version endpoints
- `scaling-benchmark`: the final YCSB runs

### Modified Capabilities

None. Earlier specs are not archived yet, everything new is in new capabilities.

## Impact

- Code: `internal/ring` (remove a shard), `internal/wal` (import with a given version, purge), `internal/store` (purge), `internal/replica` (dump, import, purge, version floor), `cmd/coordinator` (migration, admin API), `cmd/shard` (log offset in the heartbeat), `cmd/autoscaler` (new), `cmd/opsmcp` (new), `internal/mcpserver` (the JSON-RPC part of `cmd/mcp` shared with `cmd/opsmcp`)
- Infra: `infra/app.yaml` (`ShardCount`, shard-2 only when it is 2, `EXPECTED_SHARDS=shard-1`), pipeline (version in the build, presigned URLs valid for the whole session), dashboard (shards on the ring, active nodes, migration)
- AWS: 5 instances with 1 shard, 8 with 2 (limit 9)
- Out of scope: more than 2 shards (would need several replicas per instance), the bonus "agent executes its proposal", optional tasks, keeping the migration state across a coordinator restart
