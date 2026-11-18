# M3 AI design review - transcript

## Prompt

```text
You are a senior distributed systems engineer reviewing Milestone 3 of a student capstone project.

Project: a "light" DynamoDB-like key-value store, built over 5 milestones:
M1 infrastructure, M2 sharding (coordinator + consistent hashing ring, CI/CD, YCSB baseline),
M3 replication (3 replicas per shard, log, quorums), M4 monitoring, conflict resolution, MCP server,
M5 autoscaling and shard rebalancing.

Environment: AWS Academy sandbox. IAM is read-only (no roles or instance profiles),
no MSK/Kinesis/SQS, max 9 instances, $20 credit, 3-hour sessions.

Milestone 3: every shard is a replica set (1 fixed leader + 2 followers). The leader appends every
write to an append-only JSON-lines log on its disk (fsync), followers pull the log over HTTP with
long polling from their last offset and keep their own copy on disk. Version of a key = log offset.
The coordinator sends writes to the leader with W, the leader waits until W replicas have the entry.
Reads ask R replicas (round robin over all 3) and return the newest version. N=3, default W=2, R=2,
W and R can be set per request. 1 coordinator + 2 shards x 3 replicas = 7 EC2 in 2 AZs.

Attached: the OpenSpec change m3-replication, ADR-0010..0013, api/openapi.yaml, infra/app.yaml,
internal/wal, internal/replica, cmd/coordinator.

Please critique the design: replication mechanism, log and ordering, quorum logic, failure handling,
placement, benchmark plan.

For each issue give:
1. severity (high / medium / low),
2. why it matters, especially for milestones M4-M5,
3. a concrete alternative.

Be critical and specific. Do not just praise.
```

## Answer

M3 replication design review

The core is solid. There is one ordered log per shard. `Store.Apply` rejects older versions, so two concurrent writes to the same key can't be applied out of order. Followers fsync before they ack, and acks only cover a prefix of the log. A torn last line is cut off on start. Most of the problems below are at the edges: the coordinator, failure paths, what "version" means, and whether the benchmark measures what you think it measures. I've grouped them by severity.

### High

**1. A coordinator restart quietly loses data**

`tables` and `groups` live only in the coordinator's memory. `register()` in `cmd/shard` returns after its first success, so shards never register again. After a coordinator restart (systemd `Restart=always`, or every new sandbox session):

- every table is gone, so clients get "table not found",
- the client creates the table again, but the ring is empty. `ring.Get` returns `!ok`, so `localItem` stores the write in the coordinator's memory and answers 200. The write is never replicated and never reaches a disk.

This undoes the main promise of M3, and with 3-hour sessions it will happen all the time.

- Why it matters for M4-M5: failure detection in M4 needs live membership anyway, and rebalancing in M5 needs a catalog that survives restarts.
- Alternative:
  - Delete the `localItem` fallback in multi-node mode and return `503` while the ring has no shards.
  - Make the table catalog durable. The cheapest fix is a `tables.json` on the coordinator's disk. Better: send `create_table` to every shard leader as a log entry, and let the coordinator rebuild the catalog from the shards.
  - Have shards send a heartbeat (re-register) every 2-3 s. That also gives you the M4 failure detector (issue 8) for free.

**2. Version = per-shard log offset won't survive M4 or M5**

- M4 failover: a new leader hands out offsets that may already exist on the old leader as uncommitted entries. The same offset then means different values on different replicas. `Apply` rejects `old.Version >= it.Version`, so replicas with an equal version and a different value never converge, and nobody notices.
- M5 migration: a key moved from shard-1 (version 50 000) to shard-2 (offset 300) gets new writes at 301, 302, ... and `Apply` rejects all of them. The leader ignores what `Apply` returns, so it still answers 200. That is lost updates with a success code.
- Alternative: separate log position from item version now, before you have data on disk in the old format.
  - Log position: `(term, offset)`. Followers check the previous entry's term before appending, like Raft's `prevLogTerm`.
  - Item version: a hybrid logical clock timestamp set by the leader. On migration, the receiving leader moves its clock to at least the highest version it receives.
  - Have the leader check what `Apply` returns.
  - This also prepares M4's conflict resolution: either "no conflicts by construction" (terms plus cutting off the uncommitted suffix), or LWW on HLC. Pick one in an ADR now.

**3. Uncommitted writes can be read, and retrying isn't safe**

The leader runs `Append` and `apply`, and only then `waitAcks`. Followers also apply before the write commits. As a result:

- A write that returns 503 can still be read (R=1 on the leader, or R=2 that includes the leader).
- Two reads in a row can go from new to old. Read 1 includes the leader and sees the in-flight value; read 2 hits two followers that don't have it yet. R+W>N is not linearizability, and the spec's wording suggests more than you guarantee.
- ADR-0010 tells clients to "retry with the same value". That isn't idempotent: the retry gets a new offset and can overwrite another client's write made in between.
- Why it matters for M4: after failover, a value someone already read can disappear.
- Alternative:
  - Track a `commitIndex`. The leader sends it along with log responses, and replicas only serve entries up to it. Because the log is a prefix, per-request W still works: committing offset k with W=3 also commits everything before it.
  - Add an idempotency key (`X-Request-Id`, stored in the entry, deduplicated by the leader).
  - Add `If-Match: <version>` conditional puts. That is also a natural building block for M4.

**4. The benchmark will mostly measure the gp2 burst bucket**

`Log.Append` holds `l.mu` across `Write` and `fsync`, so leader writes are fully serialized, one fsync each. On XFS that is often 2 I/Os, because the file size changes.

The volumes are 8 GB gp2:

- baseline is 100 IOPS (the minimum),
- bursts reach 3000 IOPS on a 5.4M-credit bucket, which lasts about 30 minutes.

So you'll get a few hundred writes/s per leader. When the credits run out partway through a run, that drops to about 50-100/s. Each fsync also blocks `Last()` and `From()`, which stalls followers' pulls and `/health`.

- Alternative:
  - gp3: 3000 IOPS baseline, no credits, and cheaper.
  - Group commit: queue appends, run one `fsync` per batch, then wake every waiter. Followers already batch in `AppendAt`.
  - Record EBS settings and credit balance in `benchmark.md`.

**5. `/internal/*` is open to the internet**

`CoordinatorSecurityGroup` opens 8080 to `ClientCidr` (default `0.0.0.0/0`), and `POST /internal/shards` is on the same port.

Anyone can register `{"id":"shard-1","replica":"shard-1-0","role":"leader",...}`. It sorts before `shard-1-a`, so `leader()` returns it and all writes to shard-1 go to the attacker.

- Alternative: serve internal routes on a second listener (for example `:9080`) that only allows `ShardSecurityGroup`. Default `ClientCidr` to your own IP.

### Medium

**6. The leader takes followers' acks on trust**

`setAck(f, from-1)` believes whatever the follower sends, and `acked` only ever goes up. Two cases break it:

- Leader loses its log (instance replaced, `DATA_DIR` wiped): its offsets restart at 1. Followers still send `from=121`, so the leader counts them as having offsets 6, 7, ... and W=3 returns 200 while no follower has the data. The followers also wait forever for offset 121.
- Follower loses its disk: it comes back at `from=1`, but `acked` still says 120.

Followers also never check that their log matches the leader's.

- Alternative:
  - Give each log a log ID (a UUID written in the file header) plus a term, and check both on every pull.
  - Reject `from-1 > Last()`.
  - Store `acked[replica] = from-1` directly instead of `max`.
  - When the ID doesn't match, the follower needs a full resync, which means snapshots (issue 7).

**7. The whole log stays in RAM, with no snapshots**

`wal.Log.entries` holds every entry ever written, and `store` holds every item again. With YCSB's default 1 KB records and 1M records, that's about 500 MB of log plus 500 MB of store per shard, before Go's own overhead. A t3.micro has 1 GB, so it can run out of memory during `load`.

Restart replays the entire file. `serveLog` can return 500 x 1 MB in one response. ADR-0012 plans "retention" for M4, but without snapshots retention breaks both restart and follower catch-up.

- Alternative:
  - Keep only an index from offset to file position, and read entries from disk in `From`.
  - Cap responses by bytes.
  - Plan snapshot + truncate (write the store to `snapshot-<offset>.json`, then drop older log segments) and a "follower too far behind -> install snapshot" path.
  - Set `recordcount` explicitly and write it down.

**8. No failure detection, and timeouts that are too long**

A dead replica stays in its group forever. The coordinator's client has no dial timeout, only `Timeout: 5s`.

If an instance is stopped, packets to it are dropped, so with R=2 about 2/3 of reads wait up to 5 s before the fallback request goes out. Writes to a stopped leader also take 5 s before the `503`. Your own scenario ("leader stopped -> R=2 read still works") will technically pass and look broken in the demo, and the slow requests will dominate p99. If you only run `systemctl stop`, you get connection refused right away, which hides this.

- Alternative:
  - A 200-300 ms dial timeout and a per-replica request deadline.
  - Mark a replica down after missed heartbeats (issue 1), and order reads by health.
  - Optionally hedge: send the (R+1)-th request after the p95 latency.
  - You need the heartbeat for M4 failover anyway.

**9. The leader is baked into CloudFormation**

`ROLE=leader` and `LEADER_URL=${Shard1A.PrivateIp}` live in user data. In M4, promoting a follower would mean redeploying the stack. Nothing fences a second "leader" either: `leader()` just returns the first one in sorted order.

- Alternative: leave `LEADER_URL` out of the template. Followers ask the coordinator (`GET /internal/shards/{id}/leader`), and registration carries a `term`. The coordinator only accepts a leader with a higher term. Leader-side M4 logic then plugs into this without changing the infra.

**10. Placement: one AZ outage takes out a whole shard, and the coordinator is a bigger single point of failure**

With 3 replicas in 2 AZs, one AZ always has 2 of them. For shard-1, AZ-a holds the leader and c, so losing AZ-a stops writes and R=2 reads. Shard-2 has the same problem in AZ-b. Any single AZ failure kills one shard.

Meanwhile the single coordinator in `public-a` takes down everything when it dies, and the ADRs don't mention it.

- Alternative:
  - Add a third private subnet in the network stack (subnets are free) and put one replica per AZ.
  - Name the coordinator as a single point of failure in ADR-0013.
  - In M4, consider running coordinator logic on two nodes, or letting clients route using `/internal/shards`.

**11. M5 instance budget: ADR-0013 lumps two different options together**

Option 2 rejects "several replicas on one EC2" because one node would hold two replicas of the same shard. That reason doesn't apply to replicas of different shards on one node.

- Alternative: split shards from nodes (multi-group, like Cassandra or CockroachDB ranges). For example, 6 nodes host 4 shards x 3 replicas with leaders spread across them, and no node holds two replicas of the same shard. M5 can then add shards without adding instances, and autoscaling adds a node and moves replicas onto it. Write this up now as "rejected for M3, planned for M5" so the ADR isn't wrong.

**12. Ring membership changes reroute keys and nothing migrates them**

Any key written while only shard-1 had registered moves to shard-2 once shard-2 registers. The data stays behind on shard-1 and can't be reached.

- Alternative: add `EXPECTED_SHARDS=shard-1,shard-2` and return `503` until every listed shard has a leader. In M5, ring changes have to go through an explicit migration step.

**13. The benchmark plan mixes up its variables**

Going from M2 to M3 W=2/R=2 changes fsync, replication, and read fan-out at the same time, so you can't say which one costs what. Also missing from the plan:

- R variation: R=2 means two replica reads per get, so a shard's read capacity is only 1.5x one node, not 3x.
- Staleness: count stale reads at W=1/R=1. That is the result that actually supports ADR-0010.
- Degraded runs: one follower down; a follower catching up while the load runs.
- Run conditions: where YCSB runs (from a laptop, WAN latency hides everything; use an instance in the VPC, which is 8 of 9), t3 CPU credits (unlimited mode costs money), and at least 3 repeats with a warm-up.

Suggested matrix:

1. M2 baseline
2. M3 W=1, R=1, which isolates the cost of fsync
3. W=2, R=1
4. W=2, R=2
5. W=3, R=1
6. W=2, R=2 with one follower down

### Low

**14. Keys aren't URL-escaped**

`fmt.Sprintf("%s/internal/items/%s/%s?w=%d", ...)` inserts the decoded key as is. A key like `x?w=1&` turns into `...?w=1&?w=2`, and the leader reads `w=1`, so the client gets around W. Keys containing `/`, `%` or spaces also break. Use `url.PathEscape`.

**15. A failed write or fsync in `wal.write` leaves garbage in the middle of the file**

Later appends land after the broken bytes, and on restart `Open` cuts the file at that point, losing entries that were already acked. Postgres's answer is fail-stop: on a write or fsync error, cut the file back to the last good size or exit the process. Don't keep going.

**16. A missing `DATA_DIR` silently gives an in-memory log**

`Open("")` returns a log with no file. Refuse to start a shard without `DATA_DIR`.

**17. API and proposal nits**

- `w` is documented on GET and `r` on PUT.
- The 503 description for GET mentions "leader is down for a write".
- `X-Replicas` on a write only shows the leader. An `X-Acks: 2` header would be more useful in the demo.
- The proposal says "a shard survives losing one node", but losing the leader stops writes. Say "survives losing a follower; leader loss = read-only until M4".

### What to fix before the defense, in order

1. Remove the local fallback, persist tables, add heartbeat re-registration (issue 1)
2. Separate internal port / restrict the security group (issue 5)
3. Switch to gp3 and add group commit, otherwise the benchmark numbers mislead (issue 4)
4. Check the log ID and `from-1 <= Last()` in `setAck` (issue 6)
5. Short dial timeouts (issue 8)
6. ADR changes, no code: version format and terms (issue 2), commit visibility and idempotency (issue 3), multi-shard nodes for M5 (issue 11)

Items 1, 2, 4 and 5 are each under 50 lines and fix bugs a reviewer can trigger live. Issues 2, 3 and 11 matter most to settle on paper before M4, because they change the on-disk format and the infra.
