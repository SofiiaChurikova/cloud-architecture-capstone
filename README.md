# cloud-architecture-capstone

Capstone for the Cloud Architectures course: a small DynamoDB-like key-value store, built in 5 milestones in the AWS Academy sandbox.

| Milestone | Status |
|---|---|
| **M1 - Infrastructure setup** | done |
| M2 - Sharding | - |
| M3 - Replication | - |
| M4 - Monitoring, failures, MCP | - |
| M5 - Production | - |

## What's where

```
openspec/changes/m1-infrastructure-setup/   proposal, specs, tasks (written before the code)
docs/adr/                                   architecture decisions
infra/network.yaml                          VPC stack (deployed)
infra/iam-readonly.yaml                     read-only policy + role (only validated, the sandbox doesn't allow IAM)
docs/m1/                                    M1 reports: permissions, quotas, budget, recreate, AI review
docs/m1/evidence/                           command outputs from the sandbox, one file per task
docs/m1/screenshots/                        console screenshots, numbered by task
```

## How to run M1

You need AWS CLI v2 and a running session in AWS Academy *Cloud Architecting -> Sandbox environment*.
Copy the credentials from **AWS Details -> AWS CLI** into a profile, for example `aws-sandbox`.

```bash
export AWS_PROFILE=aws-sandbox
export AWS_DEFAULT_REGION=us-east-1

# network stack
aws cloudformation deploy --stack-name capstone-network --template-file infra/network.yaml

# IAM template - only validate it, deploying fails with AccessDenied in the sandbox
aws cloudformation validate-template --template-body file://infra/iam-readonly.yaml
```

## How to check it

```bash
cfn-lint infra/*.yaml                         # no output = no errors
openspec validate m1-infrastructure-setup     # should say the change is valid

aws cloudformation describe-stacks --stack-name capstone-network \
  --query 'Stacks[0].[StackStatus,Outputs]'   # CREATE_COMPLETE + 3 exports

aws ec2 describe-subnets --filters "Name=tag:Name,Values=public-a,public-b,private-a,private-b" \
  --query 'Subnets[].[Tags[?Key==`Name`]|[0].Value,CidrBlock,AvailabilityZone,MapPublicIpOnLaunch]' \
  --output table                              # 4 subnets in 2 AZs
```

The public route table has `0.0.0.0/0 -> igw-...`, the private one only `10.20.0.0/16 -> local`.

## How to delete it

```bash
aws cloudformation delete-stack --stack-name capstone-network
aws cloudformation wait stack-delete-complete --stack-name capstone-network
```

The sandbox also deletes everything when the 3-hour session ends. The stack comes back from the template in 1-2 minutes (`docs/m1/recreate.md`).

## Network

```
VPC 10.20.0.0/16 (us-east-1)
|- us-east-1a  public-a  10.20.0.0/24   -> internet via IGW
|              private-a 10.20.16.0/20  -> local only
|- us-east-1b  public-b  10.20.1.0/24   -> internet via IGW
               private-b 10.20.32.0/20  -> local only
```

Decisions: [ADR-0001 IaC tool](docs/adr/0001-iac-tool.md), [ADR-0002 stack layout](docs/adr/0002-stack-layout.md), [ADR-0003 CIDR](docs/adr/0003-cidr-scheme.md)

Reports: [permissions](docs/m1/labrole.md), [quotas](docs/m1/quotas.md), [budget](docs/m1/budget.md), [recreate](docs/m1/recreate.md), [AI review](docs/m1/ai-review.md)

## What changed since the last milestone

M1 is the first milestone.

## AI usage

The spec and ADRs were written before the templates. AI helped with explanations and drafts, and a separate AI reviewed the design - the review and my decisions are in `docs/m1/ai-review.md`.
