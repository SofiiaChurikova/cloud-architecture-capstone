# Spec Delta

## Purpose

Quorum writes and reads with versions, so a read with R + W > N sees the last committed write.

## ADDED Requirements

### Requirement: Every item has a version
Every item SHALL have a version: the log offset of its last write. Replicas SHALL return it in the `X-Version` header, and a replica SHALL NOT overwrite an item with an older version.

#### Scenario: Version in the answer
- **WHEN** a client reads an item
- **THEN** the answer has `X-Version` with the offset of the write that the item comes from

### Requirement: Write commits after W replicas
A write SHALL be committed when W replicas have it on disk (the leader counts as one). The coordinator SHALL answer `200` only after that, and `503` if W replicas don't confirm in time.

#### Scenario: W=2 with one follower down
- **WHEN** one follower is down and a client writes with W=2
- **THEN** the leader and the other follower have the write and the client gets `200`

#### Scenario: W=3 with one follower down
- **WHEN** one follower is down and a client writes with W=3
- **THEN** the client gets `503`

### Requirement: Read returns the newest of R replicas
A read SHALL ask R replicas of the shard and return the item with the highest version. If the newest version is a delete, the read SHALL return `404`.

#### Scenario: One replica is behind
- **WHEN** one replica has an old version of a key and the others have the new one, and a client reads with R=2
- **THEN** at least one of the 2 answers has the new version and the client gets the new version

### Requirement: N / W / R can be set
N SHALL be 3. W and R SHALL have defaults (W=2, R=2) and SHALL be settable for one request with the `w` and `r` query parameters, from 1 to 3.

#### Scenario: Read with R=3
- **WHEN** a client reads `/tables/users/items/42?r=3`
- **THEN** the coordinator asks all 3 replicas

#### Scenario: Wrong value
- **WHEN** a client sends `w=0` or `r=4`
- **THEN** the coordinator answers `400`
