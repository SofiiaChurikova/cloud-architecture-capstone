# Spec Delta

## Purpose

Shards can be added and removed while the store runs, and only the keys whose owner changed are moved.

## ADDED Requirements

### Requirement: Adding a shard
When a new shard registers with a leader and 2 followers, the coordinator SHALL put it on the ring and start moving to it the keys that now belong to it. Only one move SHALL run at a time.

#### Scenario: Second shard joins
- **WHEN** the store has only shard-1 and shard-2 registers
- **THEN** shard-2 is on the ring, and the keys that now map to shard-2 are copied to it in the background

### Requirement: Only moved keys are copied
The move SHALL copy only the keys whose owner on the new ring is different from the old ring, with their versions (values and deletes). After the copy the old shard SHALL drop those keys.

#### Scenario: Key that stays
- **WHEN** a key maps to shard-1 on the old and the new ring
- **THEN** it is not copied

### Requirement: Reads and writes during the move
During a move, writes SHALL go to the owner on the new ring. A read SHALL ask the new owner, and if the new owner has never seen the key (no value and no delete), ask the old owner. The new owner SHALL give every new write a version greater than any version on the old owner.

#### Scenario: Read before the key was copied
- **WHEN** a client reads a key that has not been copied yet
- **THEN** it gets the value from the old shard

#### Scenario: Write during the move
- **WHEN** a client writes a key during the move and later the copy of the old value arrives
- **THEN** the new value stays (last write wins)

### Requirement: Removing a shard
`POST /admin/shards/{id}/drain` (with the API token) SHALL move all keys of that shard to the other shards in the same way, and then remove the shard from the ring. A removed shard SHALL NOT come back from heartbeats of its old replicas; a shard with the same ID SHALL be accepted again only when its leader has an empty log.

#### Scenario: Drain shard-2
- **WHEN** the autoscaler drains shard-2
- **THEN** all its keys are on shard-1, shard-2 is not on the ring, and its old replicas can't register again

### Requirement: Move status
The coordinator SHALL show the move state (idle, adding, draining, which shard, copied keys) on `GET /admin/migration` and as metrics.

#### Scenario: Wait for a drain
- **WHEN** the autoscaler calls `GET /admin/migration` after a drain
- **THEN** it sees `draining` until the copy is done and `idle` after
