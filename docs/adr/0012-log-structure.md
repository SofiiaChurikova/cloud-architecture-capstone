# ADR-0012: Log structure and ordering

**Status:** Accepted

## Context
The leader writes every change into a log, followers apply it. I need to decide what one entry looks like, where the log lives, what order is guaranteed, and what a version is.

## Options
Format: **JSON lines** (one JSON object per line, easy to read with `cat`) or a binary format (smaller, faster).

Where: **a file on each node's EBS disk**, or only in memory (lost on restart, so no durability).

Flush: **fsync on every write** (slower, but an acked write is on disk) or let the OS flush later (faster, last writes can be lost on a crash).

## Decision
- One file per node: `$DATA_DIR/log.jsonl`. Every line is `{"offset":N,"table":..,"key":..,"value":{..}}`, a delete has `"deleted":true` and no value.
- Offsets go 1, 2, 3, ... without gaps, per shard. Only the leader gives out offsets.
- The version of an item = the offset of its last write. A replica never replaces an item with a smaller version.
- Every node (leader and followers) writes the entries it has to its own file and calls fsync before it says OK.
- On start a node reads its file, rebuilds the items, and knows its last offset.

## Why
- Order: one leader per shard gives one order for all writes of the shard. Two writes to the same key get different offsets, the later one wins.
- The offset works as the version and as the follower's position at the same time, nothing extra to keep.
- fsync on every write is what "durably ack" means in the task. I measure what it costs with YCSB.
- JSON lines are easy to check by hand during the demo.

## Consequences
- (+) Total order inside a shard, nothing between shards (not needed: a key lives in one shard).
- (+) A restarted node gets its data back from the file.
- (-) The log only grows, there is no compaction. Fine for M3 sessions of 3 hours, cleanup by retention is in M4.
- (-) fsync makes every write slower.
- (-) If a node crashes in the middle of writing a line, the last line is broken. On start I stop at the first broken line, that entry is pulled again from the leader.

**Where it lives:** `internal/wal`, `internal/store` (versions), `internal/replica`
