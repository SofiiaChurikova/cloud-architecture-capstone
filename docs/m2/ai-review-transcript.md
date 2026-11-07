# M2 AI design review - transcript

## Prompt

```text
You are a senior AWS solutions architect reviewing Milestone 2 of a student capstone project.

Project: a "light" DynamoDB-like key-value store, built over 5 milestones:
M1 infrastructure, M2 sharding (coordinator + consistent hashing, CI/CD, YCSB baseline),
M3 replication (3 replicas per shard, quorums), M4 monitoring + MCP server,
M5 autoscaling and shard rebalancing.

Environment: AWS Academy sandbox. IAM is read-only (no roles or instance profiles),
no ECS/Fargate, SQS, API Gateway, CodePipeline/CodeBuild, max 9 instances,
$20 credit, sessions of 3 hours and everything is deleted after a session.

Milestone 2 includes: REST + OpenAPI API, a Go coordinator (tables, single-node mode,
forwards items to shards by a consistent hashing ring), 2 Go shards with in-memory storage
that register in the coordinator, EC2 app stack in CloudFormation, S3 gateway endpoint for
the private subnets, GitHub Actions pipeline, YCSB benchmark.

Attached: the OpenSpec proposal and specs, ADR-0004..0009, api/openapi.yaml,
infra/network.yaml, infra/app.yaml, .github/workflows/pipeline.yml.

Please critique the design: API, ring and partition key, code delivery to private subnets,
CI/CD, coordinator metadata, benchmark setup.

For each issue give:
1. severity (high / medium / low),
2. why it matters, especially for milestones M3-M5,
3. a concrete alternative.

Be critical and specific. Do not just praise.
```

## Answer

**Summary.** The structure is solid: the ADRs are clear, the specs match the code paths, and the S3 endpoint decision is sensible. The biggest risks are elsewhere:

- Lifecycle bugs. Single-node fallback plus one-time shard registration can silently lose data.
- Security. The internal registration endpoint is open to the internet.
- M3/M5 planning. The current replication plan doesn't fit under the 9-instance cap.
- Benchmark. It isn't defined well enough to compare across milestones.

### 1. Coordinator, ring, metadata

- **1.1 Single-node fallback plus one-time registration loses data silently - HIGH.** Writes that arrive before the shards register stay on the coordinator; after registration reads go to a shard and return 404. Same between shard-1 and shard-2 registrations. If the coordinator restarts (OOM on t3.micro), it comes back with no tables and an empty ring, and shards never register again. Alternative: explicit `MODE=standalone|sharded`, `EXPECTED_SHARDS=2` with 503 until the ring is complete, shard heartbeat every ~5 s as idempotent upsert, dead after ~15 s.
- **1.2 `/internal/shards` is on the public port - HIGH (security).** `ClientCidr` defaults to `0.0.0.0/0`; anyone can register a fake shard and get half of the writes; `GET /internal/shards` shows the private IPs; the item API has no auth. Alternative: `/internal/*` on a separate listener (`:9090`) open only to `ShardSecurityGroup`, `ClientCidr` required with no default, optional shared token.
- **1.3 The M3 plan (3 replicas per shard) doesn't fit the 9-instance cap - HIGH (planning).** 1 + 2 x 3 = 7, plus a YCSB client = 8; M5 adds 3 more = 11. Alternative: ADR now for Dynamo-style placement (key on the next N=3 distinct nodes clockwise), M3 with 4 nodes, and `ring.Owners(key, n)` in M2.
- **1.4 Table metadata is lost on restart, shards don't know about tables - MEDIUM.** Alternative: coordinator sends `PUT /internal/tables/{t}` to every shard and pulls tables from shards on registration, so it holds no state of its own.
- **1.5 How vnode positions are computed isn't specified - MEDIUM.** If a position depends on address or registration order, the ring changes on restart or IP change. Alternative: `xxhash64(shardID + "#" + i)`, a test that both registration orders give the same ring, write it into ADR-0005.
- **1.6 The 45-55% balance test checks one pair of IDs, not the property - MEDIUM.** With 2 x 128 random points a share follows Beta(128,128), sigma ~3.1%, so about 1 in 9 random ID pairs would fail; the test always passes for shard-1/shard-2 and may fail when shard-3 appears. Alternative: shares from arc lengths over many random ID sets, a key-movement test (3rd shard takes ~1/3, all to the new shard), maybe 256 vnodes.
- **1.7 The coordinator is both the bottleneck and a single point of failure - MEDIUM.** Two hops through one t3.micro; in M5 more shards may not raise throughput. Alternative: stateless coordinator (after 1.1, 1.4) and 2 of them; at least record coordinator CPU in every benchmark.

### 2. API

- **2.1 No item versions or conditional writes, M3 needs them - MEDIUM.** Quorums, read repair and LWW need versions; deletes need tombstones; CREATE overwrites. Alternative: `{value, version}` on shards, `ETag`, `If-None-Match: *` and `If-Match`, DELETE as a versioned tombstone.
- **2.2 `partition_key` is stored but never used - MEDIUM.** Body `{"id": 7}` vs path `/items/42` is undefined. Alternative: check `body[partition_key] == path key` or 400; reserve `/items/{pk}/{sk}`.
- **2.3 No request body size limit on an in-memory store with 1 GB RAM - MEDIUM.** Alternative: `http.MaxBytesReader` at 400 KB with 413, `ReadHeaderTimeout` and `ReadTimeout`.
- **2.4 Smaller API points - LOW.** Repeated `PUT /tables/x` returns 409 (not idempotent); JSON error bodies and `X-Error-Code` for HEAD; 503 when not ready and 504 on shard timeout instead of 502; validate table names (no `#`); a separate `api/shard-openapi.yaml`.

### 3. Code delivery (ADR-0007)

- **3.1 Presigned URLs in user data expire - MEDIUM-HIGH.** At most 1 hour; works only because the stack is recreated on every deploy; an ASG scale-out or replica replacement later can't download the binary; the coordinator downloads over the internet. Alternative: bucket policy allowing `s3:GetObject` only with `aws:SourceVpce = <endpoint id>`, add `PublicRouteTable` to the endpoint, plain `curl` with `ArtifactVersion=<sha>`.
- **3.2 Check the main premise of ADR-0009 - MEDIUM.** Learner Lab usually has `LabRole` and `LabInstanceProfile`. Alternative: run `aws iam list-instance-profiles` and use it or write that it isn't there.
- **3.3 User data is fragile - LOW.** `curl` has no retry and a failed user data still gives CREATE_COMPLETE. Alternative: `curl --retry 10 --retry-all-errors`, sha256 check, narrower endpoint policy, `After=network-online.target`, non-root user.

### 4. CI/CD

- **4.1 Nothing checks the app after deploy - HIGH.** Alternative: smoke test that polls `/health`, checks 2 shards, does PUT/GET/HEAD/DELETE and checks different `X-Shard-Id`.
- **4.2 Delete and recreate the stack on every push - MEDIUM.** Downtime, data loss, new IP. Alternative: Launch Template + ASG per role with `AutoScalingRollingUpdate`.
- **4.3 No concurrency control on deploys - MEDIUM.** Alternative: `concurrency: { group: deploy-sandbox, cancel-in-progress: false }`.
- **4.4 Pipeline hygiene - LOW.** `go test -race`, `go-version-file`, `pull_request` trigger, cfn-lint and OpenAPI lint, secrets in an Environment, `permissions: contents: read`, early `sts get-caller-identity`.

### 5. Benchmark

- **5.1 The benchmark spec isn't reproducible - HIGH.** Which YCSB and binding, workloads, record count, field sizes, threads, run length aren't fixed. Alternative: commit workloads and a script; A, B, C; 100k records x 10 fields x 100 B; threads 1/8/32/64 for 120 s; 3 runs, median; p50/p95/p99 plus git SHA, instance types, AZs.
- **5.2 Where the client runs isn't specified - HIGH.** From Kyiv it adds ~120 ms RTT. Alternative: YCSB from EC2 in the coordinator's AZ, a separate `bench.yaml` stack.
- **5.3 Go HTTP client defaults distort the numbers - MEDIUM.** `DefaultTransport` keeps 2 idle connections per host, so at 32+ threads the coordinator keeps opening new connections to shards. Alternative: own `http.Transport` with `MaxIdleConnsPerHost: 256` and a ~2 s request timeout.
- **5.4 t3 burstable CPU - MEDIUM.** Credits run out or surplus credits are billed. Alternative: set `CreditSpecification`, same instance type every milestone, warm-up, record `CPUCreditBalance`.
- **5.5 The benchmark doesn't show what sharding adds - MEDIUM.** No reference point; about half the requests cross AZs. Alternative: coordinator alone, 1 shard, 2 shards; latency per shard.

### 6. Wording in the docs

- "Durability comes in M3 with replication" is inaccurate: in-memory replicas give availability, not durability.
- The specs leave out error cases the OpenAPI already has (400, 404 for a missing table, 409) and a readiness scenario.
- ADR-0009 says "no saved data format", but the registration payload is a format too; add a `version` field.

### What to fix before M3

1. Explicit mode, ready-to-serve gating, heartbeats, tables sent to shards (1.1, 1.4)
2. Separate internal listener, required `ClientCidr` (1.2)
3. ADR on replica placement, `Owners(key, n)` (1.3)
4. Endpoint-limited bucket policy (3.1), check `LabInstanceProfile` (3.2)
5. Smoke test and concurrency control (4.1, 4.3)
6. Fixed benchmark setup run from inside AWS (5.1, 5.2)
7. Item versions in the shard storage model (2.1)
