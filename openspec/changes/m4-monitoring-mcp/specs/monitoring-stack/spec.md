# Spec Delta

## Purpose

Metrics are collected, shown on a dashboard and alerted on, in AWS, from code.

## ADDED Requirements

### Requirement: Monitoring instance
The app stack SHALL create one more EC2 instance (8 of 9) with Prometheus and Grafana. It SHALL reach the coordinator on 9080 and the replicas on 8081, and its UIs (9090, 3000) SHALL be open only to `ClientCidr`.

#### Scenario: Deploy
- **WHEN** the app stack is deployed
- **THEN** Grafana opens on port 3000 and Prometheus shows the coordinator and 6 replicas as targets

### Requirement: Targets without static IPs
Prometheus SHALL get the list of replicas from the coordinator (`GET /internal/targets` on 9080, HTTP service discovery format), because the replica IPs change on every deploy.

#### Scenario: New deploy
- **WHEN** the stack is recreated and the replicas get new IPs
- **THEN** Prometheus scrapes the new IPs without config changes

### Requirement: Dashboard as code
The Grafana dashboard SHALL be a file in the repo and SHALL show cluster health (alive replicas), throughput, error rate, p50/p95/p99, replication lag and keys per shard.

#### Scenario: Open the dashboard
- **WHEN** Grafana starts
- **THEN** the dashboard is there without manual setup

### Requirement: Alerts
Alert rules SHALL be a file in the repo and SHALL fire for: a node down, a shard with fewer than 3 alive replicas, more than 0.5% `5xx`, p99 above the SLO threshold, replication lag above 5 s.

#### Scenario: Node down
- **WHEN** a replica is stopped
- **THEN** the "node down" alert fires within about a minute

### Requirement: Internal API not public
The coordinator SHALL serve `/internal/*` and `/metrics` only on its internal port (9080), which only the replicas and the monitoring instance can reach. Replica and shard IDs SHALL match `^[a-z0-9-]{1,32}$` and addresses SHALL be `http://host:port`.

#### Scenario: Register from outside
- **WHEN** someone calls `POST /internal/shards` on the public port 8080
- **THEN** they get `404`
