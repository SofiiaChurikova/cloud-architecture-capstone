# Proposal

## Why

After M3 the store is replicated, but I can't see what happens inside it: how slow it is, how far followers are behind, which node is down. I only find out when a request fails. M4 adds monitoring and alerts, so a failure is visible before a user tells me. It also adds conflict resolution (versions by time, last write wins) and an MCP server, so an AI agent can use the store with clear limits.

## What Changes

- The coordinator and every replica expose Prometheus metrics on `/metrics`: requests, errors, latency (p50/p95/p99 from histograms), alive replicas, keys per shard, replication lag
- Every request gets a trace ID (`X-Trace-Id`). The coordinator passes it to the replicas, and all services log in JSON with this ID
- Prometheus + Grafana on one more EC2 (8 of 9). The dashboard and the alert rules are files in the repo. Alerts: node down, high p99, replication lag
- The version of an item is a leader timestamp instead of the log offset. Replicas and reads keep the newest version (last write wins)
- The coordinator can require a token for the API (`API_TOKEN`), and `/internal/*` moves to a port that only the cluster can reach (after the AI review)
- A new program `cmd/mcp`: an MCP server over stdio with tools for EXISTS / CREATE / READ / DELETE that calls the coordinator API

## Capabilities

### New Capabilities

- `metrics`: what the coordinator and replicas measure and how they expose it
- `request-tracing`: trace IDs and JSON logs
- `monitoring-stack`: Prometheus, Grafana, dashboard and alerts in AWS
- `conflict-resolution`: timestamp versions and last write wins
- `mcp-server`: tools for an AI agent, what it may and may not do, auth

### Modified Capabilities

None. M2 and M3 specs are not archived yet, everything new is in new capabilities.

## Impact

- Code: `internal/metrics` (new), `internal/replica` (metrics, lag, timestamps), `internal/wal` (timestamp in the entry), `internal/store` (LWW), `cmd/coordinator` (metrics, trace IDs, token, targets for Prometheus), `cmd/shard`, `cmd/mcp` (new), `bench/kvstore` (token)
- Infra: `infra/app.yaml` (monitoring instance, ports 9090 and 3000), `monitoring/` (Prometheus config, alert rules, Grafana dashboard), pipeline uploads the monitoring files and builds the MCP binary
- No new libraries: the Prometheus text format and MCP over stdio (JSON-RPC, one message per line) are small text protocols, written by hand in `internal/metrics` and `cmd/mcp`. The Prometheus client and the MCP Go SDK pull in packages from `golang.org/x` that I couldn't download in my build environment
- Out of scope: the optional parts of M4 (custom merge strategy, incident tickets, backups), central log storage, leader failover
