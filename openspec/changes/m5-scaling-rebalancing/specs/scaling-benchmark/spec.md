# Spec Delta

## Purpose

The final benchmark shows how the store behaves with autoscaling on, compared with M2.

## ADDED Requirements

### Requirement: Three load patterns
YCSB SHALL run with autoscaling on in three patterns: slow growth of the load, a sudden spike, and long high load. The report SHALL show latency, throughput and the number of active nodes over time, and compare with the M2 baseline.

#### Scenario: Spike
- **WHEN** the load jumps from low to high
- **THEN** the report shows when p99 crossed the threshold, when the new shard joined and how p99 changed after
