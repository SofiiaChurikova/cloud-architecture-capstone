# ADR-0015: Which metrics I collect and where

**Status:** Accepted

## Context
M4 asks for latency (p50/p95/p99), throughput and error rate for the coordinator and the shards, plus a dashboard with cluster health, replication lag and shard distribution, and alerts. I need to decide what exactly to measure, which of these are SLIs, and where to collect them.

The sandbox limits the choice: instances have no IAM role, so they can't push metrics or logs to CloudWatch themselves. CloudWatch only has the EC2 metrics (CPU, network), not latency of my API.

## Options
1. **CloudWatch** - built in, but only EC2 metrics without a role. Latency and lag would need an agent with credentials.
2. **Datadog / Sentry** (free with the student pack) - an agent on every instance and an API key on 8 machines, and the data leaves AWS.
3. **Prometheus + Grafana on one more EC2** - every service has `/metrics`, Prometheus pulls them, Grafana shows them. No credentials needed, everything is in the VPC.

## Decision
Option 3. Prometheus pulls `/metrics` every 10 s. The replica list comes from the coordinator (`/internal/targets`), because replica IPs change on every deploy.

What I collect:

| Metric | Where | Why |
|---|---|---|
| `kv_requests_total{op,status}` | coordinator, replicas | throughput and error rate |
| `kv_request_duration_seconds{op}` (histogram) | coordinator, replicas | p50 / p95 / p99 |
| `kv_replicas_alive{shard}` | coordinator | cluster health, node down |
| `kv_keys{shard,replica}` | replicas | shard distribution |
| `kv_log_offset{shard,replica}` | replicas | is a follower catching up |
| `kv_replication_lag_seconds{follower}`, `kv_replication_lag_entries{follower}` | leader | replication lag |
| `up` (from Prometheus) | all | node down |

**SLIs** (what a user feels) are only two, measured on the coordinator, because that's what the client talks to:
- availability: share of item requests that are not `5xx`;
- latency: p99 of reads and of writes.

The rest (lag, keys, offsets, replica latency) are not SLIs. A user doesn't see replication lag directly, it explains *why* an SLI got worse. If everything were an SLI, every small change would look like an incident.

## Consequences
- (+) Everything is in the repo: Prometheus config, alert rules, Grafana dashboard.
- (+) The same metrics are what M5 needs for autoscaling on p99.
- (-) One more instance (8 of 9) and about $0.01/h more.
- (-) Prometheus keeps data only on its own disk. When the stack is recreated, the history is gone.
- (-) Histograms give an approximate p99 (bucket bounds), not an exact one like YCSB.

**Where it lives:** `internal/metrics`, `cmd/coordinator` (`/metrics`, `/internal/targets`), `internal/replica` (`/metrics`, lag), `monitoring/`, `infra/app.yaml`
