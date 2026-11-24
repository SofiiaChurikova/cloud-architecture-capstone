# Tasks

## 1. Decisions

- [x] 1.1 ADR-0015: which metrics, which of them are SLIs, where they are collected
- [x] 1.2 ADR-0016: SLO values
- [x] 1.3 ADR-0017: timestamp versions and last write wins
- [x] 1.4 ADR-0018: what an agent may do through MCP and how it is authorised

## 2. Code

- [x] 2.1 `/metrics` on the coordinator and replicas
- [x] 2.2 Replication lag on the leader
- [x] 2.3 Trace IDs + JSON logs
- [x] 2.4 Timestamp versions + last write wins
- [x] 2.5 `API_TOKEN` on the coordinator, token in the YCSB binding
- [x] 2.6 `cmd/mcp`: tools, schemas, errors
- [x] 2.7 `monitoring/`: Prometheus config, alert rules, Grafana dashboard
- [x] 2.8 Tests + local run (binaries + Prometheus + official MCP client)
- [x] 2.9 After the AI review: internal port 9080 (ADR-0019), ack checks on the leader, 5xx out of the latency histogram, error-rate alert, log fields, MCP create-only + delete limit + annotations

## 3. AWS

- [x] 3.1 `infra/app.yaml`: monitoring instance, security groups; pipeline uploads monitoring files
- [x] 3.2 Demo: dashboard, stop a node -> "node down" alert
- [x] 3.3 MCP from Claude Desktop against the AWS stack

## 4. Defense

- [x] 4.1 Budget for M4
- [x] 4.2 AI design review
- [x] 4.3 README
