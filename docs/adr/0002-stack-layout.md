# ADR-0002: Separate network and application stacks

**Status:** Accepted

## Context
The network is created once in M1. The app (coordinator and shards) will change in every milestone and from M2 it will be deployed by CI/CD.

## Options
1. **One stack** - simple, but every app deploy also touches the network.
2. **Network stack + app stack** - changes in the app don't affect the network.

## Decision
Two stacks. The network stack exports the VPC and subnet IDs, the app stack imports them.

## Why
- The app changes all the time, the network doesn't.
- The network is free and the app costs money, so I can delete only the app stack to save sandbox credits.

## Consequences
- (+) An app deploy can't break the network.
- (-) Stacks have to be deleted in the right order: app first, then network.

**Where it lives:** `infra/network.yaml`, the `Outputs` section with exports `capstone-network-VpcId`, `capstone-network-PublicSubnetIds`, `capstone-network-PrivateSubnetIds`
