# ADR-0011: How replicas are synchronised

**Status:** Accepted

## Context
The 3 replicas of a shard have to end up with the same data. The milestone doc suggests a leader that takes writes and a log that followers consume. The sandbox has no MSK, Kinesis or SQS, and IAM is read-only, so instances can't get their own AWS credentials.

## Options
1. **Leaderless** (lecture 8.1, Dynamo) - the coordinator writes to all replicas itself. No leader to lose, but concurrent writes to one key can conflict, and conflict resolution is only in M4.
2. **Leader + Kafka / RabbitMQ** - a real log broker. MSK isn't available, a self-hosted broker takes 1-3 of the 9 instances.
3. **Leader + log file in S3** - instances would need an IAM role to write to S3, and I can't create one. Presigned URLs for every write would expire after the session.
4. **Leader + log on the leader's disk, followers pull it over HTTP** - the leader keeps the log, followers ask "give me entries from offset X", like a Kafka consumer.

## Decision
Option 4. Every shard has a fixed leader. The leader appends each write to its log and applies it. Followers send `GET /internal/log?from=X` to the leader in a loop. If there is nothing new, the leader holds the request up to 1 s (long polling), so new entries come right away. The `from` value also tells the leader what the follower already has on disk, and that's how the leader counts W.

Reads go to all 3 replicas through the coordinator, in turn, so followers also take read load.

## Why
- One writer per shard = one order of writes, no conflicts in M3 (the task says conflicts are for M4).
- No extra service and no extra instance, works inside the sandbox limits.
- Pull with an offset is the same idea as a Kafka consumer group: the follower knows where it is and can continue after a restart.

## Consequences
- (+) Simple, every write has one place where its order is decided.
- (+) Followers can catch up after any downtime, as long as the leader has the log.
- (-) The leader is a single point for writes of its shard. If it's down, writes to that shard fail until it's back. Leader failover is M4.
- (-) Followers ask the leader all the time, even when nothing changes (one request per second while idle).

**Where it lives:** `internal/replica` (leader `write`, `serveLog`, follower `Follow`), `cmd/shard/main.go`
