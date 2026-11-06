# ADR-0007: Shards get the code from S3

**Status:** Accepted

## Context
The shards are in the private subnets from M1, there is no internet there. But a shard has to download the Go binary when it starts. This was point 1.1 in the M1 AI review.

## Options
1. **S3 gateway endpoint** - free, gives the private subnets access to S3 only.
2. **Shards in public subnets** - simplest, but then the private subnets from M1 are not used.
3. **NAT instance** - full internet, but takes 1 of the 9 instances.

## Decision
S3 gateway endpoint. The pipeline puts the binary in S3, the shard downloads it on start.

## Why
- Free. A NAT gateway is ~$33/month, more than the $20 budget.
- Shards stay private like in M1.
- Doesn't take an instance.

## Consequences
- (+) Shards get the code without internet.
- (-) The private route table gets a second route (to S3), so the M1 network spec has to be updated.

**Where it lives:** `infra/network.yaml` (endpoint), `infra/app.yaml` (download in user data)
