# Spec Delta

## Purpose

The leader writes every change into an append-only log, followers apply the same log in the same order.

## ADDED Requirements

### Requirement: Leader appends every write to a log
The leader SHALL append every write and delete as one entry to an append-only log, with offsets 1, 2, 3, ... in the order it got them. A delete SHALL be an entry too (a tombstone).

#### Scenario: Two writes to the same key
- **WHEN** the leader gets two writes for the same key, one after another
- **THEN** they get two different offsets, and the later write has the bigger offset

### Requirement: Log is on disk
Every replica SHALL keep the log in a file on its disk and flush it to disk before it counts as written.

#### Scenario: Node restart
- **WHEN** a replica restarts
- **THEN** it reads its log file and has the same items and the same last offset as before the restart

### Requirement: Followers apply the log in order
Followers SHALL pull entries from the leader starting at their next offset and apply them in offset order.

#### Scenario: Follower catches up
- **WHEN** the leader has entries up to offset 100 and a follower has up to 90
- **THEN** the follower asks the leader for entries from 91 and applies 91..100 in this order

### Requirement: Resync only the missing range
After a restart a follower SHALL continue from its own last offset, not from the beginning of the log.

#### Scenario: Follower was down
- **WHEN** a follower with last offset 90 is down while the leader writes up to 120, and then starts again
- **THEN** it rebuilds 1..90 from its own file and asks the leader only for 91..120
