# Tasks

## 1. Decisions

- [x] 1.1 ADR-0015: which metrics, which of them are SLIs, where they are collected
- [x] 1.2 ADR-0016: SLO values
- [x] 1.3 ADR-0017: timestamp versions and last write wins
- [x] 1.4 ADR-0018: what an agent may do through MCP and how it is authorised

## 2. Code

- [ ] 2.1 `/metrics` on the coordinator and replicas
- [ ] 2.2 Replication lag on the leader
- [ ] 2.3 Trace IDs + JSON logs
- [ ] 2.4 Timestamp versions + last write wins
- [ ] 2.5 `API_TOKEN` on the coordinator, token in the YCSB binding
- [ ] 2.6 `cmd/mcp`: tools, schemas, errors
- [ ] 2.7 `monitoring/`: Prometheus config, alert rules, Grafana dashboard
- [ ] 2.8 Tests + local run with docker compose

## 3. AWS

- [ ] 3.1 `infra/app.yaml`: monitoring instance, security groups; pipeline uploads monitoring files
- [ ] 3.2 Demo: dashboard, stop a node -> "node down" alert
- [ ] 3.3 MCP from Claude Desktop against the AWS stack

## 4. Defense

- [ ] 4.1 Budget for M4
- [ ] 4.2 AI design review
- [ ] 4.3 README
