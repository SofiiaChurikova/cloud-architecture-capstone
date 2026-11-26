# Spec Delta

## Purpose

The number of shards follows the load, by tail latency.

## ADDED Requirements

### Requirement: Scale on write p99
The autoscaler SHALL read the write p99 of the coordinator from Prometheus every 15 s. It SHALL add a shard when p99 is above 60 ms for 2 minutes, and remove one when p99 is below 20 ms (or there is no traffic) for 5 minutes. After any action it SHALL wait 5 minutes before the next one. It SHALL NOT scale on CPU.

#### Scenario: Load grows
- **WHEN** the store has 1 shard and write p99 stays above 60 ms for 2 minutes
- **THEN** the autoscaler sets `ShardCount=2`

#### Scenario: Load is gone
- **WHEN** the store has 2 shards and write p99 stays below 20 ms for 5 minutes
- **THEN** the autoscaler drains shard-2, waits until the move is done, and then sets `ShardCount=1`

### Requirement: Instances from the template
Shard instances SHALL be created and deleted only through the app stack (`ShardCount` 1 or 2). New replicas SHALL register with the coordinator on their own.

#### Scenario: Scale up
- **WHEN** `ShardCount` changes from 1 to 2
- **THEN** CloudFormation creates shard-2-a/b/c and they start serving traffic after the move
