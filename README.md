# cloud-architecture-capstone

Capstone for the Cloud Architectures course: a small DynamoDB-like key-value store, built in 5 milestones in the AWS Academy sandbox.

| Milestone | Status |
|---|---|
| M1 - Infrastructure setup | done |
| M2 - Sharding | done |
| M3 - Replication | done |
| M4 - Monitoring, failures, MCP | done |
| **M5 - Production** | done |

## What's where

```
openspec/changes/m5-scaling-rebalancing/ M5 proposal, specs, tasks (written before the code)
openspec/changes/m4-monitoring-mcp/ M4 change
openspec/changes/m3-replication/   M3 change
openspec/changes/m2-sharding/      M2 change
openspec/changes/archive/          M1 change, archived
openspec/specs/                    current specs
docs/adr/                          architecture decisions (0001-0003 M1, 0004-0009 M2, 0010-0014 M3, 0015-0019 M4, 0020-0022 M5)
api/openapi.yaml                   the API
cmd/coordinator/                   coordinator: tables, ring, replica sets, quorum reads
cmd/shard/                         one replica: leader or follower of a shard
cmd/mcp/                           MCP server over stdio for AI agents
cmd/opsmcp/                        read-only MCP server for the ops agent (alerts, metrics, cluster)
cmd/autoscaler/                    scales shards on write p99 (runs on my laptop)
internal/mcpserver/                MCP JSON-RPC over stdio, shared by cmd/mcp and cmd/opsmcp
internal/metrics/                  Prometheus metrics, trace IDs, JSON request logs
internal/ring/                     consistent hashing ring (xxHash64, 128 vnodes)
internal/wal/                      append-only log on disk (offsets, replay on start)
internal/replica/                  leader (log + wait for W) and follower (pull from offset)
internal/store/                    in-memory items with versions
infra/network.yaml                 VPC stack (M1) + S3 endpoint (M2)
infra/app.yaml                     coordinator + 1 or 2 shards x 3 replicas (ShardCount) + monitoring on EC2
monitoring/                        Prometheus config, alert rules, Grafana dashboard
infra/iam-readonly.yaml            read-only policy (M1, only validated)
.github/workflows/pipeline.yml     build + deploy
bench/                             YCSB binding and workload
docs/m1/ ... docs/m5/               reports, evidence, AI reviews
```

## How to run M5

### Locally

Needs Go 1.22.

```bash
go test ./...
docker compose up --build                       # coordinator + shard-1 + shard-2 + Prometheus + Grafana

# 1000 keys, then take shard-2 away and look at the keys per shard (Grafana "Keys per shard")
curl -X POST -H "Authorization: Bearer $TOKEN" localhost:8080/admin/shards/shard-2/drain
curl -H "Authorization: Bearer $TOKEN" localhost:8080/admin/migration    # draining -> idle, last move: copied/total
curl localhost:8080/health                                               # "shards":1
curl localhost:8080/version
```

Starting the shard-2 containers again with empty data puts shard-2 back on the ring and moves its keys to it. Output of my run: `docs/m5/evidence/local-run.md`.

### In AWS

The pipeline deploys the stack with `ShardCount=2` (8 instances). Then on my laptop, with the lab credentials (`AWS_PROFILE=aws-sandbox`):

```bash
KV_TOKEN=$TOKEN ./bin/autoscaler --prometheus http://<monitoring-ip>:9090 --coordinator http://<coordinator-ip>:8080
```

It goes down to 1 shard after 5 minutes without load and back to 2 when write p99 is above 60 ms for 2 minutes (ADR-0020). `--dry-run` only logs the decisions.

Ops agent in Claude Desktop: `docs/m5/ops-agent.md`. Benchmark: `docs/m5/benchmark.md`. Budget: `docs/m5/budget.md`. What happened in my session: `docs/m5/evidence/aws-demo.md`.

## How to run M4

### Locally

Needs Go 1.22 (or Docker).

```bash
go test ./...
docker compose up --build        # M3 cluster + Prometheus (:9090) + Grafana (:3000)
                                 # (to curl 9080 from the host, add "9080:9080" to the coordinator ports)

curl -i -X PUT localhost:8080/tables/users -d '{"partition_key":"id"}'
curl -i -X PUT localhost:8080/tables/users/items/42 -d '{"id":"42"}'    # X-Trace-Id and X-Version (leader time) in the answer
docker compose logs | grep <trace id>                                   # the same request in the coordinator and replica logs
curl localhost:9080/metrics                                             # coordinator metrics (internal port)
curl localhost:9080/internal/targets                                    # what Prometheus scrapes
```

Grafana: http://localhost:3000/d/kvstore. Alerts: http://localhost:9090/alerts. Stop a replica (`docker compose stop shard-1-c`) and `NodeDown`, `ShardMissingReplicas` and `ReplicationLag` fire in about a minute.

With `API_TOKEN` set on the coordinator, `/tables` needs `Authorization: Bearer <token>`. `/internal/*` and `/metrics` are only on the internal port 9080, which in AWS only the replicas and the monitoring node can reach.

MCP server for Claude Desktop: `docs/m4/mcp.md`.

Output of my run: `docs/m4/evidence/local-run.md`.

### In AWS

New credentials in the repo secrets, and an `API_TOKEN` secret for the store. Push to `main`, the pipeline recreates `capstone-app`, now with 8 instances. The stack outputs have the coordinator, Prometheus and Grafana URLs.

```bash
curl http://<coordinator-ip>:8080/health                     # "shards":2,"replicas":6
open http://<monitoring-ip>:3000/d/kvstore                   # dashboard
```

Budget: `docs/m4/budget.md`.

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

- Shards can be added and removed while the store runs. A new shard goes on the ring and the coordinator moves to it only the keys that now belong to it; during the move writes go to the new owner and reads ask the new owner first, then the old one. `POST /admin/shards/{id}/drain` does the same in the other direction (ADR-0021).
- The new owner never gives a version below the old shard's greatest version, so a write during a move always beats the copy of the old value.
- A removed shard can't come back from its old replicas; the same ID is accepted again only from replicas with an empty log.
- The app stack has `ShardCount` (1 or 2); shard-2 exists only when it is 2. `cmd/autoscaler` changes it on write p99 (> 60 ms or > 1% failed writes for 2 min: up; < 20 ms and under 100 writes/s, or no traffic, for 5 min: drain + down; no data: no action; 5 min cooldown) (ADR-0020).
- `cmd/opsmcp`: read-only MCP server for an ops agent - alerts, PromQL, cluster state; it suggests scale up / down / rebalance and can't do anything (ADR-0022).
- `/version` on the coordinator and replicas (git commit from the pipeline); `/health` shows shards on the ring and the move state.
- Dashboard: shards on the ring, active nodes, shard move progress.
- Found in the AWS run: a shard leader stops answering at ~260 000-320 000 log entries, because the M3 log is kept in memory and never compacted (`docs/m5/benchmark.md`).

## AI usage

The spec and ADRs were written before the code. AI helped with explanations and drafts, and a separate AI reviewed the design - the reviews and my decisions are in `docs/m1/ai-review.md`, `docs/m2/ai-review.md`, `docs/m3/ai-review.md`, `docs/m4/ai-review.md` and `docs/m5/ai-review.md`.
