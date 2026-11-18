# Spec Delta

## Purpose

Numbers that show what replication costs, compared with M2.

## ADDED Requirements

### Requirement: YCSB with replication
The store SHALL be benchmarked with the same YCSB workload as in M2 for W=1, W=2 and W=3, and the results SHALL be compared with the M2 baseline.

#### Scenario: Run the benchmark
- **WHEN** workload A runs against the coordinator with each W value
- **THEN** throughput and read / update latency (average and p99) for each run are saved to `docs/m3/benchmark.md` next to the M2 numbers
