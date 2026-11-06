# Proposal

## Why

Before creating the KV-store we need a network and an understanding of what access we have in the sandbox, so we must learn what `LabRole` allows and denies. We describe in code what the sandbox will not let us create and prove that the base infrastructure can be rebuilt from code.

## What Changes

- `LabRole` analysis: what it allows/denies
- Read-only IAM policy and role as a CloudFormation template
- VPC: 2 public subnets with an Internet Gateway, 2 private subnets with no internet access
- CloudFormation network stack
- Service quotas and their limitations
- Milestone 2 budget for 2 shards compared against the sandbox credit ceiling
- Recreating the stack from scratch

## Capabilities

### New Capabilities

- `network-foundation`: VPC, subnets, routing
- `readonly-access-policy`: read-only access policy and IAM roles
- `stack-recreatability`: ability to remove infra and rebuild it from code

### Modified Capabilities

None - first change in the project.

## Impact

- New files: `infra/network.yaml`, `infra/iam-readonly.yaml`, ADRs in `docs/adr/`, reports in `docs/m1/`
- AWS resources: 1 VPC, 4 subnets (2 public, 2 private), 1 Internet Gateway, 2 route tables
