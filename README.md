# cloud-architecture-capstone

Capstone for the Cloud Architectures course: a small DynamoDB-like key-value store, built in 5 milestones in the AWS Academy sandbox.

| Milestone | Status |
|---|---|
| M1 - Infrastructure setup | done |
| M2 - Sharding | done |
| M3 - Replication | done |
| **M4 - Monitoring, failures, MCP** | in progress |
| M5 - Production | - |

## What's where

```
openspec/changes/m3-replication/   M3 proposal, specs, tasks (written before the code)
openspec/changes/m2-sharding/      M2 change
openspec/changes/archive/          M1 change, archived
openspec/specs/                    current specs
docs/adr/                          architecture decisions (0001-0003 M1, 0004-0009 M2, 0010-0014 M3)
api/openapi.yaml                   the API
cmd/coordinator/                   coordinator: tables, ring, replica sets, quorum reads
cmd/shard/                         one replica: leader or follower of a shard
internal/ring/                     consistent hashing ring (xxHash64, 128 vnodes)
internal/wal/                      append-only log on disk (offsets, replay on start)
internal/replica/                  leader (log + wait for W) and follower (pull from offset)
internal/store/                    in-memory items with versions
infra/network.yaml                 VPC stack (M1) + S3 endpoint (M2)
infra/app.yaml                     coordinator + 2 shards x 3 replicas on EC2
infra/iam-readonly.yaml            read-only policy (M1, only validated)
.github/workflows/pipeline.yml     build + deploy
bench/                             YCSB binding and workload
docs/m1/, docs/m2/, docs/m3/       reports, evidence, AI reviews
```

## How to run M3

### Locally

Needs Go 1.22 (or Docker).

```bash
go test ./...
docker compose up --build        # coordinator + shard-1-a/b/c + shard-2-a/b/c

curl localhost:8080/health                    # "shards":2,"replicas":6
curl localhost:8080/internal/shards           # every shard with its leader and 2 followers, alive true/false
curl -X PUT localhost:8080/tables/users -d '{"partition_key":"id"}'
curl -i -X PUT localhost:8080/tables/users/items/42 -d '{"id":"42"}'       # w=2 by default
curl -i "localhost:8080/tables/users/items/42?r=1"                         # run a few times: X-Replicas changes
curl -i "localhost:8080/tables/users/items/42?r=3"                         # asks all 3 replicas
```

Headers in the answer: `X-Shard-Id` (shard), `X-Replicas` (which replicas answered), `X-Version` (log offset of the last write).

Stop a follower and start it again:

```bash
docker compose stop shard-1-b
curl -i -X PUT "localhost:8080/tables/users/items/42?w=3" -d '{"id":"42","v":2}'   # 503, only 2 replicas
curl -i -X PUT localhost:8080/tables/users/items/42 -d '{"id":"42","v":3}'          # 200 with w=2
docker compose start shard-1-b
docker compose logs shard-1-b | grep resync                                        # continues from its offset
```

Output of my run: `docs/m3/evidence/local-run.md`.

### In AWS

Same as M2: new credentials in the repo secrets, push to `main`, the pipeline recreates `capstone-app`, now with 7 instances.

```bash
curl http://<coordinator-ip>:8080/health      # "shards":2,"replicas":6
```

Benchmark: `docs/m3/benchmark.md`. Budget: `docs/m3/budget.md`.

## How to run M2

M2 is the same commands with 2 single shards, see the `m2` tag.

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

- Every shard is a replica set now: 1 leader + 2 followers, 7 instances in total (ADR-0013).
- The leader appends every write to an append-only log on disk, followers pull it from their offset and resync after a restart (ADR-0011, ADR-0012).
- Every item has a version (the log offset), deletes are tombstones.
- Quorum writes and reads with N=3, default W=2 and R=2, changeable per request with `?w=` / `?r=` (ADR-0010).
- Reads are spread over all 3 replicas of a shard.
- Replicas can reach each other on 8081 (followers pull from the leader).
- Replicas send a heartbeat every 2 s. A replica without one for 6 s is down: reads skip it, a write to a dead leader gets `503` at once (ADR-0014).
- The coordinator keeps tables in `$DATA_DIR/tables.json` and waits for all shards from `EXPECTED_SHARDS` before it takes requests (ADR-0014).
- YCSB for different W / R against the M2 baseline in `docs/m3/benchmark.md`.

## AI usage

The spec and ADRs were written before the code. AI helped with explanations and drafts, and a separate AI reviewed the design - the reviews and my decisions are in `docs/m1/ai-review.md`, `docs/m2/ai-review.md` and `docs/m3/ai-review.md`.
