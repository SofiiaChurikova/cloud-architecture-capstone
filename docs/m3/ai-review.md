# M3 - AI design review

I gave an AI chat my M3 OpenSpec change, ADR-0010..0013, the OpenAPI file, the app template and the replication code, and asked it to critique the design. The prompt and the answer are in `ai-review-transcript.md`.

It said the core (one ordered log per shard, versions, fsync before ack, cutting a broken tail) is fine, and most problems are around it: the coordinator after a restart, failure handling, what a version means after M4/M5, and the benchmark plan. I checked every point against the code before deciding.

## My decisions

| # | AI point | Decision | Why | Change |
|---|---|---|---|---|
| 1 | **1 + 12** A coordinator restart loses tables and replicas, shards register only once, with an empty ring a write stays in the coordinator memory with `200` (HIGH) | **Accept** | True, I checked it in the code. I already planned this in the M2 review (point 1.1 / 1.4: "heartbeats and readiness go to M3"). | ADR-0014: replicas register every 2 s, tables in `$DATA_DIR/tables.json`, `EXPECTED_SHARDS` - `503` until both shards have a leader, no single-node fallback then. Tests `TestWaitsForExpectedShards`, `TestTablesSurviveRestart` |
| 2 | **8** No failure detection, no dial timeout, a stopped node makes reads wait 5 s (MEDIUM) | **Accept** | I checked it locally: I froze a follower with `kill -STOP` (like a stopped EC2, packets get no answer) and one of 6 reads took 5.0 s. My AWS demo is exactly "stop a follower", so it would look broken. | ADR-0014: dial timeout 300 ms, a replica with no heartbeat for 6 s is down, reads ask live replicas first, a write to a dead leader gets `503` right away. After 6 s all reads took ~1 ms again. Test `TestDeadReplicasAreSkipped` |
| 3 | **2** Version = log offset won't survive failover (M4) and moving keys (M5), needs term + hybrid clock now (HIGH) | **Refute for M3** | In M3 every shard has one fixed leader and the set of shards is fixed (out of scope in the proposal). Inside one leader the offset only grows, so it's a correct version. A term only makes sense together with leader election, which is M4. Changing the format later is cheap: the pipeline recreates the stack, so there are no old logs to migrate. | ADR-0010 consequences: "version needs a term or a clock in M4" |
| 4 | **4** The benchmark will measure the gp2 burst bucket, not the system, use gp3 (HIGH) | **Refute** | I counted it. One run = 10 000 inserts + ~25 000 updates, split over 2 leaders = ~17 500 fsync per leader. Even with 2 I/O per fsync, 4 runs use ~140 000 I/O of the 5.4M credit bucket, so the burst never runs out. And M2 had the same 8 GB gp2, so M2 vs M3 compares the same disks. The cost of fsync alone is the W=1, R=1 run. | disk type and this reasoning in `docs/m3/benchmark.md` |
| 5 | **3** ADR-0010 says "retry with the same value", but that's not idempotent; uncommitted writes can be read; R + W > N is not linearizability (HIGH) | **Accept (docs)** | True: replicas apply an entry before it's committed. A commit index, idempotency keys and `If-Match` belong to M4 (conflicts, failover). For M3 the ADR must not promise more than the code does. | ADR-0010 consequences rewritten |
| 6 | **14** Keys are not URL-escaped, a key `x?w=1&` turns W=3 into W=1 (LOW) | **Accept** | Reproduced it with a test: on the old code the leader got key `x` and `w=1`. | `url.PathEscape` in the coordinator, test `TestKeyIsEscaped` |
| 7 | **17** `w` documented on GET, `r` on PUT, wrong 503 texts, "a shard survives losing one node" (LOW) | **Accept** | True. | `api/openapi.yaml`: `w` only on PUT / DELETE, `r` on GET / HEAD, 503 texts. Proposal: "keeps working when a follower is down, read-only when the leader is down" |

## Notes for later

- **5** `/internal/*` on the public port - same as in the M2 review, goes to M4 together with a second listener. In sessions I can pass my IP as `ClientCidr`.
- **6, 9** Log ID / term on every pull, leader not baked into the template - this is leader failover, M4.
- **10** One replica per AZ needs a third private subnet, and the coordinator is a single point of failure - M4.
- **11** Several shards per node (different shards, never two replicas of one shard on one node) - this is how M5 can add shards without new instances. I'll rewrite ADR-0013 option 2 in M5.
- **7** Whole log in RAM, no snapshots - fine for 10 000 records (a few MB), snapshots + log retention in M4.
- **13** YCSB already runs from CloudShell in us-east-1, not from a laptop (same as the M2 review, point 5.2). A run with one follower down can be added if the session has time.
- **15, 16** Fail-stop on a failed fsync, refuse to start without `DATA_DIR` - small, M4.
