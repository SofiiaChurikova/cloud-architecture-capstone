# Spec Delta

## Purpose

When replicas or answers disagree about a key, the store picks one value the same way everywhere: the last write wins.

## ADDED Requirements

### Requirement: Timestamp version
The leader SHALL give every write a version: its clock in microseconds, and always greater than the previous version it gave (also after a restart). The version SHALL be stored in the log entry and returned in `X-Version`.

#### Scenario: Two writes in the same microsecond
- **WHEN** two writes get the same clock value
- **THEN** the second one gets the previous version + 1

### Requirement: Last write wins on replicas
A replica SHALL keep the item with the greater version and SHALL ignore an entry with a smaller or equal version.

#### Scenario: Old entry arrives later
- **WHEN** a replica gets an entry with a smaller version than the item it has
- **THEN** the item doesn't change

### Requirement: Last write wins on reads
A read with R > 1 SHALL return the answer with the greatest version. A delete with a greater version wins over a value.

#### Scenario: Replicas disagree
- **WHEN** two replicas return different versions of a key
- **THEN** the client gets the one with the greater version
