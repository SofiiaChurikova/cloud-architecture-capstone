# Tasks

## 1. Decisions

- [x] 1.1 ADR-0010: values of N / W / R
- [x] 1.2 ADR-0011: replica synchronisation mechanism
- [x] 1.3 ADR-0012: log structure and ordering guarantees
- [x] 1.4 ADR-0013: replica placement and the instance limit

## 2. Code

- [x] 2.1 Log: append-only file with offsets, replay on start
- [x] 2.2 Store with versions and tombstones
- [x] 2.3 Leader: append, apply, wait for W replicas
- [x] 2.4 Follower: pull from offset, apply, resync after restart
- [x] 2.5 Coordinator: replica sets, writes to the leader, quorum reads spread over replicas
- [x] 2.6 `api/openapi.yaml`: `w` / `r`, `X-Version`, `503`
- [x] 2.7 Tests + local run with 2 shards x 3 replicas
- [x] 2.8 After the AI review: heartbeats, tables on the coordinator disk, `EXPECTED_SHARDS`, dial timeout, escaped keys (ADR-0014)

## 3. AWS

- [x] 3.1 `infra/app.yaml`: 7 instances
- [x] 3.2 Demo in AWS: stop a follower, write, start it, read
- [x] 3.3 YCSB for W=1/2/3, `docs/m3/benchmark.md`

## 4. Defense

- [x] 4.1 Budget for M3
- [x] 4.2 AI design review
- [x] 4.3 README
