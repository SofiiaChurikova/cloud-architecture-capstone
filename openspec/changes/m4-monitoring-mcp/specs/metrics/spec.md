# Spec Delta

## Purpose

The coordinator and every replica measure their requests and state, so latency, throughput, errors and replication lag can be seen and alerted on.

## ADDED Requirements

### Requirement: Metrics endpoint
The coordinator (port 8080) and every replica (port 8081) SHALL expose metrics in the Prometheus text format on `GET /metrics`.

#### Scenario: Scrape a replica
- **WHEN** Prometheus calls `GET /metrics` on a replica
- **THEN** it gets `200` with the metrics below

### Requirement: Request metrics
The coordinator and replicas SHALL count requests by operation and status code and SHALL record request duration in a histogram, so p50, p95 and p99, throughput and error rate can be computed from them.

#### Scenario: Error rate
- **WHEN** some writes get `503`
- **THEN** the counter for `status="503"` grows and the error rate on the dashboard goes up

### Requirement: Cluster state metrics
The coordinator SHALL expose how many replicas of every shard are alive. Every replica SHALL expose how many keys it holds and its last log offset.

#### Scenario: A replica stops
- **WHEN** a replica sends no heartbeat for 6 s
- **THEN** the alive replicas of its shard go from 3 to 2

### Requirement: Replication lag
The leader SHALL expose, for every follower, how many entries it is behind and how many seconds ago the oldest entry it hasn't confirmed was written (0 when it has everything).

#### Scenario: Follower is down
- **WHEN** a follower is stopped and the leader keeps getting writes
- **THEN** the lag in seconds for this follower keeps growing
