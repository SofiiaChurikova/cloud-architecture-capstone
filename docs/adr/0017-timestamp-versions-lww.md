# ADR-0017: Timestamp versions and last write wins

**Status:** Accepted (changes the version from ADR-0012)

## Context
M4 asks for versions (vector clocks or timestamps) and a last-write-wins (LWW) strategy. In M3 the version was the log offset, which only means something inside one leader's log.

In my design every shard has one leader that orders all its writes, so two writes to one key never happen "at the same time" on different nodes. Conflicts are still possible: a replica can get entries in a different order after a resync, and a read with R > 1 gets different versions from different replicas. The store needs one rule for which value wins.

## Options
1. **Vector clocks** - each version keeps a counter per node. They show when two writes are concurrent, but then the client or a merge function has to decide. With one leader per shard there are no concurrent writes, so I'd pay the complexity for nothing.
2. **Wall clock timestamp** - simple, but the clock can go back (NTP), and two writes can get the same value.
3. **Leader timestamp that only grows** - the leader takes its clock in microseconds, and if it's not greater than the last version, uses last + 1. After a restart it reads the greatest version from its log.

## Decision
Option 3 with last write wins:
- version = leader timestamp (microseconds), always growing, stored in the log entry, returned in `X-Version`;
- a replica keeps the entry with the greater version and ignores older ones;
- a read with R > 1 returns the answer with the greatest version; a delete (tombstone) with a greater version wins over a value.

The log offset stays as it is - it's still used for replication and acks.

## Consequences
- (+) A version is a real time, so "which write was later" is clear in logs and on the dashboard.
- (+) Small change: one new field in the log entry.
- (-) LWW loses data when two writes really race: the earlier one is just gone. With one leader the leader decides the order, so this is the same as before.
- (-) If keys move between shards (M5), two leaders with different clocks could compare versions. Clock skew between them can make an older write win. Vector clocks or a hybrid clock would be needed then.
- (-) No custom merge (it's optional in M4).

**Where it lives:** `internal/wal` (`Version` in the entry), `internal/replica` (gives versions), `internal/store` (`Apply`), `cmd/coordinator` (`read`)
