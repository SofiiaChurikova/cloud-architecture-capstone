# Proposal

## Why

In M1 I made the network. Now the store itself has to work: create a table, put / read / check / delete items, and split the data between 2 shards with a consistent hashing ring. Also a pipeline for deploy and a YCSB benchmark, to compare with in M3 and M5.

## What Changes

- REST API described in OpenAPI: create a table, CREATE / READ / EXISTS / DELETE for items
- Coordinator in Go: first works alone, then sends requests to shards
- Shard in Go: keeps items in memory, registers in the coordinator on start
- Consistent hashing ring in the coordinator
- CloudFormation app stack: 1 coordinator + 2 shards on EC2
- CI/CD pipeline that deploys infra and app
- YCSB benchmark, results saved as the baseline

## Capabilities

### New Capabilities

- `kv-api`: tables and item operations
- `sharding-ring`: which shard gets which key
- `app-deployment`: app stack and pipeline
- `benchmark-baseline`: YCSB results

### Modified Capabilities

- `network-foundation`: private subnets now also have a route to S3 through a gateway endpoint (ADR-0007)

## Impact

- Code: `cmd/coordinator`, `cmd/shard`, `internal/ring`, `internal/store`, `api/openapi.yaml`
- Infra: `infra/app.yaml`, `.github/workflows/`
- AWS: 3 EC2 instances
- Data and coordinator metadata are only in memory in M2 (ADR-0009). Replication in M3 gives availability, not durability: replicas are in memory too.
