# ADR-0009: Where the coordinator keeps its metadata

**Status:** Accepted

## Context
The coordinator keeps 2 small things: the tables (name + partition key) and the list of shards for the ring. Lecture 8 asks where this metadata lives: RDS, DynamoDB or a file in S3. The choice should follow the access pattern.

## Options
1. **In the coordinator memory** - shards register again when they start.
2. **DynamoDB table** - survives a restart, but the coordinator needs an IAM role to call DynamoDB.
3. **File in S3** - same, needs an IAM role.
4. **RDS** - a whole database for 2 tiny lists, costs money and also needs access set up.

## Decision
In memory for M2.

## Why
- Sandbox IAM is read-only, so I can't create an instance role. Without it the coordinator EC2 can't call DynamoDB or S3 by itself.
- Access pattern: very little data, read on every request, written only when a table or a shard is added. Memory fits that.
- The pipeline recreates the whole app stack, so the coordinator and the shards always start together.

## Consequences
- (+) Nothing extra to run or pay for.
- (-) If only the coordinator restarts, tables and the ring are lost. Shards register only once on start, so they would need a restart too.
- (-) No saved data format yet, so the pipeline has no schema migrations to run.

**Where it lives:** `cmd/coordinator/main.go` (`tables` and `ring`), `cmd/shard/main.go` (`register`)
