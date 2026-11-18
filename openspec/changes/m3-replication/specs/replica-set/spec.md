# Spec Delta

## Purpose

Every shard is a set of replicas with one leader.

## ADDED Requirements

### Requirement: Every shard has three replicas
Every shard SHALL run 3 replicas: 1 leader and 2 followers. All of them hold all keys of the shard.

#### Scenario: Replicas register
- **WHEN** the 3 replicas of `shard-1` start
- **THEN** each of them registers in the coordinator with the shard id, its own replica id, its role and its address
- **AND** `GET /internal/shards` shows `shard-1` with 3 replicas, one of them the leader

### Requirement: Replicas send heartbeats
Every replica SHALL register again every 2 s. The coordinator SHALL mark a replica as down if it hasn't heard from it for 6 s, SHALL ask down replicas only when the live ones are not enough for R, and SHALL answer `503` right away for a write to a shard whose leader is down.

#### Scenario: Coordinator restarts
- **WHEN** the coordinator restarts and loses its list of replicas
- **THEN** within 2 s every replica registers again and requests work as before

#### Scenario: Follower stops answering
- **WHEN** a follower of `shard-1` stops answering
- **THEN** after 6 s `GET /internal/shards` shows it with `alive: false`
- **AND** reads of `shard-1` with R=2 go to the other 2 replicas without waiting for it

### Requirement: Coordinator waits for all shards
When the coordinator is started with a list of expected shards (`EXPECTED_SHARDS`), it SHALL answer item requests with `503` until every expected shard has a registered leader, and SHALL NOT store items itself.

#### Scenario: Shards not registered yet
- **WHEN** a client puts an item and `shard-2` hasn't registered yet
- **THEN** the coordinator answers `503` and nothing is stored

### Requirement: Tables survive a coordinator restart
When the coordinator has `DATA_DIR`, it SHALL keep the list of tables in `$DATA_DIR/tables.json` and load it on start.

#### Scenario: Restart
- **WHEN** a table is created and the coordinator restarts
- **THEN** the table still exists and its items can be read

### Requirement: Only the leader takes writes
Writes and deletes of a shard SHALL go to its leader. A follower SHALL refuse writes.

#### Scenario: Write through the coordinator
- **WHEN** a client puts an item
- **THEN** the coordinator sends it to the leader of the shard that owns the key

#### Scenario: Write sent to a follower
- **WHEN** a write is sent directly to a follower
- **THEN** the follower answers `409` and doesn't change anything

### Requirement: Reads are spread over all replicas
The coordinator SHALL spread reads of a shard over all its replicas, not only the leader.

#### Scenario: Many reads
- **WHEN** the same key is read many times with R=1
- **THEN** the reads are served by all 3 replicas of the shard, in turn

### Requirement: Leader is down
If the leader of a shard is down, writes to this shard SHALL fail with `503`, and reads SHALL still work while R replicas are up.

#### Scenario: Leader stopped
- **WHEN** the leader of `shard-1` is stopped
- **THEN** a write to a key of `shard-1` returns `503`
- **AND** a read of that key with R=2 still returns the item from the 2 followers
