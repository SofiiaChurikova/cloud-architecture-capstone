# Spec Delta

## Purpose

Numbers for the sharded store, to compare with M3 and M5.

## ADDED Requirements

### Requirement: Store is benchmarked with YCSB
The store SHALL be benchmarked with YCSB through the coordinator in AWS, and the results SHALL be saved.

#### Scenario: Run the benchmark
- **WHEN** a YCSB workload runs against the coordinator
- **THEN** throughput and read / write latency are saved to `docs/m2/benchmark.md`
