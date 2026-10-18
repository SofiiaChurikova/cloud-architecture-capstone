# ADR-0003: CIDR addressing scheme

**Status:** Accepted

## Context
The VPC needs public and private subnets. Later milestones add more nodes (shards, replicas, autoscaling) to the private subnets. Subnet size can't be changed after it's created.

## Options
1. **All subnets `/24`** (251 IPs each) - simple, but private subnets could run out of space.
2. **Small public `/24`, big private `/20`** (4091 IPs each) - space where it's actually needed.

## Decision
VPC `10.20.0.0/16` in 2 Availability Zones:

| Subnet | CIDR | Usable IPs |
|---|---|---|
| Public A | `10.20.0.0/24` | 251 |
| Public B | `10.20.1.0/24` | 251 |
| Private A | `10.20.16.0/20` | 4091 |
| Private B | `10.20.32.0/20` | 4091 |

AWS reserves 5 addresses in every subnet, so `/24` = 256 - 5 = 251 and `/20` = 4096 - 5 = 4091.

## Why
- Public subnets will only have a load balancer, so they can be small.
- All nodes and replicas will be in private subnets, so they need more space.
- `10.20.x.x` instead of the default `10.0.x.x`, so it won't overlap if I peer VPCs later.
- 2 AZs is the minimum for high availability.

## Consequences
- (+) Enough space to grow without changing the network.
- (-) Most addresses stay unused, but that's free.
- (-) With 3 replicas per shard in 2 AZs, one AZ always has 2 of the 3 replicas, so if that AZ goes down the quorum is lost. That's fine for M1. If needed, I'll add a third AZ in M3 - the ranges `10.20.2.0/24` and `10.20.48.0/20` are free for it (see `docs/m1/ai-review.md`).

**Where it lives:** `infra/network.yaml`, the `CidrBlock` of the VPC and subnets, and `!Select [0, !GetAZs ""]` / `!Select [1, ...]` for the AZs
