# ADR-0006: Partition key strategy

**Status:** Accepted

## Context
Every table has a partition key. I need to decide what is hashed to pick the shard.

## Options
1. **Hash only the partition key** - same key from two tables lands on the same shard.
2. **Hash table name + partition key** - tables are spread separately.

## Decision
Ring position = `xxhash(table + "#" + partition_key)`.

## Why
- Like DynamoDB from lecture 4: `hash(partition key)` picks the partition.
- With the table name in the hash, two tables with the same keys don't all go to one shard.
- The partition key should have many different values (like user id), so the keys spread evenly.

## What the key looks like (lecture 8: Long vs UUIDv7)
The key is a string that the client sends, the store doesn't generate ids.
- **Long (auto-increment)** - someone has to hand out the next number, that would be the coordinator, so one more bottleneck and one more thing to keep. And numbers go in order, so with range sharding all new writes would land on one shard.
- **UUIDv7** - the client makes it alone, no coordination. It's also sorted by time, could be useful later for a sort key.

So I suggest UUIDv7 (or any id with many values) for clients. For the ring it doesn't matter much, because the key is hashed anyway.

## Consequences
- (+) Simple and even.
- (-) A hot key (one key with a lot of traffic) still goes to one shard. That's fine for M2.

**Where it lives:** `internal/ring`
