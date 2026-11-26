# Tasks

## 1. Decisions

- [x] 1.1 ADR-0020: scaling unit, metric and thresholds
- [x] 1.2 ADR-0021: rebalancing algorithm
- [x] 1.3 ADR-0022: limits of the ops agent

## 2. Code

- [x] 2.1 Ring: remove a shard
- [x] 2.2 Replica: dump, import with a given version, purge, version floor
- [x] 2.3 Coordinator: add / drain a shard, background migration, reads with fallback, admin API, metrics
- [x] 2.4 `/version` on the coordinator and replicas
- [x] 2.5 `cmd/autoscaler`
- [x] 2.6 `cmd/opsmcp` + agent instructions
- [x] 2.7 Dashboard: shards on the ring, active nodes, migration
- [x] 2.8 Tests + local run

## 3. AWS

- [x] 3.1 `infra/app.yaml`: `ShardCount`; pipeline
- [ ] 3.2 Demo: scale down to 1 shard and back to 2 with the autoscaler, keys per shard before / after
- [ ] 3.3 Ops agent in Claude Desktop
- [ ] 3.4 YCSB with autoscaling: slow growth, spike, long load; compare with M2

## 4. Defense

- [x] 4.1 Budget for M5
- [ ] 4.2 AI design review
- [x] 4.3 README
