# M2 - AI design review

I gave an AI chat my M2 OpenSpec change, ADR-0004..0009, the OpenAPI file, both templates and the pipeline, and asked it to critique the design. The prompt and the answer are in `ai-review-transcript.md`.

It found problems with the coordinator lifecycle (single-node mode + one-time registration), the open `/internal/shards` endpoint, the 9-instance cap for M3, and the benchmark setup. It liked the ADRs, the specs and the S3 endpoint decision. It didn't see `bench/` and `docs/m2/benchmark.md`, so part of section 5 is already covered there.

## My decisions

| # | AI point | Decision | Why | Change |
|---|---|---|---|---|
| 1 | **5.3** Go's default HTTP transport keeps 2 idle connections per host, the baseline will be too low (MEDIUM) | **Accept** | True, and it matters for M2 because this is the baseline for M3 and M5. With 16 YCSB threads the coordinator would open new connections to shards all the time. Cheap to fix now. | own `http.Transport` with `MaxIdleConnsPerHost: 256` in `cmd/coordinator` and in `bench/kvstore` |
| 2 | **5.2** The client location isn't set, from Kyiv it adds ~120 ms (HIGH) | **Refute** | Already decided in `docs/m2/benchmark.md`: YCSB runs from CloudShell in `us-east-1`, close to the cluster. The workload file and the go-ycsb binding are in `bench/` (that's also most of 5.1). A separate EC2 for YCSB would take one more of the 9 instances. | nothing |
| 3 | **1.1 / 1.4** Single-node mode + one-time registration can lose data, a coordinator restart loses tables and the ring (HIGH) | **Refute for M2**, accept for M3 | In M2 the pipeline always recreates the whole stack, so coordinator and shards start together, and YCSB starts only after `/health` shows 2 shards. This risk is already in the consequences of ADR-0009. In M3 membership has to be reliable anyway (replicas), so heartbeats and readiness go there. | nothing in M2, plan for M3 |
| 4 | **3.2** The sandbox may have `LabRole` / `LabInstanceProfile`, then ADR-0007 and ADR-0009 are wrong (MEDIUM) | **Refute** | It's not Learner Lab. In M1 I checked: there is no `LabRole`, I work as `voclabs`, and IAM create calls are denied (`docs/m1/labrole.md`). | nothing |
| 5 | **6** "Durability comes in M3 with replication" is wrong, in-memory replicas give availability (LOW) | **Accept** | True, if the stack is recreated all replicas are gone together. | proposal wording fixed |

## Notes for later

- **1.3** The 9-instance cap in M3: 1 + 2 x 3 = 7 already. I'll write the replica placement ADR at the start of M3 (Dynamo-style "next N nodes on the ring" looks like the way to fit).
- **1.5** Vnode positions already don't depend on address or order: `xxhash(shardID + "-" + i)` in `internal/ring`.
- **1.2** Separate listener for `/internal/*` and a required `ClientCidr` - for M4 (failures). In sessions I can pass my IP as `ClientCidr`.
- **2.1** Versions per item and tombstones - this is M3 work (quorums, read repair).
- **3.1, 4.1, 4.2** Presigned URLs, smoke test after deploy and rolling updates matter once there is Auto Scaling (M5) and replicas (M3).
