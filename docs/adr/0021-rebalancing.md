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

## Consequences
- (+) Writes and reads keep working during the move.
- (+) Only keys that changed owner are copied (consistent hashing: about 1/N of the keys).
- (+) Uses what is already there: versions, last write wins, the log.
- (-) During the move a read of a not-yet-copied key costs two round trips.
- (-) The move state is only in the coordinator's memory. If the coordinator restarts in the middle, the move has to be started again by hand.
- (-) With 10 000 keys the dump is a few MB in one answer. For much more data it would need paging.

**Where it lives:** `cmd/coordinator` (migration), `internal/replica` (dump, import, purge, floor), `internal/wal`, `internal/store`, `internal/ring`
