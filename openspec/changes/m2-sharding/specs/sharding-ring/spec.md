# Spec Delta

## Purpose

Splits the data between shards with a consistent hashing ring.

## ADDED Requirements

### Requirement: Coordinator works alone
Without shards, the coordinator SHALL store items itself.

#### Scenario: No shards
- **WHEN** no shard is registered and a client puts and reads an item
- **THEN** both requests work

### Requirement: Shards register on start
A shard SHALL register in the coordinator when it starts.

#### Scenario: Shard joins
- **WHEN** a shard sends `POST /internal/shards` with its id and address
- **THEN** the coordinator adds it to the ring

### Requirement: Writes go to the right shard
The coordinator SHALL hash the key and send the write to the shard that owns it on the ring.

#### Scenario: Same key, same shard
- **WHEN** the same key is written twice
- **THEN** both writes go to the same shard

### Requirement: Reads go to the same shard
Reads, checks and deletes SHALL use the same ring as writes.

#### Scenario: Read after write
- **WHEN** an item is written and then read through the coordinator
- **THEN** the read returns what was written

### Requirement: Keys are spread evenly
Each shard SHALL get about the same number of keys.

#### Scenario: 2 shards
- **WHEN** 100 000 random keys are put on a ring with 2 shards
- **THEN** each shard gets between 45% and 55% of them
