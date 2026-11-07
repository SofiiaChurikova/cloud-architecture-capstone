# cloud-architecture-capstone

Capstone for the Cloud Architectures course: a small DynamoDB-like key-value store, built in 5 milestones in the AWS Academy sandbox.

| Milestone | Status |
|---|---|
| M1 - Infrastructure setup | done |
| **M2 - Sharding** | done |
| M3 - Replication | - |
| M4 - Monitoring, failures, MCP | - |
| M5 - Production | - |

## What's where

```
openspec/changes/m2-sharding/      M2 proposal, specs, tasks (written before the code)
openspec/changes/archive/          M1 change, archived
openspec/specs/                    current specs
docs/adr/                          architecture decisions (0001-0003 M1, 0004-0009 M2)
api/openapi.yaml                   the API
cmd/coordinator/                   coordinator: tables, ring, sends items to shards
cmd/shard/                         shard: in-memory items, registers in the coordinator
internal/ring/                     consistent hashing ring (xxHash64, 128 vnodes)
internal/store/                    in-memory store + HTTP handler for items
infra/network.yaml                 VPC stack (M1) + S3 endpoint (M2)
infra/app.yaml                     coordinator + 2 shards on EC2
infra/iam-readonly.yaml            read-only policy (M1, only validated)
.github/workflows/pipeline.yml     build + deploy
bench/                             YCSB binding and workload
docs/m1/, docs/m2/                 reports, evidence, AI reviews
```

## How to run M2

### Locally

Needs Go 1.22 (or Docker).

```bash
go test ./...
docker compose up --build        # or run the binaries from cmd/ by hand

curl -X PUT localhost:8080/tables/users -d '{"partition_key":"id"}'
curl -i -X PUT localhost:8080/tables/users/items/42 -d '{"id":"42","name":"Sofiia"}'   # CREATE
curl -i localhost:8080/tables/users/items/42                                       # READ
curl -I localhost:8080/tables/users/items/42                                       # EXISTS
curl -i -X DELETE localhost:8080/tables/users/items/42                             # DELETE
curl localhost:8080/internal/shards                                                # the ring
```

`X-Shard-Id` in the answer shows which shard has the key. Output of my run: `docs/m2/evidence/local-run.md`.

### In AWS

The pipeline does it (ADR-0008). At the start of a sandbox session I put the new credentials into repo secrets:

```bash
gh secret set AWS_ACCESS_KEY_ID
gh secret set AWS_SECRET_ACCESS_KEY
gh secret set AWS_SESSION_TOKEN
```

Then push to `main` (or run the workflow by hand). Build + tests run on every push, deploy only from `main`:
upload binaries to S3 -> deploy `capstone-network` -> recreate `capstone-app`. The coordinator URL is in the stack outputs.

```bash
aws cloudformation describe-stacks --stack-name capstone-app --query 'Stacks[0].Outputs'
curl http://<coordinator-ip>:8080/health      # {"shards":2,"status":"ok"}
```

Benchmark: `docs/m2/benchmark.md` (~6000 ops/sec, p99 ~21 ms). Results and configs were also saved to the versioned bucket `s3://capstone-code-590183815018/ycsb/m2/` (sandbox session of 2026-11-07, the sandbox deletes it after the session, so the copy in `docs/m2/evidence/ycsb/` is the main one). Budget: `docs/m2/budget.md`.

### Delete

```bash
aws cloudformation delete-stack --stack-name capstone-app
aws cloudformation wait stack-delete-complete --stack-name capstone-app
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

- The store itself: REST API (ADR-0004), coordinator and 2 shards in Go, all in memory.
- Consistent hashing ring: xxHash64 with 128 virtual nodes per shard (ADR-0005), the key is `table#partition_key` (ADR-0006).
- Network: private subnets now have a route to S3 through a free gateway endpoint, so shards can download the code without internet (ADR-0007). The M1 network spec is updated for this.
- App stack `infra/app.yaml`: coordinator in a public subnet, shards in the private ones, shards are reachable only from the coordinator.
- CI/CD with GitHub Actions, because CodePipeline is not in the sandbox (ADR-0008).
- Coordinator metadata (tables, ring) is kept in memory (ADR-0009).
- YCSB baseline in `docs/m2/benchmark.md`, to compare in M3 and M5.

## AI usage

The spec and ADRs were written before the code. AI helped with explanations and drafts, and a separate AI reviewed the design - the reviews and my decisions are in `docs/m1/ai-review.md` and `docs/m2/ai-review.md`.
