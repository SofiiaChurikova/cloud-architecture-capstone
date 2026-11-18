# ADR-0014: Heartbeats and tables on the coordinator disk

**Status:** Accepted (changes ADR-0009 for M3)

## Context
In M2 the coordinator kept tables and the ring only in memory, and shards registered once (ADR-0009). That was fine because the pipeline always starts everything together. In M3 it's not enough:
- the data is on the replicas' disks now, but if the coordinator restarts alone, the tables and the replica list are gone, and replicas never register again;
- with an empty ring the coordinator falls back to single-node mode and keeps the write in its own memory, answers 200, and the write is never replicated;
- the coordinator doesn't know when a replica is dead. A stopped EC2 doesn't refuse connections, it just doesn't answer, so a read waits 5 s for it.

Both the M2 and the M3 AI reviews point at this (M2 review point 1.1 / 1.4, M3 review points 1, 8, 12).

## Options
1. **Keep it as in M2** - simple, but a coordinator restart loses data, and the demo with a stopped node is slow.
2. **Heartbeats + tables in a file on the coordinator disk** - replicas register every 2 s, the coordinator remembers when it last heard from each one, and writes the tables to `$DATA_DIR/tables.json`.
3. **Tables as log entries on every shard** - the coordinator rebuilds the tables from the shards. Nothing on the coordinator disk, but every shard leader needs a second kind of log entry, and creating a table has to reach all shards.
4. **DynamoDB / S3 for metadata** - still not possible without an IAM role (ADR-0009).

## Decision
Option 2:
- every replica calls `POST /internal/shards` every 2 s;
- a replica not heard from for 6 s is down. Reads ask live replicas first. A write to a shard with a dead leader gets `503` right away;
- `EXPECTED_SHARDS=shard-1,shard-2`: until both shards have a leader, item requests get `503`, and single-node mode is off. Without this variable single-node mode works as in M2;
- tables are written to `$DATA_DIR/tables.json` (temp file + rename) and loaded on start;
- the coordinator's dial timeout is 300 ms, so a dead address fails fast.

## Why
- About 50 lines, no new service, no IAM.
- The table list is tiny and changes rarely, so one file is enough.
- Heartbeats are the base for M4: failure detection and choosing a new leader both need to know who is alive.
- `EXPECTED_SHARDS` also fixes keys moving between shards when the second shard registers later than the first one.

## Consequences
- (+) A coordinator restart doesn't lose tables, and replicas come back in 2 s.
- (+) The "stop a follower" demo doesn't hang on the dead node after 6 s.
- (-) In the first 6 s after a node dies, a read that hits it can still wait up to 5 s.
- (-) The list of shards is fixed in the template. In M5, when shards are added, this has to become a real membership change.
- (-) The coordinator is still one instance. If it dies, everything is down until it's back. That's for M4.

**Where it lives:** `cmd/coordinator/main.go` (`alive`, `missingShards`, `loadTables`, `saveTables`), `cmd/shard/main.go` (`register`), `infra/app.yaml` and `docker-compose.yml` (`EXPECTED_SHARDS`, `DATA_DIR`)
