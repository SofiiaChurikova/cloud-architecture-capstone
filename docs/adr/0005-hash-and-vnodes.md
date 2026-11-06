# ADR-0005: Hash function and virtual nodes

**Status:** Accepted

## Context
The coordinator uses a consistent hashing ring to pick a shard. I need a hash function and the number of virtual nodes per shard. With only 2 shards and 1 point each, one shard can get much more keys than the other.

## Options

Hash function:
1. **MD5** - spreads keys well, but slow (it's made for security).
2. **xxHash64** - fast and spreads keys well.

Virtual nodes per shard: 1 (very uneven), or around 100-200 (close to even).

## Decision
xxHash64, 128 virtual nodes per shard.

## Why
- Fast and keys are spread well.
- 128 virtual nodes is enough to make 2 shards almost equal. I check it with a test: each shard gets 45-55% of 100 000 random keys.

## Consequences
- (+) Even load, and a new shard takes only part of the keys.
- (-) The number of virtual nodes shouldn't be changed later, otherwise a lot of keys move.

**Where it lives:** `internal/ring`
