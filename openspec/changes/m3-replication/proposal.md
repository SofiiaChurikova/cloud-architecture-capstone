# Proposal

## Why

In M2 every key lives on one shard, in memory. If the shard dies, its keys are gone. In M3 every shard gets 3 replicas, so a shard keeps working when one follower is down (if the leader is down, the shard is read-only until M4), and its data survives a restart. The milestone doc suggests leader-based replication, so that's what I do.

## What Changes

- Every shard is a replica set: 1 leader + 2 followers
- The leader takes all writes of its shard and appends them to an append-only log
- Followers pull the log from the leader, track their offset and after a restart continue from it
- The log is also kept on disk, so a restarted node rebuilds its data from it
- Every key has a version (the log offset of its last write)
- Quorum writes and reads: write commits after W replicas have it, read asks R replicas and returns the newest version. Default N=3, W=2, R=2, can be set per request
- Reads are spread over all 3 replicas of a shard
- Replicas send heartbeats to the coordinator, the coordinator keeps the list of tables on disk and waits until all shards are registered (this was planned in the M2 AI review)
- App stack: 1 coordinator + 2 shards x 3 replicas = 7 EC2
- YCSB for different W / R, compared with the M2 baseline

## Capabilities

### New Capabilities

- `replica-set`: leader and followers of a shard, who takes writes, who serves reads
- `replication-log`: the log, its order, offsets, resync after restart
- `quorum`: N / W / R, versions, newest version wins on read
- `replica-deployment`: 7 nodes on EC2
- `replication-benchmark`: YCSB with replication vs the M2 baseline

### Modified Capabilities

None. The M2 specs are not archived yet (M2 is not defended), so everything new is in new capabilities.

## Impact

- Code: `internal/wal` (log), `internal/replica` (leader / follower), `internal/store` (versions), `cmd/shard` (heartbeat), `cmd/coordinator` (replica sets, quorum, tables on disk), `api/openapi.yaml`, `bench/kvstore`
- Infra: `infra/app.yaml` (7 instances, shards can reach each other on 8081)
- AWS: 7 of the 9 allowed instances, more cross-AZ traffic
- Out of scope: leader failover and conflict resolution (M4), adding shards (M5), Merkle trees (optional, not now)
