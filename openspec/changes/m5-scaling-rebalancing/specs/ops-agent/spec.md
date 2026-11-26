# Spec Delta

## Purpose

An AI agent reads the alerts and metrics and suggests what to do, with reasons. A human decides.

## ADDED Requirements

### Requirement: Read-only tools
`cmd/opsmcp` SHALL be an MCP server over stdio with `get_alerts`, `query_metric` and `get_cluster`. All of them SHALL only read (Prometheus API, coordinator `/health`, `/admin/migration`). There SHALL be no tool that changes anything.

#### Scenario: Agent looks at the cluster
- **WHEN** the agent calls `get_alerts` while a replica is down
- **THEN** it gets the firing alerts with their shard and replica labels

### Requirement: Proposal with reasons
The agent instructions SHALL make the agent answer with one of: scale up, scale down, rebalance, do nothing - with the metrics it is based on and what to check after.

#### Scenario: High p99
- **WHEN** write p99 is above 60 ms with 1 shard
- **THEN** the agent suggests scale up and says which metric it used
