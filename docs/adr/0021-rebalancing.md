# ADR-0021: Rebalancing when a shard is added or removed

**Status:** Accepted

## Context
When the set of shards changes, some keys get a new owner on the consistent hashing ring (ADR-0005). M5 asks to move only those keys in the background and keep the old shard serving reads until the move is done.

What I already have: every replica keeps all its items with versions (leader time, ADR-0017), deletes are tombstones, and a replica keeps the greater version (last write wins).

## Options
1. **Stop writes, copy, switch** - simple and exact, but the shard is read-only during the move.
2. **Write to the old owner during the move, then copy the changes again** - needs a second pass for keys written during the first one.
3. **Write to the new owner at once, read new-then-old, copy in the background with versions** - no stop and no second pass: if a key was written on the new owner during the move, its version is greater, so the old copy that arrives later is ignored.

## Decision
Option 3. The coordinator runs one move at a time:

1. **Start.** The coordinator builds the new ring (with the new shard, or without the drained one). It asks each shard that loses keys for its greatest version and tells each receiving leader to never give a version below it. Then writes for a key go to its owner on the new ring.
2. **Reads during the move.** Ask the new owner. If it has never seen the key (no value and no delete), ask the old owner.
3. **Copy.** For every shard that loses keys: take a dump of its items (with versions and deletes) from its leader, keep only the keys whose owner changed, and send them in batches to the new owner's leader. The leader writes them to its log with their own versions, so its followers get them too, and keeps only the newer version.
4. **Clean up.** When adding a shard, the old shard drops the keys it gave away (a purge entry in its log, so its followers drop them too; only if the version wasn't changed since). When draining, the whole shard goes away after the move.
5. **Switch.** The new ring becomes the only one; the old owner is not asked any more.

A drained shard can't come back from its old replicas' heartbeats; a shard with the same ID is accepted again only when its leader has an empty log (new instances).

Changed after the AI review (`docs/m5/ai-review.md`):
- **No fence before the switch.** Writes that started before the switch can still land on the old owner. Two things make this safe without stopping writes: the grace sleep before the dump (10 s) is twice the coordinator's request timeout to a shard (5 s), so every write the client got `200` for is done before the dump; and the floor sent to the new owner is the old owner's greatest version **plus the grace time**, so a new write on the new owner wins over anything the old owner wrote in that window, even with a few seconds of clock skew.
- **Late writes on the old owner are dropped.** After the purge the coordinator takes the dump again; any moved key still there was written by a request that already timed out (the client got an error), so it is purged too.
- **A shard joins only when all 3 of its replicas are alive**, not just the leader, otherwise W=2 writes fail during the move.
- **Only `ALLOWED_SHARDS` (shard-1, shard-2) can register**, and when the ring is empty only a shard from `EXPECTED_SHARDS` becomes the first one.
- **Import and purge** are one log write (one fsync) per batch, and a retried batch still waits for W acks, so `200` always means W replicas have it.
- **A failed read on the old owner** during the move is `503`, not `404`.

## Consequences
- (+) Writes and reads keep working during the move.
- (+) Only keys that changed owner are copied (consistent hashing: about 1/N of the keys).
- (+) Uses what is already there: versions, last write wins, the log.
- (-) During the move a read of a not-yet-copied key costs two round trips.
- (-) The move state is only in the coordinator's memory. If the coordinator restarts in the middle, the move has to be started again by hand. Changing `ShardCount` changes only the `Shard2*` resources, so a scale up or down doesn't restart the coordinator.
- (-) Correctness depends on the coordinator timeout being shorter than the grace sleep. If one of them is changed, the other has to be changed too.
- (-) Retries never give up: if the leader of a shard dies in the middle (there is no failover, ADR-0011), the move waits until it is back.
- (-) With 10 000 keys the dump is a few MB in one answer. For much more data it would need paging.

**Where it lives:** `cmd/coordinator` (migration), `internal/replica` (dump, import, purge, floor), `internal/wal`, `internal/store`, `internal/ring`
